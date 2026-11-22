package tests

import (
	"reflect"
	"testing"
)

// companionJSON is who travels with the Owner in a Party, and what Nando
// needs to know of their calendar.
type companionJSON struct {
	Name             string   `json:"name"`
	Calendars        []string `json:"calendars"`
	WorkWeekdays     []string `json:"workWeekdays"`
	HolidayCalendar  string   `json:"holidayCalendar"`
	UsualLeaveDays   int      `json:"usualLeaveDays"`
	RemoteDaysAbroad int      `json:"remoteDaysAbroad"`
}

// settingsJSON is Travel's settings, as the Tools read them.
type settingsJSON struct {
	Home                []string      `json:"home"`
	Companion           companionJSON `json:"companion"`
	SerpAPIMonthlyLimit int           `json:"serpApiMonthlyLimit"`
}

// defaultSettings are Travel's settings until the Owner first changes them:
// no home or Companion yet, a Monday-to-Friday work week, and SerpApi's free
// 250 searches a month.
var defaultSettings = settingsJSON{
	Home: []string{},
	Companion: companionJSON{
		Calendars:        []string{},
		WorkWeekdays:     []string{"monday", "tuesday", "wednesday", "thursday", "friday"},
		UsualLeaveDays:   7,
		RemoteDaysAbroad: 2,
	},
	SerpAPIMonthlyLimit: 250,
}

func TestTravelSettingsStartAtTheirDefaults(t *testing.T) {
	_, phone := signedInBrowser(t)

	got := callTool[settingsJSON](t, phone.tools(), "travel_get_settings", nil)

	if !reflect.DeepEqual(got, defaultSettings) {
		t.Errorf("travel_get_settings = %+v, want the defaults %+v", got, defaultSettings)
	}
}

func TestTheAssistantChangesOnlyTheTravelSettingsItSends(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	got := callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{
		"home":      []string{"AAA", "AAB"},
		"companion": map[string]any{"name": "Sam", "calendars": []string{"sam@example.com"}},
	})
	want := defaultSettings
	want.Home = []string{"AAA", "AAB"}
	want.Companion.Name = "Sam"
	want.Companion.Calendars = []string{"sam@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after setting home and the Companion: %+v, want %+v", got, want)
	}

	// Within the Companion, only the fields sent change.
	got = callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{
		"companion":           map[string]any{"workWeekdays": []string{"monday", "tuesday", "wednesday", "thursday"}, "remoteDaysAbroad": 1},
		"serpApiMonthlyLimit": 100,
	})
	want.Companion.WorkWeekdays = []string{"monday", "tuesday", "wednesday", "thursday"}
	want.Companion.RemoteDaysAbroad = 1
	want.SerpAPIMonthlyLimit = 100
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after changing the work week, remote days and limit: %+v, want %+v", got, want)
	}
	if got := callTool[settingsJSON](t, session, "travel_get_settings", nil); !reflect.DeepEqual(got, want) {
		t.Errorf("travel_get_settings = %+v, want what was saved %+v", got, want)
	}
}

func TestInvalidTravelSettingsAreRefusedAndChangeNothing(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	for _, args := range []map[string]any{
		{},
		{"home": []string{"AA"}},
		{"home": []string{"aaa"}},
		{"home": []string{"AAA", "AAA"}},
		{"companion": map[string]any{}},
		{"companion": map[string]any{"workWeekdays": []string{"funday"}}},
		{"companion": map[string]any{"workWeekdays": []string{"Monday"}}},
		{"companion": map[string]any{"workWeekdays": []string{"monday", "monday"}}},
		{"companion": map[string]any{"calendars": []string{""}}},
		{"companion": map[string]any{"usualLeaveDays": -1}},
		{"companion": map[string]any{"usualLeaveDays": 31}},
		{"companion": map[string]any{"remoteDaysAbroad": 3}},
		{"companion": map[string]any{"remoteDaysAbroad": -1}},
		{"serpApiMonthlyLimit": -1},
		{"serpApiMonthlyLimit": 10001},
		{"serpApiMonthlyLimit": "lots"},
		// A good field doesn't save when another is refused.
		{"home": []string{"AAA"}, "companion": map[string]any{"remoteDaysAbroad": 5}},
	} {
		toolError(t, session, "travel_save_settings", args)
	}

	if got := callTool[settingsJSON](t, session, "travel_get_settings", nil); !reflect.DeepEqual(got, defaultSettings) {
		t.Errorf("after only refused saves: %+v, want the defaults %+v", got, defaultSettings)
	}
}
