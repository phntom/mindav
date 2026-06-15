package auth_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phntom/mindav/internal/auth"
)

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestMissingUserForbidden(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next should not be called") })
	m := &auth.Middleware{Next: next, Client: http.DefaultClient}

	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/webdav/x", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestPassThroughUser(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.Header.Get("X-Auth-Request-User") })
	m := &auth.Middleware{Next: next, Client: http.DefaultClient}

	req := httptest.NewRequest("GET", "/v1/webdav/x", nil)
	req.Header.Set("X-Auth-Request-User", "alice@example.com")
	m.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "alice@example.com" {
		t.Fatalf("user = %q, want alice@example.com", seen)
	}
}

func TestKixTokenResolvesEmail(t *testing.T) {
	token := strings.Repeat("a", 26)
	mm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/users/me" || r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"email":"resolved@kix.co.il"}`))
	}))
	defer mm.Close()

	var seen string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = r.Header.Get("X-Auth-Request-User") })
	m := &auth.Middleware{Next: next, MattermostURL: mm.URL, Client: mm.Client()}

	req := httptest.NewRequest("GET", "/v1/webdav/x", nil)
	req.Header.Set("X-Auth-Request-User", "kixtoken@")
	req.Header.Set("Authorization", basic("ignored", token))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if seen != "resolved@kix.co.il" {
		t.Fatalf("resolved user = %q, want resolved@kix.co.il (code=%d)", seen, rec.Code)
	}
}

func TestKixTokenBadFormatForbidden(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next should not be called") })
	m := &auth.Middleware{Next: next, Client: http.DefaultClient}

	req := httptest.NewRequest("GET", "/v1/webdav/x", nil)
	req.Header.Set("X-Auth-Request-User", "kixtoken@")
	req.Header.Set("Authorization", basic("user", "too-short"))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
