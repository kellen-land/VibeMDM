package appicons

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	id := "0b9f3c1e-5d2a-4c8e-9f00-123456789abc"
	if _, _, err := d.Read(id); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	v1, err := d.Save(id, []byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	v2, _ := d.Save(id, []byte("two"))
	if v1 == v2 || d.Version(id) != v2 || d.Versions()[id] != v2 {
		t.Fatalf("versions: %s %s %v", v1, v2, d.Versions())
	}
	if b, v, _ := d.Read(id); string(b) != "two" || v != v2 {
		t.Fatalf("read %q %s", b, v)
	}
	if m, _ := filepath.Glob(filepath.Join(d.Path, "*")); len(m) != 1 {
		t.Fatalf("old icon kept: %v", m)
	}
	if _, err := d.Save("../etc/passwd", []byte("x")); err == nil {
		t.Fatal("path traversal accepted")
	}
	os.WriteFile(filepath.Join(d.Path, ".upload-123"), nil, 0o600)
	if n := d.Prune(func(string) bool { return false }); n != 2 {
		t.Fatalf("pruned %d", n)
	}
	d.Save(id, []byte("x"))
	d.Delete(id)
	if d.Version(id) != "" {
		t.Fatal("not deleted")
	}
}
