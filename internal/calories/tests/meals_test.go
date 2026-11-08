package tests

import (
	"encoding/json"
	"net/http"
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

func ptr(s string) *string { return &s }

// createMeal saves meal and returns its ID.
func (d *device) createMeal(meal mealJSON) string {
	d.nando.t.Helper()
	rec := d.must(http.MethodPost, "/api/calories/meals", meal, http.StatusCreated)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		d.nando.t.Fatalf("POST /api/calories/meals: body %q has no id", rec.Body)
	}
	return created.ID
}

var onigiri = mealJSON{
	Name:  "Tuna mayo onigiri",
	Kcal:  230,
	Store: ptr("7-Eleven"),
	Emoji: ptr("🍙"),
	Tags:  []string{"snack"},
}

func TestOwnerSavesAMeal(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	id := phone.createMeal(onigiri)

	want := onigiri
	want.ID = id
	want.ImageURL = nil
	if got := get[[]mealJSON](phone, "/api/calories/meals"); !reflect.DeepEqual(got, []mealJSON{want}) {
		t.Errorf("GET /api/calories/meals = %+v, want [%+v]", got, want)
	}
}

func TestDeletingAMealArchivesItUntilRestored(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	id := phone.createMeal(onigiri)
	phone.mustLogEntry("2026-10-21", map[string]any{"mealId": id, "servings": 1})

	phone.must(http.MethodDelete, "/api/calories/meals/"+id, nil, http.StatusNoContent)

	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 0 {
		t.Errorf("meals after deleting = %+v, want none", got)
	}
	if got := get[[]mealJSON](phone, "/api/calories/meals?archived=true"); len(got) != 1 || got[0].ID != id || !got[0].Archived {
		t.Errorf("archived meals = %+v, want the onigiri, archived", got)
	}
	if got := phone.entries("2026-10-21"); len(got) != 1 || got[0].Name != onigiri.Name {
		t.Errorf("entries after archiving their Meal = %+v, want the onigiri Entry kept", got)
	}
	if rec := phone.logEntry("2026-10-21", map[string]any{"mealId": id, "servings": 1}); rec.Code != http.StatusBadRequest {
		t.Errorf("log an archived Meal: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	phone.must(http.MethodPost, "/api/calories/meals/"+id+"/restore", nil, http.StatusNoContent)

	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 1 || got[0].ID != id || got[0].Archived {
		t.Errorf("meals after restoring = %+v, want the onigiri back", got)
	}
}

func TestQuickListIsPinnedThenMostLoggedInTheLast30Days(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
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
	phone.must(http.MethodDelete, "/api/calories/meals/"+salad, nil, http.StatusNoContent)

	var got []string
	for _, m := range get[[]mealJSON](phone, "/api/calories/meals/quick") {
		got = append(got, m.Name)
	}
	if want := []string{"Coffee", "Onigiri", "Natto", "Banana", "Yogurt"}; !slices.Equal(got, want) {
		t.Errorf("quick list = %q, want %q", got, want)
	}
}

func TestInvalidMealsAreRejected(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	id := phone.createMeal(onigiri)
	with := func(change func(m *mealJSON)) mealJSON {
		m := onigiri
		change(&m)
		return m
	}

	for name, body := range map[string]any{
		"not JSON":         "{",
		"no name":          with(func(m *mealJSON) { m.Name = " " }),
		"name too long":    with(func(m *mealJSON) { m.Name = strings.Repeat("x", 101) }),
		"no kcal":          map[string]any{"name": "Onigiri"},
		"negative kcal":    with(func(m *mealJSON) { m.Kcal = -1 }),
		"too many kcal":    with(func(m *mealJSON) { m.Kcal = 10001 }),
		"store too long":   with(func(m *mealJSON) { m.Store = ptr(strings.Repeat("x", 101)) }),
		"emoji too long":   with(func(m *mealJSON) { m.Emoji = ptr("🍙🍙🍙🍙🍙🍙🍙🍙🍙") }),
		"imageUrl not URL": with(func(m *mealJSON) { m.ImageURL = ptr("photo.jpg") }),
		"imageUrl http":    with(func(m *mealJSON) { m.ImageURL = ptr("http://example.com/a.jpg") }),
		"empty tag":        with(func(m *mealJSON) { m.Tags = []string{" "} }),
		"tag too long":     with(func(m *mealJSON) { m.Tags = []string{strings.Repeat("x", 31)} }),
	} {
		if rec := phone.do(http.MethodPost, "/api/calories/meals", body); rec.Code != http.StatusBadRequest {
			t.Errorf("create, %s: status = %d, body %q; want %d", name, rec.Code, rec.Body, http.StatusBadRequest)
		}
		if rec := phone.do(http.MethodPut, "/api/calories/meals/"+id, body); rec.Code != http.StatusBadRequest {
			t.Errorf("edit, %s: status = %d, body %q; want %d", name, rec.Code, rec.Body, http.StatusBadRequest)
		}
	}
	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 1 || got[0].Name != onigiri.Name {
		t.Errorf("meals = %+v, want only the unchanged onigiri", got)
	}
}

func TestChangingAMealOrEntryThatDoesNotExistIsNotFound(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	const missing = "6af05d493ea403a78bf5997b"

	for _, req := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/calories/meals/" + missing, onigiri},
		{http.MethodDelete, "/api/calories/meals/" + missing, nil},
		{http.MethodPost, "/api/calories/meals/" + missing + "/restore", nil},
		{http.MethodPut, "/api/calories/entries/" + missing, map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 1}},
	} {
		if rec := phone.do(req.method, req.path, req.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, body %q; want %d", req.method, req.path, rec.Code, rec.Body, http.StatusNotFound)
		}
	}
	// Deleting an Entry that doesn't exist succeeds, like Core's deletes.
	phone.must(http.MethodDelete, "/api/calories/entries/"+missing, nil, http.StatusNoContent)
}

func TestEditingAnArchivedMealKeepsItArchived(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	id := phone.createMeal(onigiri)
	phone.must(http.MethodDelete, "/api/calories/meals/"+id, nil, http.StatusNoContent)

	renamed := onigiri
	renamed.Name = "Salmon onigiri"
	phone.must(http.MethodPut, "/api/calories/meals/"+id, renamed, http.StatusNoContent)

	if got := get[[]mealJSON](phone, "/api/calories/meals"); len(got) != 0 {
		t.Errorf("meals in use after editing an archived one = %+v, want none", got)
	}
	got := get[[]mealJSON](phone, "/api/calories/meals?archived=true")
	if len(got) != 1 || got[0].Name != "Salmon onigiri" || !got[0].Archived {
		t.Errorf("archived meals = %+v, want the renamed Salmon onigiri, still archived", got)
	}
}
