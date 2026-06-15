// Package auth gates WebDAV requests on the X-Auth-Request-User header set by
// the fronting oauth2-proxy. The special user "kixtoken@" lets non-browser
// clients (e.g. KeePass) authenticate with a Mattermost personal access token
// supplied as the HTTP basic-auth password; the token is validated against the
// Mattermost API and the resolved email becomes the effective user.
package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

const tokenUser = "kixtoken@"

var tokenRe = regexp.MustCompile(`^[a-z0-9]{26}$`)

// Middleware authenticates a request then delegates to Next.
type Middleware struct {
	Next          http.Handler
	MattermostURL string
	Client        *http.Client
}

func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := r.Header.Get("X-Auth-Request-User")
	if user == "" {
		forbidden(w, "Access Denied")
		return
	}
	if user == tokenUser {
		email, msg := m.resolveToken(r)
		if msg != "" {
			forbidden(w, msg)
			return
		}
		r.Header.Set("X-Auth-Request-User", email)
	}
	m.Next.ServeHTTP(w, r)
}

// resolveToken extracts the basic-auth password as a Mattermost personal access
// token, validates it, and returns the user's email. A non-empty second return
// value is a client-facing error message.
func (m *Middleware) resolveToken(r *http.Request) (email, errMsg string) {
	const badAuth = "Please use basic auth with token in the password field"
	const badToken = "Invalid personal access token, please generate at https://kix.co.il Account Settings > Security"
	const unauthorized = "Unauthorized personal access token, please generate at https://kix.co.il Account Settings > Security"

	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Basic" {
		return "", badAuth
	}
	payload, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", badAuth
	}
	creds := strings.SplitN(string(payload), ":", 2)
	if len(creds) != 2 {
		return "", badAuth
	}
	token := creds[1]
	if !tokenRe.MatchString(token) {
		return "", badToken
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, m.MattermostURL+"/api/v4/users/me", nil)
	if err != nil {
		return "", unauthorized
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := m.Client.Do(req)
	if err != nil {
		return "", unauthorized
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", unauthorized
	}
	var me struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil || me.Email == "" {
		return "", unauthorized
	}
	return me.Email, ""
}

func forbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"Error": msg})
}
