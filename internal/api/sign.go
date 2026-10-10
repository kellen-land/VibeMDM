package api

import "github.com/vibemdm/vibemdm/internal/enroll"

func signTest(certPEM, keyPEM string) ([]byte, error) {
	return enroll.Sign([]byte("<?xml version=\"1.0\"?><plist version=\"1.0\"><dict/></plist>"), certPEM, keyPEM)
}
