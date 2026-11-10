package model

import (
	"context"
	"time"
)

// OAuthClient is a Connector's registration as an OAuth client: the app
// asking to become a Connector, before and after the Owner approves it.
type OAuthClient struct {
	ID string `bson:"_id"`
	// Name is the client_name it registered with, which its Connector is
	// called by.
	Name         string    `bson:"name"`
	RedirectURIs []string  `bson:"redirectUris"`
	CreatedAt    time.Time `bson:"createdAt"`
	// ExpiresAt is when an unapproved registration is deleted.
	ExpiresAt time.Time `bson:"expiresAt"`
}

// OAuthClientsRepository reads and writes the oauth_clients collection.
type OAuthClientsRepository interface {
	Insert(ctx context.Context, client OAuthClient) error
	// Find returns the client with id, unless its registration has expired
	// at now.
	Find(ctx context.Context, id string, now time.Time) (OAuthClient, error)
}

// The JSON shapes below are what the API answers with and is sent.

// ClientMetadataJSON is what a Connector sends to register (RFC 7591).
type ClientMetadataJSON struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// ClientJSON is a Connector's registration as the API answers it: its
// client_id and the metadata as registered, which may differ from what was
// sent (always a public client, with defaults filled in).
type ClientJSON struct {
	ClientID         string `json:"client_id"`
	ClientIDIssuedAt int64  `json:"client_id_issued_at"`
	ClientMetadataJSON
}
