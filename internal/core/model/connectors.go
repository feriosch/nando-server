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
	// LastUsedAt is when it last called /mcp, or nil if it never has.
	LastUsedAt *time.Time `bson:"lastUsedAt,omitempty"`
	// Tokens are its current tokens, once it has exchanged its code.
	Tokens ConnectorTokens `bson:",inline"`
	// RetiredRefreshTokenHashes are its latest replaced refresh tokens, so a
	// reused one is recognised: it may have been stolen, so the Connector is
	// removed.
	RetiredRefreshTokenHashes [][]byte `bson:"retiredRefreshTokenHashes,omitempty"`
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
	// FindByAccessToken returns the Connector whose access token has hash,
	// unless the token has expired at now.
	FindByAccessToken(ctx context.Context, hash []byte, now time.Time) (Connector, error)
	// FindByRefreshToken returns the Connector whose current refresh token
	// has hash, expired or not.
	FindByRefreshToken(ctx context.Context, hash []byte) (Connector, error)
	// FindByRetiredRefreshToken returns the Connector that had a refresh
	// token with hash before it was replaced.
	FindByRetiredRefreshToken(ctx context.Context, hash []byte) (Connector, error)
	// Rotate replaces the tokens of the Connector with id, only if its
	// refresh token still has oldHash, which it then retires. If not, it
	// returns ErrNotFound: the token was used meanwhile.
	Rotate(ctx context.Context, id string, oldHash []byte, tokens ConnectorTokens) error
	// Delete removes the Connector with id and so its tokens. Deleting one
	// that doesn't exist also succeeds.
	Delete(ctx context.Context, id string) error
	// List returns every Connector, oldest approved first.
	List(ctx context.Context) ([]Connector, error)
	// MarkUsed records that the Connector with id called /mcp at.
	MarkUsed(ctx context.Context, id string, at time.Time) error
}

// The JSON shapes below are what the API answers with.

// ConnectorJSON is a Connector as the API lists it. LastUsedAt is null if it
// has never called /mcp.
type ConnectorJSON struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ApprovedAt time.Time  `json:"approvedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// TokenJSON is the token endpoint's answer (RFC 6749 section 5.1): a
// Connector's new access and refresh tokens.
type TokenJSON struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}
