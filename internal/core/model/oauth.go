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

// OAuthErrorJSON is an OAuth error answer (RFC 6749 section 5.2, RFC 7591
// section 3.2.2).
type OAuthErrorJSON struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}
