package service

import (
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// OAuthService is Nando's own OAuth 2.1 authorization server, which lets a
// Connector get tokens for /mcp. It serves the metadata a Connector starts
// from under /.well-known/.
type OAuthService struct {
	// apiURL is the API's own URL, https://api.<domain>: the issuer.
	apiURL string
}

// NewOAuth returns the OAuthService of the API at apiURL.
func NewOAuth(apiURL string) *OAuthService {
	return &OAuthService{apiURL: apiURL}
}

// Routes adds the metadata documents to mux.
func (s *OAuthService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.protectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.authorizationServerMetadata)
}

// protectedResourceMetadata answers that /mcp takes tokens from this server.
func (s *OAuthService) protectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, model.ProtectedResourceMetadataJSON{
		Resource:             s.apiURL + "/mcp",
		AuthorizationServers: []string{s.apiURL},
	})
}

// authorizationServerMetadata answers with this server's endpoints: public
// clients only, authorization codes with S256 PKCE, and refresh tokens.
func (s *OAuthService) authorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, model.AuthorizationServerMetadataJSON{
		Issuer:                            s.apiURL,
		AuthorizationEndpoint:             s.apiURL + "/oauth/authorize",
		TokenEndpoint:                     s.apiURL + "/oauth/token",
		RegistrationEndpoint:              s.apiURL + "/oauth/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	})
}
