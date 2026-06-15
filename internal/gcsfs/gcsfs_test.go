package gcsfs_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"golang.org/x/net/webdav"

	"github.com/phntom/mindav/internal/gcsfs"
	"github.com/phntom/mindav/internal/rewrite"
)

const (
	testBucket = "test-bucket"
	testUser   = "test@example.com"
)

func newStack(t *testing.T, seed ...fakestorage.Object) (*storage.Client, http.Handler) {
	t.Helper()
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{InitialObjects: seed})
	if err != nil {
		t.Fatalf("fake gcs server: %v", err)
	}
	t.Cleanup(server.Stop)
	if len(seed) == 0 {
		server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: testBucket})
	}
	client := server.Client()
	dav := &webdav.Handler{
		Prefix:     "/v1/webdav",
		FileSystem: gcsfs.New(client, testBucket),
		LockSystem: webdav.NewMemLS(),
	}
	return client, &rewrite.Handler{Next: dav}
}

func do(t *testing.T, h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Host = "pwd.example.com"
	req.Header.Set("X-Auth-Request-User", testUser)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func content(t *testing.T, c *storage.Client, key string) (string, error) {
	t.Helper()
	r, err := c.Bucket(testBucket).Object(key).NewReader(context.Background())
	if err != nil {
		return "", err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	return string(b), err
}

func TestWebDAVRoundTrip(t *testing.T) {
	client, h := newStack(t)
	userKey := func(name string) string { return "u/" + testUser + "/" + name }

	if rec := do(t, h, "PUT", "/v1/webdav/hello.txt", "hi there", nil); rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d body=%s", rec.Code, rec.Body.String())
	}
	if got, err := content(t, client, userKey("hello.txt")); err != nil || got != "hi there" {
		t.Fatalf("stored object = %q err=%v (want namespaced under u/<user>/)", got, err)
	}

	if rec := do(t, h, "GET", "/v1/webdav/hello.txt", "", nil); rec.Code != http.StatusOK || rec.Body.String() != "hi there" {
		t.Fatalf("GET = %d %q", rec.Code, rec.Body.String())
	}

	rec := do(t, h, "PROPFIND", "/v1/webdav/", "", map[string]string{"Depth": "1"})
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("PROPFIND = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hello.txt") {
		t.Fatalf("PROPFIND body missing hello.txt: %s", rec.Body.String())
	}

	rec = do(t, h, "MOVE", "/v1/webdav/hello.txt", "", map[string]string{
		"Destination":  "http://pwd.example.com/webdav/moved.txt",
		"X-URI-Prefix": "http://pwd.example.com/webdav",
	})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("MOVE = %d body=%s", rec.Code, rec.Body.String())
	}
	if got, err := content(t, client, userKey("moved.txt")); err != nil || got != "hi there" {
		t.Fatalf("moved object = %q err=%v", got, err)
	}
	if _, err := client.Bucket(testBucket).Object(userKey("hello.txt")).Attrs(context.Background()); !errors.Is(err, storage.ErrObjectNotExist) {
		t.Fatalf("source still present after MOVE: err=%v", err)
	}

	if rec := do(t, h, "DELETE", "/v1/webdav/moved.txt", "", nil); rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if _, err := client.Bucket(testBucket).Object(userKey("moved.txt")).Attrs(context.Background()); !errors.Is(err, storage.ErrObjectNotExist) {
		t.Fatalf("object present after DELETE: err=%v", err)
	}
}

func TestReadSeededObject(t *testing.T) {
	_, h := newStack(t, fakestorage.Object{
		ObjectAttrs: fakestorage.ObjectAttrs{BucketName: testBucket, Name: "u/" + testUser + "/existing.txt"},
		Content:     []byte("seed-data"),
	})
	if rec := do(t, h, "GET", "/v1/webdav/existing.txt", "", nil); rec.Code != http.StatusOK || rec.Body.String() != "seed-data" {
		t.Fatalf("GET seeded = %d %q", rec.Code, rec.Body.String())
	}
}

func TestGetMissing404(t *testing.T) {
	_, h := newStack(t)
	if rec := do(t, h, "GET", "/v1/webdav/nope.txt", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing = %d, want 404", rec.Code)
	}
}
