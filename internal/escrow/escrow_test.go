package escrow

import (
	"crypto/x509"
	"testing"

	"github.com/smallstep/pkcs7"
	"howett.net/plist"
)

func TestEscrowRoundTrip(t *testing.T) {
	id, certPEM, keyPEM, err := NewIdentity("Acme")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := ParseIdentity(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate macOS: encrypt a PRK to the escrow certificate.
	prk := "ABCD-EFGH-IJKL-MNOP-QRST-UVWX"
	cms, err := pkcs7.Encrypt([]byte(prk), []*x509.Certificate{id.Cert})
	if err != nil {
		t.Fatal(err)
	}
	got, err := id2.DecryptPRK(cms)
	if err != nil || got != prk {
		t.Fatalf("decrypt: %v %q", err, got)
	}
	if Fingerprint(prk) != "…-UVWX" {
		t.Errorf("fingerprint %q", Fingerprint(prk))
	}
	prof, err := id.EscrowProfile("Acme")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if _, err := plist.Unmarshal(prof, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["PayloadIdentifier"] != EscrowProfileIdentifier {
		t.Error("identifier")
	}
	enf, err := EnforceProfile("Acme", EnforceOptions{MaxBypassAttempts: 3, DeferAtLogout: true, ShowRecoveryKey: true})
	if err != nil {
		t.Fatal(err)
	}
	o, err := ParseEnforceOptions(enf)
	if err != nil || o.MaxBypassAttempts != 3 || !o.DeferAtLogout || !o.ShowRecoveryKey {
		t.Fatalf("round trip: %+v %v", o, err)
	}
	if _, err := plist.Unmarshal(enf, &doc); err != nil {
		t.Fatal(err)
	}
}
