package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/server"
	"github.com/feriosch/nando-server/internal/testmongo"
)

const (
	webOrigin     = "https://web.example.com"
	sessionCookie = "nando_session"
)

// clock is the time Core sees; tests set it and move it forward by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }
func (c *clock) Set(t time.Time)         { c.now = t }

// nando is the whole HTTP API over a fresh MongoDB, as main wires it.
type nando struct {
	t       *testing.T
	core    *core.Core
	handler http.Handler
	clock   *clock
}

func newNando(t *testing.T) *nando {
	t.Helper()
	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(testmongo.URI(t)))
	if err != nil {
		t.Fatalf("connect MongoDB: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	// Real time to start with: go-webauthn checks its ceremony timeouts against it.
	clk := &clock{now: time.Now()}
	c, err := core.New(ctx, client.Database("core"), core.Config{
		Domain:  "example.com",
		Origins: []string{webOrigin},
		Now:     clk.Now,
	})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return &nando{
		t:       t,
		core:    c,
		handler: server.New(server.Config{AllowedOrigins: []string{webOrigin}, Core: c}),
		clock:   clk,
	}
}

// issueEnrollmentCode does what `nando-server enroll` does.
func (n *nando) issueEnrollmentCode() string {
	n.t.Helper()
	code, err := n.core.IssueEnrollmentCode(context.Background())
	if err != nil {
		n.t.Fatalf("IssueEnrollmentCode: %v", err)
	}
	return code
}

// device is one browser (or Home Screen app) using Nando, with its own cookies
// and, once it has one, a passkey.
type device struct {
	nando     *nando
	userAgent string
	session   *http.Cookie
	passkey   *passkey
}

func (n *nando) newDevice(userAgent string) *device {
	return &device{nando: n, userAgent: userAgent}
}

// do sends a request from the web app, carrying and keeping the session cookie
// the way a browser would.
func (d *device) do(method, path string, body []byte) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Origin", webOrigin)
	req.Header.Set("User-Agent", d.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if d.session != nil {
		req.AddCookie(d.session)
	}
	rec := httptest.NewRecorder()
	d.nando.handler.ServeHTTP(rec, req)
	if c := sessionCookieIn(rec); c != nil {
		d.session = c
		if c.MaxAge < 0 { // Max-Age=0: the browser deletes the cookie
			d.session = nil
		}
	}
	return rec
}

// enroll registers a new passkey on this device with code, returning the
// response that ends the ceremony (or the first one that fails). If the
// server excludes the passkey this device already has, the browser refuses
// and enroll returns the begin response, as no finish request is sent.
func (d *device) enroll(code string) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/auth/register/begin", mustJSON(d.nando.t, map[string]string{"code": code}))
	if rec.Code != http.StatusOK {
		return rec
	}
	if d.passkey != nil && d.passkey.excludedBy(d.nando.t, rec.Body.Bytes()) {
		return rec
	}
	d.passkey = &passkey{}
	return d.do(http.MethodPost, "/auth/register/finish", d.passkey.create(d.nando.t, rec.Body.Bytes()))
}

// mustEnroll enrolls with a fresh Enrollment Code and fails the test if that
// doesn't work.
func (d *device) mustEnroll() {
	d.nando.t.Helper()
	if rec := d.enroll(d.nando.issueEnrollmentCode()); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("enroll: status = %d, body %q", rec.Code, rec.Body)
	}
}

// signIn signs this device in with its passkey, returning the response that
// ends the ceremony (or the first one that fails).
func (d *device) signIn() *httptest.ResponseRecorder {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/auth/signin/begin", nil)
	if rec.Code != http.StatusOK {
		return rec
	}
	return d.do(http.MethodPost, "/auth/signin/finish", d.passkey.get(d.nando.t, rec.Body.Bytes()))
}

// mustSignIn signs in and fails the test if that doesn't work.
func (d *device) mustSignIn() {
	d.nando.t.Helper()
	if rec := d.signIn(); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("sign in: status = %d, body %q", rec.Code, rec.Body)
	}
}

// syncedFrom gives this device the passkey on other, as iCloud Keychain does.
func (d *device) syncedFrom(other *device) *device {
	d.passkey = other.passkey
	return d
}

func TestAPIWithoutASessionIsUnauthorized(t *testing.T) {
	n := newNando(t)

	rec := n.newDevice("iPhone").do(http.MethodGet, "/api/sessions", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/sessions without a session: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOwnerEnrollsAPasskeyWithAnEnrollmentCode(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")

	if rec := phone.enroll(n.issueEnrollmentCode()); rec.Code != http.StatusNoContent {
		t.Fatalf("enroll: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions after enrolling: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestOwnerSignsInWithTheirPasskey(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	mac := n.newDevice("Mac").syncedFrom(phone)

	if rec := mac.signIn(); rec.Code != http.StatusNoContent {
		t.Fatalf("sign in: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := mac.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions after signing in: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestUsedEnrollmentCodeIsRejected(t *testing.T) {
	n := newNando(t)
	code := n.issueEnrollmentCode()
	if rec := n.newDevice("iPhone").enroll(code); rec.Code != http.StatusNoContent {
		t.Fatalf("first enroll: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := n.newDevice("iPhone").enroll(code); rec.Code != http.StatusUnauthorized {
		t.Fatalf("enroll again with the same code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestEnrollmentCodeExpiresAfter15Minutes(t *testing.T) {
	n := newNando(t)
	stale := n.issueEnrollmentCode()
	n.clock.Advance(14 * time.Minute)
	fresh := n.issueEnrollmentCode()
	n.clock.Advance(2 * time.Minute)

	if rec := n.newDevice("iPhone").enroll(stale); rec.Code != http.StatusUnauthorized {
		t.Errorf("enroll with a 16-minute-old code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := n.newDevice("iPhone").enroll(fresh); rec.Code != http.StatusNoContent {
		t.Errorf("enroll with a 2-minute-old code: status = %d, body %q", rec.Code, rec.Body)
	}
}

func TestSessionCookieCoversWebAndAPIButNoScripts(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	c := phone.session
	if c == nil {
		t.Fatal("enroll set no session cookie")
	}
	if c.Domain != "example.com" || c.Path != "/" {
		t.Errorf("cookie scope = %s%s, want example.com/ (both web. and api.)", c.Domain, c.Path)
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie HttpOnly=%t Secure=%t SameSite=%v, want HttpOnly, Secure, SameSite=Lax", c.HttpOnly, c.Secure, c.SameSite)
	}
	if want := 90 * 24 * 60 * 60; c.MaxAge != want {
		t.Errorf("cookie Max-Age = %d, want %d (90 days)", c.MaxAge, want)
	}
}

func TestUnusedSessionEndsAfter90Days(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	n.clock.Advance(90*24*time.Hour + time.Minute)

	if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/sessions 90 days after signing in: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestSessionInUseStaysSignedIn(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	for day := 60; day <= 180; day += 60 {
		n.clock.Advance(60 * 24 * time.Hour)
		if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sessions on day %d, 60 days after last use: status = %d, want %d", day, rec.Code, http.StatusOK)
		}
	}
}

func TestSessionRenewsAtMostOnceADay(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	n.clock.Advance(23 * time.Hour)
	rec := phone.do(http.MethodGet, "/api/sessions", nil)
	if got := rec.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("23 hours after signing in: Set-Cookie = %q, want none", got)
	}

	n.clock.Advance(2 * time.Hour)
	rec = phone.do(http.MethodGet, "/api/sessions", nil)
	renewed := sessionCookieIn(rec)
	if renewed == nil {
		t.Fatal("25 hours after signing in: no session cookie set, want it renewed")
	}
	if want := 90 * 24 * 60 * 60; renewed.MaxAge != want {
		t.Errorf("renewed cookie Max-Age = %d, want %d (90 days)", renewed.MaxAge, want)
	}
}

// sessionCookieIn returns the session cookie rec sets, if any.
func sessionCookieIn(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

// sessionJSON is one session as GET /api/sessions lists it.
type sessionJSON struct {
	ID        string `json:"id"`
	UserAgent string `json:"userAgent"`
	Current   bool   `json:"current"`
}

func (d *device) listSessions() []sessionJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/sessions", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("GET /api/sessions: status = %d, body %q", rec.Code, rec.Body)
	}
	var sessions []sessionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		d.nando.t.Fatalf("GET /api/sessions: %v in %q", err, rec.Body)
	}
	return sessions
}

func TestOwnerSeesEachSignedInDevice(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	n.clock.Advance(time.Minute)
	mac := n.newDevice("Mac").syncedFrom(phone)
	mac.mustSignIn()

	got := mac.listSessions()

	if len(got) != 2 {
		t.Fatalf("sessions = %+v, want the iPhone's and the Mac's", got)
	}
	for i, want := range []sessionJSON{{UserAgent: "iPhone", Current: false}, {UserAgent: "Mac", Current: true}} {
		if got[i].UserAgent != want.UserAgent || got[i].Current != want.Current {
			t.Errorf("session %d = %+v, want userAgent %q, current %t", i, got[i], want.UserAgent, want.Current)
		}
		if got[i].ID == "" {
			t.Errorf("session %d has no id", i)
		}
	}
}

func TestDeletingASessionSignsThatDeviceOut(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	n.clock.Advance(time.Minute)
	mac := n.newDevice("Mac").syncedFrom(phone)
	mac.mustSignIn()
	phoneSession := mac.listSessions()[0]

	if rec := mac.do(http.MethodDelete, "/api/sessions/"+phoneSession.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the iPhone's session: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("iPhone after its session was deleted: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := mac.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
		t.Errorf("Mac after deleting the iPhone's session: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// passkeyJSON is one passkey as GET /api/passkeys lists it.
type passkeyJSON struct {
	ID string `json:"id"`
}

func (d *device) listPasskeys() []passkeyJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/passkeys", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("GET /api/passkeys: status = %d, body %q", rec.Code, rec.Body)
	}
	var passkeys []passkeyJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &passkeys); err != nil {
		d.nando.t.Fatalf("GET /api/passkeys: %v in %q", err, rec.Body)
	}
	return passkeys
}

func TestOwnerSeesEachPasskey(t *testing.T) {
	n := newNando(t)
	// One passkey in iCloud Keychain, one in Google Password Manager.
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	n.clock.Advance(time.Minute)
	pixel := n.newDevice("Pixel")
	pixel.mustEnroll()

	got := phone.listPasskeys()

	want := []string{b64.EncodeToString(phone.passkey.id), b64.EncodeToString(pixel.passkey.id)}
	if len(got) != len(want) {
		t.Fatalf("passkeys = %+v, want ids %q", got, want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("passkey %d id = %q, want %q", i, got[i].ID, want[i])
		}
	}
}

func TestDeletedPasskeyCanNoLongerSignIn(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	id := phone.listPasskeys()[0].ID

	if rec := phone.do(http.MethodDelete, "/api/passkeys/"+id, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the passkey: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := n.newDevice("Mac").syncedFrom(phone).signIn(); rec.Code != http.StatusUnauthorized {
		t.Errorf("sign in with the deleted passkey: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := phone.listPasskeys(); len(got) != 0 {
		t.Errorf("passkeys after deleting the only one = %+v, want none", got)
	}
}

func TestReEnrollingADeviceKeepsItsPasskey(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	// Without the server excluding it, the authenticator would silently replace
	// this passkey with a new one, leaving the old one listed but unusable.
	phone.enroll(n.issueEnrollmentCode())

	if got := phone.listPasskeys(); len(got) != 1 || got[0].ID != b64.EncodeToString(phone.passkey.id) {
		t.Errorf("passkeys after enrolling the iPhone again = %+v, want only %q", got, b64.EncodeToString(phone.passkey.id))
	}
}

func TestEnrollmentCodeRegistersOnlyOnePasskeyEvenWhenBegunTwice(t *testing.T) {
	n := newNando(t)
	code := mustJSON(t, map[string]string{"code": n.issueEnrollmentCode()})
	phone, mac := n.newDevice("iPhone"), n.newDevice("Mac")
	phoneOptions := phone.do(http.MethodPost, "/auth/register/begin", code)
	macOptions := mac.do(http.MethodPost, "/auth/register/begin", code)
	phone.passkey, mac.passkey = &passkey{}, &passkey{}

	first := phone.do(http.MethodPost, "/auth/register/finish", phone.passkey.create(t, phoneOptions.Body.Bytes()))
	second := mac.do(http.MethodPost, "/auth/register/finish", mac.passkey.create(t, macOptions.Body.Bytes()))

	if first.Code != http.StatusNoContent {
		t.Fatalf("first finish: status = %d, body %q", first.Code, first.Body)
	}
	if second.Code != http.StatusUnauthorized {
		t.Errorf("second finish with the same code: status = %d, want %d", second.Code, http.StatusUnauthorized)
	}
	if got := phone.listPasskeys(); len(got) != 1 || got[0].ID != b64.EncodeToString(phone.passkey.id) {
		t.Errorf("passkeys = %+v, want only the iPhone's %q", got, b64.EncodeToString(phone.passkey.id))
	}
}

func TestDeletingTheCurrentSessionClearsItsCookie(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	current := phone.listSessions()[0]

	rec := phone.do(http.MethodDelete, "/api/sessions/"+current.ID, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the current session: status = %d, body %q", rec.Code, rec.Body)
	}
	c := sessionCookieIn(rec)
	if c == nil || c.MaxAge >= 0 {
		t.Fatalf("DELETE the current session: session cookie = %v, want one with Max-Age=0", c)
	}
	// A browser deletes a cookie only if the deleting one has the same scope.
	if c.Domain != "example.com" || c.Path != "/" {
		t.Errorf("clearing cookie scope = %s%s, want example.com/", c.Domain, c.Path)
	}
}
