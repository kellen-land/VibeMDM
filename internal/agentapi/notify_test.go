package agentapi

import (
	"testing"
	"time"
)

func TestNotifierWakeAndCheckin(t *testing.T) {
	n := NewNotifier()
	ch, unsub := n.Subscribe("c1")
	defer unsub()
	if !n.Waiting("c1") || n.OnlineCount() != 1 {
		t.Fatal("expected c1 to be waiting")
	}
	n.RequestCheckin("c1", "c2")
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not woken")
	}
	if !n.TakeCheckin("c1") || n.TakeCheckin("c1") {
		t.Fatal("check-in flag should be returned exactly once")
	}
	if !n.TakeCheckin("c2") {
		t.Fatal("offline computer should keep its pending check-in flag")
	}
	if n.Waiting("c1") {
		t.Fatal("subscription should be consumed after notify")
	}
}
