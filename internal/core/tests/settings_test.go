package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// wednesday is a Wednesday at noon UTC, in the week starting Monday 2026-10-19.
var wednesday = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// settingsJSON is Core's settings as GET /api/settings answers them.
type settingsJSON struct {
	TimeZone string `json:"timeZone"`
	HeightCm *int   `json:"heightCm"`
}

func (d *browser) getSettings() settingsJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/settings", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("GET /api/settings: status = %d, body %q", rec.Code, rec.Body)
	}
	var s settingsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		d.nando.t.Fatalf("GET /api/settings: %v in %q", err, rec.Body)
	}
	return s
}

func TestSettingsStartAsDefaults(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	if got := phone.getSettings(); got.TimeZone != "UTC" || got.HeightCm != nil {
		t.Errorf("settings before any are saved = %+v, want UTC and no height", got)
	}
}

func TestOwnerSavesTheirSettings(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	height := 175
	saved := settingsJSON{TimeZone: "Asia/Tokyo", HeightCm: &height}

	if rec := phone.do(http.MethodPut, "/api/settings", mustJSON(t, saved)); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT /api/settings: status = %d, body %q", rec.Code, rec.Body)
	}

	if got := phone.getSettings(); got.TimeZone != "Asia/Tokyo" || got.HeightCm == nil || *got.HeightCm != 175 {
		t.Errorf("settings after saving = %+v (height %v), want Asia/Tokyo and 175 cm", got, got.HeightCm)
	}
}

func TestInvalidSettingsAreRejected(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	for name, body := range map[string]string{
		"unknown time zone":      `{"timeZone": "Mars/Olympus", "heightCm": 175}`,
		"no time zone":           `{"timeZone": "", "heightCm": 175}`,
		"the server's time zone": `{"timeZone": "Local", "heightCm": 175}`,
		"height of zero":         `{"timeZone": "UTC", "heightCm": 0}`,
		"height over 3 m":        `{"timeZone": "UTC", "heightCm": 301}`,
		"fractional height":      `{"timeZone": "UTC", "heightCm": 175.5}`,
		"not JSON":               `timeZone=UTC`,
	} {
		if rec := phone.do(http.MethodPut, "/api/settings", []byte(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT /api/settings with %s: status = %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}

	if got := phone.getSettings(); got.TimeZone != "UTC" || got.HeightCm != nil {
		t.Errorf("settings after only invalid saves = %+v, want the defaults", got)
	}
}

// The Weigh-in reminder moved to Calories (ADR 0012).
func TestSettingsNoLongerHoldTheWeighInReminderAndAWebAppThatSendsItStillSaves(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	// As a web app from before the move sends them, with a reminder it can't keep.
	body := `{"timeZone": "Asia/Tokyo", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "funday", "time": "07:15"}}`
	if rec := phone.do(http.MethodPut, "/api/settings", []byte(body)); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT /api/settings with a Weigh-in reminder: status = %d, body %q; want it ignored", rec.Code, rec.Body)
	}

	rec := phone.do(http.MethodGet, "/api/settings", nil)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["weighInReminder"]; ok || got["timeZone"] != "Asia/Tokyo" || got["heightCm"] != 175.0 {
		t.Errorf("settings = %v, want Asia/Tokyo and 175 cm, without a weighInReminder", got)
	}
}

func TestTheWeighInRoutesAreNoLongerCores(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/weigh-ins"},
		{http.MethodPut, "/api/weigh-ins/2026-10-19"},
		{http.MethodDelete, "/api/weigh-ins/2026-10-19"},
		{http.MethodGet, "/api/body"},
	} {
		if rec := phone.do(route.method, route.path, []byte(`{"kg": 72.4}`)); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want %d", route.method, route.path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestCoreSendsNoWeighInReminderOfItsOwn(t *testing.T) {
	monday8am := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	n, _, telegram := signedInWithBots(t, monday8am)
	// As the old server saved it, switched on for now.
	if _, err := n.db.Collection("settings").InsertOne(context.Background(), bson.D{
		{Key: "_id", Value: "settings"},
		{Key: "timeZone", Value: "UTC"},
		{Key: "weighInReminder", Value: bson.D{{Key: "enabled", Value: true}, {Key: "weekday", Value: 1}, {Key: "time", Value: "08:00"}}},
	}); err != nil {
		t.Fatal(err)
	}

	n.mustTick()

	if got := received(telegram); len(got) != 0 {
		t.Errorf("received %q, want nothing: Calories sends the Weigh-in reminder now", got)
	}
}
