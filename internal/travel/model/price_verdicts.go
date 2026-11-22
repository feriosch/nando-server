package model

import (
	"context"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// PriceVerdict is Google's judgement of a Trip's Party's price on some dates,
// which Nando fetched from Google Flights through SerpApi (ADR 0013).
type PriceVerdict struct {
	ID         string        `bson:"_id"`
	TripID     string        `bson:"tripId"`
	Party      PartyKey      `bson:"party"`
	DepartDate coremodel.Day `bson:"departDate"`
	ReturnDate coremodel.Day `bson:"returnDate"`
	// Day is the Day in the Owner Time Zone it was fetched on: asking again
	// that Day answers it without another call.
	Day       coremodel.Day `bson:"day"`
	FetchedAt time.Time     `bson:"fetchedAt"`
	// Search is what SerpApi was asked, without the key: the Trip's
	// airports, cabin and adults then, so a Trip changed since is asked again.
	Search string `bson:"search"`
	// Level is low, typical or high.
	Level        string `bson:"level"`
	LowestPrice  int    `bson:"lowestPrice"`
	TypicalRange []int  `bson:"typicalRange"`
	// History is Google's price history: [Unix seconds, price] pairs.
	History          [][2]int64 `bson:"history"`
	CheapestPrice    int        `bson:"cheapestPrice"`
	CheapestAirlines []string   `bson:"cheapestAirlines"`
}

// PriceVerdictsRepository reads and writes the price_verdicts collection.
type PriceVerdictsRepository interface {
	Insert(ctx context.Context, verdict PriceVerdict) error
	// FindOn returns the verdict fetched on day for a Trip's Party's dates
	// with search, or Core's utils.ErrNotFound.
	FindOn(ctx context.Context, tripID string, party PartyKey, depart, back, day coremodel.Day, search string) (PriceVerdict, error)
	// List returns a Trip's Party's latest verdicts, newest first, at most
	// limit.
	List(ctx context.Context, tripID string, party PartyKey, limit int) ([]PriceVerdict, error)
	// DeleteTrip deletes every verdict of the Trip with id.
	DeleteTrip(ctx context.Context, tripID string) error
}

// SerpAPICallsRepository reads and writes the serpapi_calls collection: one
// document per call to SerpApi, which bills each, answered or not.
type SerpAPICallsRepository interface {
	Insert(ctx context.Context, at time.Time) error
	// CountSince counts the calls at or after since.
	CountSince(ctx context.Context, since time.Time) (int, error)
}

// The JSON shapes below are what the Tools answer with.

// PriceVerdictJSON is a Price Verdict as the Tools answer it.
type PriceVerdictJSON struct {
	TripID           string     `json:"tripId"`
	Party            string     `json:"party"`
	DepartDate       string     `json:"departDate"`
	ReturnDate       string     `json:"returnDate"`
	Level            string     `json:"level"`
	LowestPrice      int        `json:"lowestPrice"`
	TypicalRange     []int      `json:"typicalRange"`
	History          [][2]int64 `json:"history"`
	CheapestPrice    int        `json:"cheapestPrice"`
	CheapestAirlines []string   `json:"cheapestAirlines"`
	FetchedAt        string     `json:"fetchedAt"`
}
