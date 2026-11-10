package model

// The JSON shapes below are how Nando, as its own OAuth 2.1 authorization
// server, tells a Connector where to get a token for /mcp.

// ProtectedResourceMetadataJSON is /mcp's protected resource metadata
// (RFC 9728): what it is and which authorization server issues its tokens.
type ProtectedResourceMetadataJSON struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

// AuthorizationServerMetadataJSON is the authorization server metadata
// (RFC 8414): its endpoints and what they support.
type AuthorizationServerMetadataJSON struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

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

// OAuthErrorJSON is an OAuth error answer (RFC 6749 section 5.2, RFC 7591
// section 3.2.2).
type OAuthErrorJSON struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}
