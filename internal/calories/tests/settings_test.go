package tests

import (
	"net/http"
	"testing"
)

// notificationSettingJSON is when one of the Calories Notifications is sent.
type notificationSettingJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

// settingsJSON is what the Calories Settings Section saves, as the API reads and writes it.
type settingsJSON struct {
	NoEntriesReminder notificationSettingJSON `json:"noEntriesReminder"`
	EveningSummary    notificationSettingJSON `json:"eveningSummary"`
}

func TestCaloriesNotificationsAreOffUntilTheOwnerTurnsThemOn(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	want := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: false, Time: "14:00"},
		EveningSummary:    notificationSettingJSON{Enabled: false, Time: "21:00"},
	}
	if got := get[settingsJSON](phone, "/api/calories/settings"); got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestOwnerSavesWhenCaloriesNotificationsAreSent(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	saved := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: true, Time: "13:30"},
		EveningSummary:    notificationSettingJSON{Enabled: true, Time: "22:00"},
	}

	phone.must(http.MethodPut, "/api/calories/settings", saved, http.StatusNoContent)

	if got := get[settingsJSON](phone, "/api/calories/settings"); got != saved {
		t.Errorf("settings = %+v, want %+v", got, saved)
	}
}

func TestInvalidCaloriesNotificationSettingsAreRejected(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	valid := map[string]any{"enabled": true, "time": "20:00"}

	for _, body := range []any{
		"settings",
		map[string]any{"noEntriesReminder": valid},
		map[string]any{"eveningSummary": valid},
		map[string]any{"noEntriesReminder": valid, "eveningSummary": map[string]any{"enabled": true, "time": "20:15"}},
		map[string]any{"noEntriesReminder": map[string]any{"enabled": true, "time": "24:00"}, "eveningSummary": valid},
		map[string]any{"noEntriesReminder": map[string]any{"enabled": true}, "eveningSummary": valid},
		map[string]any{"noEntriesReminder": valid, "eveningSummary": map[string]any{"time": "21:00"}},
	} {
		if rec := phone.do(http.MethodPut, "/api/calories/settings", body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT settings %v: status = %d, want 400", body, rec.Code)
		}
	}
}
