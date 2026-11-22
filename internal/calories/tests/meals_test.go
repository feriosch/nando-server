package tests

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// wednesday is a Day in the middle of a week, at noon UTC.
var wednesday = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// mealJSON is a Meal as the API reads and writes it.
type mealJSON struct {
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name"`
	Kcal     int      `json:"kcal"`
	Store    *string  `json:"store"`
	Emoji    *string  `json:"emoji"`
	ImageURL *string  `json:"imageUrl"`
	Tags     []string `json:"tags"`
	Pinned   bool     `json:"pinned"`
	Archived bool     `json:"archived"`
}

func ptr[T any](v T) *T { return &v }

// createMeal saves meal as a new Meal, as the Calories Assistant does, and
// returns its ID.
func (d *browser) createMeal(meal mealJSON) string {
	d.nando.t.Helper()
	args := map[string]any{"name": meal.Name, "kcal": meal.Kcal, "store": meal.Store, "emoji": meal.Emoji, "pinned": meal.Pinned}
	if meal.Tags != nil {
		args["tags"] = meal.Tags
	}
	saved := callTool[mealJSON](d.nando.t, d.tools(), "calories_save_meal", args)
	if saved.ID == "" {
		d.nando.t.Fatalf("calories_save_meal: no Meal in the answer")
	}
	return saved.ID
}

// archiveMeal archives the Meal with id, as the Calories Assistant does once
// the Owner said yes.
func (d *browser) archiveMeal(id string) {
	d.nando.t.Helper()
	callTool[mealJSON](d.nando.t, d.tools(), "calories_archive_meal", map[string]any{"id": id})
}

// mealsInQuickListOrder is the names of the Meals in use, in the quick
// list's order, as calories_list_meals answers them.
func (d *browser) mealsInQuickListOrder() []string {
	d.nando.t.Helper()
	var names []string
	for _, m := range callTool[mealsJSON](d.nando.t, d.tools(), "calories_list_meals", nil).Meals {
		names = append(names, m.Name)
	}
	return names
}

var onigiri = mealJSON{
	Name:  "Tuna mayo onigiri",
	Kcal:  230,
	Store: ptr("7-Eleven"),
	Emoji: ptr("🍙"),
	Tags:  []string{"snack"},
}

func TestOwnerSavesAMeal(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	id := phone.createMeal(onigiri)

	want := onigiri
	want.ID = id
	want.ImageURL = nil
	if got := get[[]mealJSON](phone, "/api/calories/meals"); !reflect.DeepEqual(got, []mealJSON{want}) {
		t.Errorf("GET /api/calories/meals = %+v, want [%+v]", got, want)
	}
}

func TestArchivingAMealKeepsItOutOfUseUntilRestored(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	id := phone.createMeal(onigiri)
	phone.mustLogEntry("2026-10-21", map[string]any{"mealId": id, "servings": 1})

	phone.archiveMeal(id)

	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 0 {
		t.Errorf("meals after archiving = %+v, want none", got)
	}
	if got := get[[]mealJSON](phone, "/api/calories/meals?archived=true"); len(got) != 1 || got[0].ID != id || !got[0].Archived {
		t.Errorf("archived meals = %+v, want the onigiri, archived", got)
	}
	if got := phone.entries("2026-10-21"); len(got) != 1 || got[0].Name != onigiri.Name {
		t.Errorf("entries after archiving their Meal = %+v, want the onigiri Entry kept", got)
	}
	if refused := phone.logEntry(phone.tools(), "2026-10-21", map[string]any{"mealId": id, "servings": 1}); refused == "" {
		t.Errorf("logged an archived Meal; want it refused")
	}

	callTool[mealJSON](t, phone.tools(), "calories_restore_meal", map[string]any{"id": id})

	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 1 || got[0].ID != id || got[0].Archived {
		t.Errorf("meals after restoring = %+v, want the onigiri back", got)
	}
}

func TestQuickListIsPinnedThenMostLoggedInTheLast30Days(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	meal := func(name string, pinned bool) string {
		return phone.createMeal(mealJSON{Name: name, Kcal: 100, Pinned: pinned})
	}
	logTimes := func(id string, times int) {
		today := n.clock.Now().UTC().Format(time.DateOnly)
		for range times {
			phone.mustLogEntry(today, map[string]any{"mealId": id, "servings": 1})
		}
	}
	meal("Coffee", true)
	natto := meal("Natto", false)
	onigiri := meal("Onigiri", false)
	banana := meal("Banana", false)
	meal("Yogurt", false)
	salad := meal("Salad", false)
	logTimes(banana, 5) // a month ago: no longer counts

	n.clock.Set(wednesday.AddDate(0, 0, 30))
	logTimes(onigiri, 3)
	logTimes(natto, 1)
	logTimes(salad, 4)
	phone.archiveMeal(salad)

	if got, want := phone.mealsInQuickListOrder(), []string{"Coffee", "Onigiri", "Natto", "Banana", "Yogurt"}; !slices.Equal(got, want) {
		t.Errorf("calories_list_meals = %q, want %q", got, want)
	}
}

func TestInvalidMealsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	id := phone.createMeal(onigiri)
	session := phone.tools()

	for _, meal := range map[string]map[string]any{
		"no name":        {"name": " ", "kcal": 230},
		"name too long":  {"name": strings.Repeat("x", 101), "kcal": 230},
		"negative kcal":  {"name": "Onigiri", "kcal": -1},
		"too many kcal":  {"name": "Onigiri", "kcal": 10001},
		"store too long": {"name": "Onigiri", "kcal": 230, "store": strings.Repeat("x", 101)},
		"emoji too long": {"name": "Onigiri", "kcal": 230, "emoji": "🍙🍙🍙🍙🍙🍙🍙🍙🍙"},
		"empty tag":      {"name": "Onigiri", "kcal": 230, "tags": []string{" "}},
		"tag too long":   {"name": "Onigiri", "kcal": 230, "tags": []string{strings.Repeat("x", 31)}},
	} {
		toolError(t, session, "calories_save_meal", meal)
		edit := map[string]any{"id": id}
		for k, v := range meal {
			edit[k] = v
		}
		toolError(t, session, "calories_save_meal", edit)
	}
	// A new Meal needs its kcal; an edit keeps the saved one.
	toolError(t, session, "calories_save_meal", map[string]any{"name": "Onigiri"})
	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 1 || got[0].Name != onigiri.Name {
		t.Errorf("meals = %+v, want only the unchanged onigiri", got)
	}
}

func TestChangingAMealOrEntryThatDoesNotExistIsRefused(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()
	const missing = "6af05d493ea403a78bf5997b"

	toolError(t, session, "calories_save_meal", map[string]any{"id": missing, "name": "Onigiri", "kcal": 230})
	toolError(t, session, "calories_archive_meal", map[string]any{"id": missing})
	toolError(t, session, "calories_restore_meal", map[string]any{"id": missing})
	toolError(t, session, "calories_update_entry", map[string]any{"id": missing, "name": "Sushi"})
}

func TestEditingAnArchivedMealKeepsItArchived(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	id := phone.createMeal(onigiri)
	phone.archiveMeal(id)

	callTool[mealJSON](t, phone.tools(), "calories_save_meal", map[string]any{"id": id, "name": "Salmon onigiri"})

	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 0 {
		t.Errorf("meals in use after editing an archived one = %+v, want none", got)
	}
	got := get[[]mealJSON](phone, "/api/calories/meals?archived=true")
	if len(got) != 1 || got[0].Name != "Salmon onigiri" || !got[0].Archived {
		t.Errorf("archived meals = %+v, want the renamed Salmon onigiri, still archived", got)
	}
}
