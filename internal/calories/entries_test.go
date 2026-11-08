package calories_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// entryJSON is an Entry as the API lists it.
type entryJSON struct {
	ID             string  `json:"id"`
	MealID         *string `json:"mealId"`
	Name           string  `json:"name"`
	KcalPerServing int     `json:"kcalPerServing"`
	Servings       int     `json:"servings"`
	Kcal           int     `json:"kcal"`
}

// logEntry logs entry on day and returns the response.
func (d *device) logEntry(day string, entry any) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	return d.do(http.MethodPost, "/api/calories/days/"+day+"/entries", entry)
}

// mustLogEntry logs entry on day and returns the new Entry's ID.
func (d *device) mustLogEntry(day string, entry any) string {
	d.nando.t.Helper()
	rec := d.must(http.MethodPost, "/api/calories/days/"+day+"/entries", entry, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		d.nando.t.Fatalf("POST entries: body %q has no id", rec.Body)
	}
	return created.ID
}

func (d *device) entries(day string) []entryJSON {
	d.nando.t.Helper()
	return get[[]entryJSON](d, "/api/calories/days/"+day+"/entries")
}

func TestEditingAMealNeverChangesItsEntries(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	mealID := phone.createMeal(onigiri)
	entryID := phone.mustLogEntry("2026-10-21", map[string]any{"mealId": mealID, "servings": 2})

	bigger := onigiri
	bigger.Name, bigger.Kcal = "Tuna mayo onigiri (large)", 310
	phone.must(http.MethodPut, "/api/calories/meals/"+mealID, bigger, http.StatusNoContent)

	want := []entryJSON{{ID: entryID, MealID: &mealID, Name: "Tuna mayo onigiri", KcalPerServing: 230, Servings: 2, Kcal: 460}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after editing the Meal = %+v, want %+v", got, want)
	}
	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 1 || got[0].Name != bigger.Name || got[0].Kcal != 310 {
		t.Errorf("meals after editing = %+v, want the edited Meal", got)
	}
}

// equalEntries compares Entries by value, following MealID.
func equalEntries(a, b []entryJSON) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if (x.MealID == nil) != (y.MealID == nil) || (x.MealID != nil && *x.MealID != *y.MealID) {
			return false
		}
		x.MealID, y.MealID = nil, nil
		if x != y {
			return false
		}
	}
	return true
}

// saveTimeZone sets the Owner Time Zone through Core's settings API.
func (d *device) saveTimeZone(name string) {
	d.nando.t.Helper()
	d.must(http.MethodPut, "/api/settings", map[string]any{
		"timeZone":        name,
		"heightCm":        nil,
		"weighInReminder": map[string]any{"enabled": false, "weekday": "monday", "time": "08:00"},
	}, http.StatusNoContent)
}

func TestEntriesCanBeLoggedOnlyInTheEditableWindow(t *testing.T) {
	// 16:00 UTC on Wednesday is already 01:00 on Thursday 2026-10-22 in Tokyo,
	// so the Editable Window is 2026-10-15 to 2026-10-22.
	_, phone := signedInPhone(t, wednesday.Add(4*time.Hour))
	phone.saveTimeZone("Asia/Tokyo")
	meal := map[string]any{"mealId": phone.createMeal(onigiri), "servings": 1}

	for day, want := range map[string]int{
		"2026-10-23": http.StatusConflict, // tomorrow
		"2026-10-22": http.StatusCreated,  // today
		"2026-10-15": http.StatusCreated,  // 7 Days ago
		"2026-10-14": http.StatusConflict, // 8 Days ago
	} {
		if rec := phone.logEntry(day, meal); rec.Code != want {
			t.Errorf("log an Entry on %s: status = %d, body %q; want %d", day, rec.Code, rec.Body, want)
		}
	}
}

func TestOwnerLogsAOneOffEntryWithServings(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	id := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	want := []entryJSON{{ID: id, Name: "Sushi", KcalPerServing: 45, Servings: 12, Kcal: 540}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries = %+v, want %+v", got, want)
	}
	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 0 {
		t.Errorf("meals = %+v, want none: a one-off Entry isn't a Meal", got)
	}
}

func TestAOneOffEntryCanBeSavedAsAMeal(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	id := phone.mustLogEntry("2026-10-21", map[string]any{
		"name": "Salmon nigiri", "kcalPerServing": 60, "servings": 2, "saveAsMeal": true,
	})

	meals := get[[]mealJSON](phone, "/api/calories/meals")
	if len(meals) != 1 || meals[0].Name != "Salmon nigiri" || meals[0].Kcal != 60 {
		t.Fatalf("meals = %+v, want the saved Salmon nigiri at 60 kcal", meals)
	}
	want := []entryJSON{{ID: id, MealID: &meals[0].ID, Name: "Salmon nigiri", KcalPerServing: 60, Servings: 2, Kcal: 120}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries = %+v, want %+v", got, want)
	}
}

func TestInvalidEntriesAreRejected(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	mealID := phone.createMeal(onigiri)

	for name, tc := range map[string]struct {
		day   string
		entry any
	}{
		"day not a date":        {"2026-10-32", map[string]any{"mealId": mealID, "servings": 1}},
		"not JSON":              {"2026-10-21", "{"},
		"zero servings":         {"2026-10-21", map[string]any{"mealId": mealID, "servings": 0}},
		"no servings":           {"2026-10-21", map[string]any{"mealId": mealID}},
		"fractional servings":   {"2026-10-21", map[string]any{"mealId": mealID, "servings": 1.5}},
		"too many servings":     {"2026-10-21", map[string]any{"mealId": mealID, "servings": 101}},
		"unknown Meal":          {"2026-10-21", map[string]any{"mealId": "6af05d493ea403a78bf5997b", "servings": 1}},
		"one-off without name":  {"2026-10-21", map[string]any{"name": "  ", "kcalPerServing": 45, "servings": 1}},
		"one-off name too long": {"2026-10-21", map[string]any{"name": strings.Repeat("x", 101), "kcalPerServing": 45, "servings": 1}},
		"negative kcal":         {"2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": -1, "servings": 1}},
		"too many kcal":         {"2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 10001, "servings": 1}},
		"one-off without kcal":  {"2026-10-21", map[string]any{"name": "Sushi", "servings": 1}},
	} {
		if rec := phone.logEntry(tc.day, tc.entry); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body %q; want %d", name, rec.Code, rec.Body, http.StatusBadRequest)
		}
	}
	if got := phone.entries("2026-10-21"); len(got) != 0 {
		t.Errorf("entries = %+v, want none", got)
	}
}

func TestOwnerEditsAnEntryOnlyInTheEditableWindow(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	id := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	phone.must(http.MethodPut, "/api/calories/entries/"+id,
		map[string]any{"name": "Sushi (salmon)", "kcalPerServing": 50, "servings": 10}, http.StatusNoContent)

	want := []entryJSON{{ID: id, Name: "Sushi (salmon)", KcalPerServing: 50, Servings: 10, Kcal: 500}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after editing = %+v, want %+v", got, want)
	}

	n.clock.Set(wednesday.AddDate(0, 0, 8)) // 2026-10-21 has left the Editable Window
	rec := phone.do(http.MethodPut, "/api/calories/entries/"+id, map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 1})
	if rec.Code != http.StatusConflict {
		t.Errorf("edit an Entry 8 Days ago: status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after the rejected edit = %+v, want %+v", got, want)
	}
}

func TestOwnerDeletesAnEntryOnlyInTheEditableWindow(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	mistake := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 600, "servings": 1})
	kept := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	phone.must(http.MethodDelete, "/api/calories/entries/"+mistake, nil, http.StatusNoContent)

	want := []entryJSON{{ID: kept, Name: "Sushi", KcalPerServing: 45, Servings: 12, Kcal: 540}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after deleting = %+v, want %+v", got, want)
	}

	n.clock.Set(wednesday.AddDate(0, 0, 8))
	if rec := phone.do(http.MethodDelete, "/api/calories/entries/"+kept, nil); rec.Code != http.StatusConflict {
		t.Errorf("delete an Entry 8 Days ago: status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after the rejected delete = %+v, want %+v", got, want)
	}
}
