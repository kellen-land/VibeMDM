package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ddmPost sends a DeclarativeManagement check-in and returns the response body.
func (d *device) ddmPost(ctx context.Context, endpoint string, data []byte) ([]byte, error) {
	msg := map[string]any{"MessageType": "DeclarativeManagement", "UDID": d.udid, "Endpoint": endpoint}
	if data != nil {
		msg["Data"] = data
	}
	out, code, err := d.post(ctx, encodePlist(msg), true)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("%s: HTTP %d: %s", endpoint, code, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// ddmSync behaves like macOS after a DeclarativeManagement command: fetch the
// tokens and manifest, download each declaration, and report status.
func (d *device) ddmSync() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	time.Sleep(300 * time.Millisecond) // let the command acknowledgement go first
	if _, err := d.ddmPost(ctx, "tokens", nil); err != nil {
		d.log.Warn("ddm tokens", "err", err)
		return
	}
	body, err := d.ddmPost(ctx, "declaration-items", nil)
	if err != nil {
		d.log.Warn("ddm items", "err", err)
		return
	}
	var manifest struct {
		Declarations map[string][]struct{ Identifier, ServerToken string }
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		d.log.Warn("ddm manifest", "err", err)
		return
	}
	sectionKind := map[string]string{"Activations": "activation", "Configurations": "configuration", "Assets": "asset", "Management": "management"}
	declStatus := map[string][]map[string]any{"activations": {}, "configurations": {}, "assets": {}, "management": {}}
	var enforcement map[string]any
	for section, items := range manifest.Declarations {
		kind := sectionKind[section]
		for _, it := range items {
			raw, err := d.ddmPost(ctx, "declaration/"+kind+"/"+it.Identifier, nil)
			valid := "valid"
			var decl struct {
				Type    string
				Payload map[string]any
			}
			if err != nil || json.Unmarshal(raw, &decl) != nil {
				valid = "invalid"
			}
			if decl.Type == "com.apple.configuration.softwareupdate.enforcement.specific" {
				enforcement = decl.Payload
			}
			declStatus[strings.ToLower(section)] = append(declStatus[strings.ToLower(section)],
				map[string]any{"identifier": it.Identifier, "valid": valid, "active": valid == "valid", "server-token": it.ServerToken})
		}
	}
	su := map[string]any{"install-state": "none"}
	if enforcement != nil {
		target, _ := enforcement["TargetOSVersion"].(string)
		if compareVersions(d.osVer, target) < 0 {
			su["install-state"] = "downloading"
			su["pending-version"] = map[string]any{"os-version": target, "build-version": enforcement["TargetBuildVersion"]}
			su["install-reason"] = map[string]any{"reason": []any{"declarative-enforcement"}}
		}
	}
	report := map[string]any{
		"StatusItems": map[string]any{
			"device": map[string]any{
				"operating-system": map[string]any{"version": d.osVer, "build-version": d.build},
				"model":            map[string]any{"identifier": d.model},
			},
			"softwareupdate": su,
		},
		"Declarations": declStatus,
		"Errors":       []any{},
	}
	b, _ := json.Marshal(report)
	if _, err := d.ddmPost(ctx, "status", b); err != nil {
		d.log.Warn("ddm status", "err", err)
		return
	}
	d.log.Info("ddm synced", "declarations", len(manifest.Declarations["Configurations"])+len(manifest.Declarations["Activations"]), "install_state", su["install-state"])
}

func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
