package calories_test

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

	"github.com/feriosch/nando-server/internal/calories"
	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/server"
	"github.com/feriosch/nando-server/internal/testmongo"
	"github.com/feriosch/nando-server/internal/testpasskey"
)

const webOrigin = "https://web.example.com"

// clock is the time Nando sees; tests set it by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time  { return c.now }
func (c *clock) Set(t time.Time) { c.now = t }

// nando is the whole HTTP API, Core and Calories, over a fresh MongoDB, as
// main wires it.
type nando struct {
	t       *testing.T
	core    *core.Core
	handler http.Handler
	clock   *clock
}

func newNando(t *testing.T) *nando {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(testmongo.URI(t)))
	if err != nil {
		t.Fatalf("connect MongoDB: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	// Real time to start with: go-webauthn checks its ceremony timeouts against it.
	clk := &clock{now: time.Now()}
	c, err := core.New(context.Background(), client.Database("core"), core.Config{
		Domain:               "example.com",
		Origins:              []string{webOrigin},
		Now:                  clk.Now,
		VerifySchedulerToken: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	cal, err := calories.NewCaloriesModule(context.Background(), client.Database("calories"), c.Service())
	if err != nil {
		t.Fatalf("NewCaloriesModule: %v", err)
	}
	return &nando{
		t:    t,
		core: c,
		handler: server.New(server.Config{
			AllowedOrigins: []string{webOrigin},
			Core:           c,
			Modules:        []server.Module{cal},
		}),
		clock: clk,
	}
}

// device is the Owner's signed-in iPhone.
type device struct {
	nando   *nando
	session *http.Cookie
}

// signedInPhone returns Nando with an enrolled, signed-in iPhone, its clock
// set to now.
func signedInPhone(t *testing.T, now time.Time) (*nando, *device) {
	t.Helper()
	n := newNando(t)
	phone := &device{nando: n}
	code, err := n.core.IssueEnrollmentCode(context.Background())
	if err != nil {
		t.Fatalf("IssueEnrollmentCode: %v", err)
	}
	passkey := testpasskey.New(webOrigin)
	rec := phone.must(http.MethodPost, "/auth/register/begin", map[string]string{"code": code}, http.StatusOK)
	phone.must(http.MethodPost, "/auth/register/finish", json.RawMessage(passkey.Create(t, rec.Body.Bytes())), http.StatusNoContent)
	n.clock.Set(now)
	return n, phone
}

// do sends a request from the web app with this device's session cookie;
// body, unless nil, is sent as JSON.
func (d *device) do(method, path string, body any) *httptest.ResponseRecorder {
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
func (d *device) must(method, path string, body any, status int) *httptest.ResponseRecorder {
	t := d.nando.t
	t.Helper()
	rec := d.do(method, path, body)
	if rec.Code != status {
		t.Fatalf("%s %s: status = %d, body %q; want %d", method, path, rec.Code, rec.Body, status)
	}
	return rec
}

// get GETs path, which must answer 200, and decodes its JSON into a T.
func get[T any](d *device, path string) T {
	t := d.nando.t
	t.Helper()
	rec := d.must(http.MethodGet, path, nil, http.StatusOK)
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("GET %s: %v in %q", path, err, rec.Body)
	}
	return v
}
