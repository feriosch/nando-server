package tests

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// nightsJSON is how long a Trip's stay may be.
type nightsJSON struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// windowJSON is a range of departure dates a Trip may fall in.
type windowJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
}

// avoidJSON is a range of dates a Trip must not touch.
type avoidJSON struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// partyJSON is who a Trip is priced for.
type partyJSON struct {
	Key    string `json:"key"`
	Adults int    `json:"adults"`
}

// tripJSON is a Trip as the Tools read it.
type tripJSON struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Origins         []string     `json:"origins"`
	Destinations    []string     `json:"destinations"`
	Nights          nightsJSON   `json:"nights"`
	Windows         []windowJSON `json:"windows"`
	Avoid           []avoidJSON  `json:"avoid"`
	Cabin           string       `json:"cabin"`
	CheckedBags     int          `json:"checkedBags"`
	MaxStops        int          `json:"maxStops"`
	SeparateTickets bool         `json:"separateTickets"`
	Parties         []partyJSON  `json:"parties"`
	Status          string       `json:"status"`
	StatusChangedOn string       `json:"statusChangedOn"`
	Notes           string       `json:"notes"`
}

// beachTrip is the least a new Trip needs: a name, where to, how long and
// when, made up for tests.
var beachTrip = map[string]any{
	"name":         "Beach week",
	"destinations": []string{"BBB"},
	"nights":       map[string]any{"min": 6, "max": 9},
	"windows":      []map[string]any{{"from": "2027-05-10", "to": "2027-06-30"}},
}

func TestANewTripLeavesFromHomeAndIsWatchedForTheOwnerAlone(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"home": []string{"AAA"}})

	got := callTool[tripJSON](t, session, "travel_save_trip", beachTrip)

	if got.ID == "" {
		t.Fatalf("travel_save_trip answered %+v, without an id", got)
	}
	want := tripJSON{
		ID:              got.ID,
		Name:            "Beach week",
		Origins:         []string{"AAA"},
		Destinations:    []string{"BBB"},
		Nights:          nightsJSON{Min: 6, Max: 9},
		Windows:         []windowJSON{{From: "2027-05-10", To: "2027-06-30"}},
		Avoid:           []avoidJSON{},
		Cabin:           "economy",
		CheckedBags:     1,
		MaxStops:        1,
		SeparateTickets: true,
		Parties:         []partyJSON{{Key: "owner", Adults: 1}},
		Status:          "watching",
		StatusChangedOn: "2026-11-22",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("new Trip = %+v, want %+v", got, want)
	}
	if read := callTool[tripJSON](t, session, "travel_get_trip", map[string]any{"id": got.ID}); !reflect.DeepEqual(read, want) {
		t.Errorf("travel_get_trip = %+v, want %+v", read, want)
	}
}

// newTrip saves beachTrip, leaving from home "AAA", with the fields of more
// over it, and answers it.
func newTrip(t *testing.T, phone *browser, more map[string]any) tripJSON {
	t.Helper()
	session := phone.tools()
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"home": []string{"AAA"}})
	args := map[string]any{}
	for k, v := range beachTrip {
		args[k] = v
	}
	for k, v := range more {
		args[k] = v
	}
	return callTool[tripJSON](t, session, "travel_save_trip", args)
}

func TestSavingATripWithItsIDChangesOnlyTheFieldsSent(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	session := phone.tools()
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"companion": map[string]any{"name": "Sam"}})

	got := callTool[tripJSON](t, session, "travel_save_trip", map[string]any{
		"id":              trip.ID,
		"origins":         []string{"AAA", "AAC"},
		"avoid":           []map[string]any{{"from": "2027-05-29", "to": "2027-06-06", "reason": "a wedding"}},
		"cabin":           "premium_economy",
		"checkedBags":     2,
		"maxStops":        0,
		"separateTickets": false,
		"parties":         []map[string]any{{"key": "owner", "adults": 1}, {"key": "with_companion", "adults": 2}},
		"notes":           "nonstops only from AAA",
	})

	want := trip
	want.Origins = []string{"AAA", "AAC"}
	want.Avoid = []avoidJSON{{From: "2027-05-29", To: "2027-06-06", Reason: "a wedding"}}
	want.Cabin = "premium_economy"
	want.CheckedBags = 2
	want.MaxStops = 0
	want.SeparateTickets = false
	want.Parties = []partyJSON{{Key: "owner", Adults: 1}, {Key: "with_companion", Adults: 2}}
	want.Notes = "nonstops only from AAA"
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after changing it: %+v, want %+v", got, want)
	}

	got = callTool[tripJSON](t, session, "travel_save_trip", map[string]any{
		"id": trip.ID, "nights": map[string]any{"min": 7, "max": 8}, "name": "Beach, longer",
	})
	want.Nights = nightsJSON{Min: 7, Max: 8}
	want.Name = "Beach, longer"
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after changing the name and nights: %+v, want %+v", got, want)
	}
	if read := callTool[tripJSON](t, session, "travel_get_trip", map[string]any{"id": trip.ID}); !reflect.DeepEqual(read, want) {
		t.Errorf("travel_get_trip = %+v, want %+v", read, want)
	}
}

// names are the names of trips, in order.
func names(trips []tripJSON) []string {
	got := []string{}
	for _, trip := range trips {
		got = append(got, trip.Name)
	}
	return got
}

func TestOnlyWatchingTripsAreListedUnlessAnotherStatusIsAsked(t *testing.T) {
	n, phone := signedInBrowser(t)
	beach := newTrip(t, phone, map[string]any{"name": "Beach week"})
	city := newTrip(t, phone, map[string]any{"name": "City break"})
	family := newTrip(t, phone, map[string]any{"name": "Family visit"})

	// A fresh Connector session: an earlier one's access token wouldn't
	// outlive moving the clock on by Days.
	n.clock.Set(today.AddDate(0, 0, 3))
	session := phone.tools()
	booked := callTool[tripJSON](t, session, "travel_set_trip_status", map[string]any{"id": city.ID, "status": "booked"})
	if booked.Status != "booked" || booked.StatusChangedOn != "2026-11-25" {
		t.Errorf("after booking it: status %q since %q, want booked since 2026-11-25", booked.Status, booked.StatusChangedOn)
	}
	callTool[tripJSON](t, session, "travel_set_trip_status", map[string]any{"id": family.ID, "status": "dropped"})

	for _, tc := range []struct {
		args map[string]any
		want []string
	}{
		{nil, []string{"Beach week"}},
		{map[string]any{"status": "watching"}, []string{"Beach week"}},
		{map[string]any{"status": "booked"}, []string{"City break"}},
		{map[string]any{"status": "dropped"}, []string{"Family visit"}},
		{map[string]any{"status": "all"}, []string{"Beach week", "City break", "Family visit"}},
	} {
		got := callTool[struct {
			Trips []tripJSON `json:"trips"`
		}](t, session, "travel_list_trips", tc.args)
		if !reflect.DeepEqual(names(got.Trips), tc.want) {
			t.Errorf("travel_list_trips %v = %v, want %v", tc.args, names(got.Trips), tc.want)
		}
	}

	// A Booked or Dropped Trip can be watched again.
	again := callTool[tripJSON](t, session, "travel_set_trip_status", map[string]any{"id": family.ID, "status": "watching"})
	if again.Status != "watching" {
		t.Errorf("after watching it again: status %q, want watching", again.Status)
	}
	if beach.Status != "watching" {
		t.Errorf("a new Trip's status = %q, want watching", beach.Status)
	}
}

func TestDeletingATripRemovesItForGood(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	session := phone.tools()

	callTool[map[string]any](t, session, "travel_delete_trip", map[string]any{"id": trip.ID})

	toolError(t, session, "travel_get_trip", map[string]any{"id": trip.ID})
	got := callTool[struct {
		Trips []tripJSON `json:"trips"`
	}](t, session, "travel_list_trips", map[string]any{"status": "all"})
	if len(got.Trips) != 0 {
		t.Errorf("travel_list_trips after deleting the only Trip = %v, want none", names(got.Trips))
	}
	toolError(t, session, "travel_delete_trip", map[string]any{"id": trip.ID})
}

func TestOnlyDeletingATripIsDestructiveAndOnlyReadsAreReadOnly(t *testing.T) {
	_, phone := signedInBrowser(t)

	tools, err := phone.tools().ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := map[string][2]bool{ // readOnly, destructive
		"travel_get_settings": {true, false}, "travel_save_settings": {false, false},
		"travel_list_trips": {true, false}, "travel_get_trip": {true, false},
		"travel_save_trip": {false, false}, "travel_set_trip_status": {false, false},
		"travel_delete_trip":      {false, true},
		"travel_start_fare_check": {false, false}, "travel_record_fares": {false, false},
		"travel_finish_fare_check": {false, false}, "travel_fare_stats": {true, false},
		"travel_get_price_verdict": {false, false}, "travel_list_price_verdicts": {true, false},
		"travel_record_fare_alert": {false, false}, "travel_list_fare_alerts": {true, false},
		"travel_set_target_fare": {false, false},
	}
	seen := 0
	for _, tool := range tools.Tools {
		w, ok := want[tool.Name]
		if !ok {
			continue
		}
		seen++
		destructive := tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		if tool.Annotations.ReadOnlyHint != w[0] || (!w[0] && destructive != w[1]) {
			t.Errorf("%s: readOnly %v, destructive %v; want %v, %v", tool.Name, tool.Annotations.ReadOnlyHint, destructive, w[0], w[1])
		}
	}
	if seen != len(want) {
		t.Errorf("found %d of the %d Travel Tools", seen, len(want))
	}
}

// with is beachTrip with the fields of more over it.
func with(more map[string]any) map[string]any {
	args := map[string]any{}
	for k, v := range beachTrip {
		args[k] = v
	}
	for k, v := range more {
		args[k] = v
	}
	return args
}

// without is beachTrip without field.
func without(field string) map[string]any {
	args := with(nil)
	delete(args, field)
	return args
}

func TestATripThatCantBeWatchedIsRefusedAndChangesNothing(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil) // sets home to AAA
	session := phone.tools()

	window := func(from, to string) []map[string]any { return []map[string]any{{"from": from, "to": to}} }
	party := func(key string, adults int) map[string]any { return map[string]any{"key": key, "adults": adults} }
	for _, args := range []map[string]any{
		without("name"),
		without("destinations"),
		without("nights"),
		without("windows"),
		with(map[string]any{"name": "  "}),
		with(map[string]any{"name": strings.Repeat("x", 101)}),
		with(map[string]any{"destinations": []string{}}),
		with(map[string]any{"destinations": []string{"BB"}}),
		with(map[string]any{"origins": []string{"AAA", "AAA"}}),
		with(map[string]any{"origins": []string{}}),
		with(map[string]any{"nights": map[string]any{"min": 0, "max": 5}}),
		with(map[string]any{"nights": map[string]any{"min": 8, "max": 7}}),
		with(map[string]any{"nights": map[string]any{"min": 1, "max": 61}}),
		with(map[string]any{"windows": []map[string]any{}}),
		with(map[string]any{"windows": window("2027-06-30", "2027-05-10")}),
		with(map[string]any{"windows": window("2026-11-01", "2026-11-21")}), // over before today
		with(map[string]any{"windows": window("2027-5-10", "2027-06-30")}),
		with(map[string]any{"windows": window("2027-02-30", "2027-03-10")}),
		with(map[string]any{"avoid": []map[string]any{{"from": "2027-06-06", "to": "2027-05-29"}}}),
		with(map[string]any{"avoid": []map[string]any{{"from": "2027-05-01", "to": "2027-07-01"}}}), // covers the whole window
		with(map[string]any{"avoid": []map[string]any{ // together cover the whole window
			{"from": "2027-06-01", "to": "2027-07-01"}, {"from": "2027-05-01", "to": "2027-05-31"},
		}}),
		with(map[string]any{"cabin": "steerage"}),
		with(map[string]any{"checkedBags": 3}),
		with(map[string]any{"checkedBags": -1}),
		with(map[string]any{"maxStops": 4}),
		with(map[string]any{"parties": []map[string]any{}}),
		with(map[string]any{"parties": []map[string]any{party("owner", 1), party("owner", 1)}}),
		with(map[string]any{"parties": []map[string]any{party("owner", 2)}}),
		with(map[string]any{"parties": []map[string]any{party("friends", 3)}}),
		with(map[string]any{"parties": []map[string]any{party("with_companion", 2)}}), // no Companion set
		with(map[string]any{"nights": "a week"}),
		{"id": "nosuchtrip", "name": "Renamed"},
		{"id": trip.ID, "windows": window("2027-06-30", "2027-05-10")},
		{"id": trip.ID, "name": "Renamed", "maxStops": 9},
	} {
		toolError(t, session, "travel_save_trip", args)
	}
	toolError(t, session, "travel_set_trip_status", map[string]any{"id": trip.ID, "status": "gone"})
	toolError(t, session, "travel_set_trip_status", map[string]any{"id": "nosuchtrip", "status": "booked"})
	toolError(t, session, "travel_list_trips", map[string]any{"status": "someday"})
	toolError(t, session, "travel_get_trip", map[string]any{})

	got := callTool[struct {
		Trips []tripJSON `json:"trips"`
	}](t, session, "travel_list_trips", map[string]any{"status": "all"})
	if len(got.Trips) != 1 || !reflect.DeepEqual(got.Trips[0], trip) {
		t.Errorf("after only refused saves, Trips = %+v; want only %+v, unchanged", got.Trips, trip)
	}
}

func TestATripWithTheCompanionNeedsTwoAdultsAndACompanion(t *testing.T) {
	_, phone := signedInBrowser(t)
	newTrip(t, phone, nil)
	session := phone.tools()
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"companion": map[string]any{"name": "Sam"}})

	toolError(t, session, "travel_save_trip", with(map[string]any{"parties": []map[string]any{{"key": "with_companion", "adults": 1}}}))
	got := callTool[tripJSON](t, session, "travel_save_trip", with(map[string]any{
		"parties": []map[string]any{{"key": "with_companion", "adults": 2}},
	}))
	if want := []partyJSON{{Key: "with_companion", Adults: 2}}; !reflect.DeepEqual(got.Parties, want) {
		t.Errorf("parties = %+v, want %+v", got.Parties, want)
	}
}

func TestANewTripNeedsAnOriginWhenThereIsNoHome(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	toolError(t, session, "travel_save_trip", beachTrip)
	got := callTool[tripJSON](t, session, "travel_save_trip", with(map[string]any{"origins": []string{"AAC"}}))
	if want := []string{"AAC"}; !reflect.DeepEqual(got.Origins, want) {
		t.Errorf("origins = %v, want %v", got.Origins, want)
	}
}

func TestALowercaseTripNameSortsAmongTheOthers(t *testing.T) {
	_, phone := signedInBrowser(t)
	newTrip(t, phone, map[string]any{"name": "City break"})
	newTrip(t, phone, map[string]any{"name": "beach week"})
	newTrip(t, phone, map[string]any{"name": "Alps"})

	got := callTool[struct {
		Trips []tripJSON `json:"trips"`
	}](t, phone.tools(), "travel_list_trips", nil)
	if want := []string{"Alps", "beach week", "City break"}; !reflect.DeepEqual(names(got.Trips), want) {
		t.Errorf("travel_list_trips = %v, want %v", names(got.Trips), want)
	}
}

func TestATripWithTheCompanionCanStillBeChangedOnceThereIsNoCompanion(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"companion": map[string]any{"name": "Sam"}})
	trip := newTrip(t, phone, map[string]any{"parties": []map[string]any{{"key": "with_companion", "adults": 2}}})
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"companion": map[string]any{"name": ""}})

	got := callTool[tripJSON](t, session, "travel_save_trip", map[string]any{"id": trip.ID, "notes": "check again in May"})
	if got.Notes != "check again in May" {
		t.Errorf("notes = %q, want the new ones", got.Notes)
	}
	toolError(t, session, "travel_save_trip", map[string]any{
		"id": trip.ID, "parties": []map[string]any{{"key": "with_companion", "adults": 3}},
	})
}

func TestATripWhoseWindowsAreOverCantBeWatchedAgain(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, map[string]any{"windows": []map[string]any{{"from": "2026-12-01", "to": "2026-12-10"}}})
	callTool[tripJSON](t, phone.tools(), "travel_set_trip_status", map[string]any{"id": trip.ID, "status": "dropped"})

	n.clock.Set(today.AddDate(0, 1, 0))
	session := phone.tools()
	toolError(t, session, "travel_set_trip_status", map[string]any{"id": trip.ID, "status": "watching"})
	callTool[tripJSON](t, session, "travel_save_trip", map[string]any{
		"id": trip.ID, "windows": []map[string]any{{"from": "2027-02-01", "to": "2027-02-10"}},
	})
	if got := callTool[tripJSON](t, session, "travel_set_trip_status", map[string]any{"id": trip.ID, "status": "watching"}); got.Status != "watching" {
		t.Errorf("after giving it a window ahead: status %q, want watching", got.Status)
	}
}
