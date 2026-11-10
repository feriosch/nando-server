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
	// Tokens are its current tokens, once it has exchanged its code.
	Tokens ConnectorTokens `bson:",inline"`
}

// ConnectorTokens are a Connector's current access and refresh tokens,
// stored only as hashes, like Session tokens.
type ConnectorTokens struct {
	AccessTokenHash  []byte    `bson:"accessTokenHash,omitempty"`
	AccessExpiresAt  time.Time `bson:"accessExpiresAt,omitempty"`
	RefreshTokenHash []byte    `bson:"refreshTokenHash,omitempty"`
	RefreshExpiresAt time.Time `bson:"refreshExpiresAt,omitempty"`
}

// ConnectorsRepository reads and writes the connectors collection.
type ConnectorsRepository interface {
	Insert(ctx context.Context, connector Connector) error
	// SetTokens replaces the tokens of the Connector with id.
	SetTokens(ctx context.Context, id string, tokens ConnectorTokens) error
}

// The JSON shapes below are what the API answers with.

// TokenJSON is the token endpoint's answer (RFC 6749 section 5.1): a
// Connector's new access and refresh tokens.
type TokenJSON struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}
