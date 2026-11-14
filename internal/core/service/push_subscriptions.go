package service

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// PushSubscriptionsService handles requests about Push Subscriptions, and
// sends the Owner's test Notification. The NotificationsService sends the
// others, at the tick.
type PushSubscriptionsService struct {
	pushSubscriptions model.PushSubscriptionsRepository
	vapid             VAPID
	now               func() time.Time
}

// NewPushSubscriptions returns the PushSubscriptionsService, giving the web
// app vapid's public key to subscribe with and signing test Notifications
// with vapid, with now as its clock.
func NewPushSubscriptions(pushSubscriptions model.PushSubscriptionsRepository, vapid VAPID, now func() time.Time) *PushSubscriptionsService {
	return &PushSubscriptionsService{pushSubscriptions: pushSubscriptions, vapid: vapid, now: now}
}

// Routes adds the Push Subscription routes to mux, under
// /api/push-subscriptions.
func (s *PushSubscriptionsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/push-subscriptions/vapid-public-key", s.getVAPIDPublicKey)
	mux.HandleFunc("POST /api/push-subscriptions", s.postPushSubscription)
	mux.HandleFunc("DELETE /api/push-subscriptions", s.deletePushSubscription)
	mux.HandleFunc("POST /api/push-subscriptions/test", s.sendTest)
}

// getVAPIDPublicKey answers the key the web app passes to
// pushManager.subscribe as its applicationServerKey.
func (s *PushSubscriptionsService) getVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, model.VAPIDPublicKeyJSON{PublicKey: s.vapid.PublicKey})
}

// checkPushSubscription explains what makes j unusable for sending, if anything.
func checkPushSubscription(j model.PushSubscriptionJSON) error {
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

func (s *PushSubscriptionsService) postPushSubscription(w http.ResponseWriter, r *http.Request) {
	var j model.PushSubscriptionJSON
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
	if err := s.pushSubscriptions.Save(r.Context(), sub); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deletePushSubscription unregisters the Push Subscription whose endpoint is
// the query's endpoint.
func (s *PushSubscriptionsService) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint == "" {
		http.Error(w, "endpoint is required", http.StatusBadRequest)
		return
	}
	if err := s.pushSubscriptions.Delete(r.Context(), endpoint); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testNotification is what the Owner's test Notification says.
var testNotification = model.Notification{Title: "Test Notification", Body: "Notifications reach this device.", URL: "/settings"}

// sendTest sends the test Notification to every Push Subscription at once,
// outside the tick, and answers what came of it.
func (s *PushSubscriptionsService) sendTest(w http.ResponseWriter, r *http.Request) {
	subs, err := s.pushSubscriptions.List(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	sent := model.TestSentJSON{Failed: []model.TestFailedJSON{}}
	for _, sub := range subs {
		err := deliver(r.Context(), s.vapid, sub, testNotification)
		switch {
		case err == nil:
			sent.Sent++
		case errors.Is(err, errPushSubscriptionGone):
			// The Owner uninstalled Nando there, or turned Notifications off.
			if err := s.pushSubscriptions.Delete(r.Context(), sub.Endpoint); err != nil {
				utils.ServerError(w, err)
				return
			}
			sent.Removed++
		default:
			sent.Failed = append(sent.Failed, testFailed(sub, err))
		}
	}
	utils.WriteJSON(w, http.StatusOK, sent)
}

// testFailed reports that sending the test Notification to sub failed with
// err, naming the push service by its host and leaving out the endpoint, which
// identifies the device.
func testFailed(sub model.PushSubscription, err error) model.TestFailedJSON {
	failed := model.TestFailedJSON{Reason: err.Error()}
	if endpoint, parseErr := url.Parse(sub.Endpoint); parseErr == nil {
		failed.PushService = endpoint.Host
	}
	var answered pushServiceError
	var request *url.Error
	switch {
	case errors.As(err, &answered):
		failed.Status = answered.Status
	case errors.As(err, &request):
		// Its message quotes the endpoint; keep only what went wrong.
		failed.Reason = request.Err.Error()
	}
	return failed
}
