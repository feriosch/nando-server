package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/testconnector"
)

// connect has the Owner approve a Connector with the iPhone's Passkey and
// returns its MCP session, as Claude would have it.
func (d *browser) connect() *mcp.ClientSession {
	d.nando.t.Helper()
	return testconnector.Connect(d.nando.t, d.nando.handler, d.passkey, webOrigin)
}

// callTool calls the Tool name with args and decodes its result into a T,
// failing the test if the call or the Tool fails.
func callTool[T any](t *testing.T, session *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %+v", name, result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s answered %#v, want JSON text", name, result.Content[0])
	}
	var v T
	if err := json.Unmarshal([]byte(text.Text), &v); err != nil {
		t.Fatalf("%s: %v in %q", name, err, text.Text)
	}
	return v
}

// caloriesJSON is a Day's Consumed Calories against its Daily Limit.
type caloriesJSON struct {
	Day              string `json:"day"`
	ConsumedCalories *int   `json:"consumedCalories"`
	DailyLimit       *int   `json:"dailyLimit"`
	Remaining        *int   `json:"remaining"`
	OverLimit        *bool  `json:"overLimit"`
}

// loggedJSON is what a Tool that changes an Entry answers: the Entry, and
// how its Day now stands against the Daily Limit.
type loggedJSON struct {
	Entry entryJSON    `json:"entry"`
	Day   caloriesJSON `json:"day"`
}

func TestAConnectorLogsWhatTheOwnerAteTodayAndSeesTheDayAgainstTheDailyLimit(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 1})
	session := phone.connect()

	// No Day: what the Owner "just ate" is today's.
	got := callTool[loggedJSON](t, session, "calories_log_entry",
		map[string]any{"name": "Chicken curry", "kcalPerServing": 750, "servings": 1})

	if got.Entry.ID == "" || got.Entry.Name != "Chicken curry" || got.Entry.MealID != nil || got.Entry.Kcal != 750 {
		t.Errorf("entry = %+v, want the one-off Chicken curry of 750 kcal", got.Entry)
	}
	want := caloriesJSON{Day: "2026-10-21", ConsumedCalories: ptr(980), DailyLimit: ptr(2000), Remaining: ptr(1020), OverLimit: ptr(false)}
	if !reflect.DeepEqual(got.Day, want) {
		t.Errorf("day = %+v, want %+v", got.Day, want)
	}
	if entries := phone.entries("2026-10-21"); len(entries) != 2 || entries[1].ID != got.Entry.ID {
		t.Errorf("the web app's Entries for today = %+v, want the Onigiri and then the curry", entries)
	}
}

func TestAConnectorReadsADayAsTheWebAppShowsIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	phone.mustLogEntry("2026-10-20", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 1})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})
	session := phone.connect()

	for day, args := range map[string]map[string]any{
		"2026-10-20": {"day": "2026-10-20"},
		// No Day is today.
		"2026-10-21": {},
	} {
		got := callTool[dayJSON](t, session, "calories_get_day", args)
		if want := get[dayJSON](phone, "/api/calories/days/"+day); !reflect.DeepEqual(got, want) || len(got.Entries) != 1 {
			t.Errorf("calories_get_day %v = %+v,\nwant the web app's %+v", args, got, want)
		}
	}
}

func TestAConnectorCorrectsOnlyWhatItSendsOfAnEntry(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	mealID := phone.createMeal(onigiri)
	id := phone.mustLogEntry("2026-10-20", map[string]any{"mealId": mealID, "servings": 1})
	session := phone.connect()

	// "It was three, not one."
	got := callTool[loggedJSON](t, session, "calories_update_entry", map[string]any{"id": id, "servings": 3})

	want := entryJSON{ID: id, MealID: &mealID, Name: "Tuna mayo onigiri", KcalPerServing: 230, Servings: 3, Kcal: 690}
	if !equalEntries([]entryJSON{got.Entry}, []entryJSON{want}) {
		t.Errorf("entry = %+v, want %+v", got.Entry, want)
	}
	if want := (caloriesJSON{Day: "2026-10-20", ConsumedCalories: ptr(690), DailyLimit: ptr(2000), Remaining: ptr(1310), OverLimit: ptr(false)}); !reflect.DeepEqual(got.Day, want) {
		t.Errorf("day = %+v, want %+v", got.Day, want)
	}
	if entries := phone.entries("2026-10-20"); !equalEntries(entries, []entryJSON{want}) {
		t.Errorf("the web app's Entries = %+v, want %+v", entries, want)
	}
}

func TestAConnectorDeletesAnEntryAndSeesWhatIsLeftOfTheDay(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	keep := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 1})
	twice := phone.mustLogEntry("2026-10-21", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 1})
	session := phone.connect()

	got := callTool[struct {
		Day caloriesJSON `json:"day"`
	}](t, session, "calories_delete_entry", map[string]any{"id": twice})

	if want := (caloriesJSON{Day: "2026-10-21", ConsumedCalories: ptr(230), DailyLimit: ptr(2000), Remaining: ptr(1770), OverLimit: ptr(false)}); !reflect.DeepEqual(got.Day, want) {
		t.Errorf("day = %+v, want %+v", got.Day, want)
	}
	if entries := phone.entries("2026-10-21"); len(entries) != 1 || entries[0].ID != keep {
		t.Errorf("the web app's Entries = %+v, want only the first Onigiri", entries)
	}
}

// toolError calls the Tool name with args, which must fail, and returns
// what it says.
func toolError(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || !result.IsError || len(result.Content) == 0 {
		t.Fatalf("%s with %v: %v, %+v; want the Tool to fail", name, args, err, result)
	}
	return result.Content[0].(*mcp.TextContent).Text
}

func TestAConnectorCantChangeADayOutsideTheEditableWindow(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	old := phone.mustLogEntry("2026-10-14", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 1})
	n.clock.Set(wednesday.AddDate(0, 0, 1))
	session := phone.connect()

	for name, args := range map[string]map[string]any{
		"calories_log_entry":    {"day": "2026-10-14", "name": "Ramen", "kcalPerServing": 900, "servings": 1},
		"calories_update_entry": {"id": old, "servings": 2},
		"calories_delete_entry": {"id": old},
	} {
		if got := toolError(t, session, name, args); !strings.Contains(got, "only today and the 7 Days before it") {
			t.Errorf("%s on a Day outside the Editable Window says %q, want why it can't", name, got)
		}
	}
	if entries := phone.entries("2026-10-14"); len(entries) != 1 || entries[0].Servings != 1 {
		t.Errorf("the old Day's Entries = %+v, want them unchanged", entries)
	}
}

// inForceJSON is what calories_get_daily_limit answers.
type inForceJSON struct {
	DailyLimit *dailyLimitJSON `json:"dailyLimit"`
}

func TestAConnectorReadsTheDailyLimitInForce(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.connect()

	if got := callTool[inForceJSON](t, session, "calories_get_daily_limit", nil); got.DailyLimit != nil {
		t.Errorf("before one is set, dailyLimit = %+v, want null", got.DailyLimit)
	}

	phone.setDailyLimit(1800)
	got := callTool[inForceJSON](t, session, "calories_get_daily_limit", nil)
	if got.DailyLimit == nil || got.DailyLimit.Kcal != 1800 || got.DailyLimit.EffectiveFrom != "2026-10-21" {
		t.Errorf("dailyLimit = %+v, want 1800 kcal from 2026-10-21", got.DailyLimit)
	}
}

// mealsJSON is what calories_list_meals answers.
type mealsJSON struct {
	Meals []mealJSON `json:"meals"`
}

func TestAConnectorListsTheMealsAsTheQuickListDoes(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	curry := phone.createMeal(mealJSON{Name: "Chicken curry", Kcal: 750, Tags: []string{}})
	phone.createMeal(onigiri)
	gone := phone.createMeal(mealJSON{Name: "Melon pan", Kcal: 400, Tags: []string{}})
	phone.archiveMeal(gone)
	// The most logged lead, though "Tuna" comes after "Chicken".
	phone.mustLogEntry("2026-10-21", map[string]any{"mealId": curry, "servings": 1})
	session := phone.connect()

	got := callTool[mealsJSON](t, session, "calories_list_meals", nil)
	if len(got.Meals) != 2 || got.Meals[0].ID != curry || got.Meals[1].Name != onigiri.Name {
		t.Errorf("calories_list_meals = %+v, want the Chicken curry, most logged, then the onigiri", got.Meals)
	}
	archived := callTool[mealsJSON](t, session, "calories_list_meals", map[string]any{"archived": true})
	if len(archived.Meals) != 1 || archived.Meals[0].ID != gone || !archived.Meals[0].Archived {
		t.Errorf("calories_list_meals archived = %+v, want only the Melon pan", archived.Meals)
	}
}

func TestAConnectorSavesANewMealWithItsMealTags(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.connect()

	got := callTool[mealJSON](t, session, "calories_save_meal", map[string]any{
		"name": "Tuna mayo onigiri", "kcal": 230, "store": "7-Eleven", "emoji": "🍙", "tags": []string{"snack"},
	})

	want := onigiri
	want.ID = got.ID
	if got.ID == "" || !reflect.DeepEqual(got, want) {
		t.Errorf("calories_save_meal = %+v, want %+v", got, want)
	}
	if meals := get[[]mealJSON](phone, "/api/calories/meals"); !reflect.DeepEqual(meals, []mealJSON{want}) {
		t.Errorf("the web app's Meals = %+v, want %+v", meals, want)
	}
}

func TestAConnectorChangesOnlyWhatItSendsOfAMeal(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	id := phone.createMeal(onigiri)
	session := phone.connect()

	// The label says 250 kcal.
	got := callTool[mealJSON](t, session, "calories_save_meal", map[string]any{"id": id, "kcal": 250})

	want := onigiri
	want.ID, want.Kcal = id, 250
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calories_save_meal = %+v, want %+v", got, want)
	}
	if meals := get[[]mealJSON](phone, "/api/calories/meals"); !reflect.DeepEqual(meals, []mealJSON{want}) {
		t.Errorf("the web app's Meals = %+v, want %+v", meals, want)
	}
	if got := toolError(t, session, "calories_save_meal", map[string]any{"id": "6af05d493ea403a78bf5997b", "kcal": 250}); !strings.Contains(got, "no such Meal") {
		t.Errorf("saving a Meal that doesn't exist says %q, want that there's no such Meal", got)
	}
}

func TestAConnectorArchivesAMealAndRestoresIt(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	id := phone.createMeal(onigiri)
	logged := phone.mustLogEntry("2026-10-21", map[string]any{"mealId": id, "servings": 1})
	session := phone.connect()

	if got := callTool[mealJSON](t, session, "calories_archive_meal", map[string]any{"id": id}); got.ID != id || !got.Archived {
		t.Errorf("calories_archive_meal = %+v, want the Meal, archived", got)
	}
	if meals := get[[]mealJSON](phone, "/api/calories/meals"); len(meals) != 0 {
		t.Errorf("the web app's Meals in use = %+v, want none", meals)
	}
	if entries := phone.entries("2026-10-21"); len(entries) != 1 || entries[0].ID != logged {
		t.Errorf("the Day's Entries = %+v, want the one logged from the Meal kept", entries)
	}

	if got := callTool[mealJSON](t, session, "calories_restore_meal", map[string]any{"id": id}); got.ID != id || got.Archived {
		t.Errorf("calories_restore_meal = %+v, want the Meal, in use", got)
	}
	if meals := get[[]mealJSON](phone, "/api/calories/meals"); len(meals) != 1 || meals[0].ID != id {
		t.Errorf("the web app's Meals in use = %+v, want the restored one", meals)
	}
}

func TestTheConsentPageSaysWhatCaloriessToolsLetAConnectorDo(t *testing.T) {
	n := newNando(t)

	// Core's Messenger and Filing Record Tools are offered too, as these tests' Nando has bots (#128).
	got := slices.DeleteFunc(testconnector.ToolsDescriptions(t, n.handler, webOrigin), func(d string) bool {
		return strings.Contains(d, "Telegram") || strings.Contains(d, "files your Gmail")
	})

	// Deleting and archiving are said, as the Owner is asked first.
	if want := []string{"read your Days and Daily Limit, log, correct and delete what you eat, save, change and archive your Meals, " +
		"read, log and delete your weekly Weigh-ins, and set your Daily Limit and when Calories Notifications come"}; !slices.Equal(got, want) {
		t.Errorf("toolsDescriptions = %q, want %q", got, want)
	}
}

// hints is what a Tool's annotations tell the Connector.
type hints struct{ readOnly, destructive bool }

func TestEveryToolSaysWhetherItChangesOrDestroysAnything(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	listed, err := phone.connect().ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	got := map[string]hints{}
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "messenger_") || strings.HasPrefix(tool.Name, "core_") {
			continue // Core's, offered as these tests' Nando has bots (#128, #168)
		}
		if !strings.HasPrefix(tool.Name, "calories_") {
			t.Errorf("Tool %s: a Module's Tool names start with the Module's name", tool.Name)
		}
		got[tool.Name] = hints{tool.Annotations.ReadOnlyHint, tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint}
	}
	lookUp, write, destroy := hints{readOnly: true}, hints{}, hints{destructive: true}
	want := map[string]hints{
		"calories_get_day":         lookUp,
		"calories_get_daily_limit": lookUp,
		// Past Days keep the limit they had.
		"calories_set_daily_limit": write,
		"calories_get_settings":    lookUp,
		"calories_save_settings":   write,
		"calories_list_meals":      lookUp,
		"calories_log_entry":       write,
		"calories_update_entry":    write,
		"calories_save_meal":       write,
		"calories_restore_meal":    write,
		"calories_delete_entry":    destroy,
		// Archiving can be undone, but takes a Meal out of use.
		"calories_archive_meal":    destroy,
		"calories_list_weigh_ins":  lookUp,
		"calories_log_weigh_in":    write,
		"calories_delete_weigh_in": destroy,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tools and their hints =\n%v\nwant\n%v", got, want)
	}
}

func TestAConnectorLogsServingsOfASavedMealOnAnEarlierDay(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.setDailyLimit(2000)
	mealID := phone.createMeal(onigiri)
	session := phone.connect()

	// "Had 2 of the 7-Eleven onigiri yesterday."
	got := callTool[loggedJSON](t, session, "calories_log_entry", map[string]any{"day": "2026-10-20", "mealId": mealID, "servings": 2})

	want := entryJSON{ID: got.Entry.ID, MealID: &mealID, Name: "Tuna mayo onigiri", KcalPerServing: 230, Servings: 2, Kcal: 460}
	if !equalEntries([]entryJSON{got.Entry}, []entryJSON{want}) {
		t.Errorf("entry = %+v, want %+v", got.Entry, want)
	}
	if want := (caloriesJSON{Day: "2026-10-20", ConsumedCalories: ptr(460), DailyLimit: ptr(2000), Remaining: ptr(1540), OverLimit: ptr(false)}); !reflect.DeepEqual(got.Day, want) {
		t.Errorf("day = %+v, want %+v", got.Day, want)
	}
	if entries := phone.entries("2026-10-20"); !equalEntries(entries, []entryJSON{want}) {
		t.Errorf("the web app's Entries for 2026-10-20 = %+v, want %+v", entries, want)
	}
	if entries := phone.entries("2026-10-21"); len(entries) != 0 {
		t.Errorf("today's Entries = %+v, want none", entries)
	}
}
