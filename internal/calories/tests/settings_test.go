package tests

import (
	"testing"
)

// notificationSettingJSON is when one of the Calories Notifications is sent.
type notificationSettingJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

// weighInReminderJSON is when the weekly Weigh-in reminder is sent.
type weighInReminderJSON struct {
	Enabled bool   `json:"enabled"`
	Weekday string `json:"weekday"`
	Time    string `json:"time"`
}

// defaultWeighInReminder is the Weigh-in reminder until the Owner changes it.
var defaultWeighInReminder = weighInReminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"}

// settingsJSON is when the Calories Notifications are sent, as the API reads them.
type settingsJSON struct {
	NoEntriesReminder notificationSettingJSON `json:"noEntriesReminder"`
	EveningSummary    notificationSettingJSON `json:"eveningSummary"`
	WeighInReminder   weighInReminderJSON     `json:"weighInReminder"`
}

func TestCaloriesNotificationsAreOffUntilTheOwnerTurnsThemOn(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	want := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: false, Time: "14:00"},
		EveningSummary:    notificationSettingJSON{Enabled: false, Time: "21:00"},
		WeighInReminder:   defaultWeighInReminder,
	}
	if got := get[settingsJSON](phone, "/api/calories/settings"); got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}
}

func TestTheAssistantReadsWhenCaloriesNotificationsAreSent(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	got := callTool[settingsJSON](t, phone.tools(), "calories_get_settings", nil)

	want := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: false, Time: "14:00"},
		EveningSummary:    notificationSettingJSON{Enabled: false, Time: "21:00"},
		WeighInReminder:   defaultWeighInReminder,
	}
	if got != want {
		t.Errorf("calories_get_settings = %+v, want the defaults %+v", got, want)
	}
}

func TestTheAssistantChangesOnlyTheNotificationSettingsItSends(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()

	got := callTool[settingsJSON](t, session, "calories_save_settings", map[string]any{
		"eveningSummary": map[string]any{"enabled": true, "time": "22:00"},
	})
	want := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: false, Time: "14:00"}, // not sent, so as it was
		EveningSummary:    notificationSettingJSON{Enabled: true, Time: "22:00"},
		WeighInReminder:   defaultWeighInReminder,
	}
	if got != want {
		t.Errorf("after turning the evening summary on at 22:00: %+v, want %+v", got, want)
	}

	// Within one Notification, only the fields sent change.
	got = callTool[settingsJSON](t, session, "calories_save_settings", map[string]any{
		"noEntriesReminder": map[string]any{"time": "13:30"},
		"eveningSummary":    map[string]any{"enabled": false},
	})
	want = settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: false, Time: "13:30"},
		EveningSummary:    notificationSettingJSON{Enabled: false, Time: "22:00"},
		WeighInReminder:   defaultWeighInReminder,
	}
	if got != want {
		t.Errorf("after moving the reminder and turning the summary off: %+v, want %+v", got, want)
	}
	if got := get[settingsJSON](phone, "/api/calories/settings"); got != want {
		t.Errorf("the web app's settings = %+v, want %+v", got, want)
	}
}

// saveCaloriesSettings sets when both Calories Notifications are sent, as
// the Calories Assistant does.
func (d *browser) saveCaloriesSettings(noEntriesReminder, eveningSummary notificationSettingJSON) {
	d.nando.t.Helper()
	callTool[settingsJSON](d.nando.t, d.tools(), "calories_save_settings", map[string]any{
		"noEntriesReminder": map[string]any{"enabled": noEntriesReminder.Enabled, "time": noEntriesReminder.Time},
		"eveningSummary":    map[string]any{"enabled": eveningSummary.Enabled, "time": eveningSummary.Time},
	})
}

func TestInvalidCaloriesNotificationSettingsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()
	phone.saveCaloriesSettings(notificationSettingJSON{Enabled: true, Time: "14:00"}, notificationSettingJSON{Enabled: true, Time: "21:00"})

	for _, args := range []map[string]any{
		{},
		{"eveningSummary": map[string]any{"enabled": true, "time": "20:15"}},
		{"noEntriesReminder": map[string]any{"time": "24:00"}},
		{"noEntriesReminder": map[string]any{"enabled": "yes"}},
		{"eveningSummary": map[string]any{}},
		{"eveningSummary": "21:00"},
	} {
		toolError(t, session, "calories_save_settings", args)
	}
	want := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: true, Time: "14:00"},
		EveningSummary:    notificationSettingJSON{Enabled: true, Time: "21:00"},
		WeighInReminder:   defaultWeighInReminder,
	}
	if got := get[settingsJSON](phone, "/api/calories/settings"); got != want {
		t.Errorf("settings after refused changes = %+v, want unchanged %+v", got, want)
	}
}
