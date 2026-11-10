package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	clients    model.OAuthClientsRepository
	requests   model.AuthorizationRequestsRepository
	connectors model.ConnectorsRepository
	codes      model.AuthorizationCodesRepository
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
	// authorizationCodeLifetime is how long an approved Connector has to
	// exchange its code for tokens.
	authorizationCodeLifetime = time.Minute
	// accessTokenLifetime is how long a Connector's access token works on /mcp.
	accessTokenLifetime = time.Hour
	// refreshTokenLifetime is how long a Connector's refresh token lasts
	// unused, the same as a Session.
	refreshTokenLifetime = sessionLifetime
)

// NewOAuth returns the OAuthService of the API at apiURL, whose consent page
// is on the web app at webURL, with now as its clock.
func NewOAuth(
	clients model.OAuthClientsRepository,
	requests model.AuthorizationRequestsRepository,
	connectors model.ConnectorsRepository,
	codes model.AuthorizationCodesRepository,
	apiURL, webURL string,
	now func() time.Time,
) *OAuthService {
	return &OAuthService{
		clients:    clients,
		requests:   requests,
		connectors: connectors,
		codes:      codes,
		apiURL:     apiURL,
		consentURL: webURL + "/connect",
		now:        now,
	}
}

// Routes adds the metadata documents, the OAuth endpoints, and the consent
// page's routes under /auth/connect/ except approving, which needs a
// Passkey and so is the IdentityService's.
func (s *OAuthService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.protectedResourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.authorizationServerMetadata)
	mux.HandleFunc("POST /oauth/register", s.register)
	mux.HandleFunc("GET /oauth/authorize", s.authorize)
	mux.HandleFunc("POST /oauth/token", s.token)
	mux.HandleFunc("GET /auth/connect/{request}", s.pendingRequest)
	mux.HandleFunc("POST /auth/connect/{request}/deny", s.deny)
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
	expiresAt := now.Add(unapprovedClientLifetime)
	client := model.OAuthClient{
		ID:           rand.Text(),
		Name:         metadata.ClientName,
		RedirectURIs: metadata.RedirectURIs,
		CreatedAt:    now,
		ExpiresAt:    &expiresAt,
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

// errNoRequest means a pending request is unknown, already answered or expired.
var errNoRequest = errors.New("no pending request")

// requestError answers a consent page call for a request that isn't pending.
func requestError(w http.ResponseWriter, err error) {
	if errors.Is(err, errNoRequest) || errors.Is(err, utils.ErrNotFound) {
		http.Error(w, "that request is unknown, already answered or expired; connect the app again", http.StatusNotFound)
		return
	}
	utils.ServerError(w, err)
}

// pendingRequest answers which app asks to become a Connector, for the
// consent page to show before the Owner approves it.
func (s *OAuthService) pendingRequest(w http.ResponseWriter, r *http.Request) {
	request, err := s.requests.Find(r.Context(), r.PathValue("request"), s.now())
	if err != nil {
		requestError(w, err)
		return
	}
	client, err := s.clients.Find(r.Context(), request.ClientID, s.now())
	if err != nil {
		requestError(w, err)
		return
	}
	redirect, err := url.Parse(request.RedirectURI)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, model.AuthorizationRequestJSON{
		ClientName:   client.Name,
		RedirectHost: redirect.Hostname(),
	})
}

// findRequest returns the pending request with id, or errNoRequest.
func (s *OAuthService) findRequest(ctx context.Context, id string) (model.AuthorizationRequest, error) {
	request, err := s.requests.Find(ctx, id, s.now())
	if errors.Is(err, utils.ErrNotFound) {
		return request, errNoRequest
	}
	return request, err
}

// deny answers a request the Owner refused: the browser goes back to the
// Connector with access_denied.
func (s *OAuthService) deny(w http.ResponseWriter, r *http.Request) {
	request, err := s.requests.Take(r.Context(), r.PathValue("request"), s.now())
	if err != nil {
		requestError(w, err)
		return
	}
	redirect, err := withQuery(request.RedirectURI, url.Values{"error": {"access_denied"}, "state": {request.State}})
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, model.RedirectJSON{Redirect: redirect})
}

// approve answers a request the Owner approved with a Passkey: it creates the
// Connector, keeps its registration, and returns where to send the browser,
// the redirect URI with a single-use authorization code. It returns
// errNoRequest if the request was answered or expired meanwhile.
func (s *OAuthService) approve(ctx context.Context, requestID string) (redirect string, err error) {
	now := s.now()
	request, err := s.requests.Take(ctx, requestID, now)
	if errors.Is(err, utils.ErrNotFound) {
		return "", errNoRequest
	}
	if err != nil {
		return "", err
	}
	client, err := s.clients.Find(ctx, request.ClientID, now)
	if errors.Is(err, utils.ErrNotFound) {
		return "", errNoRequest
	}
	if err != nil {
		return "", err
	}
	connector := model.Connector{ID: rand.Text(), ClientID: client.ID, Name: client.Name, ApprovedAt: now}
	if err := s.connectors.Insert(ctx, connector); err != nil {
		return "", err
	}
	if err := s.clients.Approve(ctx, client.ID); err != nil {
		return "", err
	}
	code := rand.Text()
	err = s.codes.Insert(ctx, model.AuthorizationCode{
		Hash:          sha256Of(code),
		ConnectorID:   connector.ID,
		ClientID:      client.ID,
		RedirectURI:   request.RedirectURI,
		CodeChallenge: request.CodeChallenge,
		ExpiresAt:     now.Add(authorizationCodeLifetime),
	})
	if err != nil {
		return "", err
	}
	return withQuery(request.RedirectURI, url.Values{"code": {code}, "state": {request.State}})
}

// token answers a Connector's token request (RFC 6749 section 3.2), never
// to be cached.
func (s *OAuthService) token(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "the body isn't a form")
		return
	}
	switch grant := r.PostForm.Get("grant_type"); grant {
	case "authorization_code":
		s.exchangeCode(w, r)
	case "refresh_token":
		s.refreshTokens(w, r)
	case "":
		oauthError(w, http.StatusBadRequest, "invalid_request", "grant_type is required")
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

// exchangeCode gives an approved Connector its first tokens for its
// authorization code (RFC 6749 section 4.1.3, with PKCE). Any attempt uses
// the code up, even a refused one, so a wrong code_verifier can't be retried.
// A code exchanged again may have been stolen, so its Connector is removed,
// with any tokens it got (RFC 6749 section 4.1.2).
func (s *OAuthService) exchangeCode(w http.ResponseWriter, r *http.Request) {
	form := r.PostForm
	now := s.now()
	code, err := s.codes.Use(r.Context(), sha256Of(form.Get("code")), now)
	if errors.Is(err, utils.ErrNotFound) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code is unknown or expired")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if code.UsedAt != nil {
		log.Printf("Connector %s removed: its authorization code was exchanged twice", code.ConnectorID)
		if err := s.connectors.Delete(r.Context(), code.ConnectorID); err != nil {
			utils.ServerError(w, err)
			return
		}
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code was already used, so the Connector is removed; connect the app again")
		return
	}
	if form.Get("client_id") != code.ClientID || form.Get("redirect_uri") != code.RedirectURI {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the code was issued to another client or redirect_uri")
		return
	}
	if !pkceMatches(form.Get("code_verifier"), code.CodeChallenge) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier doesn't match the code_challenge")
		return
	}
	s.issueTokens(w, now, func(tokens model.ConnectorTokens) error {
		return s.connectors.SetTokens(r.Context(), code.ConnectorID, tokens)
	})
}

// refreshTokens gives a Connector new tokens for its refresh token (RFC 6749
// section 6), replacing both: the old refresh token is retired. A retired one
// used again may have been stolen, so its Connector is removed (OAuth 2.1
// section 4.3.1); so is one refreshed twice at once, as only the first
// rotates it.
func (s *OAuthService) refreshTokens(w http.ResponseWriter, r *http.Request) {
	form := r.PostForm
	now := s.now()
	hash := sha256Of(form.Get("refresh_token"))
	connector, err := s.connectors.FindByRefreshToken(r.Context(), hash)
	if errors.Is(err, utils.ErrNotFound) {
		s.refuseRefreshToken(w, r, hash)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if form.Get("client_id") != connector.ClientID {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was issued to another client")
		return
	}
	if !connector.Tokens.RefreshExpiresAt.After(now) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token expired unused; connect the app again")
		return
	}
	s.issueTokens(w, now, func(tokens model.ConnectorTokens) error {
		err := s.connectors.Rotate(r.Context(), connector.ID, hash, tokens)
		if errors.Is(err, utils.ErrNotFound) {
			if removeErr := s.removeReusedConnector(r.Context(), connector); removeErr != nil {
				return removeErr
			}
		}
		return err
	})
}

// refuseRefreshToken answers a refresh token that isn't any Connector's
// current one. If it is a retired one, its Connector is removed.
func (s *OAuthService) refuseRefreshToken(w http.ResponseWriter, r *http.Request, hash []byte) {
	connector, err := s.connectors.FindByRetiredRefreshToken(r.Context(), hash)
	if errors.Is(err, utils.ErrNotFound) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is unknown")
		return
	}
	if err == nil {
		err = s.removeReusedConnector(r.Context(), connector)
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was already used, so the Connector is removed; connect the app again")
}

// removeReusedConnector removes connector, whose refresh token was used twice.
func (s *OAuthService) removeReusedConnector(ctx context.Context, connector model.Connector) error {
	log.Printf("Connector %s (%s) removed: a refresh token was used twice", connector.ID, connector.Name)
	return s.connectors.Delete(ctx, connector.ID)
}

// pkceMatches reports whether verifier is the one the S256 challenge was
// made from (RFC 7636 section 4.6).
func pkceMatches(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}

// issueTokens makes a new access token and refresh token, has save store
// them as a Connector's, and answers with them. If save finds no Connector to
// store them on, it was removed, and the request is refused.
func (s *OAuthService) issueTokens(w http.ResponseWriter, now time.Time, save func(model.ConnectorTokens) error) {
	access, refresh := rand.Text(), rand.Text()
	err := save(model.ConnectorTokens{
		AccessTokenHash:  sha256Of(access),
		AccessExpiresAt:  now.Add(accessTokenLifetime),
		RefreshTokenHash: sha256Of(refresh),
		RefreshExpiresAt: now.Add(refreshTokenLifetime),
	})
	if errors.Is(err, utils.ErrNotFound) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "the Connector has been removed; connect the app again")
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, model.TokenJSON{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessTokenLifetime / time.Second),
		RefreshToken: refresh,
	})
}

// withQuery is uri with params added to its query.
func withQuery(uri string, params url.Values) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for key, values := range params {
		q[key] = values
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// redirectError sends the browser back to a trusted redirectURI with an OAuth
// error and the Connector's state, if it sent one.
func redirectError(w http.ResponseWriter, r *http.Request, redirectURI, code, description, state string) {
	params := url.Values{"error": {code}, "error_description": {description}}
	if state != "" {
		params.Set("state", state)
	}
	redirect, err := withQuery(redirectURI, params)
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
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
