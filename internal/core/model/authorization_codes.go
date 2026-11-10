package model

import (
	"context"
	"time"
)

// AuthorizationCode is the single-use code an approved Connector exchanges
// for its first tokens, stored only as a hash.
type AuthorizationCode struct {
	Hash        []byte `bson:"_id"`
	ConnectorID string `bson:"connectorId"`
	// ClientID and RedirectURI are what the exchange must repeat.
	ClientID    string `bson:"clientId"`
	RedirectURI string `bson:"redirectUri"`
	// CodeChallenge is the S256 PKCE challenge the exchange's verifier must match.
	CodeChallenge string    `bson:"codeChallenge"`
	ExpiresAt     time.Time `bson:"expiresAt"`
}

// AuthorizationCodesRepository reads and writes the authorization_codes
// collection.
type AuthorizationCodesRepository interface {
	Insert(ctx context.Context, code AuthorizationCode) error
	// Take deletes and returns the code with hash if it is unexpired at now,
	// so each one is exchanged only once.
	Take(ctx context.Context, hash []byte, now time.Time) (AuthorizationCode, error)
}
