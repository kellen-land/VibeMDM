package server

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/micromdm/nanomdm/push/nanopush"
	"golang.org/x/net/http2"
)

// apnsClientFor returns a nanopush client factory whose transport rewrites
// requests to target (used for testing against a fake APNs endpoint).
func apnsClientFor(target string) nanopush.NewClient {
	return func(cert *tls.Certificate) (*http.Client, error) {
		u, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{Certificates: []tls.Certificate{*cert}, InsecureSkipVerify: strings.HasPrefix(target, "https://localhost") || strings.HasPrefix(target, "https://127.")},
		}
		if u.Scheme == "https" {
			http2.ConfigureTransport(tr)
		}
		return &http.Client{Timeout: 30 * time.Second, Transport: &rewriteTransport{base: tr, target: u}}, nil
	}
}

type rewriteTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = t.target.Scheme
	r2.URL.Host = t.target.Host
	r2.Host = t.target.Host
	return t.base.RoundTrip(r2)
}
