package model

// The JSON shapes below are what the web app reads under /api/travel/: they
// show no single collection.

// TripsJSON is the list of Trips, Watching ones first.
type TripsJSON struct {
	Trips []TripSummaryJSON `json:"trips"`
}

// TripSummaryJSON is a Trip in the list of Trips.
type TripSummaryJSON struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Origins      []string           `json:"origins"`
	Destinations []string           `json:"destinations"`
	Status       string             `json:"status"`
	Parties      []PartySummaryJSON `json:"parties"`
}

// PartySummaryJSON is a Party's line in the list of Trips: its best Fare now
// (the cheapest found in the last 7 days), the change in the best per person
// since last week, its Target Fare and its latest Price Verdict's level. Each
// is null when there is none.
type PartySummaryJSON struct {
	Key           string    `json:"key"`
	Adults        int       `json:"adults"`
	Best          *FareJSON `json:"best"`
	SinceLastWeek *int      `json:"sinceLastWeek"`
	TargetFare    *int      `json:"targetFare"`
	VerdictLevel  *string   `json:"verdictLevel"`
}

// TripPageJSON is a Trip's page.
type TripPageJSON struct {
	Trip    TripJSON        `json:"trip"`
	Parties []PartyPageJSON `json:"parties"`
}

// PartyPageJSON is a Party on a Trip's page: the best per person each Day it
// was checked, the cheapest Fare per departure week, the top current Fares,
// the best current one on one ticket (to show beside Separate Tickets), its
// latest Price Verdicts and its Target Fare.
type PartyPageJSON struct {
	Key              string             `json:"key"`
	Adults           int                `json:"adults"`
	History          []DayFareJSON      `json:"history"`
	ByWeek           []WeekFareJSON     `json:"byWeek"`
	Top              []FareJSON         `json:"top"`
	BestSingleTicket *FareJSON          `json:"bestSingleTicket"`
	Verdicts         []PriceVerdictJSON `json:"verdicts"`
	TargetFare       *int               `json:"targetFare"`
}

// DayFareJSON is the best Fare per person found on a Day.
type DayFareJSON struct {
	Day       string `json:"day"`
	PerPerson int    `json:"perPerson"`
}

// CardsJSON is Travel's Home Cards.
type CardsJSON struct {
	Cards []HomeCardJSON `json:"cards"`
}

// HomeCardJSON is a Watching Trip's Home Card: the cheaper Party's best Fare
// per person now, and its trend since last week (down, flat or up). Party,
// PerPerson and Trend are null when there is none.
type HomeCardJSON struct {
	TripID    string  `json:"tripId"`
	Name      string  `json:"name"`
	Party     *string `json:"party"`
	PerPerson *int    `json:"perPerson"`
	Trend     *string `json:"trend"`
}
