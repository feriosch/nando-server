package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"github.com/feriosch/nando-server/internal/core/model"
)

// VAPID is what signs every push message, so push services know it is from
// Nando.
type VAPID struct {
	// PublicKey and PrivateKey are the key pair, base64url as
	// webpush.GenerateVAPIDKeys returns them.
	PublicKey  string
	PrivateKey string
	// Contact is how a push service can reach Nando's operator, a mailto: or
	// https: URL; Apple requires one.
	Contact string
}

// pushMessageTTL is how long a push service keeps a push message it couldn't
// deliver yet, such as while the device is off.
const pushMessageTTL = 12 * time.Hour

// pushTimeout is how long Core waits for a push service to take a push
// message before giving up on it.
const pushTimeout = 10 * time.Second

// pushServiceError is a push service answering a push message with an
// unsuccessful status.
type pushServiceError struct {
	Status int
}

func (e pushServiceError) Error() string {
	return "push service answered " + e.status()
}

// status is the status the push service answered, such as "410 Gone".
func (e pushServiceError) status() string {
	return fmt.Sprintf("%d %s", e.Status, http.StatusText(e.Status))
}

// gone reports whether the push service no longer knows the Push
// Subscription: the Owner revoked it or uninstalled that copy of Nando.
func (e pushServiceError) gone() bool {
	return e.Status == http.StatusNotFound || e.Status == http.StatusGone
}

// deliver sends n to sub as a push message through sub's push service,
// signed with vapid. It returns a pushServiceError if the push service
// answers a failure, gone if it no longer knows sub. It gives up after
// pushTimeout.
func deliver(ctx context.Context, vapid VAPID, sub model.PushSubscription, n model.Notification) error {
	ctx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	message, err := json.Marshal(model.PushMessageJSON{Title: n.Title, Body: n.Body, URL: n.URL})
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, message, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      vapid.Contact,
		VAPIDPublicKey:  vapid.PublicKey,
		VAPIDPrivateKey: vapid.PrivateKey,
		TTL:             int(pushMessageTTL.Seconds()),
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return pushServiceError{Status: resp.StatusCode}
	}
	return nil
}
