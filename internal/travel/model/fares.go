package model

import (
	"context"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// FareCheck is one pass in which the Travel Assistant searches for Fares and
// records them.
type FareCheck struct {
	ID        string        `bson:"_id"`
	Kind      FareCheckKind `bson:"kind"`
	StartedAt time.Time     `bson:"startedAt"`
	// FinishedAt is nil until the run finishes the check.
	FinishedAt *time.Time `bson:"finishedAt"`
	// Sources are the fare sites the check searched.
	Sources []string `bson:"sources"`
	Summary string   `bson:"summary"`
	Errors  []string `bson:"errors"`
}

// FareCheckKind is which Fare Check: the Daily or the Weekly one Nando asked
// for, or one the Owner asked for in the chat.
type FareCheckKind string

// The kinds of Fare Check.
const (
	DailyCheck  FareCheckKind = "daily"
	WeeklyCheck FareCheckKind = "weekly"
	AskedCheck  FareCheckKind = "asked"
)

// FareCheckKinds are every kind of Fare Check.
var FareCheckKinds = []FareCheckKind{DailyCheck, WeeklyCheck, AskedCheck}

// FareChecksRepository reads and writes the fare_checks collection.
type FareChecksRepository interface {
	// Insert stores a new Fare Check, giving it an ID, and returns that ID.
	Insert(ctx context.Context, check FareCheck) (string, error)
	// Find returns the Fare Check with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (FareCheck, error)
	// Finish records how the unfinished Fare Check with id ended, or returns
	// Core's utils.ErrNotFound if there is no such unfinished one.
	Finish(ctx context.Context, id string, at time.Time, sources []string, summary string, errors []string) error
	// LastFinished returns when the last finished Fare Check of kind
	// finished, or the zero time if none has.
	LastFinished(ctx context.Context, kind FareCheckKind) (time.Time, error)
}

// Fare is one priced round trip found for a Trip and Party.
type Fare struct {
	ID              string        `bson:"_id"`
	TripID          string        `bson:"tripId"`
	Party           PartyKey      `bson:"party"`
	CheckID         string        `bson:"checkId"`
	FoundAt         time.Time     `bson:"foundAt"`
	Source          FareSource    `bson:"source"`
	DepartDate      coremodel.Day `bson:"departDate"`
	ReturnDate      coremodel.Day `bson:"returnDate"`
	Nights          int           `bson:"nights"`
	Outbound        Leg           `bson:"outbound"`
	Inbound         Leg           `bson:"inbound"`
	SeparateTickets bool          `bson:"separateTickets"`
	Cabin           Cabin         `bson:"cabin"`
	// BagsIncluded is whether Total includes the Trip's checked bags: not
	// every source prices them.
	BagsIncluded bool `bson:"bagsIncluded"`
	// Currency is always JPY.
	Currency   string `bson:"currency"`
	Total      int    `bson:"total"`
	PerPerson  int    `bson:"perPerson"`
	BookingURL string `bson:"bookingUrl"`
	// LeaveNeeded is set only on a Fare for the Party with the Companion.
	LeaveNeeded *LeaveNeeded `bson:"leaveNeeded"`
}

// Leg is one way of a Fare.
type Leg struct {
	Flights         []Flight `bson:"flights"`
	Stops           int      `bson:"stops"`
	DurationMinutes int      `bson:"durationMinutes"`
}

// Flight is one flight of a Leg. Departs and Arrives are local times,
// "YYYY-MM-DDTHH:MM".
type Flight struct {
	Carrier string `bson:"carrier"`
	Number  string `bson:"number"`
	From    string `bson:"from"`
	To      string `bson:"to"`
	Departs string `bson:"departs"`
	Arrives string `bson:"arrives"`
}

// LeaveNeeded is how many days off the Companion would take for a Fare, and
// how many with the remote days the settings allow.
type LeaveNeeded struct {
	Days           int `bson:"days"`
	WithRemoteDays int `bson:"withRemoteDays"`
}

// FareSource is where a Fare was found.
type FareSource string

// The sources of Fares (ADR 0013).
const (
	FromKiwi       FareSource = "kiwi"
	FromExpedia    FareSource = "expedia"
	FromSkiplagged FareSource = "skiplagged"
	FromSlickTrip  FareSource = "slicktrip"
	FromPriceAlert FareSource = "price_alert"
	FromOther      FareSource = "other"
)

// FareSources are every source of Fares.
var FareSources = []FareSource{FromKiwi, FromExpedia, FromSkiplagged, FromSlickTrip, FromPriceAlert, FromOther}

// FaresRepository reads and writes the fares collection.
type FaresRepository interface {
	// Insert stores a new Fare, giving it an ID, and returns that ID.
	Insert(ctx context.Context, fare Fare) (string, error)
	// List returns the Fares of a Trip's Party found at or after since,
	// cheapest per person first.
	List(ctx context.Context, tripID string, party PartyKey, since time.Time) ([]Fare, error)
	// Find returns the Fare with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Fare, error)
	// DeleteTrip deletes every Fare of the Trip with id.
	DeleteTrip(ctx context.Context, tripID string) error
}

// The JSON shapes below are what the Tools answer with and are sent.

// FareJSON is a Fare as the Tools answer it, and, without the fields Nando
// sets (id, checkId, foundAt, nights, currency, perPerson), as
// travel_record_fares is sent it.
type FareJSON struct {
	ID              string           `json:"id"`
	TripID          string           `json:"tripId"`
	Party           string           `json:"party"`
	CheckID         string           `json:"checkId"`
	FoundAt         string           `json:"foundAt"`
	Source          string           `json:"source"`
	DepartDate      string           `json:"departDate"`
	ReturnDate      string           `json:"returnDate"`
	Nights          int              `json:"nights"`
	Outbound        LegJSON          `json:"outbound"`
	Inbound         LegJSON          `json:"inbound"`
	SeparateTickets bool             `json:"separateTickets"`
	Cabin           string           `json:"cabin"`
	BagsIncluded    bool             `json:"bagsIncluded"`
	Currency        string           `json:"currency"`
	Total           int              `json:"total"`
	PerPerson       int              `json:"perPerson"`
	BookingURL      string           `json:"bookingUrl"`
	LeaveNeeded     *LeaveNeededJSON `json:"leaveNeeded"`
}

// LegJSON is one way of a Fare.
type LegJSON struct {
	Flights         []FlightJSON `json:"flights"`
	Stops           int          `json:"stops"`
	DurationMinutes int          `json:"durationMinutes"`
}

// FlightJSON is one flight of a way.
type FlightJSON struct {
	Carrier string `json:"carrier"`
	Number  string `json:"number"`
	From    string `json:"from"`
	To      string `json:"to"`
	Departs string `json:"departs"`
	Arrives string `json:"arrives"`
}

// LeaveNeededJSON is the Companion's days off for a Fare.
type LeaveNeededJSON struct {
	Days           int `json:"days"`
	WithRemoteDays int `json:"withRemoteDays"`
}

// FareCheckStartJSON is what travel_start_fare_check answers: the new
// check's id, and when the last one of each kind finished (RFC 3339), or
// null.
type FareCheckStartJSON struct {
	ID           string             `json:"id"`
	LastFinished map[string]*string `json:"lastFinished"`
}

// FareCheckJSON is a Fare Check as travel_finish_fare_check answers it.
type FareCheckJSON struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	StartedAt  string   `json:"startedAt"`
	FinishedAt *string  `json:"finishedAt"`
	Sources    []string `json:"sources"`
	Summary    string   `json:"summary"`
	Errors     []string `json:"errors"`
}

// RecordedFaresJSON is what travel_record_fares answers: the Fares it kept,
// and the ones it refused with why, by their place in the list sent.
type RecordedFaresJSON struct {
	Recorded []RecordedFareJSON `json:"recorded"`
	Refused  []RefusedFareJSON  `json:"refused"`
}

// RecordedFareJSON is a Fare travel_record_fares kept.
type RecordedFareJSON struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
}

// RefusedFareJSON is a Fare travel_record_fares refused, and why.
type RefusedFareJSON struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// FareStatsJSON is what travel_fare_stats answers for a Trip and Party.
type FareStatsJSON struct {
	LowestEver   *FareJSON      `json:"lowestEver"`
	BestThisWeek *FareJSON      `json:"bestThisWeek"`
	BestLastWeek *FareJSON      `json:"bestLastWeek"`
	ByWeek       []WeekFareJSON `json:"byWeek"`
	Top          []FareJSON     `json:"top"`
	// TargetFare is the Party's Target Fare per person, or null.
	TargetFare *int `json:"targetFare"`
}

// WeekFareJSON is the cheapest Fare departing in a week, by its Monday.
type WeekFareJSON struct {
	Week string   `json:"week"`
	Fare FareJSON `json:"fare"`
}
