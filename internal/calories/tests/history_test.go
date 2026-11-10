package tests

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/feriosch/nando-server/internal/calories/model"
)

func bptr(b bool) *bool { return &b }

func TestHistoryShowsEachDayAgainstItsDailyLimitAndUnloggedDaysAsNotLogged(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday.AddDate(0, 0, -2)) // Monday 2026-10-19
	phone.setDailyLimit(2000)
	n.clock.Set(wednesday)
	phone.setDailyLimit(1800)
	phone.mustLogEntry("2026-10-18", map[string]any{"name": "Curry", "kcalPerServing": 500, "servings": 1})
	phone.mustLogEntry("2026-10-19", map[string]any{"name": "Pizza", "kcalPerServing": 300, "servings": 7})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 2})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Water", "kcalPerServing": 0, "servings": 1})
	n.clock.Set(wednesday.AddDate(0, 0, 1)) // Thursday, nothing logged yet

	got := get[[]model.CaloriesJSON](phone, "/api/calories/history?from=2026-10-18&to=2026-10-22")

	want := []model.CaloriesJSON{
		{Day: "2026-10-22", DailyLimit: iptr(1800)},
		{Day: "2026-10-21", ConsumedCalories: iptr(1800), DailyLimit: iptr(1800), Remaining: iptr(0), OverLimit: bptr(false)},
		{Day: "2026-10-20", DailyLimit: iptr(2000)},
		{Day: "2026-10-19", ConsumedCalories: iptr(2100), DailyLimit: iptr(2000), Remaining: iptr(-100), OverLimit: bptr(true)},
		// Before the first Daily Limit, which covers it too.
		{Day: "2026-10-18", ConsumedCalories: iptr(500), DailyLimit: iptr(2000), Remaining: iptr(1500), OverLimit: bptr(false)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("history =\n%s\nwant\n%s", showHistory(got), showHistory(want))
	}
}

func TestHistoryWithoutADailyLimitIsNeverOverIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 5})

	got := get[[]model.CaloriesJSON](phone, "/api/calories/history?from=2026-10-20&to=2026-10-21")

	want := []model.CaloriesJSON{
		{Day: "2026-10-21", ConsumedCalories: iptr(4500), OverLimit: bptr(false)},
		{Day: "2026-10-20"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("history =\n%s\nwant\n%s", showHistory(got), showHistory(want))
	}
}

func TestInvalidHistoryRangesAreRejected(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	for _, query := range []string{
		"",
		"?from=2026-10-21",
		"?to=2026-10-21",
		"?from=2026-10-22&to=2026-10-21",
		"?from=21-10-2026&to=2026-10-21",
		"?from=2025-10-20&to=2026-10-21", // 367 Days
		"?from=2026-10-21&to=2026-10-22", // tomorrow isn't past yet
	} {
		if rec := phone.do(http.MethodGet, "/api/calories/history"+query, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("GET history%s: status = %d, want 400", query, rec.Code)
		}
	}
	if got := get[[]model.CaloriesJSON](phone, "/api/calories/history?from=2025-10-21&to=2026-10-21"); len(got) != 366 {
		t.Errorf("history of 366 Days has %d", len(got))
	}
}

func showHistory(days []model.CaloriesJSON) string {
	s := ""
	for _, d := range days {
		s += showCalories(d) + "\n"
	}
	return s
}
