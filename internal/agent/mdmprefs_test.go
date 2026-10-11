package agent

import (
	"context"
	"testing"
	"time"
)

func TestWaitForMDMBootstrap(t *testing.T) {
	prefs := map[string]string{"ServerURL": "https://mdm.example.com", "EnrollToken": "vmbt_x"}
	b, err := WaitForMDMBootstrap(context.Background(), time.Second, func(_ context.Context, k string) string { return prefs[k] })
	if err != nil || b.ServerURL != "https://mdm.example.com" || b.EnrollToken != "vmbt_x" {
		t.Fatalf("%+v %v", b, err)
	}
	if _, err := WaitForMDMBootstrap(context.Background(), 0, func(context.Context, string) string { return "" }); err == nil {
		t.Fatal("expected an error when the profile never arrives")
	}
}
