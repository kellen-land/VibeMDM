package profilevars

import (
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestExpand(t *testing.T) {
	c := &model.Computer{Name: "Lab <1>", SerialNumber: "C02X", LastUser: "kim", AssignedEmail: "kim@example.com"}
	in := `<string>$COMPUTERNAME / $SERIALNUMBER / $USERNAME / $EMAIL / $ROOM / $5 off</string>`
	got := Expand(in, ForComputer(c))
	want := `<string>Lab &lt;1&gt; / C02X / kim / kim@example.com /  / $5 off</string>`
	if got != want {
		t.Fatalf("got %s", got)
	}
}
