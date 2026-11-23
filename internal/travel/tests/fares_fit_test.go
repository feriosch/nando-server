package tests

import (
	"net/http"
	"testing"
)

func TestAFareThatNoLongerFitsItsTripLeavesTheStatsPagesAndCardsUntilItFitsAgain(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	record(t, phone, "weekly",
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, nil),
		fare(trip, "owner", "2027-06-02", "2027-06-09", 130000, nil))

	// The Owner now avoids the cheaper Fare's dates.
	callTool[tripJSON](t, phone.tools(), "travel_save_trip", map[string]any{
		"id": trip.ID, "avoid": []map[string]any{{"from": "2027-05-15", "to": "2027-05-16", "reason": "a wedding"}},
	})

	s := stats(t, phone, trip, "owner")
	if s.LowestEver == nil || s.LowestEver.Total != 130000 || len(s.ByWeek) != 1 || len(s.Top) != 1 {
		t.Errorf("stats after avoiding its dates = %+v; want only the 130000 Fare", s)
	}
	page := get[tripPageJSON](phone, "/api/travel/trips/"+trip.ID, http.StatusOK)
	if got := totals(page.Parties[0].Top); len(got) != 1 || got[0] != 130000 {
		t.Errorf("the Trip page's top = %v, want [130000]", got)
	}
	if len(page.Parties[0].History) != 1 || page.Parties[0].History[0].PerPerson != 130000 {
		t.Errorf("the Trip page's history = %+v, want the 130000 Fare's Day only", page.Parties[0].History)
	}
	cards := get[struct {
		Cards []homeCardJSON `json:"cards"`
	}](phone, "/api/travel/cards", http.StatusOK)
	if len(cards.Cards) != 1 || cards.Cards[0].PerPerson == nil || *cards.Cards[0].PerPerson != 130000 {
		t.Errorf("cards = %+v, want the Trip at 130000", cards.Cards)
	}

	// The wedding is off: the Fare fits again, and counts again.
	callTool[tripJSON](t, phone.tools(), "travel_save_trip", map[string]any{"id": trip.ID, "avoid": []map[string]any{}})
	if s := stats(t, phone, trip, "owner"); s.LowestEver == nil || s.LowestEver.Total != 100000 {
		t.Errorf("lowestEver once the dates are clear again = %+v, want the 100000 Fare", s.LowestEver)
	}
}

func TestFaresOutsideTheTripsNightsWindowsOrAirportsAreNotCounted(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	record(t, phone, "weekly",
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, nil), // 7 nights, to BBB
		fare(trip, "owner", "2027-06-20", "2027-06-29", 110000, nil)) // 9 nights, to BBB

	for _, change := range []struct {
		name string
		args map[string]any
		want int
	}{
		{"nights 8–9", map[string]any{"nights": map[string]any{"min": 8, "max": 9}}, 110000},
		{"a window from June", map[string]any{"nights": map[string]any{"min": 6, "max": 9}, "windows": []map[string]any{{"from": "2027-06-01", "to": "2027-06-30"}}}, 110000},
	} {
		args := map[string]any{"id": trip.ID}
		for k, v := range change.args {
			args[k] = v
		}
		callTool[tripJSON](t, phone.tools(), "travel_save_trip", args)
		if s := stats(t, phone, trip, "owner"); s.LowestEver == nil || s.LowestEver.Total != change.want {
			t.Errorf("after %s: lowestEver = %+v, want the %d Fare", change.name, s.LowestEver, change.want)
		}
	}

	// Flying somewhere else now: no Fare goes there.
	callTool[tripJSON](t, phone.tools(), "travel_save_trip", map[string]any{"id": trip.ID, "destinations": []string{"CCC"}})
	if s := stats(t, phone, trip, "owner"); s.LowestEver != nil || len(s.Top) != 0 {
		t.Errorf("after changing the destination: stats = %+v, want none", s)
	}
	got, _ := record(t, phone, "asked", fare(trip, "owner", "2027-06-02", "2027-06-09", 90000, nil))
	if len(got.Refused) != 1 {
		t.Errorf("recording a Fare to BBB for a Trip to CCC = %+v, want it refused", got)
	}
}
