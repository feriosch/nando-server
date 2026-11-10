package service

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// OAuthService is Nando's own OAuth 2.1 authorization server, which lets a
// Connector get tokens for /mcp. It serves the metadata a Connector starts
// from under /.well-known/, and the OAuth endpoints under /oauth/.
type OAuthService struct {
	// apiURL is the API's own URL, https://api.<domain>: the issuer.
	apiURL string
	now    func() time.Time
}

const (
	// maxClientNameLength is the most characters a Connector's name may have.
	maxClientNameLength = 100
	// maxRedirectURIs is the most redirect URIs a Connector may register.
	maxRedirectURIs = 10
)

// NewOAuth returns the OAuthService of the API at apiURL, with now as its clock.
func NewOAuth(apiURL string, now func() time.Time) *OAuthService {
	return &OAuthService{apiURL: apiURL, now: now}
}

// Routes adds the metadata documents and the OAuth endpoints to mux.
func (s *OAuthService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.protectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.authorizationServerMetadata)
	mux.HandleFunc("POST /oauth/register", s.register)
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

// register registers a Connector as a public client (RFC 7591): it never
// gets a client secret, whatever it asks for, since PKCE protects its codes.
func (s *OAuthService) register(w http.ResponseWriter, r *http.Request) {
	var metadata model.ClientMetadataJSON
	if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "the body isn't client metadata")
		return
	}
	if len(metadata.RedirectURIs) == 0 {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must list at least one URI")
		return
	}
	if len(metadata.RedirectURIs) > maxRedirectURIs {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata",
			fmt.Sprintf("redirect_uris must list at most %d URIs", maxRedirectURIs))
		return
	}
	for _, uri := range metadata.RedirectURIs {
		if err := checkRedirectURI(uri); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
			return
		}
	}
	metadata.ClientName = strings.TrimSpace(metadata.ClientName)
	if metadata.ClientName == "" {
		first, _ := url.Parse(metadata.RedirectURIs[0])
		metadata.ClientName = first.Hostname()
	}
	if utf8.RuneCountInString(metadata.ClientName) > maxClientNameLength {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata",
			fmt.Sprintf("client_name must be at most %d characters", maxClientNameLength))
		return
	}
	if len(metadata.GrantTypes) == 0 {
		metadata.GrantTypes = []string{"authorization_code"}
	}
	for _, grant := range metadata.GrantTypes {
		if grant != "authorization_code" && grant != "refresh_token" {
			oauthError(w, http.StatusBadRequest, "invalid_client_metadata",
				"grant_types may only be authorization_code and refresh_token")
			return
		}
	}
	if len(metadata.ResponseTypes) == 0 {
		metadata.ResponseTypes = []string{"code"}
	}
	if slices.ContainsFunc(metadata.ResponseTypes, func(t string) bool { return t != "code" }) {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "response_types may only be code")
		return
	}
	metadata.TokenEndpointAuthMethod = "none"
	utils.WriteJSON(w, http.StatusCreated, model.ClientJSON{
		ClientID:           rand.Text(),
		ClientIDIssuedAt:   s.now().Unix(),
		ClientMetadataJSON: metadata,
	})
}

// checkRedirectURI accepts an https: URL, or plain http: to localhost or
// 127.0.0.1 on any port, where a local app such as Claude Code listens. It
// refuses a fragment or user info, as an authorization code must reach only
// the registered page.
func checkRedirectURI(uri string) error {
	u, err := url.Parse(uri)
	switch {
	case err != nil || u.Host == "" || u.Hostname() == "":
		return fmt.Errorf("redirect URI %q is not an absolute URL", uri)
	case u.Fragment != "" || strings.Contains(uri, "#"):
		return fmt.Errorf("redirect URI %q must not have a fragment", uri)
	case u.User != nil:
		return fmt.Errorf("redirect URI %q must not have user info", uri)
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"):
		return nil
	default:
		return fmt.Errorf("redirect URI %q must be https:, or http: to localhost or 127.0.0.1", uri)
	}
}

// oauthError answers an OAuth error.
func oauthError(w http.ResponseWriter, status int, code, description string) {
	utils.WriteJSON(w, status, model.OAuthErrorJSON{Error: code, ErrorDescription: description})
}
