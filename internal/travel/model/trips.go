package model

import (
	"context"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// Trip is a journey the Owner may take, watched until they book it or drop
// it. It is the Owner's own data: the repo never names one (ADR 0013).
type Trip struct {
	ID   string `bson:"_id"`
	Name string `bson:"name"`
	// Origins and Destinations are airport or city codes.
	Origins      []string `bson:"origins"`
	Destinations []string `bson:"destinations"`
	Nights       Nights   `bson:"nights"`
	// Windows are the ranges of departure dates the Trip may fall in.
	Windows []Window `bson:"windows"`
	// Avoid are the ranges of dates the Trip must not touch.
	Avoid           []Avoid `bson:"avoid"`
	Cabin           Cabin   `bson:"cabin"`
	CheckedBags     int     `bson:"checkedBags"`
	MaxStops        int     `bson:"maxStops"`
	SeparateTickets bool    `bson:"separateTickets"`
	Parties         []Party `bson:"parties"`
	Status          Status  `bson:"status"`
	// StatusChangedOn is the Day the Trip got its Status.
	StatusChangedOn coremodel.Day `bson:"statusChangedOn"`
	// Notes is free text the Travel Assistant keeps for itself.
	Notes string `bson:"notes"`
	// TargetFares are the prices per person, one per Party at most, below
	// which the Owner wants to hear, which only SetTargetFares changes.
	TargetFares []TargetFare `bson:"targetFares"`
}

// TargetFare is the price per person below which the Owner wants to hear
// about a Party's Fares, and the Day they agreed it.
type TargetFare struct {
	Party     PartyKey      `bson:"party"`
	PerPerson int           `bson:"perPerson"`
	AgreedOn  coremodel.Day `bson:"agreedOn"`
}

// Nights is how long the stay may be, Min to Max nights.
type Nights struct {
	Min int `bson:"min"`
	Max int `bson:"max"`
}

// Window is a range of departure Days, From to To included.
type Window struct {
	From coremodel.Day `bson:"from"`
	To   coremodel.Day `bson:"to"`
	Note string        `bson:"note"`
}

// Avoid is a range of Days, From to To included, the Trip must not touch.
type Avoid struct {
	From   coremodel.Day `bson:"from"`
	To     coremodel.Day `bson:"to"`
	Reason string        `bson:"reason"`
}

// Cabin is the class a Trip is searched in.
type Cabin string

// The cabins a Trip may be searched in.
const (
	Economy        Cabin = "economy"
	PremiumEconomy Cabin = "premium_economy"
	Business       Cabin = "business"
	First          Cabin = "first"
)

// PartyKey is which Party: the Owner alone, or with the Companion.
type PartyKey string

// The Parties a Trip may be priced for.
const (
	OwnerAlone    PartyKey = "owner"
	WithCompanion PartyKey = "with_companion"
)

// Party is who a Trip is priced for, and how many adults that is.
type Party struct {
	Key    PartyKey `bson:"key"`
	Adults int      `bson:"adults"`
}

// Status is whether a Trip is still watched.
type Status string

// A Trip is Watching until the Owner says it is Booked or Dropped.
const (
	Watching Status = "watching"
	Booked   Status = "booked"
	Dropped  Status = "dropped"
)

// TripsRepository reads and writes the trips collection.
type TripsRepository interface {
	// Insert stores a new Trip, giving it an ID, and returns that ID.
	Insert(ctx context.Context, trip Trip) (string, error)
	// Find returns the Trip with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Trip, error)
	// List returns the Trips with one of statuses, by name.
	List(ctx context.Context, statuses []Status) ([]Trip, error)
	// Update stores trip's fields over those of the Trip with its ID, all
	// but its Status and StatusChangedOn, which only SetStatus changes, or
	// returns Core's utils.ErrNotFound.
	Update(ctx context.Context, trip Trip) error
	// SetStatus sets the Status of the Trip with id, since on, changing
	// nothing else, or returns Core's utils.ErrNotFound.
	SetStatus(ctx context.Context, id string, status Status, on coremodel.Day) error
	// SetTargetFare replaces party's Target Fare on the Trip with id with
	// target, or clears it when target is nil, in one atomic update that
	// keeps the other Parties' Target Fares, changing nothing else, or
	// returns Core's utils.ErrNotFound.
	SetTargetFare(ctx context.Context, id string, party PartyKey, target *TargetFare) error
	// Delete deletes the Trip with id, or returns Core's utils.ErrNotFound.
	Delete(ctx context.Context, id string) error
}

// The JSON shapes below are what the Tools answer with and are sent.

// TripJSON is a Trip as the Tools answer it.
type TripJSON struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Origins         []string         `json:"origins"`
	Destinations    []string         `json:"destinations"`
	Nights          NightsJSON       `json:"nights"`
	Windows         []WindowJSON     `json:"windows"`
	Avoid           []AvoidJSON      `json:"avoid"`
	Cabin           string           `json:"cabin"`
	CheckedBags     int              `json:"checkedBags"`
	MaxStops        int              `json:"maxStops"`
	SeparateTickets bool             `json:"separateTickets"`
	Parties         []PartyJSON      `json:"parties"`
	Status          string           `json:"status"`
	StatusChangedOn string           `json:"statusChangedOn"`
	Notes           string           `json:"notes"`
	TargetFares     []TargetFareJSON `json:"targetFares"`
}

// TargetFareJSON is a Party's Target Fare, per person in JPY, and the Day
// the Owner agreed it.
type TargetFareJSON struct {
	Party     string `json:"party"`
	PerPerson int    `json:"perPerson"`
	AgreedOn  string `json:"agreedOn"`
}

// NightsJSON is how long the stay may be.
type NightsJSON struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// WindowJSON is a range of departure Days, YYYY-MM-DD.
type WindowJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
}

// AvoidJSON is a range of Days, YYYY-MM-DD, the Trip must not touch.
type AvoidJSON struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// PartyJSON is who a Trip is priced for.
type PartyJSON struct {
	Key    string `json:"key"`
	Adults int    `json:"adults"`
}

// SaveTripJSON is what travel_save_trip is sent: a new Trip without an id,
// or the fields to change of the Trip with id. A field left out is nil.
type SaveTripJSON struct {
	ID              string        `json:"id"`
	Name            *string       `json:"name"`
	Origins         *[]string     `json:"origins"`
	Destinations    *[]string     `json:"destinations"`
	Nights          *NightsJSON   `json:"nights"`
	Windows         *[]WindowJSON `json:"windows"`
	Avoid           *[]AvoidJSON  `json:"avoid"`
	Cabin           *string       `json:"cabin"`
	CheckedBags     *int          `json:"checkedBags"`
	MaxStops        *int          `json:"maxStops"`
	SeparateTickets *bool         `json:"separateTickets"`
	Parties         *[]PartyJSON  `json:"parties"`
	Notes           *string       `json:"notes"`
}
