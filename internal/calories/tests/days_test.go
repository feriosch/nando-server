package tests

import (
	"net/http"
	"testing"
)

// dayJSON is the Day view as the API answers it.
type dayJSON struct {
	Day              string      `json:"day"`
	Entries          []entryJSON `json:"entries"`
	ConsumedCalories *int        `json:"consumedCalories"`
	DailyLimit       *int        `json:"dailyLimit"`
	Remaining        *int        `json:"remaining"`
	OverLimit        *bool       `json:"overLimit"`
}

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

	got := get[dayJSON](phone, "/api/calories/days/2026-10-21")

	wantEntries := []entryJSON{
		{ID: onigiriID, MealID: &mealID, Name: "Tuna mayo onigiri", KcalPerServing: 230, Servings: 2, Kcal: 460},
		{ID: sushiID, Name: "Sushi", KcalPerServing: 45, Servings: 12, Kcal: 540},
	}
	if got.Day != "2026-10-21" || !equalEntries(got.Entries, wantEntries) {
		t.Errorf("day view = %s %+v, want 2026-10-21 %+v", got.Day, got.Entries, wantEntries)
	}
	if !equalInts(got.ConsumedCalories, iptr(1000)) || !equalInts(got.DailyLimit, iptr(2000)) || !equalInts(got.Remaining, iptr(1000)) ||
		!equalBools(got.OverLimit, bptr(false)) {
		t.Errorf("day view consumed/limit/remaining/over = %v/%v/%v/%v, want 1000/2000/1000/false",
			show(got.ConsumedCalories), show(got.DailyLimit), show(got.Remaining), showBool(got.OverLimit))
	}
}

// equalBools compares two optional booleans by value.
func equalBools(a, b *bool) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
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

func TestAnUnloggedDayIsNotZeroKcal(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)

	got := get[dayJSON](phone, "/api/calories/days/2026-10-21")

	if len(got.Entries) != 0 || got.ConsumedCalories != nil || got.Remaining != nil || got.OverLimit != nil || !equalInts(got.DailyLimit, iptr(2000)) {
		t.Errorf("unlogged day view = %+v consumed/limit/remaining %v/%v/%v, want no Entries, null/2000/null and overLimit null",
			got.Entries, show(got.ConsumedCalories), show(got.DailyLimit), show(got.Remaining))
	}
}

// previewJSON is what logging kcal more on a Day would make of it.
type previewJSON struct {
	Day              string `json:"day"`
	ConsumedCalories int    `json:"consumedCalories"`
	DailyLimit       *int   `json:"dailyLimit"`
	Remaining        *int   `json:"remaining"`
	OverLimit        bool   `json:"overLimit"`
}

func TestPreviewTellsWhetherAnEntryWouldTakeTheDayOverTheDailyLimit(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 1000, "servings": 1})

	for path, want := range map[string]previewJSON{
		// Reaching the Daily Limit isn't going over it.
		"/api/calories/days/2026-10-21/preview?kcal=1000": {ConsumedCalories: 2000, DailyLimit: iptr(2000), Remaining: iptr(0), OverLimit: false},
		"/api/calories/days/2026-10-21/preview?kcal=1001": {ConsumedCalories: 2001, DailyLimit: iptr(2000), Remaining: iptr(-1), OverLimit: true},
		// An Unlogged Day would be logged with just this Entry.
		"/api/calories/days/2026-10-20/preview?kcal=300": {ConsumedCalories: 300, DailyLimit: iptr(2000), Remaining: iptr(1700), OverLimit: false},
	} {
		got := get[previewJSON](phone, path)
		if got.ConsumedCalories != want.ConsumedCalories || !equalInts(got.DailyLimit, want.DailyLimit) ||
			!equalInts(got.Remaining, want.Remaining) || got.OverLimit != want.OverLimit {
			t.Errorf("GET %s = %d/%v/%v over %t, want %d/%v/%v over %t", path,
				got.ConsumedCalories, show(got.DailyLimit), show(got.Remaining), got.OverLimit,
				want.ConsumedCalories, show(want.DailyLimit), show(want.Remaining), want.OverLimit)
		}
	}
}

func TestWithoutADailyLimitNothingIsOverIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	got := get[previewJSON](phone, "/api/calories/days/2026-10-21/preview?kcal=9000")
	if got.Day != "2026-10-21" || got.ConsumedCalories != 9000 || got.DailyLimit != nil || got.Remaining != nil || got.OverLimit {
		t.Errorf("preview without a Daily Limit = %+v, want 2026-10-21, 9000, no limit, nothing remaining, not over", got)
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

	got := get[previewJSON](phone, "/api/calories/days/2026-10-21/preview?kcal=1600&replacing="+ramenID)
	if got.ConsumedCalories != 2100 || !equalInts(got.Remaining, iptr(-100)) || !got.OverLimit {
		t.Errorf("preview of editing Ramen to 1600 kcal = %+v, want 2100 consumed, -100 remaining, over", got)
	}

	for _, id := range []string{yesterdaysID, "6af05d8a1400fbb2b9e973e3"} {
		path := "/api/calories/days/2026-10-21/preview?kcal=100&replacing=" + id
		if rec := phone.do(http.MethodGet, path, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want 400 (not an Entry on that Day)", path, rec.Code)
		}
	}
}
