package tests

import (
	"testing"
)

// dailyLimitOn is the Daily Limit the Day view shows for day, or nil.
func (d *browser) dailyLimitOn(day string) *int {
	d.nando.t.Helper()
	return get[dayJSON](d, "/api/calories/days/"+day).DailyLimit
}

// dailyLimitJSON is the Daily Limit as the API answers it.
type dailyLimitJSON struct {
	Kcal          int    `json:"kcal"`
	EffectiveFrom string `json:"effectiveFrom"`
}

// setDailyLimit sets the Daily Limit to kcal from today, as the Calories
// Assistant does.
func (d *browser) setDailyLimit(kcal int) {
	d.nando.t.Helper()
	callTool[map[string]any](d.nando.t, d.tools(), "calories_set_daily_limit", map[string]any{"kcal": kcal})
}

// dailyLimitAnswer is what calories_set_daily_limit and
// calories_get_daily_limit answer.
type dailyLimitAnswer struct {
	DailyLimit *dailyLimitJSON `json:"dailyLimit"`
}

func TestTheAssistantSetsTheDailyLimitFromToday(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	got := callTool[dailyLimitAnswer](t, phone.tools(), "calories_set_daily_limit", map[string]any{"kcal": 2000})

	want := dailyLimitJSON{Kcal: 2000, EffectiveFrom: "2026-10-21"}
	if got.DailyLimit == nil || *got.DailyLimit != want {
		t.Errorf("calories_set_daily_limit = %+v, want the Daily Limit in force, %+v", got.DailyLimit, want)
	}
	if got := get[*dailyLimitJSON](phone, "/api/calories/daily-limit"); got == nil || *got != want {
		t.Errorf("the web app's Daily Limit = %+v, want %+v", got, want)
	}
}

func TestInvalidDailyLimitsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)

	for _, args := range []map[string]any{
		{},
		{"kcal": 0},
		{"kcal": 10001},
		{"kcal": 1800.5},
		{"kcal": "1800"},
	} {
		toolError(t, phone.tools(), "calories_set_daily_limit", args)
	}
	if got := get[*dailyLimitJSON](phone, "/api/calories/daily-limit"); got == nil || got.Kcal != 2000 {
		t.Errorf("daily limit after refused changes = %+v, want still 2000", got)
	}
}

func TestEachDayIsJudgedAgainstTheDailyLimitItHad(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	n.clock.Set(wednesday.AddDate(0, 0, 2)) // Friday 2026-10-23
	phone.setDailyLimit(1500)
	phone.setDailyLimit(1800) // replaces Friday's first change

	want := dailyLimitJSON{Kcal: 1800, EffectiveFrom: "2026-10-23"}
	if got := get[*dailyLimitJSON](phone, "/api/calories/daily-limit"); got == nil || *got != want {
		t.Errorf("daily limit = %+v, want %+v", got, want)
	}
	for day, want := range map[string]int{
		"2026-10-01": 2000, // before the first Daily Limit, which covers it too
		"2026-10-21": 2000,
		"2026-10-22": 2000,
		"2026-10-23": 1800,
		"2026-10-24": 1800,
	} {
		if got := phone.dailyLimitOn(day); !equalInts(got, &want) {
			t.Errorf("daily limit on %s = %v, want %d", day, show(got), want)
		}
	}
}

func TestThereIsNoDailyLimitUntilOneIsSet(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	if got := get[*dailyLimitJSON](phone, "/api/calories/daily-limit"); got != nil {
		t.Errorf("daily limit = %+v, want null", got)
	}
	if got := phone.dailyLimitOn("2026-10-21"); got != nil {
		t.Errorf("daily limit on 2026-10-21 = %v, want null", *got)
	}
}
