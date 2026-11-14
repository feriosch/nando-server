package tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/feriosch/nando-server/internal/testpush"
)

// testSentJSON is what POST /api/push-subscriptions/test answers: what came
// of the test Notification at each Push Subscription.
type testSentJSON struct {
	Sent    int `json:"sent"`
	Removed int `json:"removed"`
	Failed  []struct {
		PushService string `json:"pushService"`
		Status      int    `json:"status"`
		Reason      string `json:"reason"`
	} `json:"failed"`
}

// sendTest has the Owner send a test Notification from Settings.
func (d *browser) sendTest() testSentJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/api/push-subscriptions/test", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("POST /api/push-subscriptions/test: status = %d, body %q; want 200", rec.Code, rec.Body)
	}
	var sent testSentJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &sent); err != nil {
		d.nando.t.Fatalf("POST /api/push-subscriptions/test: %v in %q", err, rec.Body)
	}
	return sent
}

// theTestNotification is what each device shows for a test Notification.
var theTestNotification = testpush.Notification{Title: "Test Notification", Body: "Notifications reach this device.", URL: "/settings"}

func TestATestNotificationReachesEveryDeviceAtOnce(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	homeScreenApp, mac := testpush.New(t), testpush.New(t)
	phone.subscribe(homeScreenApp)
	phone.subscribe(mac)

	sent := phone.sendTest()

	if sent.Sent != 2 || sent.Removed != 0 || len(sent.Failed) != 0 {
		t.Errorf("answer = %+v; want sent to both devices", sent)
	}
	for name, push := range map[string]*testpush.Service{"Home Screen app": homeScreenApp, "Mac": mac} {
		if got := push.Received(); !slices.Equal(got, []testpush.Notification{theTestNotification}) {
			t.Errorf("%s received %+v; want the test Notification", name, got)
		}
	}
}

func TestATestNotificationCanBeSentAgainRightAway(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)

	phone.sendTest()
	again := phone.sendTest()

	if again.Sent != 1 {
		t.Errorf("second answer = %+v; want sent again", again)
	}
	if got := push.Received(); !slices.Equal(got, []testpush.Notification{theTestNotification, theTestNotification}) {
		t.Errorf("device received %+v; want the test Notification twice", got)
	}
}

func TestATestNotificationRemovesADeviceThatIsGone(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	uninstalled, mac := testpush.New(t), testpush.New(t)
	phone.subscribe(uninstalled)
	phone.subscribe(mac)
	uninstalled.Answer(http.StatusGone)

	sent := phone.sendTest()

	if sent.Sent != 1 || sent.Removed != 1 || len(sent.Failed) != 0 {
		t.Errorf("answer = %+v; want sent to the Mac and the gone device removed", sent)
	}
	phone.sendTest()
	if got := uninstalled.RequestCount(); got != 1 {
		t.Errorf("gone device was asked %d times; want once, then never again", got)
	}
}

func TestATestNotificationReportsAFailingPushServiceByItsHost(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	failing := testpush.New(t)
	phone.subscribe(failing)
	failing.Answer(http.StatusInternalServerError)

	sent := phone.sendTest()

	endpoint, err := url.Parse(failing.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	if sent.Sent != 0 || sent.Removed != 0 || len(sent.Failed) != 1 {
		t.Fatalf("answer = %+v; want one failure", sent)
	}
	failed := sent.Failed[0]
	if failed.PushService != endpoint.Host || failed.Status != http.StatusInternalServerError || failed.Reason == "" {
		t.Errorf("failure = %+v; want push service %s answering 500, with a reason", failed, endpoint.Host)
	}
	if strings.Contains(failed.Reason, endpoint.Path) {
		t.Errorf("reason %q gives away the endpoint", failed.Reason)
	}
	phone.sendTest()
	if got := failing.RequestCount(); got != 2 {
		t.Errorf("failing device was asked %d times; want it kept and asked again", got)
	}
}

func TestATestNotificationWithNoDevicesSendsNothing(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	rec := phone.do(http.MethodPost, "/api/push-subscriptions/test", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/push-subscriptions/test: status = %d, body %q; want 200", rec.Code, rec.Body)
	}
	var answer map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if failed, ok := answer["failed"].([]any); answer["sent"] != 0.0 || answer["removed"] != 0.0 || !ok || len(failed) != 0 {
		t.Errorf("answer = %s; want nothing sent and an empty failed list", rec.Body)
	}
}

func TestATestNotificationNeedsASession(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	push := testpush.New(t)
	phone.subscribe(push)

	rec := n.newBrowser("Stranger").do(http.MethodPost, "/api/push-subscriptions/test", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /api/push-subscriptions/test without a Session: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := push.RequestCount(); got != 0 {
		t.Errorf("device was asked %d times; want never", got)
	}
}
