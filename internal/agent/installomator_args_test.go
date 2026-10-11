package agent

import (
	"reflect"
	"testing"
)

func TestInstallomatorArgsAlwaysTurnOffDebug(t *testing.T) {
	got := installomatorArgs([]string{"visualstudiocode", "NOTIFY=silent"})
	if !reflect.DeepEqual(got, []string{"visualstudiocode", "NOTIFY=silent", "DEBUG=0"}) {
		t.Fatalf("older server args: %v", got)
	}
	in := []string{"visualstudiocode", "DEBUG='0'", "NOTIFY='silent'"}
	if got := installomatorArgs(in); !reflect.DeepEqual(got, in) {
		t.Fatalf("server-sent DEBUG must be kept as is: %v", got)
	}
}
