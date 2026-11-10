package tests

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/feriosch/nando-server/internal/calories/model"
)

func iptr(i int) *int { return &i }

// equalInts compares two optional numbers by value.
func equalInts(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func TestDayViewSumsTheDaysEntriesAgainstTheDailyLimit(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	mealID := phone.createMeal(onigiri)
	onigiriID := phone.mustLogEntry("2026-10-21", map[string]any{"mealId": mealID, "servings": 2})
	sushiID := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	got := get[model.DayJSON](phone, "/api/calories/days/2026-10-21")

	wantEntries := []model.EntryJSON{
		{ID: onigiriID, Day: "2026-10-21", MealID: &mealID, Name: "Tuna mayo onigiri", KcalPerServing: 230, Servings: 2, Kcal: 460},
		{ID: sushiID, Day: "2026-10-21", Name: "Sushi", KcalPerServing: 45, Servings: 12, Kcal: 540},
	}
	if !equalEntries(got.Entries, wantEntries) {
		t.Errorf("day view entries = %+v, want %+v", got.Entries, wantEntries)
	}
	want := model.CaloriesJSON{Day: "2026-10-21", ConsumedCalories: iptr(1000), DailyLimit: iptr(2000), Remaining: iptr(1000), OverLimit: bptr(false)}
	if !reflect.DeepEqual(got.CaloriesJSON, want) {
		t.Errorf("day view = %s, want %s", showCalories(got.CaloriesJSON), showCalories(want))
	}
}

// showBool shows an optional boolean, or "null".
func showBool(b *bool) any {
	if b == nil {
		return "null"
	}
	return *b
}

// show shows an optional number, or "null".
func show(i *int) any {
	if i == nil {
		return "null"
	}
	return *i
}

// showCalories shows a Day's Consumed Calories against its Daily Limit.
func showCalories(c model.CaloriesJSON) string {
	return fmt.Sprintf("%s consumed %v limit %v remaining %v over %v",
		c.Day, show(c.ConsumedCalories), show(c.DailyLimit), show(c.Remaining), showBool(c.OverLimit))
}

func TestAnUnloggedDayIsNotZeroKcal(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)

	got := get[model.DayJSON](phone, "/api/calories/days/2026-10-21")

	want := model.CaloriesJSON{Day: "2026-10-21", DailyLimit: iptr(2000)}
	if len(got.Entries) != 0 || !reflect.DeepEqual(got.CaloriesJSON, want) {
		t.Errorf("unlogged day view = %s with Entries %+v, want %s with none", showCalories(got.CaloriesJSON), got.Entries, showCalories(want))
	}
}

func TestPreviewTellsWhetherAnEntryWouldTakeTheDayOverTheDailyLimit(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 1000, "servings": 1})

	for path, want := range map[string]model.CaloriesJSON{
		// Reaching the Daily Limit isn't going over it.
		"/api/calories/days/2026-10-21/preview?kcal=1000": {Day: "2026-10-21", ConsumedCalories: iptr(2000), DailyLimit: iptr(2000), Remaining: iptr(0), OverLimit: bptr(false)},
		"/api/calories/days/2026-10-21/preview?kcal=1001": {Day: "2026-10-21", ConsumedCalories: iptr(2001), DailyLimit: iptr(2000), Remaining: iptr(-1), OverLimit: bptr(true)},
		// An Unlogged Day would be logged with just this Entry.
		"/api/calories/days/2026-10-20/preview?kcal=300": {Day: "2026-10-20", ConsumedCalories: iptr(300), DailyLimit: iptr(2000), Remaining: iptr(1700), OverLimit: bptr(false)},
	} {
		if got := get[model.CaloriesJSON](phone, path); !reflect.DeepEqual(got, want) {
			t.Errorf("GET %s = %s, want %s", path, showCalories(got), showCalories(want))
		}
	}
}

func TestWithoutADailyLimitNothingIsOverIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	got := get[model.CaloriesJSON](phone, "/api/calories/days/2026-10-21/preview?kcal=9000")
	if want := (model.CaloriesJSON{Day: "2026-10-21", ConsumedCalories: iptr(9000), OverLimit: bptr(false)}); !reflect.DeepEqual(got, want) {
		t.Errorf("preview without a Daily Limit = %s, want %s", showCalories(got), showCalories(want))
	}
}

func TestInvalidPreviewsAreRejected(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	for _, path := range []string{
		"/api/calories/days/2026-10-21/preview",
		"/api/calories/days/2026-10-21/preview?kcal=-1",
		"/api/calories/days/2026-10-21/preview?kcal=1.5",
		"/api/calories/days/2026-10-21/preview?kcal=1000001",
		"/api/calories/days/21-10-2026/preview?kcal=100",
	} {
		if rec := phone.do(http.MethodGet, path, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want 400", path, rec.Code)
		}
	}
}

func TestPreviewIsOnlyForDaysThatCanBeLoggedOn(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	for _, day := range []string{"2026-10-13", "2026-10-22"} {
		if rec := phone.do(http.MethodGet, "/api/calories/days/"+day+"/preview?kcal=100", nil); rec.Code != http.StatusConflict {
			t.Errorf("preview on %s: status = %d, want 409", day, rec.Code)
		}
	}
}

func TestPreviewOfAnEditLeavesOutTheEntryItReplaces(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	ramenID := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 1000, "servings": 1})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Gyoza", "kcalPerServing": 50, "servings": 10})
	yesterdaysID := phone.mustLogEntry("2026-10-20", map[string]any{"name": "Curry", "kcalPerServing": 700, "servings": 1})

	got := get[model.CaloriesJSON](phone, "/api/calories/days/2026-10-21/preview?kcal=1600&replacing="+ramenID)
	want := model.CaloriesJSON{Day: "2026-10-21", ConsumedCalories: iptr(2100), DailyLimit: iptr(2000), Remaining: iptr(-100), OverLimit: bptr(true)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("preview of editing Ramen to 1600 kcal = %s, want %s", showCalories(got), showCalories(want))
	}

	for _, id := range []string{yesterdaysID, "6af05d8a1400fbb2b9e973e3"} {
		path := "/api/calories/days/2026-10-21/preview?kcal=100&replacing=" + id
		if rec := phone.do(http.MethodGet, path, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want 400 (not an Entry on that Day)", path, rec.Code)
		}
	}
}
