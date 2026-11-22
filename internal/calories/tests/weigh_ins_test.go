package tests

import (
	"strings"
	"testing"
	"time"
)

// weighInJSON is one Weigh-in as the API lists it.
type weighInJSON struct {
	Week string  `json:"week"`
	Kg   float64 `json:"kg"`
}

// mustLogWeighIn logs kg as the Weigh-in for the week day is in, as the
// Calories Assistant does.
func (d *browser) mustLogWeighIn(day string, kg float64) {
	d.nando.t.Helper()
	callTool[map[string]any](d.nando.t, d.tools(), "calories_log_weigh_in", map[string]any{"day": day, "kg": kg})
}

func (d *browser) weighIns() []weighInJSON {
	d.nando.t.Helper()
	return get[[]weighInJSON](d, "/api/calories/weigh-ins")
}

func TestTheAssistantLogsThisWeeksWeighInAndTheWebAppListsIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	// No day: the Owner weighed in today, in the week starting Monday 2026-10-19.
	callTool[map[string]any](t, phone.tools(), "calories_log_weigh_in", map[string]any{"kg": 72.4})

	if got := phone.weighIns(); len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("Weigh-ins = %+v, want only 72.4 kg in the week of 2026-10-19", got)
	}
}

func TestAnyDayOfAWeekLogsThatWeeksWeighInAndALaterOneReplacesIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	phone.mustLogWeighIn("2026-10-19", 72.4) // Monday
	phone.mustLogWeighIn("2026-10-25", 71.9) // Sunday of the same week

	if got := phone.weighIns(); len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 71.9}) {
		t.Errorf("Weigh-ins = %+v, want only 71.9 kg in the week of 2026-10-19", got)
	}
}

func TestOnlyThisAndLastWeeksWeighInsCanBeLogged(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()

	for _, day := range []string{
		"2026-10-26", // next week
		"2026-10-11", // the Sunday two weeks ago
	} {
		if got := toolError(t, session, "calories_log_weigh_in", map[string]any{"day": day, "kg": 72.4}); !strings.Contains(got, "only this week's and last week's") {
			t.Errorf("calories_log_weigh_in on %s: %q, want it refused as outside this and last week", day, got)
		}
	}
	phone.mustLogWeighIn("2026-10-12", 72.8) // last Monday

	if got := phone.weighIns(); len(got) != 1 || got[0].Week != "2026-10-12" {
		t.Errorf("Weigh-ins = %+v, want only last week's", got)
	}
}

func TestInvalidWeighInsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()

	for name, args := range map[string]map[string]any{
		"day that isn't a date": {"day": "this-week", "kg": 72.4},
		"weight of zero":        {"kg": 0},
		"negative weight":       {"kg": -72.4},
		"weight over 500 kg":    {"kg": 500.1},
		"weight to 2 decimals":  {"kg": 72.45},
		"no weight":             {},
		"weight as a string":    {"kg": "72.4"},
	} {
		if got := toolError(t, session, "calories_log_weigh_in", args); got == "" {
			t.Errorf("calories_log_weigh_in with %s: refused without saying why", name)
		}
	}

	if got := phone.weighIns(); len(got) != 0 {
		t.Errorf("Weigh-ins after only invalid ones = %+v, want none", got)
	}
}

func TestTheAssistantDeletesAWeighIn(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.mustLogWeighIn("2026-10-12", 72.8)
	phone.mustLogWeighIn("2026-10-19", 72.4)

	// Any Day of the week names it.
	callTool[map[string]any](t, phone.tools(), "calories_delete_weigh_in", map[string]any{"day": "2026-10-21"})

	if got := phone.weighIns(); len(got) != 1 || got[0].Week != "2026-10-12" {
		t.Errorf("Weigh-ins after deleting this week's = %+v, want only last week's", got)
	}
}

func TestWeighInsBeforeLastWeekCanNoLongerBeChanged(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.mustLogWeighIn("2026-10-19", 72.4)
	n.clock.Set(wednesday.AddDate(0, 0, 14))
	session := phone.tools()

	for name, args := range map[string]map[string]any{
		"calories_log_weigh_in":    {"day": "2026-10-19", "kg": 71.0},
		"calories_delete_weigh_in": {"day": "2026-10-19"},
	} {
		if got := toolError(t, session, name, args); !strings.Contains(got, "only this week's and last week's") {
			t.Errorf("%s two weeks later: %q, want it refused", name, got)
		}
	}

	if got := phone.weighIns(); len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("Weigh-ins = %+v, want the one from two weeks ago unchanged", got)
	}
}

func TestChangingTheTimeZoneDoesNotMoveWeighIns(t *testing.T) {
	// Sunday evening in UTC, already Monday morning of the next week in Tokyo.
	_, phone := signedInBrowser(t, time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	phone.mustLogWeighIn("2026-10-25", 72.4)

	phone.saveTimeZone("Asia/Tokyo")

	if got := phone.weighIns(); len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("Weigh-ins after moving to Asia/Tokyo = %+v, want 72.4 kg still in the week of 2026-10-19", got)
	}
	// What moves is which week is this week: with no day, it is the next one.
	callTool[map[string]any](t, phone.tools(), "calories_log_weigh_in", map[string]any{"kg": 72.0})
	if got := phone.weighIns(); len(got) != 2 || got[0] != (weighInJSON{Week: "2026-10-26", Kg: 72.0}) {
		t.Errorf("Weigh-ins = %+v, want 72.0 kg in the week of 2026-10-26 first", got)
	}
}
