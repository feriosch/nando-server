// Package service handles Core's requests and validates them.
// It never touches the database; it reaches data only through CoreRepository.
package service

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/feriosch/nando-server/internal/core/model"
)

// Config is what the service needs to know about where Nando is served.
type Config struct {
	// Domain is the passkeys' relying-party ID and the session cookie's domain.
	Domain string
	// Origins are the web app origins passkeys are used from.
	Origins []string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Service handles Core's requests.
type Service struct {
	repo     model.CoreRepository
	webauthn *webauthn.WebAuthn
	domain   string
	now      func() time.Time
}

// New returns a Service reaching data through repo.
func New(repo model.CoreRepository, cfg Config) (*Service, error) {
	relyingParty, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.Domain,
		RPDisplayName: "Nando",
		RPOrigins:     cfg.Origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout},
		},
	})
	if err != nil {
		return nil, err
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, webauthn: relyingParty, domain: cfg.Domain, now: now}, nil
}

// Routes adds Core's routes to mux: signing in under /auth/, and everything
// else under /api/, which must be served behind RequireSession.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register/begin", s.beginRegistration)
	mux.HandleFunc("POST /auth/register/finish", s.finishRegistration)
	mux.HandleFunc("POST /auth/signin/begin", s.beginSignIn)
	mux.HandleFunc("POST /auth/signin/finish", s.finishSignIn)

	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
	mux.HandleFunc("GET /api/passkeys", s.listPasskeys)
	mux.HandleFunc("DELETE /api/passkeys/{id}", s.deletePasskey)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}
