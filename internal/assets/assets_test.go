package assets

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareMovesOldFiles(t *testing.T) {
	data, assets := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(data, Packages), 0o755)
	os.WriteFile(filepath.Join(data, Packages, "a.pkg"), []byte("a"), 0o640)
	os.WriteFile(filepath.Join(data, Packages, "b.pkg"), []byte("old"), 0o640)
	os.MkdirAll(filepath.Join(assets, Packages), 0o755)
	os.WriteFile(filepath.Join(assets, Packages, "b.pkg"), []byte("new"), 0o640)

	if err := Prepare(data, assets, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(assets, Packages, "a.pkg")); string(b) != "a" {
		t.Fatalf("a.pkg not moved: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(assets, Packages, "b.pkg")); string(b) != "new" {
		t.Fatalf("existing b.pkg overwritten: %q", b)
	}
	if _, err := os.Stat(filepath.Join(data, Packages, "a.pkg")); err == nil {
		t.Fatal("a.pkg left behind")
	}
	for _, d := range subdirs {
		if fi, err := os.Stat(filepath.Join(assets, d)); err != nil || !fi.IsDir() {
			t.Fatalf("%s not created", d)
		}
	}
}

func TestPrepareSameDir(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, Packages), 0o755)
	os.WriteFile(filepath.Join(d, Packages, "a.pkg"), []byte("a"), 0o640)
	if err := Prepare(d, d, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, Packages, "a.pkg")); err != nil {
		t.Fatal(err)
	}
}
