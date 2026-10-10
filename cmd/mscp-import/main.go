// Command mscp-import converts a macOS Security Compliance Project checkout
// into VibeMDM's embedded catalog (internal/compliance/catalog.json.gz).
//
//	go run ./cmd/mscp-import -src path/to/macos_security -release release_27.0
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vibemdm/vibemdm/internal/compliance"
)

type yResult map[string]any

type yEnforcement struct {
	Check struct {
		Shell  string  `yaml:"shell"`
		Result yResult `yaml:"result"`
	} `yaml:"check"`
	Fix struct {
		Shell string `yaml:"shell"`
	} `yaml:"fix"`
}

type yRule struct {
	ID           string               `yaml:"id"`
	Title        string               `yaml:"title"`
	Discussion   string               `yaml:"discussion"`
	References   map[string]any       `yaml:"references"`
	Platforms    map[string]yaml.Node `yaml:"platforms"`
	Tags         []string             `yaml:"tags"`
	ODV          map[string]any       `yaml:"odv"`
	Mobileconfig []struct {
		PayloadType    string           `yaml:"PayloadType"`
		PayloadContent []map[string]any `yaml:"PayloadContent"`
	} `yaml:"mobileconfig_info"`
}

type yBaseline struct {
	Title        string `yaml:"title"`
	Description  string `yaml:"description"`
	ParentValues string `yaml:"parent_values"`
	Profile      []struct {
		Section string   `yaml:"section"`
		Rules   []string `yaml:"rules"`
	} `yaml:"profile"`
}

func result(r yResult) *compliance.Result {
	for _, k := range []string{"string", "integer", "base64"} {
		if v, ok := r[k]; ok {
			return &compliance.Result{Type: k, Value: fmt.Sprint(v)}
		}
	}
	return nil
}

func main() {
	src := flag.String("src", "", "path to a macos_security checkout")
	release := flag.String("release", "", "release tag, e.g. release_27.0")
	out := flag.String("out", "internal/compliance/catalog.json.gz", "output file")
	flag.Parse()
	if *src == "" || *release == "" {
		flag.Usage()
		os.Exit(2)
	}
	data := filepath.Join(*src, "src", "mscp", "data")
	if _, err := os.Stat(data); err != nil {
		data = *src // older layout: rules/ and baselines/ at the top level
	}
	cat := compliance.Catalog{Source: "macOS Security Compliance Project (https://github.com/usnistgov/macos_security)", Release: *release,
		Rules: map[string]*compliance.Rule{}, Baselines: map[string]compliance.Baseline{}}

	err := filepath.WalkDir(filepath.Join(data, "rules"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "Vendor Description content Copyright") {
			log.Printf("skipping %s: contains Apple vendor description content (not CC BY 4.0)", path)
			return nil
		}
		var y yRule
		if err := yaml.Unmarshal(b, &y); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		mac, ok := y.Platforms["macOS"]
		if !ok || y.ID == "" {
			return nil
		}
		r := &compliance.Rule{ID: y.ID, Title: y.Title, Section: filepath.Base(filepath.Dir(path)), Discussion: strings.TrimSpace(y.Discussion),
			Tags: y.Tags, References: y.References, Platforms: map[string]compliance.PlatformVersion{}}
		var plat map[string]yaml.Node
		if err := mac.Decode(&plat); err != nil {
			return fmt.Errorf("%s platforms: %w", path, err)
		}
		for k, node := range plat {
			switch k {
			case "enforcement_info":
				var e yEnforcement
				node.Decode(&e)
				r.Check, r.Result, r.Fix = strings.TrimSpace(e.Check.Shell), result(e.Check.Result), strings.TrimSpace(e.Fix.Shell)
			case "introduced", "deprecated", "removed":
			default:
				var pv struct {
					Benchmarks  []compliance.Benchmark `yaml:"benchmarks"`
					Enforcement *yEnforcement          `yaml:"enforcement_info"`
				}
				if err := node.Decode(&pv); err != nil {
					continue
				}
				v := compliance.PlatformVersion{Benchmarks: pv.Benchmarks}
				if pv.Enforcement != nil {
					v.Check, v.Result, v.Fix = strings.TrimSpace(pv.Enforcement.Check.Shell), result(pv.Enforcement.Check.Result), strings.TrimSpace(pv.Enforcement.Fix.Shell)
				}
				r.Platforms[k] = v
			}
		}
		if y.ODV != nil {
			o := &compliance.ODV{Values: map[string]any{}}
			for k, v := range y.ODV {
				if k == "hint" {
					hb, _ := yaml.Marshal(v)
					var h struct {
						Datatype    string `yaml:"datatype"`
						Description string `yaml:"description"`
						Validation  struct {
							Min *float64 `yaml:"min"`
							Max *float64 `yaml:"max"`
						} `yaml:"validation"`
					}
					yaml.Unmarshal(hb, &h)
					o.Hint = compliance.ODVHint{Datatype: h.Datatype, Description: h.Description, Min: h.Validation.Min, Max: h.Validation.Max}
					continue
				}
				o.Values[k] = v
			}
			r.ODV = o
		}
		for _, m := range y.Mobileconfig {
			if m.PayloadType != "" {
				r.Mobileconfig = append(r.Mobileconfig, compliance.Payload{PayloadType: m.PayloadType, Content: m.PayloadContent})
			}
		}
		cat.Rules[r.ID] = r
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	files, _ := filepath.Glob(filepath.Join(data, "baselines", "macos", "*.yaml"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(data, "baselines", "*.yaml"))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		var y yBaseline
		if err := yaml.Unmarshal(b, &y); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		key := strings.TrimSuffix(filepath.Base(f), ".yaml")
		if i := strings.Index(key, "_macos_"); i > 0 {
			if cat.Platform == "" {
				cat.Platform = key[i+len("_macos_"):]
			}
			key = key[:i]
		}
		bl := compliance.Baseline{Key: key, Title: y.Title, Description: strings.TrimSpace(y.Description), ParentValues: y.ParentValues}
		for _, s := range y.Profile {
			var ids []string
			for _, id := range s.Rules {
				if _, ok := cat.Rules[id]; ok {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 {
				bl.Sections = append(bl.Sections, compliance.BaselineSection{Section: s.Section, Rules: ids})
			}
		}
		if bl.ParentValues == "" {
			bl.ParentValues = key
		}
		cat.Baselines[key] = bl
	}

	fh, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	zw, _ := gzip.NewWriterLevel(fh, gzip.BestCompression)
	enc := json.NewEncoder(zw)
	if err := enc.Encode(cat); err != nil {
		log.Fatal(err)
	}
	zw.Close()
	fh.Close()
	keys := make([]string, 0, len(cat.Baselines))
	for k := range cat.Baselines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	st, _ := os.Stat(*out)
	fmt.Printf("wrote %s (%d bytes): %d rules, baselines: %s\n", *out, st.Size(), len(cat.Rules), strings.Join(keys, ", "))
}
