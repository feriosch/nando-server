package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/japanese"
	"github.com/feriosch/nando-server/internal/server"
	"github.com/feriosch/nando-server/internal/testmongo"
	"github.com/feriosch/nando-server/internal/testpasskey"
)

const webOrigin = "https://web.example.com"

// clock is the time Nando sees; tests set it by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time  { return c.now }
func (c *clock) Set(t time.Time) { c.now = t }

// nando is the whole HTTP API, Core and Japanese, over a fresh MongoDB, as
// main wires it. The Japanese API is read-only, so tests store Words and Kanji
// straight into the japanese database, shaped as the data moved from benkyo.
type nando struct {
	t        *testing.T
	core     *core.Core
	japanese *mongo.Database
	handler  http.Handler
	clock    *clock
}

func newNando(t *testing.T) *nando {
	t.Helper()
	return newNandoWith(t, japanese.Config{})
}

// newNandoWith is newNando with the Japanese Module configured as cfg, such as
// with fake outside services.
func newNandoWith(t *testing.T, cfg japanese.Config) *nando {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(testmongo.URI(t)))
	if err != nil {
		t.Fatalf("connect MongoDB: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	vapidPrivate, vapidPublic, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	// Real time to start with: go-webauthn checks its ceremony timeouts against it.
	clk := &clock{now: time.Now()}
	c, err := core.New(context.Background(), client.Database("core"), core.Config{
		Domain:               "example.com",
		Origins:              []string{webOrigin},
		Now:                  clk.Now,
		VerifySchedulerToken: func(context.Context, string) error { return nil },
		VAPID: core.VAPID{
			PublicKey:  vapidPublic,
			PrivateKey: vapidPrivate,
			Contact:    "mailto:owner@example.com",
		},
	})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	db := client.Database("japanese")
	module, err := japanese.NewJapaneseModule(context.Background(), db, c.Service(), cfg)
	if err != nil {
		t.Fatalf("NewJapaneseModule: %v", err)
	}
	c.AddToolProvider(module.ToolProvider())
	c.AddTickJob(module.TickJob())
	return &nando{
		t:        t,
		core:     c,
		japanese: db,
		clock:    clk,
		handler: server.New(server.Config{
			AllowedOrigins: []string{webOrigin},
			Core:           c,
			Modules:        []server.Module{module},
		}),
	}
}

// store inserts docs into the japanese database's collection.
func (n *nando) store(collection string, docs ...any) {
	n.t.Helper()
	if _, err := n.japanese.Collection(collection).InsertMany(context.Background(), docs); err != nil {
		n.t.Fatalf("store in %s: %v", collection, err)
	}
}

// browser is Nando on the Owner's iPhone.
type browser struct {
	nando   *nando
	session *http.Cookie
	// passkey is the Owner's, on the iPhone.
	passkey *testpasskey.Passkey
}

// signedInBrowser returns Nando with a browser on the Owner's iPhone that has
// enrolled a Passkey and has a Session.
func signedInBrowser(t *testing.T) (*nando, *browser) {
	t.Helper()
	return signedInBrowserWith(t, japanese.Config{})
}

// signedInBrowserWith is signedInBrowser with the Japanese Module configured
// as cfg.
func signedInBrowserWith(t *testing.T, cfg japanese.Config) (*nando, *browser) {
	t.Helper()
	n := newNandoWith(t, cfg)
	phone := &browser{nando: n, passkey: testpasskey.New()}
	code, err := n.core.IssueEnrollmentCode(context.Background())
	if err != nil {
		t.Fatalf("IssueEnrollmentCode: %v", err)
	}
	rec := phone.must(http.MethodPost, "/auth/register/begin", map[string]string{"code": code}, http.StatusOK)
	phone.must(http.MethodPost, "/auth/register/finish", json.RawMessage(phone.passkey.Create(t, webOrigin, rec.Body.Bytes())), http.StatusNoContent)
	return n, phone
}

// do sends a request from the web app with this browser's Session cookie;
// body, unless nil, is sent as JSON.
func (d *browser) do(method, path string, body any) *httptest.ResponseRecorder {
	t := d.nando.t
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Origin", webOrigin)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if d.session != nil {
		req.AddCookie(d.session)
	}
	rec := httptest.NewRecorder()
	d.nando.handler.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "nando_session" {
			d.session = c
		}
	}
	return rec
}

// must sends the request and fails the test unless it is answered with status.
func (d *browser) must(method, path string, body any, status int) *httptest.ResponseRecorder {
	t := d.nando.t
	t.Helper()
	rec := d.do(method, path, body)
	if rec.Code != status {
		t.Fatalf("%s %s: status = %d, body %q; want %d", method, path, rec.Code, rec.Body, status)
	}
	return rec
}

// get GETs path, which must answer 200, and decodes its JSON into a T.
func get[T any](d *browser, path string) T {
	t := d.nando.t
	t.Helper()
	rec := d.must(http.MethodGet, path, nil, http.StatusOK)
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("GET %s: %v in %q", path, err, rec.Body)
	}
	return v
}
