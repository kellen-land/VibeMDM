package selfservice

import "testing"

func TestAppName(t *testing.T) {
	for _, bad := range []string{"../evil", "a/b", "Mac:App", ".hidden", " padded", "x\x00y", string(make([]byte, 61))} {
		if CheckAppName(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if got := AppName("", "Lands School"); got != "Lands School Self Service" {
		t.Errorf("default: %q", got)
	}
	if got := AppName("../x", "Org/Name"); got != "Self Service" {
		t.Errorf("unusable name and org: %q", got)
	}
	if got := AppName("Lands Apps", "X"); got != "Lands Apps" {
		t.Errorf("custom: %q", got)
	}
	if !ValidAccent("#0fb5ae") || ValidAccent("teal") || ValidAccent("#0fb5a") {
		t.Error("accent validation")
	}
}
