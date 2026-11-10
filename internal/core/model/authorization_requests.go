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
}
