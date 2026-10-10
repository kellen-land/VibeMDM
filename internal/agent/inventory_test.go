package agent

import "testing"

func TestParseEAResult(t *testing.T) {
	if got := ParseEAResult("noise\n<result>4.2.1</result>\n"); got != "4.2.1" {
		t.Errorf("got %q", got)
	}
	if got := ParseEAResult("  plain  "); got != "plain" {
		t.Errorf("got %q", got)
	}
}

func TestIoregValue(t *testing.T) {
	s := `    "IOPlatformSerialNumber" = "C02ABC123"
    "IOPlatformUUID" = "ABCDEF12-1234-5678-9ABC-DEF012345678"`
	if ioregValue(s, "IOPlatformSerialNumber") != "C02ABC123" || ioregValue(s, "IOPlatformUUID") != "ABCDEF12-1234-5678-9ABC-DEF012345678" {
		t.Error("ioreg parse failed")
	}
}
