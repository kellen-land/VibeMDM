package webui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFallbacks(t *testing.T) {
	h := Handler()
	for path, want := range map[string]int{
		"/computers/123":           200, // client-side route: the app page
		"/":                        200,
		"/api/v1/does-not-exist":   404,
		"/agent/v1/does-not-exist": 404,
		"/assets/index-old.js":     404,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
		if want == 404 && strings.HasPrefix(path, "/api/") && !strings.Contains(rec.Header().Get("Content-Type"), "json") {
			t.Errorf("%s: content type %q", path, rec.Header().Get("Content-Type"))
		}
	}
}
