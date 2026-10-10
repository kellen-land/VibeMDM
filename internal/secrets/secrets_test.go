package secrets

import "testing"

func TestRoundTrip(t *testing.T) {
	b, err := New("a-very-long-passphrase-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := b.Seal("ABCD-1234-EFGH")
	if err != nil || ct == "ABCD-1234-EFGH" || !IsEncrypted(ct) {
		t.Fatalf("seal: %v %q", err, ct)
	}
	pt, err := b.Open(ct)
	if err != nil || pt != "ABCD-1234-EFGH" {
		t.Fatalf("open: %v %q", err, pt)
	}
	// legacy plaintext passes through
	if v, _ := b.Open("plain"); v != "plain" {
		t.Fatal("plaintext passthrough")
	}
	// wrong key fails
	b2, _ := New("another-long-passphrase-xxxxxxxx")
	if _, err := b2.Open(ct); err == nil {
		t.Fatal("expected failure with wrong key")
	}
	// disabled box
	d, _ := New("")
	if d.Enabled() {
		t.Fatal("should be disabled")
	}
	if v, _ := d.Seal("x"); v != "x" {
		t.Fatal("disabled seal should pass through")
	}
	if _, err := d.Open(ct); err == nil {
		t.Fatal("disabled box cannot open ciphertext")
	}
	if _, err := New("short"); err == nil {
		t.Fatal("short key must be rejected")
	}
}
