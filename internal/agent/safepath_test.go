package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootOwnedExecutable(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to create root-owned files")
	}
	home, _ := os.UserHomeDir() // root's home: a root-only chain, unlike /tmp
	base, err := os.MkdirTemp(home, "safepath")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	os.Chmod(base, 0o755)
	if _, err := rootOwnedExecutable(base); err != nil {
		t.Skipf("home directory chain is not root-only here: %v", err)
	}
	bin := filepath.Join(base, "tool")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	link := filepath.Join(base, "link")
	os.Symlink(bin, link)
	if got, err := rootOwnedExecutable(link); err != nil || got != bin {
		t.Fatalf("root-owned tool refused: %q %v", got, err)
	}
	os.Chmod(bin, 0o777)
	if _, err := rootOwnedExecutable(link); err == nil {
		t.Fatal("world-writable tool accepted")
	}
	os.Chmod(bin, 0o755)
	os.Chown(base, 501, 20)
	if _, err := rootOwnedExecutable(link); err == nil {
		t.Fatal("tool in a user-owned directory accepted")
	}
}
