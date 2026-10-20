// Package server builds Nando's HTTP API.
package server

import (
	"net/http"
	"slices"
)

// Config is what the HTTP API needs to know about its environment.
type Config struct {
	// AllowedOrigins are the web app origins that may call the API with credentials.
	AllowedOrigins []string
}

// New returns the handler serving the whole API.
func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	// Not /healthz: Cloud Run reserves paths ending in "z" and answers them itself.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return cors(cfg.AllowedOrigins, mux)
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
