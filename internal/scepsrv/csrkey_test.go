package scepsrv

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestCheckCSRKey(t *testing.T) {
	small, _ := rsa.GenerateKey(rand.Reader, 1024)
	ok, _ := rsa.GenerateKey(rand.Reader, 2048)
	p224, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	p256, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if checkCSRKey(&small.PublicKey) == nil || checkCSRKey(&p224.PublicKey) == nil {
		t.Fatal("weak key accepted")
	}
	if err := checkCSRKey(&ok.PublicKey); err != nil {
		t.Fatal(err)
	}
	if err := checkCSRKey(&p256.PublicKey); err != nil {
		t.Fatal(err)
	}
}
