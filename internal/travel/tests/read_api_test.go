package tests

import (
	"net/http"
	"reflect"
	"testing"
)

// partySummaryJSON is a Party's line in the list of Trips.
type partySummaryJSON struct {
	Key           string    `json:"key"`
	Adults        int       `json:"adults"`
	Best          *fareJSON `json:"best"`
	SinceLastWeek *int      `json:"sinceLastWeek"`
	TargetFare    *int      `json:"targetFare"`
	VerdictLevel  *string   `json:"verdictLevel"`
}

// tripSummaryJSON is a Trip in the list of Trips.
type tripSummaryJSON struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Status  string             `json:"status"`
	Parties []partySummaryJSON `json:"parties"`
}

// dayFareJSON is a point of a Party's history: the best per person found
// that Day.
type dayFareJSON struct {
	Day       string `json:"day"`
	PerPerson int    `json:"perPerson"`
}

// partyPageJSON is a Party on a Trip's page.
type partyPageJSON struct {
	Key     string        `json:"key"`
	Adults  int           `json:"adults"`
	History []dayFareJSON `json:"history"`
	ByWeek  []struct {
		Week string   `json:"week"`
		Fare fareJSON `json:"fare"`
	} `json:"byWeek"`
	Top              []fareJSON         `json:"top"`
	BestSingleTicket *fareJSON          `json:"bestSingleTicket"`
	Verdicts         []priceVerdictJSON `json:"verdicts"`
	TargetFare       *int               `json:"targetFare"`
}

// tripPageJSON is a Trip's page.
type tripPageJSON struct {
	Trip    tripJSON        `json:"trip"`
	Parties []partyPageJSON `json:"parties"`
}

// homeCardJSON is a Watching Trip's Home Card.
type homeCardJSON struct {
	TripID    string  `json:"tripId"`
	Name      string  `json:"name"`
	Party     *string `json:"party"`
	PerPerson *int    `json:"perPerson"`
	Trend     *string `json:"trend"`
}

func TestTheWebAppReadsTripsWithTheirBestFaresWatchingFirst(t *testing.T) {
	n, phone := signedInBrowser(t)
	beach := newTrip(t, phone, map[string]any{"name": "Beach week"})
	city := newTrip(t, phone, map[string]any{"name": "Alps"})
	callTool[tripJSON](t, phone.tools(), "travel_set_trip_status", map[string]any{"id": city.ID, "status": "booked"})
	record(t, phone, "weekly", fare(beach, "owner", "2027-05-12", "2027-05-19", 130000, nil))
	n.clock.Set(today.AddDate(0, 0, 7))
	record(t, phone, "weekly", fare(beach, "owner", "2027-05-20", "2027-05-27", 120000, nil))
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(beach, "owner", "2027-05-20", "2027-05-27"))
	callTool[map[string]any](t, phone.tools(), "travel_set_target_fare", map[string]any{"tripId": beach.ID, "party": "owner", "perPerson": 110000})

	got := get[struct {
		Trips []tripSummaryJSON `json:"trips"`
	}](phone, "/api/travel/trips", http.StatusOK)

	if len(got.Trips) != 2 || got.Trips[0].Name != "Beach week" || got.Trips[1].Name != "Alps" || got.Trips[1].Status != "booked" {
		t.Fatalf("trips = %+v; want Beach week, Watching, then Alps, booked", got.Trips)
	}
	p := got.Trips[0].Parties
	if len(p) != 1 || p[0].Key != "owner" || p[0].Best == nil || p[0].Best.PerPerson != 120000 ||
		p[0].SinceLastWeek == nil || *p[0].SinceLastWeek != -10000 ||
		p[0].TargetFare == nil || *p[0].TargetFare != 110000 || p[0].VerdictLevel == nil || *p[0].VerdictLevel != "low" {
		t.Errorf("Beach week's parties = %+v; want owner: best 120000, -10000 since last week, target 110000, verdict low", p)
	}
	if alps := got.Trips[1].Parties; len(alps) != 1 || alps[0].Best != nil || alps[0].SinceLastWeek != nil {
		t.Errorf("Alps' parties = %+v; want owner with no Fares", alps)
	}
}

func TestATripsPageHasItsHistoryWeeksTopFaresAndVerdicts(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	record(t, phone, "weekly",
		fare(trip, "owner", "2027-05-12", "2027-05-19", 130000, nil),
		fare(trip, "owner", "2027-05-20", "2027-05-27", 110000, map[string]any{"separateTickets": true}))
	n.clock.Set(today.AddDate(0, 0, 1))
	record(t, phone, "daily", fare(trip, "owner", "2027-05-13", "2027-05-20", 125000, nil))
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-13", "2027-05-20"))

	got := get[tripPageJSON](phone, "/api/travel/trips/"+trip.ID, http.StatusOK)

	if got.Trip.ID != trip.ID || len(got.Parties) != 1 {
		t.Fatalf("page = %+v; want the Trip and its one Party", got)
	}
	p := got.Parties[0]
	if want := []dayFareJSON{{"2026-11-22", 110000}, {"2026-11-23", 125000}}; !reflect.DeepEqual(p.History, want) {
		t.Errorf("history = %+v, want %+v: the best per person each Day", p.History, want)
	}
	if len(p.ByWeek) != 2 || p.ByWeek[0].Week != "2027-05-10" || p.ByWeek[0].Fare.Total != 125000 || p.ByWeek[1].Week != "2027-05-17" {
		t.Errorf("byWeek = %+v; want 2027-05-10 at 125000, then 2027-05-17", p.ByWeek)
	}
	if got := totals(p.Top); !reflect.DeepEqual(got, []int{110000, 125000, 130000}) {
		t.Errorf("top = %v, want [110000 125000 130000]", got)
	}
	// The cheapest is on Separate Tickets, so the best on one ticket comes with it.
	if p.BestSingleTicket == nil || p.BestSingleTicket.Total != 125000 {
		t.Errorf("bestSingleTicket = %+v, want the 125000 Fare", p.BestSingleTicket)
	}
	if len(p.Verdicts) != 1 || p.Verdicts[0].Level != "low" {
		t.Errorf("verdicts = %+v, want the one low verdict", p.Verdicts)
	}

	get[tripPageJSON](phone, "/api/travel/trips/nosuchtrip", http.StatusNotFound)
}

func TestEachWatchingTripHasAHomeCardWithItsBestFareAndTrend(t *testing.T) {
	n, phone := signedInBrowser(t)
	beach := newTrip(t, phone, map[string]any{"name": "Beach week"})
	quiet := newTrip(t, phone, map[string]any{"name": "Quiet one"})
	booked := newTrip(t, phone, map[string]any{"name": "Booked one"})
	callTool[tripJSON](t, phone.tools(), "travel_set_trip_status", map[string]any{"id": booked.ID, "status": "booked"})
	record(t, phone, "weekly", fare(beach, "owner", "2027-05-12", "2027-05-19", 120000, nil))
	n.clock.Set(today.AddDate(0, 0, 7))
	record(t, phone, "weekly", fare(beach, "owner", "2027-05-20", "2027-05-27", 130000, nil))

	got := get[struct {
		Cards []homeCardJSON `json:"cards"`
	}](phone, "/api/travel/cards", http.StatusOK)

	if len(got.Cards) != 2 {
		t.Fatalf("cards = %+v; want one per Watching Trip", got.Cards)
	}
	c := got.Cards[0]
	if c.TripID != beach.ID || c.Party == nil || *c.Party != "owner" || c.PerPerson == nil || *c.PerPerson != 130000 || c.Trend == nil || *c.Trend != "up" {
		t.Errorf("Beach week's card = %+v; want owner at 130000 now, trend up from last week's 120000", c)
	}
	if q := got.Cards[1]; q.TripID != quiet.ID || q.PerPerson != nil || q.Trend != nil {
		t.Errorf("Quiet one's card = %+v; want no Fare yet", q)
	}
}

func TestTheWebAppReadsTravelsSettingsAndNeedsASession(t *testing.T) {
	_, phone := signedInBrowser(t)
	callTool[settingsJSON](t, phone.tools(), "travel_save_settings", map[string]any{"companion": map[string]any{"name": "Sam"}})

	got := get[settingsJSON](phone, "/api/travel/settings", http.StatusOK)
	if got.Companion.Name != "Sam" || got.SerpAPIMonthlyLimit != 250 {
		t.Errorf("settings = %+v; want the Companion Sam and the limit 250", got)
	}

	stranger := &browser{nando: phone.nando}
	for _, path := range []string{"/api/travel/trips", "/api/travel/trips/x", "/api/travel/cards", "/api/travel/settings"} {
		get[struct{}](stranger, path, http.StatusUnauthorized)
	}
}
