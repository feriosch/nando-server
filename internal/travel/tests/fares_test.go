package tests

import (
	"reflect"
	"testing"
	"time"
)

// flightJSON is one flight of a Fare's way.
type flightJSON struct {
	Carrier string `json:"carrier"`
	Number  string `json:"number"`
	From    string `json:"from"`
	To      string `json:"to"`
	Departs string `json:"departs"`
	Arrives string `json:"arrives"`
}

// legJSON is one way of a Fare.
type legJSON struct {
	Flights         []flightJSON `json:"flights"`
	Stops           int          `json:"stops"`
	DurationMinutes int          `json:"durationMinutes"`
}

// leaveNeededJSON is the Companion's days off for a Fare.
type leaveNeededJSON struct {
	Days           int `json:"days"`
	WithRemoteDays int `json:"withRemoteDays"`
}

// fareJSON is a Fare as the Tools answer it.
type fareJSON struct {
	ID              string           `json:"id"`
	TripID          string           `json:"tripId"`
	Party           string           `json:"party"`
	CheckID         string           `json:"checkId"`
	FoundAt         string           `json:"foundAt"`
	Source          string           `json:"source"`
	DepartDate      string           `json:"departDate"`
	ReturnDate      string           `json:"returnDate"`
	Nights          int              `json:"nights"`
	Outbound        legJSON          `json:"outbound"`
	Inbound         legJSON          `json:"inbound"`
	SeparateTickets bool             `json:"separateTickets"`
	Cabin           string           `json:"cabin"`
	BagsIncluded    bool             `json:"bagsIncluded"`
	Currency        string           `json:"currency"`
	Total           int              `json:"total"`
	PerPerson       int              `json:"perPerson"`
	BookingURL      string           `json:"bookingUrl"`
	LeaveNeeded     *leaveNeededJSON `json:"leaveNeeded"`
}

// fareCheckStartJSON is what travel_start_fare_check answers.
type fareCheckStartJSON struct {
	ID           string            `json:"id"`
	LastFinished map[string]string `json:"lastFinished"`
}

// recordedJSON is what travel_record_fares answers: the Fares it kept, and
// the ones it refused, by their place in the list sent.
type recordedJSON struct {
	Recorded []struct {
		Index int    `json:"index"`
		ID    string `json:"id"`
	} `json:"recorded"`
	Refused []struct {
		Index  int    `json:"index"`
		Reason string `json:"reason"`
	} `json:"refused"`
}

// statsJSON is what travel_fare_stats answers for a Trip and Party.
type statsJSON struct {
	LowestEver   *fareJSON `json:"lowestEver"`
	BestThisWeek *fareJSON `json:"bestThisWeek"`
	BestLastWeek *fareJSON `json:"bestLastWeek"`
	ByWeek       []struct {
		Week string   `json:"week"`
		Fare fareJSON `json:"fare"`
	} `json:"byWeek"`
	Top []fareJSON `json:"top"`
}

// nonstop is a made-up nonstop way between AAA and BBB on day.
func nonstop(from, to, day string) map[string]any {
	return map[string]any{
		"flights":         []map[string]any{{"carrier": "XX", "number": "XX1", "from": from, "to": to, "departs": day + "T10:00", "arrives": day + "T22:00"}},
		"stops":           0,
		"durationMinutes": 480,
	}
}

// fare is a Fare for trip's Party party, leaving on depart and back on back,
// at total for everyone, with the fields of more over it.
func fare(trip tripJSON, party, depart, back string, total int, more map[string]any) map[string]any {
	f := map[string]any{
		"tripId": trip.ID, "party": party, "source": "kiwi",
		"departDate": depart, "returnDate": back,
		"outbound": nonstop("AAA", "BBB", depart), "inbound": nonstop("BBB", "AAA", back),
		"cabin": "economy", "bagsIncluded": true, "total": total,
		"bookingUrl": "https://fares.example.com/1",
	}
	for k, v := range more {
		f[k] = v
	}
	return f
}

// record starts a Fare Check of kind, records fares in it, and answers what
// was recorded and the check's id.
func record(t *testing.T, phone *browser, kind string, fares ...map[string]any) (recordedJSON, string) {
	t.Helper()
	session := phone.tools()
	check := callTool[fareCheckStartJSON](t, session, "travel_start_fare_check", map[string]any{"kind": kind})
	got := callTool[recordedJSON](t, session, "travel_record_fares", map[string]any{"checkId": check.ID, "fares": fares})
	return got, check.ID
}

// stats is travel_fare_stats for trip's Party party.
func stats(t *testing.T, phone *browser, trip tripJSON, party string) statsJSON {
	t.Helper()
	return callTool[statsJSON](t, phone.tools(), "travel_fare_stats", map[string]any{"tripId": trip.ID, "party": party})
}

func TestAFareRecordedInAFareCheckIsTheLowestSeen(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)

	got, checkID := record(t, phone, "weekly", fare(trip, "owner", "2027-05-12", "2027-05-19", 112000, nil))

	if len(got.Recorded) != 1 || len(got.Refused) != 0 || got.Recorded[0].Index != 0 {
		t.Fatalf("travel_record_fares = %+v; want the one Fare recorded", got)
	}
	want := fareJSON{
		ID: got.Recorded[0].ID, TripID: trip.ID, Party: "owner", CheckID: checkID,
		FoundAt: "2026-11-22T12:00:00Z", Source: "kiwi",
		DepartDate: "2027-05-12", ReturnDate: "2027-05-19", Nights: 7,
		Outbound: legJSON{Flights: []flightJSON{{Carrier: "XX", Number: "XX1", From: "AAA", To: "BBB", Departs: "2027-05-12T10:00", Arrives: "2027-05-12T22:00"}}, DurationMinutes: 480},
		Inbound:  legJSON{Flights: []flightJSON{{Carrier: "XX", Number: "XX1", From: "BBB", To: "AAA", Departs: "2027-05-19T10:00", Arrives: "2027-05-19T22:00"}}, DurationMinutes: 480},
		Cabin:    "economy", BagsIncluded: true, Currency: "JPY", Total: 112000, PerPerson: 112000,
		BookingURL: "https://fares.example.com/1",
	}
	s := stats(t, phone, trip, "owner")
	if s.LowestEver == nil || !reflect.DeepEqual(*s.LowestEver, want) {
		t.Errorf("lowestEver = %+v, want %+v", s.LowestEver, want)
	}
}

// fareCheckJSON is a Fare Check as travel_finish_fare_check answers it.
type fareCheckJSON struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	StartedAt  string   `json:"startedAt"`
	FinishedAt *string  `json:"finishedAt"`
	Sources    []string `json:"sources"`
	Summary    string   `json:"summary"`
	Errors     []string `json:"errors"`
}

func TestAFinishedFareCheckIsWhenTheNextOneStartsFrom(t *testing.T) {
	n, phone := signedInBrowser(t)
	session := phone.tools()

	first := callTool[fareCheckStartJSON](t, session, "travel_start_fare_check", map[string]any{"kind": "daily"})
	if want := map[string]string{}; len(first.LastFinished) != 3 || first.LastFinished["daily"] != "" {
		t.Errorf("before any Fare Check, lastFinished = %v, want daily, weekly and asked all null %v", first.LastFinished, want)
	}
	n.clock.Set(today.Add(20 * time.Minute))
	got := callTool[fareCheckJSON](t, session, "travel_finish_fare_check", map[string]any{
		"id": first.ID, "sources": []string{"kiwi", "slicktrip"}, "summary": "2 new lows", "errors": []string{"expedia timed out"},
	})
	finished := "2026-11-22T12:20:00Z"
	want := fareCheckJSON{
		ID: first.ID, Kind: "daily", StartedAt: "2026-11-22T12:00:00Z", FinishedAt: &finished,
		Sources: []string{"kiwi", "slicktrip"}, Summary: "2 new lows", Errors: []string{"expedia timed out"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("travel_finish_fare_check = %+v, want %+v", got, want)
	}
	toolError(t, session, "travel_finish_fare_check", map[string]any{"id": first.ID, "summary": "again"})

	// An unfinished check doesn't count.
	callTool[fareCheckStartJSON](t, session, "travel_start_fare_check", map[string]any{"kind": "daily"})
	next := callTool[fareCheckStartJSON](t, session, "travel_start_fare_check", map[string]any{"kind": "weekly"})
	if next.LastFinished["daily"] != finished || next.LastFinished["weekly"] != "" || next.LastFinished["asked"] != "" {
		t.Errorf("lastFinished = %v, want daily %s and the others null", next.LastFinished, finished)
	}
	toolError(t, session, "travel_start_fare_check", map[string]any{"kind": "hourly"})
}

func TestEachFareThatDoesntFitItsTripIsRefusedWithWhyAndTheRestAreKept(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, map[string]any{
		"avoid":           []map[string]any{{"from": "2027-06-01", "to": "2027-06-03", "reason": "a wedding"}},
		"separateTickets": false,
	})
	session := phone.tools()
	dropped := newTrip(t, phone, map[string]any{"name": "Dropped"})
	callTool[tripJSON](t, session, "travel_set_trip_status", map[string]any{"id": dropped.ID, "status": "dropped"})

	good := fare(trip, "owner", "2027-05-12", "2027-05-19", 112000, nil)
	bad := []map[string]any{
		fare(dropped, "owner", "2027-05-12", "2027-05-19", 100000, nil),                    // not Watching
		fare(tripJSON{ID: "nosuchtrip"}, "owner", "2027-05-12", "2027-05-19", 100000, nil), // no such Trip
		fare(trip, "with_companion", "2027-05-12", "2027-05-19", 100000, nil),              // not one of its Parties
		fare(trip, "owner", "2027-05-12", "2027-05-17", 100000, nil),                       // 5 nights, under 6
		fare(trip, "owner", "2027-05-12", "2027-05-22", 100000, nil),                       // 10 nights, over 9
		fare(trip, "owner", "2027-05-01", "2027-05-08", 100000, nil),                       // departs outside the window
		fare(trip, "owner", "2027-05-28", "2027-06-04", 100000, nil),                       // touches the avoid range
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, map[string]any{"separateTickets": true}),
		fare(trip, "owner", "2027-05-12", "2027-05-19", 0, nil),      // no price
		fare(trip, "owner", "2027-05-19", "2027-05-12", 100000, nil), // back before it leaves
		fare(trip, "owner", "2027-5-12", "2027-05-19", 100000, nil),  // not a date
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, map[string]any{"source": "a friend"}),
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, map[string]any{"cabin": "steerage"}),
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, map[string]any{"leaveNeeded": map[string]any{"days": 3}}), // only for the Companion
		fare(trip, "owner", "2027-05-12", "2027-05-19", 100000, map[string]any{"bookingUrl": "javascript:alert(1)"}),
	}
	got, _ := record(t, phone, "asked", append([]map[string]any{good}, bad...)...)

	if len(got.Recorded) != 1 || got.Recorded[0].Index != 0 {
		t.Errorf("recorded = %+v; want only the first, good Fare", got.Recorded)
	}
	if len(got.Refused) != len(bad) {
		t.Fatalf("refused = %+v; want each of the %d bad Fares", got.Refused, len(bad))
	}
	for i, r := range got.Refused {
		if r.Index != i+1 || r.Reason == "" {
			t.Errorf("refused[%d] = %+v; want index %d with a reason", i, r, i+1)
		}
	}
	if s := stats(t, phone, trip, "owner"); s.LowestEver == nil || s.LowestEver.Total != 112000 {
		t.Errorf("lowestEver = %+v; want the good Fare at 112000", s.LowestEver)
	}
	toolError(t, session, "travel_record_fares", map[string]any{"checkId": "nosuchcheck", "fares": []map[string]any{good}})
}

func TestAFareForTheCompanionIsPricedPerPersonAndCarriesLeaveNeeded(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	session := phone.tools()
	callTool[settingsJSON](t, session, "travel_save_settings", map[string]any{"companion": map[string]any{"name": "Sam"}})
	callTool[tripJSON](t, session, "travel_save_trip", map[string]any{
		"id": trip.ID, "parties": []map[string]any{{"key": "owner", "adults": 1}, {"key": "with_companion", "adults": 2}},
	})

	record(t, phone, "weekly", fare(trip, "with_companion", "2027-05-12", "2027-05-19", 220461,
		map[string]any{"leaveNeeded": map[string]any{"days": 5, "withRemoteDays": 3}}))

	s := stats(t, phone, trip, "with_companion")
	if s.LowestEver == nil || s.LowestEver.PerPerson != 110231 || s.LowestEver.Total != 220461 ||
		s.LowestEver.LeaveNeeded == nil || *s.LowestEver.LeaveNeeded != (leaveNeededJSON{Days: 5, WithRemoteDays: 3}) {
		t.Errorf("lowestEver = %+v; want 220461 for 2, 110231 each, with 5 days off (3 with remote days)", s.LowestEver)
	}
	if owner := stats(t, phone, trip, "owner"); owner.LowestEver != nil {
		t.Errorf("the owner Party's lowestEver = %+v; want none: the Fare was for the Companion's", owner.LowestEver)
	}
}

// totals are the totals of fares, in order.
func totals(fares []fareJSON) []int {
	got := []int{}
	for _, f := range fares {
		got = append(got, f.Total)
	}
	return got
}

func TestFareStatsSayTheLowestTheBestOfEachWeekAndTheCheapestPerDepartureWeek(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)

	// Sunday 22 November, the last day of the week of 16 November.
	record(t, phone, "weekly", fare(trip, "owner", "2027-05-12", "2027-05-19", 150000, nil))
	// Thursday 26 November, in the week of 23 November.
	n.clock.Set(today.AddDate(0, 0, 4))
	record(t, phone, "daily",
		fare(trip, "owner", "2027-05-20", "2027-05-27", 130000, nil),
		fare(trip, "owner", "2027-06-02", "2027-06-09", 140000, nil))
	// Monday 30 November, in the week of 30 November.
	n.clock.Set(today.AddDate(0, 0, 8))
	record(t, phone, "daily", fare(trip, "owner", "2027-05-13", "2027-05-20", 125000, nil))

	// Wednesday 2 December.
	n.clock.Set(today.AddDate(0, 0, 10))
	s := stats(t, phone, trip, "owner")

	if s.LowestEver == nil || s.LowestEver.Total != 125000 {
		t.Errorf("lowestEver = %+v, want the 125000 Fare", s.LowestEver)
	}
	if s.BestThisWeek == nil || s.BestThisWeek.Total != 125000 {
		t.Errorf("bestThisWeek = %+v, want the 125000 Fare found on Monday", s.BestThisWeek)
	}
	if s.BestLastWeek == nil || s.BestLastWeek.Total != 130000 {
		t.Errorf("bestLastWeek = %+v, want the 130000 Fare found last Thursday", s.BestLastWeek)
	}
	type week struct {
		Week  string
		Total int
	}
	var byWeek []week
	for _, w := range s.ByWeek {
		byWeek = append(byWeek, week{w.Week, w.Fare.Total})
	}
	if want := []week{{"2027-05-10", 125000}, {"2027-05-17", 130000}, {"2027-05-31", 140000}}; !reflect.DeepEqual(byWeek, want) {
		t.Errorf("byWeek = %v, want %v: the cheapest Fare departing in each week, by its Monday", byWeek, want)
	}
	// The 150000 Fare was found 10 days ago, so it isn't current.
	if got, want := totals(s.Top), []int{125000, 130000, 140000}; !reflect.DeepEqual(got, want) {
		t.Errorf("top = %v, want %v", got, want)
	}
}

func TestATripWithNoFaresHasNoStats(t *testing.T) {
	_, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)

	s := stats(t, phone, trip, "owner")

	if s.LowestEver != nil || s.BestThisWeek != nil || s.BestLastWeek != nil || len(s.ByWeek) != 0 || len(s.Top) != 0 {
		t.Errorf("stats = %+v, want empty", s)
	}
	toolError(t, phone.tools(), "travel_fare_stats", map[string]any{"tripId": "nosuchtrip", "party": "owner"})
	toolError(t, phone.tools(), "travel_fare_stats", map[string]any{"tripId": trip.ID, "party": "with_companion"})
}
