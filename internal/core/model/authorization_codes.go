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
	CodeChallenge string `bson:"codeChallenge"`
	// UsedAt is when it was first exchanged. A used code is kept until it
	// expires, so one exchanged again is recognised.
	UsedAt    *time.Time `bson:"usedAt,omitempty"`
	ExpiresAt time.Time  `bson:"expiresAt"`
}

// AuthorizationCodesRepository reads and writes the authorization_codes
// collection.
type AuthorizationCodesRepository interface {
	Insert(ctx context.Context, code AuthorizationCode) error
	// Use marks the code with hash used at now, if it is unexpired, and
	// returns it as it was: with UsedAt set if it had been used before.
	Use(ctx context.Context, hash []byte, now time.Time) (AuthorizationCode, error)
}
