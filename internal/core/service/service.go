// Package service handles Core's requests and validates them.
// It never touches the database; it reaches data only through CoreRepository.
package service

import (
	"context"
	"encoding/json"
	"errors"
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
	// VerifySchedulerToken checks the bearer token of a call to /internal/tick.
	VerifySchedulerToken func(ctx context.Context, token string) error
	// VAPID signs every push message.
	VAPID VAPID
}

// Service handles Core's requests.
type Service struct {
	repo     model.CoreRepository
	webauthn *webauthn.WebAuthn
	domain   string
	now      func() time.Time

	verifySchedulerToken func(ctx context.Context, token string) error
	vapid                VAPID
	providers            []model.NotificationProvider
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
	if cfg.VerifySchedulerToken == nil {
		return nil, errors.New("VerifySchedulerToken is required")
	}
	s := &Service{
		repo:     repo,
		webauthn: relyingParty,
		domain:   cfg.Domain,
		now:      now,

		verifySchedulerToken: cfg.VerifySchedulerToken,
		vapid:                cfg.VAPID,
	}
	s.providers = []model.NotificationProvider{weighInReminder{settings: s.settings}}
	return s, nil
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

	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/weigh-ins", s.listWeighIns)
	mux.HandleFunc("PUT /api/weigh-ins/{week}", s.putWeighIn)
	mux.HandleFunc("DELETE /api/weigh-ins/{week}", s.deleteWeighIn)
	mux.HandleFunc("GET /api/body", s.getBody)

	mux.HandleFunc("GET /api/push-subscriptions/vapid-public-key", s.getVAPIDPublicKey)
	mux.HandleFunc("POST /api/push-subscriptions", s.postPushSubscription)
	mux.HandleFunc("DELETE /api/push-subscriptions", s.deletePushSubscription)
	mux.HandleFunc("POST /internal/tick", s.tick)
}

// WriteJSON answers with status and v as JSON. Modules use it too, so every
// part of the API answers alike.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// ServerError logs err and answers 500 without revealing it. Modules use it
// too.
func ServerError(w http.ResponseWriter, err error) {
	log.Print(err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}
