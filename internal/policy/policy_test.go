package policy

import (
	"testing"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestMissingCapability(t *testing.T) {
	plain := &model.Policy{UpdateInventory: true}
	accounts := &model.Policy{Accounts: []model.PolicyAccount{{Action: "create", Username: "student"}}}
	if missingCapability(plain, nil) != "" {
		t.Fatal("a policy without optional features must go to any agent")
	}
	if missingCapability(accounts, nil) == "" || missingCapability(accounts, []string{"something_else"}) == "" {
		t.Fatal("an agent without policy_accounts must not get Local Accounts actions")
	}
	if missingCapability(accounts, []string{CapLocalAccounts}) != "" {
		t.Fatal("a capable agent was refused")
	}
}
