package tests

import (
	"net/http"
	"testing"

	"github.com/feriosch/nando-server/internal/calories/model"
)

// dailyLimitOn is the Daily Limit the Day view shows for day, or nil.
func (d *browser) dailyLimitOn(day string) *int {
	d.nando.t.Helper()
	return get[model.DayJSON](d, "/api/calories/days/"+day).DailyLimit
}

// setDailyLimit sets the Daily Limit to kcal from today.
func (d *browser) setDailyLimit(kcal int) {
	d.nando.t.Helper()
	d.must(http.MethodPut, "/api/calories/daily-limit", map[string]any{"kcal": kcal}, http.StatusNoContent)
}

func TestOwnerSetsTheDailyLimitFromToday(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	phone.setDailyLimit(2000)

	want := model.DailyLimitJSON{Kcal: 2000, EffectiveFrom: "2026-10-21"}
	if got := get[*model.DailyLimitJSON](phone, "/api/calories/daily-limit"); got == nil || *got != want {
		t.Errorf("daily limit = %+v, want %+v", got, want)
	}
}

func TestInvalidDailyLimitsAreRejected(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)

	for _, body := range []any{
		map[string]any{},
		map[string]any{"kcal": 0},
		map[string]any{"kcal": 10001},
		map[string]any{"kcal": 1800.5},
		"1800",
	} {
		if rec := phone.do(http.MethodPut, "/api/calories/daily-limit", body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT daily-limit %v: status = %d, want 400", body, rec.Code)
		}
	}
	if got := get[*model.DailyLimitJSON](phone, "/api/calories/daily-limit"); got == nil || got.Kcal != 2000 {
		t.Errorf("daily limit after rejected changes = %+v, want still 2000", got)
	}
}

func TestEachDayIsJudgedAgainstTheDailyLimitItHad(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	n.clock.Set(wednesday.AddDate(0, 0, 2)) // Friday 2026-10-23
	phone.setDailyLimit(1500)
	phone.setDailyLimit(1800) // replaces Friday's first change

	want := model.DailyLimitJSON{Kcal: 1800, EffectiveFrom: "2026-10-23"}
	if got := get[*model.DailyLimitJSON](phone, "/api/calories/daily-limit"); got == nil || *got != want {
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

	if got := get[*model.DailyLimitJSON](phone, "/api/calories/daily-limit"); got != nil {
		t.Errorf("daily limit = %+v, want null", got)
	}
	if got := phone.dailyLimitOn("2026-10-21"); got != nil {
		t.Errorf("daily limit on 2026-10-21 = %v, want null", *got)
	}
}
