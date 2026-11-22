package tests

import (
	"reflect"
	"testing"
	"time"
)

// fareAlertJSON is a Fare Alert the Travel Assistant sent, as the Tools
// answer it.
type fareAlertJSON struct {
	ID      string   `json:"id"`
	FareID  string   `json:"fareId"`
	TripID  string   `json:"tripId"`
	Party   string   `json:"party"`
	Reasons []string `json:"reasons"`
	Urgent  bool     `json:"urgent"`
	SentAt  string   `json:"sentAt"`
}

// alerts lists the Fare Alerts for trip's Party party since since.
func alerts(t *testing.T, phone *browser, trip tripJSON, party, since string) []fareAlertJSON {
	t.Helper()
	got := callTool[struct {
		Alerts []fareAlertJSON `json:"alerts"`
	}](t, phone.tools(), "travel_list_fare_alerts", map[string]any{"tripId": trip.ID, "party": party, "since": since})
	return got.Alerts
}

func TestAFareAlertIsRecordedSoALaterRunDoesntRepeatIt(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	got, _ := record(t, phone, "daily", fare(trip, "owner", "2027-05-12", "2027-05-19", 112000, nil))
	fareID := got.Recorded[0].ID

	n.clock.Set(today.Add(10 * time.Minute))
	alert := callTool[fareAlertJSON](t, phone.tools(), "travel_record_fare_alert", map[string]any{
		"fareId": fareID, "reasons": []string{"lowest_seen", "low_verdict"}, "urgent": false,
	})

	want := fareAlertJSON{
		ID: alert.ID, FareID: fareID, TripID: trip.ID, Party: "owner",
		Reasons: []string{"lowest_seen", "low_verdict"}, SentAt: "2026-11-22T12:10:00Z",
	}
	if alert.ID == "" || !reflect.DeepEqual(alert, want) {
		t.Errorf("travel_record_fare_alert = %+v, want %+v", alert, want)
	}
	if list := alerts(t, phone, trip, "owner", "2026-11-22T00:00:00Z"); len(list) != 1 || !reflect.DeepEqual(list[0], want) {
		t.Errorf("travel_list_fare_alerts = %+v, want the one alert", list)
	}
	if list := alerts(t, phone, trip, "owner", "2026-11-23T00:00:00Z"); len(list) != 0 {
		t.Errorf("alerts since the next Day = %+v, want none", list)
	}
	for _, args := range []map[string]any{
		{"fareId": "nosuchfare", "reasons": []string{"lowest_seen"}},
		{"fareId": fareID, "reasons": []string{}},
		{"fareId": fareID, "reasons": []string{"a hunch"}},
		{"fareId": fareID, "reasons": []string{"big_drop", "big_drop"}},
	} {
		toolError(t, phone.tools(), "travel_record_fare_alert", args)
	}
}

func TestATargetFareTheOwnerAgreedShowsOnTheTripAndInTheStats(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	session := phone.tools()

	set := callTool[struct {
		TargetFares []struct {
			Party     string `json:"party"`
			PerPerson int    `json:"perPerson"`
			AgreedOn  string `json:"agreedOn"`
		} `json:"targetFares"`
	}](t, session, "travel_set_target_fare", map[string]any{"tripId": trip.ID, "party": "owner", "perPerson": 100000})
	if len(set.TargetFares) != 1 || set.TargetFares[0].Party != "owner" || set.TargetFares[0].PerPerson != 100000 || set.TargetFares[0].AgreedOn != "2026-11-22" {
		t.Errorf("the Trip's targetFares = %+v, want owner at 100000, agreed 2026-11-22", set.TargetFares)
	}
	got := callTool[struct {
		TargetFare *int `json:"targetFare"`
	}](t, session, "travel_fare_stats", map[string]any{"tripId": trip.ID, "party": "owner"})
	if got.TargetFare == nil || *got.TargetFare != 100000 {
		t.Errorf("stats' targetFare = %v, want 100000", got.TargetFare)
	}

	cleared := callTool[map[string]any](t, session, "travel_set_target_fare", map[string]any{"tripId": trip.ID, "party": "owner", "perPerson": nil})
	if tf, _ := cleared["targetFares"].([]any); len(tf) != 0 {
		t.Errorf("after clearing it, targetFares = %v, want none", cleared["targetFares"])
	}
	for _, args := range []map[string]any{
		{"tripId": trip.ID, "party": "with_companion", "perPerson": 100000},
		{"tripId": trip.ID, "party": "owner", "perPerson": 0},
		{"tripId": "nosuchtrip", "party": "owner", "perPerson": 100000},
	} {
		toolError(t, session, "travel_set_target_fare", args)
	}
}
