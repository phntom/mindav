// Package rewrite namespaces every request under the authenticated user's
// directory. An incoming path /v1/webdav/<rest> becomes /v1/webdav/u/<user>/<rest>
// so that object keys are u/<user>/<rest>, matching the historical layout. The
// MOVE/COPY Destination header is rewritten the same way, using X-URI-Prefix
// (set by the ingress) to locate the client-relative portion of the URL.
package rewrite

import (
	"net/http"
	"strings"
)

const prefix = "/v1/webdav/"

// Handler rewrites the request path and Destination header then delegates to Next.
type Handler struct {
	Next http.Handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-Auth-Request-User")

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	r.URL.Path = prefix + "u/" + user + "/" + rest
	r.URL.RawPath = ""

	if dest := r.Header.Get("Destination"); dest != "" {
		uriPrefixLen := len(r.Header.Get("X-URI-Prefix"))
		if uriPrefixLen <= len(dest) {
			destRest := strings.TrimPrefix(dest[uriPrefixLen:], "/")
			r.Header.Set("Destination", "http://"+r.Host+prefix+"u/"+user+"/"+destRest)
		}
	}

	h.Next.ServeHTTP(w, r)
}
