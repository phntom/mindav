// Command mindav is a WebDAV server backed by Google Cloud Storage.
//
// It preserves the behavior of the previous totoval/MinIO implementation:
// requests are authenticated via the X-Auth-Request-User header (set by the
// fronting oauth2-proxy), each user's files are namespaced under u/<user>/ in
// the bucket, and the service is served under the /v1/webdav prefix.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/net/webdav"

	"github.com/phntom/mindav/internal/auth"
	"github.com/phntom/mindav/internal/gcsfs"
	"github.com/phntom/mindav/internal/rewrite"
)

func main() {
	bucket := mustEnv("GCS_BUCKET")
	port := envOr("PORT", "8080")
	mattermostURL := envOr("MATTERMOST_URL", "https://kix.co.il")

	client, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatalf("create storage client: %v", err)
	}

	dav := &webdav.Handler{
		Prefix:     "/v1/webdav",
		FileSystem: gcsfs.New(client, bucket),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("webdav %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	handler := &auth.Middleware{
		Next:          &rewrite.Handler{Next: dav},
		MattermostURL: mattermostURL,
		Client:        &http.Client{Timeout: 10 * time.Second},
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}

	idle := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
		close(idle)
	}()

	log.Printf("mindav listening on :%s (bucket=%s)", port, bucket)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
	<-idle
	_ = client.Close()
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
