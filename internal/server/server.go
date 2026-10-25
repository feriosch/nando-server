// Package server builds Nando's HTTP API.
package server

import (
	"net/http"
	"slices"
	"strings"

	"github.com/feriosch/nando-server/internal/core"
)

// Config is what the HTTP API needs to know about its environment.
type Config struct {
	// AllowedOrigins are the web app origins that may call the API with credentials.
	AllowedOrigins []string
	// Core signs the Owner in and guards every /api/ route. Nil serves only
	// /health, for tests of the API's plumbing.
	Core *core.Core
}

// New returns the handler serving the whole API.
func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	// Not /healthz: Cloud Run reserves paths ending in "z" and answers them itself.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if cfg.Core == nil {
		return cors(cfg.AllowedOrigins, mux)
	}
	cfg.Core.Routes(mux)
	return cors(cfg.AllowedOrigins, requireSessionForAPI(cfg.Core, mux))
}

// requireSessionForAPI puts every /api/ path behind a session, whoever
// registered it, so no route there can be served to a signed-out device.
func requireSessionForAPI(c *core.Core, next http.Handler) http.Handler {
	guarded := c.RequireSession(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			guarded.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// cors lets the web app call the API with the session cookie.
func cors(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); slices.Contains(allowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
