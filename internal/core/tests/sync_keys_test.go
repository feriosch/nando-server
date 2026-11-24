package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/server"
)

// syncKeyJSON is a Sync Key as the API lists it, and, only when it is
// created, with its key.
type syncKeyJSON struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	Key        string     `json:"key,omitempty"`
}

// createSyncKey creates a Sync Key named name, approving it with this
// browser's Passkey, and returns the response that ends the ceremony (or the
// first one that fails).
func (d *browser) createSyncKey(name string) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/api/sync-keys/begin", mustJSON(d.nando.t, map[string]string{"name": name}))
	if rec.Code != http.StatusOK {
		return rec
	}
	return d.do(http.MethodPost, "/api/sync-keys/finish", d.passkey.Get(d.nando.t, d.origin, rec.Body.Bytes()))
}

// mustCreateSyncKey creates a Sync Key named name and returns it, with its key.
func (d *browser) mustCreateSyncKey(name string) syncKeyJSON {
	d.nando.t.Helper()
	rec := d.createSyncKey(name)
	var created syncKeyJSON
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &created) != nil {
		d.nando.t.Fatalf("create Sync Key %q: status = %d, body %q", name, rec.Code, rec.Body)
	}
	return created
}

// listSyncKeys answers GET /api/sync-keys.
func (d *browser) listSyncKeys() []syncKeyJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/sync-keys", nil)
	var list []syncKeyJSON
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		d.nando.t.Fatalf("GET /api/sync-keys: status = %d, body %q", rec.Code, rec.Body)
	}
	return list
}

func TestOwnerCreatesASyncKeyWithAPasskey(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	created := phone.mustCreateSyncKey("iPhone Health")

	if created.ID == "" || created.Name != "iPhone Health" || !created.CreatedAt.Equal(wednesday) || created.LastUsedAt != nil || len(created.Key) < 20 {
		t.Errorf("created Sync Key = %+v; want an id, its name, created now, never used, and a long key", created)
	}
	list := phone.listSyncKeys()
	if len(list) != 1 || list[0].ID != created.ID || list[0].Name != "iPhone Health" || list[0].Key != "" {
		t.Errorf("Sync Keys = %+v; want only %q, without its key", list, created.ID)
	}
}

// workoutsSync is a Module's route open to Sync Keys, which answers with what
// it was sent.
const workoutsSync = "POST /api/workouts/sync"

// workoutsModule is a Module with one route open to Sync Keys and one that
// isn't.
type workoutsModule struct{}

func (workoutsModule) Routes(mux *http.ServeMux) {
	echo := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
	mux.HandleFunc(workoutsSync, echo)
	mux.HandleFunc("GET /api/workouts", echo)
}

// withWorkouts serves the workouts Module too, with its sync route open to
// Sync Keys, as main wires a Module.
func (n *nando) withWorkouts() *nando {
	n.core.OpenToSyncKeys(workoutsSync)
	n.handler = server.New(server.Config{AllowedOrigins: allowedOrigins, Core: n.core, Modules: []server.Module{workoutsModule{}}})
	return n
}

// automationDo sends a request as an automation does, such as an iOS
// Shortcut: with key as its bearer token, and no Session or Origin.
func (n *nando) automationDo(method, path, key string, body []byte) *httptest.ResponseRecorder {
	n.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

func TestASyncKeyReachesARouteAModuleOpenedToIt(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.withWorkouts()
	created := phone.mustCreateSyncKey("iPhone Health")
	n.clock.Advance(time.Hour)

	rec := n.automationDo(http.MethodPost, "/api/workouts/sync", created.Key, []byte(`{"steps": 9812}`))

	if rec.Code != http.StatusOK || rec.Body.String() != `{"steps": 9812}` {
		t.Errorf("POST /api/workouts/sync with the Sync Key: status = %d, body %q; want %d and what was sent", rec.Code, rec.Body, http.StatusOK)
	}
	list := phone.listSyncKeys()
	if len(list) != 1 || list[0].LastUsedAt == nil || !list[0].LastUsedAt.Equal(wednesday.Add(time.Hour)) {
		t.Errorf("Sync Keys = %+v; want it last used an hour after it was created", list)
	}
}

func TestASyncKeyReachesNoOtherRoute(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.withWorkouts()
	key := phone.mustCreateSyncKey("iPhone Health").Key

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/workouts"},
		{http.MethodGet, "/api/sync-keys"},
		{http.MethodGet, "/api/settings"},
		{http.MethodGet, "/api/workouts/sync"},
		{http.MethodPost, "/mcp"},
		// Decodes to the open route, but the mux routes it elsewhere.
		{http.MethodPost, "/api/workouts%2Fsync"},
	} {
		if rec := n.automationDo(route.method, route.path, key, []byte(`{}`)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with the Sync Key: status = %d, body %q; want %d", route.method, route.path, rec.Code, rec.Body, http.StatusUnauthorized)
		}
	}
}

func TestASyncKeyMaySayBearerInAnyCase(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.withWorkouts()
	key := phone.mustCreateSyncKey("iPhone Health").Key

	req := httptest.NewRequest(http.MethodPost, "/api/workouts/sync", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "bearer "+key)
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/workouts/sync with \"bearer\" in lower case: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusOK)
	}
}

func TestAnUnknownSyncKeyIsRefused(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.withWorkouts()
	phone.mustCreateSyncKey("iPhone Health")

	for name, key := range map[string]string{"no key": "", "an unknown key": "not-a-sync-key"} {
		if rec := n.automationDo(http.MethodPost, "/api/workouts/sync", key, []byte(`{}`)); rec.Code != http.StatusUnauthorized {
			t.Errorf("POST /api/workouts/sync with %s: status = %d, want %d", name, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestRemovingASyncKeyCutsItOffAtOnce(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.withWorkouts()
	removed := phone.mustCreateSyncKey("Old iPhone")
	kept := phone.mustCreateSyncKey("iPhone Health")

	if rec := phone.do(http.MethodDelete, "/api/sync-keys/"+removed.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the Sync Key: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusNoContent)
	}

	if rec := n.automationDo(http.MethodPost, "/api/workouts/sync", removed.Key, []byte(`{}`)); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST with the removed Sync Key: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := n.automationDo(http.MethodPost, "/api/workouts/sync", kept.Key, []byte(`{}`)); rec.Code != http.StatusOK {
		t.Errorf("POST with the kept Sync Key: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if list := phone.listSyncKeys(); len(list) != 1 || list[0].ID != kept.ID {
		t.Errorf("Sync Keys = %+v; want only %q", list, kept.ID)
	}
	if rec := phone.do(http.MethodDelete, "/api/sync-keys/"+removed.ID, nil); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE it again: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestASyncKeyNeedsAName(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	for _, name := range []string{"", "   ", strings.Repeat("x", 51)} {
		if rec := phone.do(http.MethodPost, "/api/sync-keys/begin", mustJSON(t, map[string]string{"name": name})); rec.Code != http.StatusBadRequest {
			t.Errorf("begin a Sync Key named %q: status = %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}
	if created := phone.mustCreateSyncKey("  iPhone Health  "); created.Name != "iPhone Health" {
		t.Errorf("name = %q, want it trimmed", created.Name)
	}
	if list := phone.listSyncKeys(); len(list) != 1 {
		t.Errorf("Sync Keys = %+v; want only the named one", list)
	}
}

func TestCreatingASyncKeyNeedsASessionAndAPasskey(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	stranger := n.newBrowser("Stranger")

	if rec := stranger.do(http.MethodPost, "/api/sync-keys/begin", mustJSON(t, map[string]string{"name": "Mine"})); rec.Code != http.StatusUnauthorized {
		t.Errorf("begin without a Session: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	// A sign-in's answer can't create a Sync Key.
	options := phone.do(http.MethodPost, "/auth/signin/begin", nil)
	if rec := phone.do(http.MethodPost, "/api/sync-keys/finish", phone.passkey.Get(t, phone.origin, options.Body.Bytes())); rec.Code != http.StatusBadRequest {
		t.Errorf("finish with a sign-in's answer: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusBadRequest)
	}
	if list := phone.listSyncKeys(); len(list) != 0 {
		t.Errorf("Sync Keys = %+v; want none", list)
	}
}

func TestASyncKeyCeremonyCannotSignIn(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	mac := n.newBrowser("Mac").syncedFrom(phone)

	options := phone.do(http.MethodPost, "/api/sync-keys/begin", mustJSON(t, map[string]string{"name": "iPhone Health"}))
	rec := mac.do(http.MethodPost, "/auth/signin/finish", mac.passkey.Get(t, mac.origin, options.Body.Bytes()))

	if rec.Code != http.StatusBadRequest || mac.session != nil {
		t.Errorf("sign in with a Sync Key creation's answer: status = %d, body %q, Session %v; want %d and no Session",
			rec.Code, rec.Body, mac.session != nil, http.StatusBadRequest)
	}
}
