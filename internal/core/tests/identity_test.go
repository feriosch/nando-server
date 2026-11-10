package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/server"
	"github.com/feriosch/nando-server/internal/testmongo"
	"github.com/feriosch/nando-server/internal/testpasskey"
)

const (
	webOrigin     = "https://web.example.com"
	betaOrigin    = "https://beta.example.com" // a second copy of the web app
	evilOrigin    = "https://evil.example.com" // under the domain, but not allowed
	sessionCookie = "nando_session"
)

// allowedOrigins is ALLOWED_ORIGINS, both under the domain example.com.
var allowedOrigins = []string{webOrigin, betaOrigin}

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

	vapidPrivate, vapidPublic, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	// Real time to start with: go-webauthn checks its ceremony timeouts against it.
	clk := &clock{now: time.Now()}
	c, err := core.New(ctx, client.Database("core"), core.Config{
		Domain:               "example.com",
		Origins:              allowedOrigins,
		Now:                  clk.Now,
		VerifySchedulerToken: verifySchedulerToken,
		VAPID: core.VAPID{
			PublicKey:  vapidPublic,
			PrivateKey: vapidPrivate,
			Contact:    "mailto:owner@example.com",
		},
	})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return &nando{
		t:       t,
		core:    c,
		handler: server.New(server.Config{AllowedOrigins: allowedOrigins, Core: c}),
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

// browser is one browser or Home Screen app using Nando, with its own cookies
// and, once it has one, a Passkey on its device.
type browser struct {
	nando     *nando
	origin    string
	userAgent string
	session   *http.Cookie
	passkey   *testpasskey.Passkey
}

func (n *nando) newBrowser(userAgent string) *browser {
	return n.newBrowserAt(webOrigin, userAgent)
}

// newBrowserAt is newBrowser using the web app served from origin.
func (n *nando) newBrowserAt(origin, userAgent string) *browser {
	return &browser{nando: n, origin: origin, userAgent: userAgent}
}

// do sends a request from the web app, carrying and keeping the Session cookie
// the way a browser would.
func (d *browser) do(method, path string, body []byte) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Origin", d.origin)
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

// enroll registers a new Passkey from this browser with code, returning the
// response that ends the ceremony (or the first one that fails). If the
// server excludes the Passkey this browser already has, the browser refuses
// and enroll returns the begin response, as no finish request is sent.
func (d *browser) enroll(code string) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/auth/register/begin", mustJSON(d.nando.t, map[string]string{"code": code}))
	if rec.Code != http.StatusOK {
		return rec
	}
	if d.passkey != nil && d.passkey.ExcludedBy(d.nando.t, rec.Body.Bytes()) {
		return rec
	}
	d.passkey = testpasskey.New()
	return d.do(http.MethodPost, "/auth/register/finish", d.passkey.Create(d.nando.t, d.origin, rec.Body.Bytes()))
}

// mustEnroll enrolls with a fresh Enrollment Code and fails the test if that
// doesn't work.
func (d *browser) mustEnroll() {
	d.nando.t.Helper()
	if rec := d.enroll(d.nando.issueEnrollmentCode()); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("enroll: status = %d, body %q", rec.Code, rec.Body)
	}
}

// signIn signs in from this browser with its Passkey, starting a Session, and
// returns the response that ends the ceremony (or the first one that fails).
func (d *browser) signIn() *httptest.ResponseRecorder {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/auth/signin/begin", nil)
	if rec.Code != http.StatusOK {
		return rec
	}
	return d.do(http.MethodPost, "/auth/signin/finish", d.passkey.Get(d.nando.t, d.origin, rec.Body.Bytes()))
}

// mustSignIn signs in and fails the test if that doesn't work.
func (d *browser) mustSignIn() {
	d.nando.t.Helper()
	if rec := d.signIn(); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("sign in: status = %d, body %q", rec.Code, rec.Body)
	}
}

// syncedFrom gives this browser the Passkey of other, as iCloud Keychain does.
func (d *browser) syncedFrom(other *browser) *browser {
	d.passkey = other.passkey
	return d
}

func TestAPIWithoutASessionIsUnauthorized(t *testing.T) {
	n := newNando(t)

	rec := n.newBrowser("iPhone").do(http.MethodGet, "/api/sessions", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/sessions without a Session: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOwnerEnrollsAPasskeyWithAnEnrollmentCode(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")

	if rec := phone.enroll(n.issueEnrollmentCode()); rec.Code != http.StatusNoContent {
		t.Fatalf("enroll: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions after enrolling: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestOwnerSignsInWithTheirPasskey(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	mac := n.newBrowser("Mac").syncedFrom(phone)

	if rec := mac.signIn(); rec.Code != http.StatusNoContent {
		t.Fatalf("sign in: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := mac.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions after signing in: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPasskeyWhoseSignCountGoesUpSignsInRepeatedly(t *testing.T) {
	n := newNando(t)
	// A hardware security key, which counts its uses.
	laptop := n.newBrowser("Laptop")
	laptop.mustEnroll()

	for count := uint32(1); count <= 3; count++ {
		laptop.passkey.SetSignCount(count)
		if rec := laptop.signIn(); rec.Code != http.StatusNoContent {
			t.Fatalf("sign in with sign count %d: status = %d, body %q", count, rec.Code, rec.Body)
		}
	}
}

func TestSyncedPasskeyThatAlwaysReportsZeroKeepsSigningIn(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone") // iCloud Keychain: sign count always 0
	phone.mustEnroll()

	for i := 1; i <= 3; i++ {
		if rec := phone.signIn(); rec.Code != http.StatusNoContent {
			t.Fatalf("sign in %d with sign count 0: status = %d, body %q", i, rec.Code, rec.Body)
		}
	}
}

func TestSignInWithALowerSignCountIsRejected(t *testing.T) {
	n := newNando(t)
	// A hardware security key, which counts its uses.
	laptop := n.newBrowser("Laptop")
	laptop.mustEnroll()
	laptop.passkey.SetSignCount(5)
	laptop.mustSignIn()

	// A copy of the key that is behind the original: a sign of cloning.
	clone := n.newBrowser("Unknown")
	clone.passkey = laptop.passkey.Copy()
	clone.passkey.SetSignCount(3)

	if rec := clone.signIn(); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sign in with sign count 3 after 5: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := clone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/sessions after the rejected sign-in: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	// The rejected copy changed nothing: the original carries on from 5.
	laptop.passkey.SetSignCount(6)
	if rec := laptop.signIn(); rec.Code != http.StatusNoContent {
		t.Errorf("sign in with the original at sign count 6: status = %d, body %q", rec.Code, rec.Body)
	}
}

func TestSimultaneousSignInsNeverLowerTheSignCount(t *testing.T) {
	n := newNando(t)
	laptop := n.newBrowser("Laptop")
	laptop.mustEnroll()

	// Each round, two copies of the key finish signing in at once, one count
	// apart. Whichever order the server takes them in, the higher count must
	// end up stored. Timing decides whether a broken server shows it, so the
	// race is run several times.
	for round := uint32(1); round <= 20; round++ {
		base := 10 * round
		laptop.passkey.SetSignCount(base)
		laptop.mustSignIn()
		behind, ahead := n.newBrowser("Behind"), n.newBrowser("Ahead")
		behind.passkey, ahead.passkey = laptop.passkey.Copy(), laptop.passkey.Copy()
		behind.passkey.SetSignCount(base + 1)
		ahead.passkey.SetSignCount(base + 2)
		behindBody := behind.passkey.Get(t, behind.origin, behind.do(http.MethodPost, "/auth/signin/begin", nil).Body.Bytes())
		aheadBody := ahead.passkey.Get(t, ahead.origin, ahead.do(http.MethodPost, "/auth/signin/begin", nil).Body.Bytes())

		var wg sync.WaitGroup
		var behindCode, aheadCode int
		wg.Add(2)
		go func() {
			defer wg.Done()
			behindCode = behind.do(http.MethodPost, "/auth/signin/finish", behindBody).Code
		}()
		go func() {
			defer wg.Done()
			aheadCode = ahead.do(http.MethodPost, "/auth/signin/finish", aheadBody).Code
		}()
		wg.Wait()

		if behindCode != http.StatusNoContent || aheadCode != http.StatusNoContent {
			continue // one was refused, so nothing can have been stored out of order
		}
		replay := n.newBrowser("Replay")
		replay.passkey = laptop.passkey.Copy()
		replay.passkey.SetSignCount(base + 2)
		if rec := replay.signIn(); rec.Code != http.StatusUnauthorized {
			t.Fatalf("round %d: both sign-ins at %d and %d succeeded, then sign count %d again: status = %d, want %d (the stored count went down)",
				round, base+1, base+2, base+2, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestPasskeySignsInFromEveryAllowedOrigin(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	n.newBrowserAt(betaOrigin, "iPhone").syncedFrom(phone).mustSignIn()
}

func TestPasskeyFromAnOriginNotAllowedIsRejected(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	evil := n.newBrowserAt(evilOrigin, "iPhone").syncedFrom(phone)

	if rec := evil.signIn(); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sign in from %s: status = %d, want %d", evilOrigin, rec.Code, http.StatusUnauthorized)
	}
	if evil.session != nil {
		t.Errorf("sign in from %s set a Session cookie", evilOrigin)
	}
}

func TestUsedEnrollmentCodeIsRejected(t *testing.T) {
	n := newNando(t)
	code := n.issueEnrollmentCode()
	if rec := n.newBrowser("iPhone").enroll(code); rec.Code != http.StatusNoContent {
		t.Fatalf("first enroll: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := n.newBrowser("iPhone").enroll(code); rec.Code != http.StatusUnauthorized {
		t.Fatalf("enroll again with the same code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestEnrollmentCodeExpiresAfter15Minutes(t *testing.T) {
	n := newNando(t)
	stale := n.issueEnrollmentCode()
	n.clock.Advance(14 * time.Minute)
	fresh := n.issueEnrollmentCode()
	n.clock.Advance(2 * time.Minute)

	if rec := n.newBrowser("iPhone").enroll(stale); rec.Code != http.StatusUnauthorized {
		t.Errorf("enroll with a 16-minute-old code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := n.newBrowser("iPhone").enroll(fresh); rec.Code != http.StatusNoContent {
		t.Errorf("enroll with a 2-minute-old code: status = %d, body %q", rec.Code, rec.Body)
	}
}

func TestSessionCookieCoversWebAndAPIButNoScripts(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	c := phone.session
	if c == nil {
		t.Fatal("enroll set no Session cookie")
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
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	n.clock.Advance(90*24*time.Hour + time.Minute)

	if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/sessions 90 days after signing in: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestSessionInUseStaysSignedIn(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
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
	phone := n.newBrowser("iPhone")
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
		t.Fatal("25 hours after signing in: no Session cookie set, want it renewed")
	}
	if want := 90 * 24 * 60 * 60; renewed.MaxAge != want {
		t.Errorf("renewed cookie Max-Age = %d, want %d (90 days)", renewed.MaxAge, want)
	}
}

// sessionCookieIn returns the Session cookie rec sets, if any.
func sessionCookieIn(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

// sessionJSON is one Session as GET /api/sessions lists it.
type sessionJSON struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"userAgent"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}

func (d *browser) listSessions() []sessionJSON {
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

func TestOwnerSeesEachSession(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	n.clock.Advance(time.Minute)
	mac := n.newBrowser("Mac").syncedFrom(phone)
	mac.mustSignIn()

	got := mac.listSessions()

	if len(got) != 2 {
		t.Fatalf("Sessions = %+v, want the iPhone's and the Mac's", got)
	}
	for i, want := range []sessionJSON{{UserAgent: "iPhone", Current: false}, {UserAgent: "Mac", Current: true}} {
		if got[i].UserAgent != want.UserAgent || got[i].Current != want.Current {
			t.Errorf("Session %d = %+v, want userAgent %q, current %t", i, got[i], want.UserAgent, want.Current)
		}
		if got[i].ID == "" {
			t.Errorf("Session %d has no id", i)
		}
		if got[i].CreatedAt.IsZero() || !got[i].ExpiresAt.After(got[i].CreatedAt) {
			t.Errorf("Session %d created %v, expires %v; want both, expiring after it was created", i, got[i].CreatedAt, got[i].ExpiresAt)
		}
	}
	if gap := got[1].CreatedAt.Sub(got[0].CreatedAt); gap != time.Minute {
		t.Errorf("the Mac's Session was created %v after the iPhone's, want 1m", gap)
	}
}

func TestDeletingASessionSignsItsBrowserOut(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	n.clock.Advance(time.Minute)
	mac := n.newBrowser("Mac").syncedFrom(phone)
	mac.mustSignIn()
	phoneSession := mac.listSessions()[0]

	if rec := mac.do(http.MethodDelete, "/api/sessions/"+phoneSession.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the iPhone's Session: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := phone.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("iPhone after its Session was deleted: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := mac.do(http.MethodGet, "/api/sessions", nil); rec.Code != http.StatusOK {
		t.Errorf("Mac after deleting the iPhone's Session: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// passkeyJSON is one Passkey as GET /api/passkeys lists it.
type passkeyJSON struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

func (d *browser) listPasskeys() []passkeyJSON {
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
	// One Passkey in iCloud Keychain, one in Google Password Manager.
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	n.clock.Advance(time.Minute)
	pixel := n.newBrowser("Pixel")
	pixel.mustEnroll()

	got := phone.listPasskeys()

	want := []string{phone.passkey.ID(), pixel.passkey.ID()}
	if len(got) != len(want) {
		t.Fatalf("Passkeys = %+v, want ids %q", got, want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("Passkey %d id = %q, want %q", i, got[i].ID, want[i])
		}
	}
	if gap := got[1].CreatedAt.Sub(got[0].CreatedAt); got[0].CreatedAt.IsZero() || gap != time.Minute {
		t.Errorf("Passkeys created %v and %v, want the Pixel's 1m after the iPhone's", got[0].CreatedAt, got[1].CreatedAt)
	}
}

func TestDeletedPasskeyCanNoLongerSignIn(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	id := phone.listPasskeys()[0].ID

	if rec := phone.do(http.MethodDelete, "/api/passkeys/"+id, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the Passkey: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := n.newBrowser("Mac").syncedFrom(phone).signIn(); rec.Code != http.StatusUnauthorized {
		t.Errorf("sign in with the deleted Passkey: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := phone.listPasskeys(); len(got) != 0 {
		t.Errorf("Passkeys after deleting the only one = %+v, want none", got)
	}
}

func TestReEnrollingADeviceKeepsItsPasskey(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	// Without the server excluding it, the authenticator would silently replace
	// this Passkey with a new one, leaving the old one listed but unusable.
	phone.enroll(n.issueEnrollmentCode())

	if got := phone.listPasskeys(); len(got) != 1 || got[0].ID != phone.passkey.ID() {
		t.Errorf("Passkeys after enrolling the iPhone again = %+v, want only %q", got, phone.passkey.ID())
	}
}

func TestEnrollmentCodeRegistersOnlyOnePasskeyEvenWhenBegunTwice(t *testing.T) {
	n := newNando(t)
	code := mustJSON(t, map[string]string{"code": n.issueEnrollmentCode()})
	phone, mac := n.newBrowser("iPhone"), n.newBrowser("Mac")
	phoneOptions := phone.do(http.MethodPost, "/auth/register/begin", code)
	macOptions := mac.do(http.MethodPost, "/auth/register/begin", code)
	phone.passkey, mac.passkey = testpasskey.New(), testpasskey.New()

	first := phone.do(http.MethodPost, "/auth/register/finish", phone.passkey.Create(t, phone.origin, phoneOptions.Body.Bytes()))
	second := mac.do(http.MethodPost, "/auth/register/finish", mac.passkey.Create(t, mac.origin, macOptions.Body.Bytes()))

	if first.Code != http.StatusNoContent {
		t.Fatalf("first finish: status = %d, body %q", first.Code, first.Body)
	}
	if second.Code != http.StatusUnauthorized {
		t.Errorf("second finish with the same code: status = %d, want %d", second.Code, http.StatusUnauthorized)
	}
	if got := phone.listPasskeys(); len(got) != 1 || got[0].ID != phone.passkey.ID() {
		t.Errorf("Passkeys = %+v, want only the iPhone's %q", got, phone.passkey.ID())
	}
}

func TestDeletingTheCurrentSessionClearsItsCookie(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	current := phone.listSessions()[0]

	rec := phone.do(http.MethodDelete, "/api/sessions/"+current.ID, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the current Session: status = %d, body %q", rec.Code, rec.Body)
	}
	c := sessionCookieIn(rec)
	if c == nil || c.MaxAge >= 0 {
		t.Fatalf("DELETE the current Session: Session cookie = %v, want one with Max-Age=0", c)
	}
	// A browser deletes a cookie only if the deleting one has the same scope.
	if c.Domain != "example.com" || c.Path != "/" {
		t.Errorf("clearing cookie scope = %s%s, want example.com/", c.Domain, c.Path)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
