package rewrite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/phntom/mindav/internal/rewrite"
)

func TestRewritePathAndDestination(t *testing.T) {
	var gotPath, gotDest string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotDest = r.Header.Get("Destination")
	})
	h := &rewrite.Handler{Next: next}

	req := httptest.NewRequest("MOVE", "/v1/webdav/foo/bar.txt", nil)
	req.Host = "pwd.kix.co.il"
	req.Header.Set("X-Auth-Request-User", "u@e.com")
	req.Header.Set("Destination", "http://pwd.kix.co.il/webdav/foo/baz.txt")
	req.Header.Set("X-URI-Prefix", "http://pwd.kix.co.il/webdav")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if want := "/v1/webdav/u/u@e.com/foo/bar.txt"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if want := "http://pwd.kix.co.il/v1/webdav/u/u@e.com/foo/baz.txt"; gotDest != want {
		t.Errorf("destination = %q, want %q", gotDest, want)
	}
}

func TestRewriteRootPath(t *testing.T) {
	var gotPath string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { gotPath = r.URL.Path })
	h := &rewrite.Handler{Next: next}

	req := httptest.NewRequest("PROPFIND", "/v1/webdav/", nil)
	req.Header.Set("X-Auth-Request-User", "healthz@")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if want := "/v1/webdav/u/healthz@/"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}
