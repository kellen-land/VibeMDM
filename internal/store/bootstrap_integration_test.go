package store

import (
	"context"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

// SecurityInfo's BootstrapTokenAllowedForAuthentication is not the escrow state: a Mac enrolled
// without ADE reports "disallowed" even after it escrowed its token with SetBootstrapToken.
func TestSecurityInfoDoesNotClearBootstrapEscrow(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	udid := "BT-" + stamp
	comp, err := s.EnrollAgent(ctx, model.AgentEnrollRequest{SerialNumber: "BTS" + stamp, UDID: udid, Hostname: "bt.local",
		ComputerName: "bt-test", OSVersion: "26.6.2", Architecture: "arm64"}, HashToken("bt-"+stamp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, comp.ID) })

	if err := s.SetBootstrapTokenEscrowed(ctx, udid, true); err != nil {
		t.Fatal(err)
	}
	sec := map[string]any{"BootstrapTokenAllowedForAuthentication": "disallowed", "FDE_Enabled": true}
	if err := s.ApplyMDMSecurityInfo(ctx, udid, sec); err != nil {
		t.Fatal(err)
	}
	c, err := s.GetComputer(ctx, comp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.BootstrapTokenEscrowed == nil || !*c.BootstrapTokenEscrowed {
		t.Fatalf("escrow flag cleared by SecurityInfo: %v", c.BootstrapTokenEscrowed)
	}
	if c.SecurityInfo["BootstrapTokenAllowedForAuthentication"] != "disallowed" {
		t.Fatal("SecurityInfo not stored")
	}
	// An empty SetBootstrapToken clears escrow.
	s.SetBootstrapTokenEscrowed(ctx, udid, false)
	if c, _ := s.GetComputer(ctx, comp.ID); c.BootstrapTokenEscrowed == nil || *c.BootstrapTokenEscrowed {
		t.Fatal("escrow not cleared")
	}
}
