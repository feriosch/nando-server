package service

import (
	"crypto/rand"
	"encoding/json"
	"errors"
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
	clients  model.OAuthClientsRepository
	requests model.AuthorizationRequestsRepository
	// apiURL is the API's own URL, https://api.<domain>: the issuer.
	apiURL string
	// consentURL is the web app's consent page, where the Owner approves a
	// Connector.
	consentURL string
	now        func() time.Time
}

const (
	// maxClientNameLength is the most characters a Connector's name may have.
	maxClientNameLength = 100
	// maxRedirectURIs is the most redirect URIs a Connector may register.
	maxRedirectURIs = 10
	// unapprovedClientLifetime is how long a registration is kept before the
	// Owner approves a Connector for it.
	unapprovedClientLifetime = 24 * time.Hour
	// authorizationRequestLifetime is how long the Owner has to approve a
	// Connector on the consent page.
	authorizationRequestLifetime = 10 * time.Minute
)

// NewOAuth returns the OAuthService of the API at apiURL, whose consent page
// is on the web app at webURL, with now as its clock.
func NewOAuth(
	clients model.OAuthClientsRepository,
	requests model.AuthorizationRequestsRepository,
	apiURL, webURL string,
	now func() time.Time,
) *OAuthService {
	return &OAuthService{
		clients:    clients,
		requests:   requests,
		apiURL:     apiURL,
		consentURL: webURL + "/connect",
		now:        now,
	}
}

// Routes adds the metadata documents and the OAuth endpoints to mux.
func (s *OAuthService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.protectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.authorizationServerMetadata)
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("GET /oauth/authorize", s.authorize)
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
	now := s.now()
	client := model.OAuthClient{
		ID:           rand.Text(),
		Name:         metadata.ClientName,
		RedirectURIs: metadata.RedirectURIs,
		CreatedAt:    now,
		ExpiresAt:    now.Add(unapprovedClientLifetime),
	}
	if err := s.clients.Insert(r.Context(), client); err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusCreated, model.ClientJSON{
		ClientID:           client.ID,
		ClientIDIssuedAt:   now.Unix(),
		ClientMetadataJSON: metadata,
	})
}

// authorize starts a Connector's authorization (RFC 6749 section 4.1.1, with
// PKCE): it keeps the request and sends the Owner's browser to the consent
// page. An unknown client or a redirect URI it didn't register is shown as a
// plain error, since redirecting there could hand the error to anyone; every
// other error is sent back to the Connector at its redirect URI.
func (s *OAuthService) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	client, err := s.clients.Find(r.Context(), q.Get("client_id"), s.now())
	if errors.Is(err, utils.ErrNotFound) {
		http.Error(w, "this app isn't registered, or its registration has expired; connect it again", http.StatusBadRequest)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	redirectURI := q.Get("redirect_uri")
	if !slices.Contains(client.RedirectURIs, redirectURI) {
		http.Error(w, "redirect_uri isn't one this app registered", http.StatusBadRequest)
		return
	}
	state := q.Get("state")
	switch {
	case q.Get("response_type") == "":
		redirectError(w, r, redirectURI, "invalid_request", "response_type is required", state)
		return
	case q.Get("response_type") != "code":
		redirectError(w, r, redirectURI, "unsupported_response_type", "response_type must be code", state)
		return
	case q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256":
		redirectError(w, r, redirectURI, "invalid_request", "a code_challenge with code_challenge_method S256 is required", state)
		return
	case state == "":
		redirectError(w, r, redirectURI, "invalid_request", "state is required", state)
		return
	}

	request := model.AuthorizationRequest{
		ID:            rand.Text(),
		ClientID:      client.ID,
		RedirectURI:   redirectURI,
		CodeChallenge: q.Get("code_challenge"),
		State:         state,
		ExpiresAt:     s.now().Add(authorizationRequestLifetime),
	}
	if err := s.requests.Insert(r.Context(), request); err != nil {
		utils.ServerError(w, err)
		return
	}
	http.Redirect(w, r, s.consentURL+"?"+url.Values{"request": {request.ID}}.Encode(), http.StatusFound)
}

// redirectError sends the browser back to a trusted redirectURI with an OAuth
// error and the Connector's state, if it sent one.
func redirectError(w http.ResponseWriter, r *http.Request, redirectURI, code, description, state string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	q := u.Query()
	q.Set("error", code)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
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
