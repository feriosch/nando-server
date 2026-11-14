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

// pushTimeout is how long Core waits for a push service to take a push
// message before giving up on it.
const pushTimeout = 10 * time.Second

// pushServiceError is a push service answering a push message with an
// unsuccessful status other than gone.
type pushServiceError struct {
	Status int
}

func (e pushServiceError) Error() string {
	return fmt.Sprintf("push service answered %d %s", e.Status, http.StatusText(e.Status))
}

// deliver sends n to sub as a push message through sub's push service,
// signed with vapid. It returns errPushSubscriptionGone if the push service no
// longer knows sub, and a pushServiceError if it answers another failure. It
// gives up after pushTimeout.
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
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return errPushSubscriptionGone
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return pushServiceError{Status: resp.StatusCode}
	}
	return nil
}
