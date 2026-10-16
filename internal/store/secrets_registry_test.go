package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sealCallsPerFile is the number of Box.Seal call sites per store file. When this test fails,
// a value is being sealed somewhere new: add its column to sealedColumns (or the JSON-held
// secrets in secrets.go) so key rotation re-encrypts it, then update the count here.
var sealCallsPerFile = map[string]int{
	"agentinstall.go": 2, "filevault.go": 2, "laps.go": 3, "mobile.go": 1, "policies.go": 1, "store.go": 4, "super.go": 1,
}

func TestSealedColumnsCoverSealCalls(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`\bBox\.Seal\(`)
	got := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "secrets.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(re.FindAll(b, -1)); n > 0 {
			got[f] = n
		}
	}
	for f, n := range got {
		if sealCallsPerFile[f] != n {
			t.Errorf("%s has %d Box.Seal calls, expected %d: register the new sealed column in sealedColumns", f, n, sealCallsPerFile[f])
		}
	}
	for f := range sealCallsPerFile {
		if got[f] == 0 {
			t.Errorf("%s no longer seals anything; update sealCallsPerFile", f)
		}
	}
}
