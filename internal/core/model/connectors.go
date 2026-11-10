package model

import (
	"context"
	"time"
)

// Connector is an outside app the Owner approved, with a Passkey, to use
// Nando's Tools.
type Connector struct {
	ID string `bson:"_id"`
	// ClientID is the OAuth client it was approved for.
	ClientID string `bson:"clientId"`
	// Name is the client's name when it was approved.
	Name       string    `bson:"name"`
	ApprovedAt time.Time `bson:"approvedAt"`
}

// ConnectorsRepository reads and writes the connectors collection.
type ConnectorsRepository interface {
	Insert(ctx context.Context, connector Connector) error
}
