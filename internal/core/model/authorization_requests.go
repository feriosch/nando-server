package model

import (
	"context"
	"time"
)

// AuthorizationRequest is a Connector's request to be approved, from
// /oauth/authorize until the Owner approves or denies it on the consent page.
type AuthorizationRequest struct {
	ID          string `bson:"_id"`
	ClientID    string `bson:"clientId"`
	RedirectURI string `bson:"redirectUri"`
	// CodeChallenge is the S256 PKCE challenge the code's verifier must match.
	CodeChallenge string    `bson:"codeChallenge"`
	State         string    `bson:"state"`
	ExpiresAt     time.Time `bson:"expiresAt"`
}

// AuthorizationRequestsRepository reads and writes the
// authorization_requests collection.
type AuthorizationRequestsRepository interface {
	Insert(ctx context.Context, request AuthorizationRequest) error
	// Find returns the request with id, unless it has expired at now.
	Find(ctx context.Context, id string, now time.Time) (AuthorizationRequest, error)
	// Take deletes and returns the request with id if it is unexpired at
	// now, so each one is approved or denied only once.
	Take(ctx context.Context, id string, now time.Time) (AuthorizationRequest, error)
}

// The JSON shapes below are what the API answers with.

// AuthorizationRequestJSON is a pending request as the consent page shows
// it: which app asks, and the host it will send the Owner back to.
type AuthorizationRequestJSON struct {
	ClientName   string `json:"clientName"`
	RedirectHost string `json:"redirectHost"`
}

// RedirectJSON is where the consent page sends the browser after the Owner
// approves or denies a request: back to the Connector.
type RedirectJSON struct {
	Redirect string `json:"redirect"`
}
