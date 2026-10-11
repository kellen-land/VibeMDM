package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestPolicyLogAcceptsAnyToolOutput(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000")
	c, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "PLOG-" + stamp, SerialNumber: "PLOG" + stamp, DeviceName: "plog", Model: "Mac14,2", ProductName: "Mac14,2"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeleteComputer(ctx, c.ID) })
	p, err := s.CreatePolicy(ctx, model.Policy{Name: "plog " + stamp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DeletePolicy(ctx, p.ID) })
	id, err := s.CreatePendingPolicyLog(ctx, p.ID, c.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	bad := "ok\x00nul and \xff\xfe invalid UTF-8"
	now := time.Now()
	sub := model.AgentPolicyLogSubmit{Status: "completed", StartedAt: now, CompletedAt: now, Log: bad,
		Steps: []model.PolicyStep{{Type: "app", Name: "visualstudiocode", Status: "completed", Output: bad}}}
	if err := s.CompletePolicyLog(ctx, id, c.ID, sub); err != nil {
		t.Fatalf("policy log with NUL/invalid UTF-8 rejected: %v", err)
	}
	l, err := s.GetPolicyLog(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(l.Log, "nul and") || strings.ContainsRune(l.Log, 0) || !strings.Contains(l.Steps[0].Output, "invalid UTF-8") {
		t.Fatalf("stored log: %q / %q", l.Log, l.Steps[0].Output)
	}
}
