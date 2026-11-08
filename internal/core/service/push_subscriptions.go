package service

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/feriosch/nando-server/internal/core/model"
)

// getVAPIDPublicKey answers the key the web app passes to
// pushManager.subscribe as its applicationServerKey.
func (s *Service) getVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, struct {
		PublicKey string `json:"publicKey"`
	}{s.vapid.PublicKey})
}

// pushSubscriptionJSON is a Push Subscription as PushSubscription.toJSON()
// gives it in the browser.
type pushSubscriptionJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// checkPushSubscription explains what makes j unusable for sending, if anything.
func checkPushSubscription(j pushSubscriptionJSON) error {
	if u, err := url.Parse(j.Endpoint); err != nil || !u.IsAbs() || u.Host == "" ||
		(u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("endpoint must be the push service's URL")
	}
	if !isP256PublicKey(j.Keys.P256dh) {
		return errors.New("keys.p256dh must be a P-256 public key, base64url")
	}
	if secret, err := decodeKey(j.Keys.Auth); err != nil || len(secret) != 16 {
		return errors.New("keys.auth must be a 16-byte secret, base64url")
	}
	return nil
}

// isP256PublicKey reports whether key is a P-256 public key, base64url.
func isP256PublicKey(key string) bool {
	raw, err := decodeKey(key)
	if err != nil {
		return false
	}
	_, err = ecdh.P256().NewPublicKey(raw)
	return err == nil
}

// decodeKey decodes a key as PushSubscription.toJSON() gives it: base64url,
// usually without padding.
func decodeKey(key string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(key, "="))
}

func (s *Service) postPushSubscription(w http.ResponseWriter, r *http.Request) {
	var j pushSubscriptionJSON
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "the body isn't a PushSubscription", http.StatusBadRequest)
		return
	}
	if err := checkPushSubscription(j); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sub := model.PushSubscription{
		Endpoint:  j.Endpoint,
		P256dh:    j.Keys.P256dh,
		Auth:      j.Keys.Auth,
		CreatedAt: s.now(),
	}
	if err := s.repo.SavePushSubscription(r.Context(), sub); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deletePushSubscription unregisters the Push Subscription whose endpoint is
// the query's endpoint.
func (s *Service) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		http.Error(w, "endpoint is required", http.StatusBadRequest)
		return
	}
	if err := s.repo.DeletePushSubscription(r.Context(), endpoint); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
