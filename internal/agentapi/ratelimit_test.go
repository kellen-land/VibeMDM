package agentapi

import (
	"testing"
	"time"
)

func TestDeviceLimiter(t *testing.T) {
	l := newDeviceLimiter(3, time.Hour)
	for i := 0; i < 3; i++ {
		if !l.allow("mac-a") {
			t.Fatalf("request %d within burst refused", i)
		}
	}
	if l.allow("mac-a") {
		t.Fatal("request beyond burst allowed")
	}
	if !l.allow("mac-b") {
		t.Fatal("another Mac shares the first Mac's bucket")
	}
}
