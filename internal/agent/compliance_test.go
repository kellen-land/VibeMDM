package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestNeedsUser(t *testing.T) {
	if !needsUser(`/usr/bin/sudo -u "$CURRENT_USER" /usr/bin/defaults -currentHost read com.apple.Bluetooth PrefKeyServicesEnabled`) {
		t.Fatal("CURRENT_USER rule not detected")
	}
	if needsUser(`/usr/bin/pmset -g custom | /usr/bin/awk '/womp/ { sum+=$2 } END {print sum}'`) {
		t.Fatal("system rule flagged as per-user")
	}
}

// With nobody logged in, a per-user check must be skipped instead of running `sudo -u ""`.
func TestEvaluateRuleSkipsPerUserRulesWithoutConsoleUser(t *testing.T) {
	r := model.AgentComplianceRule{ID: "system_settings_bluetooth_sharing_disable", Expected: "0",
		Check: `/usr/bin/sudo -u "$CURRENT_USER" /usr/bin/defaults -currentHost read com.apple.Bluetooth PrefKeyServicesEnabled`,
		Fix:   `/usr/bin/sudo -u "$CURRENT_USER" /usr/bin/defaults -currentHost write com.apple.Bluetooth PrefKeyServicesEnabled -bool false`}
	res := evaluateRule(context.Background(), r, true, complianceUser{})
	if res.Status != "error" || !strings.Contains(res.Detail, "no one is logged in") {
		t.Fatalf("got %+v", res)
	}
}
