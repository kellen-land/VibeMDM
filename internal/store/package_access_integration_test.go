package store

import (
	"context"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/model"
)

func TestPackageDownloadNeedsAssignedPolicy(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	stamp := time.Now().Format("150405.000000")
	comp, err := s.UpsertComputerFromAuthenticate(ctx, MDMAuthenticateInfo{UDID: "PKG-" + stamp, SerialNumber: "P" + stamp})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := s.CreatePackage(ctx, model.Package{Name: "Tool " + stamp, Filename: "tool-" + stamp + ".pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.PackageAllowedForComputer(ctx, comp.ID, pkg.ID); err != nil || ok {
		t.Fatalf("package downloadable without any policy: %v %v", ok, err)
	}
	pol, err := s.CreatePolicy(ctx, model.Policy{Name: "Install tool " + stamp, Frequency: "Ongoing",
		Packages: []model.PolicyPackage{{PackageID: pkg.ID, Action: "install"}}})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.PackageAllowedForComputer(ctx, comp.ID, pkg.ID); ok {
		t.Fatal("package downloadable before its policy was handed to the Mac")
	}
	if _, err := s.CreatePendingPolicyLog(ctx, pol.ID, comp.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.PackageAllowedForComputer(ctx, comp.ID, pkg.ID); err != nil || !ok {
		t.Fatalf("package of a policy given to this Mac refused: %v %v", ok, err)
	}
}
