package tests

import (
	"slices"
	"testing"
	"time"
)

// weighInReminder is the weekly Weigh-in reminder as the Owner gets it.
var weighInReminder = notification{Title: "Calories", Body: "Time to log this week's weight"}

// saveWeighInReminder sets the whole Weigh-in reminder, as the Calories
// Assistant does.
func (d *browser) saveWeighInReminder(r weighInReminderJSON) {
	d.nando.t.Helper()
	callTool[settingsJSON](d.nando.t, d.tools(), "calories_save_settings", map[string]any{
		"weighInReminder": map[string]any{"enabled": r.Enabled, "weekday": r.Weekday, "time": r.Time},
	})
}

func TestTheAssistantChangesOnlyTheWeighInReminderFieldsItSends(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()

	got := callTool[settingsJSON](t, session, "calories_save_settings", map[string]any{
		"weighInReminder": map[string]any{"enabled": true},
	})
	if want := (weighInReminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"}); got.WeighInReminder != want {
		t.Errorf("after turning the Weigh-in reminder on: %+v, want %+v", got.WeighInReminder, want)
	}

	got = callTool[settingsJSON](t, session, "calories_save_settings", map[string]any{
		"weighInReminder": map[string]any{"weekday": "sunday", "time": "07:30"},
	})
	want := settingsJSON{
		NoEntriesReminder: notificationSettingJSON{Enabled: false, Time: "14:00"},
		EveningSummary:    notificationSettingJSON{Enabled: false, Time: "21:00"},
		WeighInReminder:   weighInReminderJSON{Enabled: true, Weekday: "sunday", Time: "07:30"},
	}
	if got != want {
		t.Errorf("after moving the Weigh-in reminder to Sunday 07:30: %+v, want %+v", got, want)
	}
	if got := get[settingsJSON](phone, "/api/calories/settings"); got != want {
		t.Errorf("the web app's settings = %+v, want %+v", got, want)
	}
}

func TestInvalidWeighInReminderSettingsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()

	for _, reminder := range []map[string]any{
		{},
		{"weekday": "funday"},
		{"weekday": "Monday"},
		{"time": "07:15"},
		{"enabled": "yes"},
	} {
		toolError(t, session, "calories_save_settings", map[string]any{"weighInReminder": reminder})
	}

	if got := get[settingsJSON](phone, "/api/calories/settings"); got.WeighInReminder != defaultWeighInReminder {
		t.Errorf("Weigh-in reminder after refused changes = %+v, want the default %+v", got.WeighInReminder, defaultWeighInReminder)
	}
}

func TestTheWeighInReminderIsSentByTheCaloriesBotAtItsTimeEachWeek(t *testing.T) {
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, tokyo) // Sunday 23:00 UTC
	n, phone := signedInBrowser(t, mondayMorning.Add(-time.Hour))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveWeighInReminder(weighInReminderJSON{Enabled: true, Weekday: "monday", Time: "08:00"})

	n.tickAt(mondayMorning.Add(-30 * time.Minute))
	if got := n.received(); len(got) != 0 {
		t.Fatalf("at 07:30 received %+v, want nothing yet", got)
	}

	n.tickAt(mondayMorning)
	n.tickAt(mondayMorning.Add(2 * time.Minute)) // a retried tick
	n.tickAt(mondayMorning.Add(30 * time.Minute))
	if got := n.received(); !slices.Equal(got, []notification{weighInReminder}) {
		t.Fatalf("on Monday by 08:30 received %+v, want one %+v", got, weighInReminder)
	}
	if sent := n.telegram.CallsTo("sendMessage"); sent[0].Token != "222:calories-bot" {
		t.Errorf("the Weigh-in reminder was sent by %s, want the Calories bot", sent[0].Token)
	}

	// Tuesday at 08:00 is not its weekday.
	n.tickAt(mondayMorning.AddDate(0, 0, 1))
	n.tickAt(mondayMorning.AddDate(0, 0, 7))
	if got := len(n.received()); got != 2 {
		t.Errorf("by the next Monday's 08:00 received %d reminders, want 2", got)
	}
}

func TestTheWeighInReminderIsNotSentWhileSwitchedOff(t *testing.T) {
	mondayMorning := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	n, phone := signedInBrowser(t, mondayMorning)
	phone.saveWeighInReminder(weighInReminderJSON{Enabled: false, Weekday: "monday", Time: "08:00"})

	n.tickAt(mondayMorning)

	if got := n.received(); len(got) != 0 {
		t.Errorf("received %+v, want nothing", got)
	}
}
