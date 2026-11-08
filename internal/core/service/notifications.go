package service

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"github.com/feriosch/nando-server/internal/core/model"
)

const (
	// tickInterval is how often Cloud Scheduler calls /internal/tick.
	tickInterval = 30 * time.Minute
	// sentFor is how long a sent Notification's ID is remembered: far longer
	// than any tick is retried, and longer than a provider could still be
	// returning it.
	sentFor = 30 * 24 * time.Hour
)

// AddNotificationProvider has Core ask p for its due Notifications at every tick.
func (s *Service) AddNotificationProvider(p model.NotificationProvider) {
	s.providersMu.Lock()
	defer s.providersMu.Unlock()
	s.providers = append(s.providers, p)
}

// getVAPIDPublicKey answers the key the web app passes to
// pushManager.subscribe as its applicationServerKey.
func (s *Service) getVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, struct {
		PublicKey string `json:"publicKey"`
	}{s.vapidPublicKey})
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
	if key, err := decodeKey(j.Keys.P256dh); err != nil {
		return errors.New("keys.p256dh must be a P-256 public key, base64url")
	} else if _, err := ecdh.P256().NewPublicKey(key); err != nil {
		return errors.New("keys.p256dh must be a P-256 public key, base64url")
	}
	if secret, err := decodeKey(j.Keys.Auth); err != nil || len(secret) != 16 {
		return errors.New("keys.auth must be a 16-byte secret, base64url")
	}
	return nil
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

// tick is called by Cloud Scheduler every half hour. Only the scheduler may
// call it.
func (s *Service) tick(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		http.Error(w, "a scheduler token is required", http.StatusUnauthorized)
		return
	}
	if err := s.verifySchedulerToken(r.Context(), token); err != nil {
		log.Printf("tick: rejected token: %v", err)
		http.Error(w, "not the scheduler", http.StatusUnauthorized)
		return
	}
	if err := s.sendDueNotifications(r.Context()); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sendDueNotifications sends every provider's due Notifications to every Push
// Subscription. A provider that fails doesn't stop the others; its error is
// returned once they are done, so the scheduler retries the tick.
func (s *Service) sendDueNotifications(ctx context.Context) error {
	slot := s.now().Truncate(tickInterval)
	s.providersMu.Lock()
	providers := append([]model.NotificationProvider(nil), s.providers...)
	s.providersMu.Unlock()

	subs, err := s.repo.ListPushSubscriptions(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range providers {
		due, err := p.DueNotifications(ctx, slot)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, n := range due {
			// Claimed before sending: a Notification a push service failed to
			// take is lost rather than possibly sent twice.
			claimed, err := s.repo.ClaimNotification(ctx, n.ID, s.now(), s.now().Add(sentFor))
			if err != nil {
				return err
			}
			if !claimed {
				continue
			}
			for _, sub := range subs {
				err := s.push(ctx, sub, n)
				if errors.Is(err, errPushSubscriptionGone) {
					err = s.repo.DeletePushSubscription(ctx, sub.Endpoint)
				}
				if err != nil {
					log.Printf("send Notification %s: %v", n.ID, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

// errPushSubscriptionGone means the push service no longer knows a Push
// Subscription: the Owner revoked it or uninstalled that copy of Nando.
var errPushSubscriptionGone = errors.New("the push service no longer knows this Push Subscription")

// pushMessageJSON is the Notification as the web app's service worker reads it.
type pushMessageJSON struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// push sends n to sub through sub's push service.
func (s *Service) push(ctx context.Context, sub model.PushSubscription, n model.Notification) error {
	message, err := json.Marshal(pushMessageJSON{Title: n.Title, Body: n.Body, URL: n.URL})
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, message, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      s.pushContact,
		VAPIDPublicKey:  s.vapidPublicKey,
		VAPIDPrivateKey: s.vapidPrivateKey,
		TTL:             int((12 * time.Hour).Seconds()),
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return errPushSubscriptionGone
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("push service answered %s", resp.Status)
	}
	return nil
}
