package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/money"
	"github.com/feriosch/nando-server/internal/server"
	"github.com/feriosch/nando-server/internal/testconnector"
	"github.com/feriosch/nando-server/internal/testmongo"
	"github.com/feriosch/nando-server/internal/testpasskey"
	"github.com/feriosch/nando-server/internal/testtelegram"
)

const (
	webOrigin = "https://web.example.com"
	// ownerTelegramID is the Owner's Telegram user ID, the only sender kept.
	ownerTelegramID = 777001
	webhookSecret   = "webhook-secret"
	moneyBotToken   = "444:money-bot"
)

// clock is the time Nando sees; tests set it by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time  { return c.now }
func (c *clock) Set(t time.Time) { c.now = t }

// today is the Day tests set Nando's clock to once the Owner is signed in:
// Monday 23 November 2026, at noon in UTC, the Owner Time Zone until set.
var today = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// nando is the whole HTTP API, Core and Money, over a fresh MongoDB, as main
// wires it.
type nando struct {
	t        *testing.T
	clock    *clock
	core     *core.Core
	handler  http.Handler
	telegram *testtelegram.Telegram
	// rates is the fake Frankfurter the Module fetches Rates from.
	rates *fakeFrankfurter
}

func newNando(t *testing.T) *nando {
	t.Helper()
	return newNandoWith(t, nil)
}

// newNandoWith is newNando with Money's Config changed by change, if not nil.
func newNandoWith(t *testing.T, change func(*money.Config)) *nando {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(testmongo.URI(t)))
	if err != nil {
		t.Fatalf("connect MongoDB: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	telegram := testtelegram.New(t)
	// Real time to start with: go-webauthn checks its ceremony timeouts against it.
	clk := &clock{now: time.Now()}
	c, err := core.New(context.Background(), client.Database("core"), core.Config{
		Now:                  clk.Now,
		Domain:               "example.com",
		Origins:              []string{webOrigin},
		VerifySchedulerToken: func(context.Context, string) error { return nil },
		Messenger: core.MessengerConfig{
			TelegramURL: telegram.URL, OwnerID: ownerTelegramID, WebhookSecret: webhookSecret,
			BotTokens: map[string]string{"core": "111:core-bot", "money": moneyBotToken},
		},
	})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	rates := newFakeFrankfurter(t)
	cfg := money.Config{RatesURL: rates.URL}
	if change != nil {
		change(&cfg)
	}
	m, err := money.NewMoneyModule(context.Background(), client.Database("money"), c.Service(), cfg)
	if err != nil {
		t.Fatalf("NewMoneyModule: %v", err)
	}
	c.AddAssistant(m.Assistant())
	c.AddToolProvider(m.ToolProvider())
	return &nando{
		t:     t,
		clock: clk,
		core:  c,
		handler: server.New(server.Config{
			AllowedOrigins: []string{webOrigin},
			Core:           c,
			Modules:        []server.Module{m},
		}),
		telegram: telegram,
		rates:    rates,
	}
}

// browser is Nando on the Owner's iPhone, with a Session and a Passkey.
type browser struct {
	nando   *nando
	session *http.Cookie
	passkey *testpasskey.Passkey
}

// signedInBrowser returns Nando with a browser on the Owner's iPhone that has
// enrolled a Passkey and has a Session, its clock set to today.
func signedInBrowser(t *testing.T) (*nando, *browser) {
	t.Helper()
	return signedInBrowserWith(t, nil)
}

// signedInBrowserWith is signedInBrowser over newNandoWith(t, change).
func signedInBrowserWith(t *testing.T, change func(*money.Config)) (*nando, *browser) {
	t.Helper()
	n := newNandoWith(t, change)
	phone := &browser{nando: n, passkey: testpasskey.New()}
	code, err := n.core.IssueEnrollmentCode(context.Background())
	if err != nil {
		t.Fatalf("IssueEnrollmentCode: %v", err)
	}
	rec := phone.must(http.MethodPost, "/auth/register/begin", map[string]string{"code": code}, http.StatusOK)
	phone.must(http.MethodPost, "/auth/register/finish", json.RawMessage(phone.passkey.Create(t, webOrigin, rec.Body.Bytes())), http.StatusNoContent)
	n.clock.Set(today)
	return n, phone
}

// tools is the MCP session of a Connector the Owner approves now, as the
// Money Assistant reaches Nando through one.
func (d *browser) tools() *mcp.ClientSession {
	d.nando.t.Helper()
	return testconnector.Connect(d.nando.t, d.nando.handler, d.passkey, webOrigin)
}

// must sends a request from the web app with this browser's Session cookie,
// body as JSON, and fails the test unless it is answered with status.
func (d *browser) must(method, path string, body any, status int) *httptest.ResponseRecorder {
	t := d.nando.t
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Origin", webOrigin)
	req.Header.Set("Content-Type", "application/json")
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
	if rec.Code != status {
		t.Fatalf("%s %s: status = %d, body %q; want %d", method, path, rec.Code, rec.Body, status)
	}
	return rec
}

// callTool calls the Tool name with args, which must succeed, and decodes its
// JSON answer into a T.
func callTool[T any](t *testing.T, session *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %+v", name, result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s answered %#v, want JSON text", name, result.Content[0])
	}
	var v T
	if err := json.Unmarshal([]byte(text.Text), &v); err != nil {
		t.Fatalf("%s: %v in %q", name, err, text.Text)
	}
	return v
}

// toolError calls the Tool name with args, which must fail, and returns why.
func toolError(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || !result.IsError || len(result.Content) == 0 {
		t.Fatalf("%s with %v: %v, %+v; want the Tool to fail", name, args, err, result)
	}
	return result.Content[0].(*mcp.TextContent).Text
}

// ownerSays posts the Owner's text message to assistant's bot webhook, as
// Telegram would, and fails unless it is answered 200.
func (n *nando) ownerSays(assistant string, updateID, messageID int64, text string) {
	n.t.Helper()
	body, err := json.Marshal(map[string]any{
		"update_id": updateID,
		"message": map[string]any{
			"message_id": messageID,
			"date":       time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC).Unix(),
			"chat":       map[string]any{"id": ownerTelegramID, "type": "private"},
			"from":       map[string]any{"id": ownerTelegramID, "first_name": "Owner"},
			"text":       text,
		},
	})
	if err != nil {
		n.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/messenger/telegram/"+assistant, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", webhookSecret)
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		n.t.Fatalf("POST /messenger/telegram/%s: status = %d, body %q", assistant, rec.Code, rec.Body)
	}
}
