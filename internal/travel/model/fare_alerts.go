package model

import (
	"context"
	"time"
)

// FareAlert is a message the Travel Assistant sent the Owner about a Fare,
// kept so that a later run doesn't tell them again.
type FareAlert struct {
	ID      string   `bson:"_id"`
	FareID  string   `bson:"fareId"`
	TripID  string   `bson:"tripId"`
	Party   PartyKey `bson:"party"`
	Reasons []string `bson:"reasons"`
	// Urgent is whether it went at once, outside the day's one message.
	Urgent bool      `bson:"urgent"`
	SentAt time.Time `bson:"sentAt"`
}

// FareAlertReasons are why a Fare is worth a Fare Alert (ADR 0013).
var FareAlertReasons = []string{"lowest_seen", "low_verdict", "big_drop", "below_target", "ending_soon"}

// FareAlertsRepository reads and writes the fare_alerts collection.
type FareAlertsRepository interface {
	// Insert stores a new Fare Alert, giving it an ID, and returns that ID.
	Insert(ctx context.Context, alert FareAlert) (string, error)
	// List returns a Trip's Party's Fare Alerts sent at or after since,
	// newest first.
	List(ctx context.Context, tripID string, party PartyKey, since time.Time) ([]FareAlert, error)
	// DeleteTrip deletes every Fare Alert of the Trip with id.
	DeleteTrip(ctx context.Context, tripID string) error
}

// FareAlertJSON is a Fare Alert as the Tools answer it.
type FareAlertJSON struct {
	ID      string   `json:"id"`
	FareID  string   `json:"fareId"`
	TripID  string   `json:"tripId"`
	Party   string   `json:"party"`
	Reasons []string `json:"reasons"`
	Urgent  bool     `json:"urgent"`
	SentAt  string   `json:"sentAt"`
}
