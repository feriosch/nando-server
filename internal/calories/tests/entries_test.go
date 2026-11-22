package tests

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// mustLogEntry logs entry on day, as the Calories Assistant does, and
// returns the new Entry's ID. entry is calories_log_entry's arguments but
// the day: a Meal and its Servings, or a one-off's name, kcal per serving and
// Servings.
func (d *browser) mustLogEntry(day string, entry map[string]any) string {
	d.nando.t.Helper()
	args := map[string]any{"day": day}
	for k, v := range entry {
		args[k] = v
	}
	logged := callTool[struct {
		Entry entryJSON `json:"entry"`
	}](d.nando.t, d.tools(), "calories_log_entry", args)
	if logged.Entry.ID == "" {
		d.nando.t.Fatalf("calories_log_entry on %s: no Entry in the answer", day)
	}
	return logged.Entry.ID
}

func (d *browser) entries(day string) []entryJSON {
	d.nando.t.Helper()
	return get[[]entryJSON](d, "/api/calories/days/"+day+"/entries")
}

func TestEditingAMealNeverChangesItsEntries(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	mealID := phone.createMeal(onigiri)
	entryID := phone.mustLogEntry("2026-10-21", map[string]any{"mealId": mealID, "servings": 2})

	bigger := onigiri
	bigger.Name, bigger.Kcal = "Tuna mayo onigiri (large)", 310
	callTool[mealJSON](t, phone.tools(), "calories_save_meal", map[string]any{"id": mealID, "name": bigger.Name, "kcal": bigger.Kcal})

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
func (d *browser) saveTimeZone(name string) {
	d.nando.t.Helper()
	d.must(http.MethodPut, "/api/settings", map[string]any{
		"timeZone":        name,
		"heightCm":        nil,
		"weighInReminder": map[string]any{"enabled": false, "weekday": "monday", "time": "08:00"},
	}, http.StatusNoContent)
}

// logEntry logs entry on day through calories_log_entry, and returns what
// the Tool said if it refused, or "" if it logged it.
func (d *browser) logEntry(session *mcp.ClientSession, day string, entry map[string]any) string {
	d.nando.t.Helper()
	args := map[string]any{"day": day}
	for k, v := range entry {
		args[k] = v
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "calories_log_entry", Arguments: args})
	if err != nil {
		d.nando.t.Fatalf("calories_log_entry: %v", err)
	}
	if !result.IsError {
		return ""
	}
	return result.Content[0].(*mcp.TextContent).Text
}

func TestEntriesCanBeLoggedOnlyInTheEditableWindow(t *testing.T) {
	// 16:00 UTC on Wednesday is already 01:00 on Thursday 2026-10-22 in Tokyo,
	// so the Editable Window is 2026-10-15 to 2026-10-22.
	_, phone := signedInBrowser(t, wednesday.Add(4*time.Hour))
	phone.saveTimeZone("Asia/Tokyo")
	meal := map[string]any{"mealId": phone.createMeal(onigiri), "servings": 1}
	session := phone.tools()

	for day, logged := range map[string]bool{
		"2026-10-23": false, // tomorrow
		"2026-10-22": true,  // today
		"2026-10-15": true,  // 7 Days ago
		"2026-10-14": false, // 8 Days ago
	} {
		if refused := phone.logEntry(session, day, meal); (refused == "") != logged {
			t.Errorf("log an Entry on %s: refused %q; want logged %v", day, refused, logged)
		}
	}
}

func TestOwnerLogsAOneOffEntryWithServings(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

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
	_, phone := signedInBrowser(t, wednesday)

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

func TestInvalidEntriesAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	mealID := phone.createMeal(onigiri)
	session := phone.tools()

	for name, tc := range map[string]struct {
		day   string
		entry map[string]any
	}{
		"day not a date":        {"2026-10-32", map[string]any{"mealId": mealID, "servings": 1}},
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
		"Meal saved as Meal":    {"2026-10-21", map[string]any{"mealId": mealID, "servings": 1, "saveAsMeal": true}},
	} {
		if refused := phone.logEntry(session, tc.day, tc.entry); refused == "" {
			t.Errorf("%s: logged; want it refused", name)
		}
	}
	if got := phone.entries("2026-10-21"); len(got) != 0 {
		t.Errorf("entries = %+v, want none", got)
	}
}

func TestOwnerEditsAnEntryOnlyInTheEditableWindow(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	id := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	callTool[map[string]any](t, phone.tools(), "calories_update_entry",
		map[string]any{"id": id, "name": "Sushi (salmon)", "kcalPerServing": 50, "servings": 10})

	want := []entryJSON{{ID: id, Name: "Sushi (salmon)", KcalPerServing: 50, Servings: 10, Kcal: 500}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after editing = %+v, want %+v", got, want)
	}

	n.clock.Set(wednesday.AddDate(0, 0, 8)) // 2026-10-21 has left the Editable Window
	toolError(t, phone.tools(), "calories_update_entry", map[string]any{"id": id, "name": "Sushi", "kcalPerServing": 45, "servings": 1})
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after the rejected edit = %+v, want %+v", got, want)
	}
}

func TestOwnerDeletesAnEntryOnlyInTheEditableWindow(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	mistake := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 600, "servings": 1})
	kept := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	callTool[map[string]any](t, phone.tools(), "calories_delete_entry", map[string]any{"id": mistake})

	want := []entryJSON{{ID: kept, Name: "Sushi", KcalPerServing: 45, Servings: 12, Kcal: 540}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after deleting = %+v, want %+v", got, want)
	}

	n.clock.Set(wednesday.AddDate(0, 0, 8))
	toolError(t, phone.tools(), "calories_delete_entry", map[string]any{"id": kept})
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after the rejected delete = %+v, want %+v", got, want)
	}
}

func TestInvalidEntryEditsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	id := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})
	session := phone.tools()

	for name, change := range map[string]map[string]any{
		"no name":           {"name": " "},
		"negative kcal":     {"kcalPerServing": -1},
		"zero servings":     {"servings": 0},
		"too many servings": {"servings": 101},
	} {
		change["id"] = id
		if refused := toolError(t, session, "calories_update_entry", change); refused == "" {
			t.Errorf("%s: no reason given", name)
		}
	}
	want := []entryJSON{{ID: id, Name: "Sushi", KcalPerServing: 45, Servings: 12, Kcal: 540}}
	if got := phone.entries("2026-10-21"); !equalEntries(got, want) {
		t.Errorf("entries after refused edits = %+v, want %+v", got, want)
	}
}

func TestAnEntryRecordsWhenItWasLogged(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 1})

	got := get[[]struct {
		LoggedAt string `json:"loggedAt"`
	}](phone, "/api/calories/days/2026-10-21/entries")
	if len(got) != 1 || got[0].LoggedAt != "2026-10-21T12:00:00Z" {
		t.Errorf("loggedAt = %+v, want [2026-10-21T12:00:00Z], the clock's time", got)
	}
}
