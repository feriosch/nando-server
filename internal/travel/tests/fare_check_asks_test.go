package tests

import (
	"reflect"
	"strings"
	"testing"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// fareCheckSettingJSON is when Nando asks for a Daily Fare Check.
type fareCheckSettingJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

// weeklyFareCheckSettingJSON is when Nando asks for the Weekly Fare Check.
type weeklyFareCheckSettingJSON struct {
	Enabled bool   `json:"enabled"`
	Weekday string `json:"weekday"`
	Time    string `json:"time"`
}

// fareCheckSettingsJSON is the part of Travel's settings about Fare Checks.
type fareCheckSettingsJSON struct {
	DailyFareCheck  fareCheckSettingJSON       `json:"dailyFareCheck"`
	WeeklyFareCheck weeklyFareCheckSettingJSON `json:"weeklyFareCheck"`
}

// at is the time hh:mm on day, in UTC, the Owner Time Zone until set.
func at(day time.Time, hh, mm int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, time.UTC)
}

// asks are the Travel bot's messages that ask for a Fare Check.
func (n *nando) asks() []string {
	var got []string
	for _, text := range n.travelMessages() {
		if strings.Contains(text, "Fare Check") {
			got = append(got, strings.SplitN(text, "\n", 2)[0])
		}
	}
	return got
}

func TestFareChecksAreOffUntilTheOwnerTurnsThemOn(t *testing.T) {
	_, phone := signedInBrowser(t)

	got := callTool[fareCheckSettingsJSON](t, phone.tools(), "travel_get_settings", nil)

	want := fareCheckSettingsJSON{
		DailyFareCheck:  fareCheckSettingJSON{Enabled: false, Time: "08:30"},
		WeeklyFareCheck: weeklyFareCheckSettingJSON{Enabled: false, Weekday: "sunday", Time: "10:00"},
	}
	if got != want {
		t.Errorf("Fare Check settings = %+v, want %+v", got, want)
	}
}

func TestTheDailyFareCheckIsAskedForInTheTravelChatAtItsTime(t *testing.T) {
	n, phone := signedInBrowser(t)
	newTrip(t, phone, nil)
	callTool[settingsJSON](t, phone.tools(), "travel_save_settings", map[string]any{
		"dailyFareCheck": map[string]any{"enabled": true, "time": "07:30"},
	})
	monday := today.AddDate(0, 0, 1)

	n.tickAt(at(monday, 7, 0))
	if got := n.asks(); len(got) != 0 {
		t.Fatalf("asks before 07:30 = %v, want none", got)
	}
	n.tickAt(at(monday, 7, 30))
	n.tickAt(at(monday, 7, 35)) // the next tick in the same half hour
	if got, want := n.asks(), []string{"✈️ Daily Fare Check"}; !reflect.DeepEqual(got, want) {
		t.Errorf("asks at 07:30 = %v, want %v once, by the Travel bot", got, want)
	}
	talk := n.travelTalk()
	if len(talk) != 1 || talk[0].From != coremodel.FromNando || talk[0].State != coremodel.MessageWaiting {
		t.Errorf("the Travel Talk = %+v; want the ask, waiting for a run", talk)
	}
}

func TestOnTheWeeklyFareChecksDayOnlyTheWeeklyOneIsAskedFor(t *testing.T) {
	n, phone := signedInBrowser(t)
	newTrip(t, phone, nil)
	callTool[settingsJSON](t, phone.tools(), "travel_save_settings", map[string]any{
		"dailyFareCheck":  map[string]any{"enabled": true, "time": "08:00"},
		"weeklyFareCheck": map[string]any{"enabled": true, "weekday": "sunday", "time": "09:30"},
	})
	sunday := today.AddDate(0, 0, 7)

	n.tickAt(at(sunday, 8, 0))
	n.tickAt(at(sunday, 9, 30))
	n.tickAt(at(sunday.AddDate(0, 0, 1), 8, 0)) // Monday

	if got, want := n.asks(), []string{"✈️ Weekly Fare Check", "✈️ Daily Fare Check"}; !reflect.DeepEqual(got, want) {
		t.Errorf("asks = %v, want %v: Sunday's weekly one only, then Monday's daily one", got, want)
	}
}

func TestNoFareCheckIsAskedForWhileNoTripIsWatching(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	callTool[tripJSON](t, phone.tools(), "travel_set_trip_status", map[string]any{"id": trip.ID, "status": "booked"})
	callTool[settingsJSON](t, phone.tools(), "travel_save_settings", map[string]any{
		"dailyFareCheck": map[string]any{"enabled": true, "time": "08:00"},
	})

	n.tickAt(at(today.AddDate(0, 0, 1), 8, 0))

	if got := n.asks(); len(got) != 0 {
		t.Errorf("asks with no Watching Trip = %v, want none", got)
	}
}

func TestTheAssistantMovesTheFareChecksAndBadTimesAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	got := callTool[fareCheckSettingsJSON](t, session, "travel_save_settings", map[string]any{
		"weeklyFareCheck": map[string]any{"weekday": "saturday"},
	})
	want := fareCheckSettingsJSON{
		DailyFareCheck:  fareCheckSettingJSON{Enabled: false, Time: "08:30"},
		WeeklyFareCheck: weeklyFareCheckSettingJSON{Enabled: false, Weekday: "saturday", Time: "10:00"},
	}
	if got != want {
		t.Errorf("after moving the weekly one to Saturday: %+v, want %+v", got, want)
	}
	for _, args := range []map[string]any{
		{"dailyFareCheck": map[string]any{}},
		{"dailyFareCheck": map[string]any{"time": "08:15"}},
		{"weeklyFareCheck": map[string]any{"weekday": "funday"}},
		{"weeklyFareCheck": map[string]any{"time": "25:00"}},
	} {
		toolError(t, session, "travel_save_settings", args)
	}
}
