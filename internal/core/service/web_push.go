package service

import (
	"context"
	"encoding/json"
	"errors"
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

// errPushSubscriptionGone means the push service no longer knows a Push
// Subscription: the Owner revoked it or uninstalled that copy of Nando.
var errPushSubscriptionGone = errors.New("the push service no longer knows this Push Subscription")

// pushMessageJSON is what a push message carries: one Notification, as the
// web app's service worker reads it. An empty field is left out, so the
// service worker shows its default ("Nando" for the title, Home for the url).
type pushMessageJSON struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	URL   string `json:"url,omitempty"`
}

// deliver sends n to sub as a push message through sub's push service. It
// returns errPushSubscriptionGone if the push service no longer knows sub.
func (s *Service) deliver(ctx context.Context, sub model.PushSubscription, n model.Notification) error {
	message, err := json.Marshal(pushMessageJSON{Title: n.Title, Body: n.Body, URL: n.URL})
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, message, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      s.vapid.Contact,
		VAPIDPublicKey:  s.vapid.PublicKey,
		VAPIDPrivateKey: s.vapid.PrivateKey,
		TTL:             int(pushMessageTTL.Seconds()),
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
