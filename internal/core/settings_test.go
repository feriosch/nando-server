package core_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// settingsJSON is Core's settings as GET /api/settings answers them.
type settingsJSON struct {
	TimeZone        string       `json:"timeZone"`
	HeightCm        *int         `json:"heightCm"`
	WeighInReminder reminderJSON `json:"weighInReminder"`
}

type reminderJSON struct {
	Enabled bool   `json:"enabled"`
	Weekday string `json:"weekday"`
	Time    string `json:"time"`
}

func (d *device) getSettings() settingsJSON {
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
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	got := phone.getSettings()

	want := settingsJSON{
		TimeZone:        "UTC",
		HeightCm:        nil,
		WeighInReminder: reminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"},
	}
	if got.TimeZone != want.TimeZone || got.HeightCm != nil || got.WeighInReminder != want.WeighInReminder {
		t.Errorf("settings before any are saved = %+v, want %+v", got, want)
	}
}

func TestOwnerSavesTheirSettings(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	height := 175
	saved := settingsJSON{
		TimeZone:        "Asia/Tokyo",
		HeightCm:        &height,
		WeighInReminder: reminderJSON{Enabled: true, Weekday: "sunday", Time: "07:30"},
	}

	if rec := phone.do(http.MethodPut, "/api/settings", mustJSON(t, saved)); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT /api/settings: status = %d, body %q", rec.Code, rec.Body)
	}

	got := phone.getSettings()
	if got.TimeZone != "Asia/Tokyo" || got.HeightCm == nil || *got.HeightCm != 175 || got.WeighInReminder != saved.WeighInReminder {
		t.Errorf("settings after saving = %+v (height %v), want %+v (height 175)", got, got.HeightCm, saved)
	}
}

func TestInvalidSettingsAreRejected(t *testing.T) {
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()

	for name, body := range map[string]string{
		"unknown time zone":         `{"timeZone": "Mars/Olympus", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:30"}}`,
		"no time zone":              `{"timeZone": "", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:30"}}`,
		"the server's time zone":    `{"timeZone": "Local", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:30"}}`,
		"height of zero":            `{"timeZone": "UTC", "heightCm": 0, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:30"}}`,
		"height over 3 m":           `{"timeZone": "UTC", "heightCm": 301, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:30"}}`,
		"fractional height":         `{"timeZone": "UTC", "heightCm": 175.5, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:30"}}`,
		"unknown weekday":           `{"timeZone": "UTC", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "funday", "time": "07:30"}}`,
		"time off the half hour":    `{"timeZone": "UTC", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "07:15"}}`,
		"time past midnight":        `{"timeZone": "UTC", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "24:00"}}`,
		"time without leading zero": `{"timeZone": "UTC", "heightCm": 175, "weighInReminder": {"enabled": true, "weekday": "sunday", "time": "7:30"}}`,
		"not JSON":                  `timeZone=UTC`,
	} {
		if rec := phone.do(http.MethodPut, "/api/settings", []byte(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT /api/settings with %s: status = %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}

	if got := phone.getSettings(); got.TimeZone != "UTC" || got.HeightCm != nil || got.WeighInReminder.Enabled {
		t.Errorf("settings after only invalid saves = %+v, want the defaults", got)
	}
}
