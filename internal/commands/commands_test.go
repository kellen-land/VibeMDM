package commands

import (
	"strings"
	"testing"
)

func TestDeviceInformation(t *testing.T) {
	cmd, err := DeviceInformation()
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Command.RequestType != "DeviceInformation" || cmd.CommandUUID == "" {
		t.Fatalf("bad command: %+v", cmd)
	}
	if !strings.Contains(string(cmd.Raw), "<string>SerialNumber</string>") {
		t.Error("queries missing")
	}
}

func TestDeviceLockRequiresPIN(t *testing.T) {
	if _, err := DeviceLock("12", "", ""); err == nil {
		t.Fatal("expected PIN error")
	}
	cmd, err := DeviceLock("123456", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cmd.Raw), "<key>PIN</key>") {
		t.Error("PIN missing")
	}
}

func TestRawAssignsUUID(t *testing.T) {
	cmd, err := Raw([]byte(`<?xml version="1.0"?><plist version="1.0"><dict><key>Command</key><dict><key>RequestType</key><string>ProfileList</string></dict></dict></plist>`))
	if err != nil {
		t.Fatal(err)
	}
	if cmd.CommandUUID == "" || cmd.Command.RequestType != "ProfileList" {
		t.Fatalf("bad: %+v", cmd)
	}
	if _, err := Raw([]byte(`<plist version="1.0"><dict/></plist>`)); err == nil {
		t.Error("expected error for missing Command")
	}
}
