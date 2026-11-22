package tests

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

// tokyo is the Owner Time Zone in these tests, so their Days and clock times
// differ from UTC's.
var tokyo = mustLoadLocation("Asia/Tokyo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// tickAt sets the clock to now and ticks as Cloud Scheduler does, failing the
// test unless the tick succeeds.
func (n *nando) tickAt(now time.Time) {
	n.t.Helper()
	n.clock.Set(now)
	req, err := http.NewRequest(http.MethodPost, "/internal/tick", nil)
	if err != nil {
		n.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer scheduler-token")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		n.t.Fatalf("tick at %s: status = %d, body %q", now, rec.Code, rec.Body)
	}
}

// on is the Notification setting switched on at clock time hhmm.
func on(hhmm string) notificationSettingJSON {
	return notificationSettingJSON{Enabled: true, Time: hhmm}
}

// off is the Notification setting switched off, though set for hhmm.
func off(hhmm string) notificationSettingJSON {
	return notificationSettingJSON{Enabled: false, Time: hhmm}
}

// tokyoTime is the moment it is hh:mm on Thursday 2026-10-22 in Tokyo.
func tokyoTime(hh, mm int) time.Time {
	return time.Date(2026, 10, 22, hh, mm, 0, 0, tokyo)
}

func TestTheNoEntriesReminderIsSentAtItsTimeWhenTodayHasNoEntries(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveCaloriesSettings(on("14:00"), off("21:00"))

	n.tickAt(tokyoTime(13, 30))
	n.tickAt(tokyoTime(14, 0))
	n.tickAt(tokyoTime(14, 30))

	want := []notification{{Title: "Calories", Body: "Nothing logged yet today"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestTheNoEntriesReminderIsNotSentOnceTodayHasAnEntry(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveCaloriesSettings(on("08:00"), off("21:00"))

	// 08:00 on Thursday in Tokyo is still Wednesday in UTC, but Wednesday's
	// Entry isn't today's.
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 1})
	n.tickAt(tokyoTime(8, 0))

	want := []notification{{Title: "Calories", Body: "Nothing logged yet today"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("on Thursday, received %+v, want %+v", got, want)
	}

	// On Friday, something is logged before 08:00 Tokyo time.
	n.clock.Set(tokyoTime(7, 0).Add(24 * time.Hour))
	phone.mustLogEntry("2026-10-23", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 1})
	n.tickAt(tokyoTime(8, 0).Add(24 * time.Hour))

	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("by Friday, received %+v, want only Thursday's reminder %+v", got, want)
	}
}

func TestTheNoEntriesReminderIsNotSentWhileSwitchedOff(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveCaloriesSettings(off("14:00"), off("21:00"))

	n.tickAt(tokyoTime(14, 0))

	if got := n.received(); len(got) != 0 {
		t.Errorf("received %+v, want nothing", got)
	}
}

func TestTheEveningSummaryTellsTheOwnerTodaysConsumedCaloriesAgainstTheDailyLimit(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.setDailyLimit(2000)
	phone.saveCaloriesSettings(off("14:00"), on("21:30"))
	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})
	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Ramen", "kcalPerServing": 1000, "servings": 1})

	n.tickAt(tokyoTime(21, 0))
	n.tickAt(tokyoTime(21, 30))
	n.tickAt(tokyoTime(21, 31)) // Cloud Scheduler retries a tick it thinks failed
	n.tickAt(tokyoTime(22, 0))

	want := []notification{{Title: "Calories", Body: "Today: 1460 / 2000 kcal"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestTheEveningSummaryWithoutADailyLimitTellsOnlyTodaysConsumedCalories(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveCaloriesSettings(off("14:00"), on("21:00"))
	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})

	n.tickAt(tokyoTime(21, 0))

	want := []notification{{Title: "Calories", Body: "Today: 460 kcal"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestTheEveningSummaryOfAnUnloggedDaySaysNothingWasLogged(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.setDailyLimit(2000)
	phone.saveCaloriesSettings(off("14:00"), on("21:00"))

	n.tickAt(tokyoTime(21, 0))

	want := []notification{{Title: "Calories", Body: "Nothing logged today"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v (an Unlogged Day isn't 0 kcal)", got, want)
	}
}

func TestTheEveningSummaryIsNotSentWhileSwitchedOff(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveCaloriesSettings(off("14:00"), off("21:00"))
	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})

	n.tickAt(tokyoTime(21, 0))

	if got := n.received(); len(got) != 0 {
		t.Errorf("received %+v, want nothing", got)
	}
}

func TestOnlyTheEveningSummaryIsSentWhenTheNoEntriesReminderIsSetForTheSameTime(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.saveCaloriesSettings(on("21:00"), on("21:00"))

	n.tickAt(tokyoTime(21, 0))

	want := []notification{{Title: "Calories", Body: "Nothing logged today"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want only the evening summary %+v", got, want)
	}
}
