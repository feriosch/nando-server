package tests

import (
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
