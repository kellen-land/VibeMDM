package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"howett.net/plist"

	"github.com/vibemdm/vibemdm/internal/agentinstall"
)

// installAgentPackage plays mdmclient for InstallEnterpriseApplication: it fetches the
// manifest and package, checks the chunk MD5s and that the package is signed, then
// "runs the postinstall": enroll the agent with the token from the bootstrap profile.
func (d *device) installAgentPackage(manifestURL string) error {
	get := func(url string) ([]byte, error) {
		resp, err := d.client.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 300<<20))
	}
	mb, err := get(manifestURL)
	if err != nil {
		return err
	}
	var man struct {
		Items []struct {
			Assets []struct {
				URL     string   `plist:"url"`
				MD5Size int64    `plist:"md5-size"`
				MD5s    []string `plist:"md5s"`
			} `plist:"assets"`
		} `plist:"items"`
	}
	if _, err := plist.Unmarshal(mb, &man); err != nil || len(man.Items) == 0 || len(man.Items[0].Assets) == 0 {
		return errors.New("invalid manifest")
	}
	a := man.Items[0].Assets[0]
	pkg, err := get(a.URL)
	if err != nil {
		return err
	}
	for i, want := range a.MD5s {
		start := int64(i) * a.MD5Size
		end := min(start+a.MD5Size, int64(len(pkg)))
		if start > end {
			return errors.New("package shorter than the manifest says")
		}
		sum := md5.Sum(pkg[start:end])
		if hex.EncodeToString(sum[:]) != want {
			return fmt.Errorf("chunk %d checksum mismatch", i)
		}
	}
	signer, err := agentinstall.PackageSigner(bytes.NewReader(pkg))
	if err != nil {
		return err
	}
	prof, ok := d.profiles[agentinstall.BootstrapIdentifier]
	if !ok {
		return errors.New("bootstrap profile not installed")
	}
	token := bootstrapToken(prof)
	if token == "" {
		return errors.New("bootstrap profile has no enrollment token")
	}
	d.log.Info("simulated agent package installed", "signed_by", signer)
	if d.agentStarted {
		return nil
	}
	d.agentStarted = true
	go func() {
		if err := d.enrollAgent(d.ctx, token); err != nil {
			d.log.Error("agent enrollment from MDM bootstrap failed", "err", err)
			return
		}
		d.agentLoop(d.ctx, d.agentEvery)
	}()
	return nil
}

func bootstrapToken(prof map[string]any) string {
	content, _ := prof["PayloadContent"].([]any)
	for _, c := range content {
		p, _ := c.(map[string]any)
		if str(p["PayloadType"]) != "com.apple.ManagedClient.preferences" {
			continue
		}
		dom, _ := p["PayloadContent"].(map[string]any)[agentinstall.PrefsDomain].(map[string]any)
		forced, _ := dom["Forced"].([]any)
		for _, f := range forced {
			s, _ := f.(map[string]any)["mcx_preference_settings"].(map[string]any)
			if t := str(s["EnrollToken"]); t != "" {
				return t
			}
		}
	}
	return ""
}
