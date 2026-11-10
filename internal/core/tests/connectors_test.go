package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// protectedResourceMetadata is where a Connector learns how to get a token
// for /mcp, on the API's own host under the domain example.com.
const protectedResourceMetadata = "https://api.example.com/.well-known/oauth-protected-resource"

func TestMCPWithoutAConnectorTokenIsUnauthorized(t *testing.T) {
	n := newNando(t)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /mcp without a token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	want := `Bearer resource_metadata="` + protectedResourceMetadata + `"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}

// protectedResourceJSON is the protected resource metadata (RFC 9728) a
// Connector reads first.
type protectedResourceJSON struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

// authorizationServerJSON is the authorization server metadata (RFC 8414) a
// Connector reads next, to find the OAuth endpoints.
type authorizationServerJSON struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

// getMetadata GETs a /.well-known/ document as a Connector does, with no
// Session or Origin, and decodes it into a T.
func getMetadata[T any](n *nando, path string) T {
	n.t.Helper()
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		n.t.Fatalf("GET %s: status = %d, body %q", path, rec.Code, rec.Body)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		n.t.Fatalf("GET %s: %v in %q", path, err, rec.Body)
	}
	return v
}

func TestAConnectorFindsTheOAuthEndpointsFromTheMetadata(t *testing.T) {
	n := newNando(t)

	resource := getMetadata[protectedResourceJSON](n, "/.well-known/oauth-protected-resource")
	wantResource := protectedResourceJSON{
		Resource:             "https://api.example.com/mcp",
		AuthorizationServers: []string{"https://api.example.com"},
	}
	if !reflect.DeepEqual(resource, wantResource) {
		t.Errorf("protected resource metadata = %+v, want %+v", resource, wantResource)
	}

	server := getMetadata[authorizationServerJSON](n, "/.well-known/oauth-authorization-server")
	wantServer := authorizationServerJSON{
		Issuer:                            "https://api.example.com",
		AuthorizationEndpoint:             "https://api.example.com/oauth/authorize",
		TokenEndpoint:                     "https://api.example.com/oauth/token",
		RegistrationEndpoint:              "https://api.example.com/oauth/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
	}
	if !reflect.DeepEqual(server, wantServer) {
		t.Errorf("authorization server metadata = %+v,\nwant %+v", server, wantServer)
	}
}

// clientJSON is a Connector's registration (RFC 7591), as it is sent and
// as the API answers it.
type clientJSON struct {
	ClientID                string   `json:"client_id,omitempty"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	ClientSecret            string   `json:"client_secret,omitempty"`
}

// oauthErrorJSON is an OAuth error answer.
type oauthErrorJSON struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// claude is a registration like the Claude app's.
var claude = clientJSON{
	ClientName:              "Claude",
	RedirectURIs:            []string{"https://claude.ai/api/mcp/auth_callback"},
	GrantTypes:              []string{"authorization_code", "refresh_token"},
	ResponseTypes:           []string{"code"},
	TokenEndpointAuthMethod: "none",
}

// connectorDo sends a request as a Connector does: from its own server or a
// local app, with no Session or Origin.
func (n *nando) connectorDo(method, path string, body []byte) *httptest.ResponseRecorder {
	n.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

// register registers client and returns the response.
func (n *nando) register(client any) *httptest.ResponseRecorder {
	n.t.Helper()
	return n.connectorDo(http.MethodPost, "/oauth/register", mustJSON(n.t, client))
}

// mustRegister registers client and returns the registration as answered.
func (n *nando) mustRegister(client clientJSON) clientJSON {
	n.t.Helper()
	rec := n.register(client)
	if rec.Code != http.StatusCreated {
		n.t.Fatalf("POST /oauth/register: status = %d, body %q", rec.Code, rec.Body)
	}
	var registered clientJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &registered); err != nil {
		n.t.Fatalf("POST /oauth/register: %v in %q", err, rec.Body)
	}
	return registered
}

func TestAConnectorRegistersAsAPublicClient(t *testing.T) {
	n := newNando(t)

	got := n.mustRegister(claude)

	if got.ClientID == "" {
		t.Errorf("registration has no client_id: %+v", got)
	}
	want := claude
	want.ClientID = got.ClientID
	if !reflect.DeepEqual(got, want) {
		t.Errorf("registration = %+v, want %+v", got, want)
	}
	if again := n.mustRegister(claude); again.ClientID == got.ClientID {
		t.Errorf("two registrations share client_id %q", got.ClientID)
	}
}

func TestAConnectorAskingForAClientSecretIsRegisteredAsAPublicClient(t *testing.T) {
	n := newNando(t)
	asked := claude
	asked.TokenEndpointAuthMethod = "client_secret_basic"

	got := n.mustRegister(asked)

	if got.TokenEndpointAuthMethod != "none" || got.ClientSecret != "" {
		t.Errorf("registration asking for a secret = %+v, want token_endpoint_auth_method none and no client_secret", got)
	}
}

func TestAConnectorWithoutANameIsNamedAfterItsRedirectHost(t *testing.T) {
	n := newNando(t)

	got := n.mustRegister(clientJSON{RedirectURIs: []string{"https://chat.example.org/oauth/callback"}})

	if got.ClientName != "chat.example.org" {
		t.Errorf("client_name = %q, want the redirect URI's host, chat.example.org", got.ClientName)
	}
}

func TestLocalAppsMayRedirectToLoopbackOverHTTP(t *testing.T) {
	n := newNando(t)

	for _, uri := range []string{"http://localhost:53682/callback", "http://127.0.0.1:8080/cb", "http://localhost/callback"} {
		got := n.mustRegister(clientJSON{ClientName: "Claude Code", RedirectURIs: []string{uri}})
		if !reflect.DeepEqual(got.RedirectURIs, []string{uri}) {
			t.Errorf("redirect_uris = %q, want [%q]", got.RedirectURIs, uri)
		}
	}
}

func TestRegistrationsWithAnUntrustworthyRedirectURIAreRejected(t *testing.T) {
	n := newNando(t)

	for _, uris := range [][]string{
		nil,
		{},
		{"http://example.com/callback"}, // plain HTTP off the device
		{"http://localhost.example.com/callback"}, // only looks local
		{"http://[::1]:8080/callback"},            // not one of the two loopback names
		{"ftp://example.com/callback"},
		{"javascript:alert(1)"},
		{"/callback"},
		{"https:///callback"},
		{"https://claude.ai/callback#fragment"},
		{"https://user@claude.ai/callback"},
		{"https://claude.ai/api/mcp/auth_callback", "http://example.com/callback"},
	} {
		rec := n.register(clientJSON{ClientName: "Claude", RedirectURIs: uris})
		var got oauthErrorJSON
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusBadRequest || got.Error != "invalid_redirect_uri" {
			t.Errorf("register with redirect_uris %q: status = %d, body %q; want 400 invalid_redirect_uri", uris, rec.Code, rec.Body)
		}
	}
}

func TestInvalidRegistrationsAreRejected(t *testing.T) {
	n := newNando(t)
	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("https://claude.ai/callback/%d", i)
	}

	for name, body := range map[string]any{
		"not JSON":       "{",
		"name too long":  clientJSON{ClientName: strings.Repeat("x", 101), RedirectURIs: claude.RedirectURIs},
		"too many URIs":  clientJSON{ClientName: "Claude", RedirectURIs: tooMany},
		"implicit grant": clientJSON{ClientName: "Claude", RedirectURIs: claude.RedirectURIs, GrantTypes: []string{"implicit"}},
		"token response": clientJSON{ClientName: "Claude", RedirectURIs: claude.RedirectURIs, ResponseTypes: []string{"token"}},
	} {
		rec := n.register(body)
		var got oauthErrorJSON
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusBadRequest || got.Error != "invalid_client_metadata" {
			t.Errorf("%s: status = %d, body %q; want 400 invalid_client_metadata", name, rec.Code, rec.Body)
		}
	}
}

// verifier is a PKCE code verifier, as a Connector makes one up for each
// authorization.
const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

// challengeOf is the S256 code challenge of a code verifier (RFC 7636).
func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// authorizeQuery is the query a Connector sends the Owner's browser to
// /oauth/authorize with.
func authorizeQuery(client clientJSON, state string) url.Values {
	return url.Values{
		"response_type":         {"code"},
		"client_id":             {client.ClientID},
		"redirect_uri":          {client.RedirectURIs[0]},
		"code_challenge":        {challengeOf(verifier)},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
}

// authorize opens /oauth/authorize with query in the Owner's browser, which
// sends no Origin on a top-level navigation.
func (n *nando) authorize(query url.Values) *httptest.ResponseRecorder {
	n.t.Helper()
	return n.connectorDo(http.MethodGet, "/oauth/authorize?"+query.Encode(), nil)
}

// redirectOf is where a response redirects the browser, or fails the test.
func redirectOf(t *testing.T, rec *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body %q; want a redirect, %d", rec.Code, rec.Body, http.StatusFound)
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location %q: %v", rec.Header().Get("Location"), err)
	}
	return u
}

func TestAuthorizeSendsTheOwnerToTheConsentPage(t *testing.T) {
	n := newNando(t)
	client := n.mustRegister(claude)

	first := redirectOf(t, n.authorize(authorizeQuery(client, "xyz")))
	second := redirectOf(t, n.authorize(authorizeQuery(client, "xyz")))

	for _, u := range []*url.URL{first, second} {
		if u.Scheme != "https" || u.Host != "web.example.com" || u.Path != "/connect" || u.Query().Get("request") == "" {
			t.Errorf("authorize redirects to %s, want https://web.example.com/connect?request=…", u)
		}
	}
	if first.Query().Get("request") == second.Query().Get("request") {
		t.Errorf("two authorizations share request %q", first.Query().Get("request"))
	}
}

func TestAuthorizeShowsAnErrorRatherThanRedirectToAnUntrustedURI(t *testing.T) {
	n := newNando(t)
	client := n.mustRegister(claude)
	with := func(key, value string) url.Values {
		q := authorizeQuery(client, "xyz")
		if value == "" {
			q.Del(key)
		} else {
			q.Set(key, value)
		}
		return q
	}

	for name, query := range map[string]url.Values{
		"no client_id":             with("client_id", ""),
		"unknown client_id":        with("client_id", "unknown"),
		"no redirect_uri":          with("redirect_uri", ""),
		"unregistered redirect":    with("redirect_uri", "https://evil.example.com/callback"),
		"redirect with extra path": with("redirect_uri", claude.RedirectURIs[0]+"/more"),
		"redirect with a query":    with("redirect_uri", claude.RedirectURIs[0]+"?next=evil"),
	} {
		rec := n.authorize(query)
		if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" {
			t.Errorf("%s: status = %d, Location %q; want %d and no redirect", name, rec.Code, rec.Header().Get("Location"), http.StatusBadRequest)
		}
	}
}

func TestAuthorizeSendsOtherErrorsBackToTheConnector(t *testing.T) {
	n := newNando(t)
	client := n.mustRegister(claude)
	with := func(key, value string) url.Values {
		q := authorizeQuery(client, "xyz")
		if value == "" {
			q.Del(key)
		} else {
			q.Set(key, value)
		}
		return q
	}

	for name, tc := range map[string]struct {
		query     url.Values
		wantError string
		wantState string
	}{
		"token response type": {with("response_type", "token"), "unsupported_response_type", "xyz"},
		"no response type":    {with("response_type", ""), "invalid_request", "xyz"},
		"no code challenge":   {with("code_challenge", ""), "invalid_request", "xyz"},
		"no challenge method": {with("code_challenge_method", ""), "invalid_request", "xyz"},
		"plain challenge":     {with("code_challenge_method", "plain"), "invalid_request", "xyz"},
		"no state":            {with("state", ""), "invalid_request", ""},
	} {
		u := redirectOf(t, n.authorize(tc.query))
		back := *u
		back.RawQuery = ""
		got := u.Query()
		if back.String() != claude.RedirectURIs[0] || got.Get("error") != tc.wantError || got.Get("state") != tc.wantState {
			t.Errorf("%s: redirects to %s, want %s?error=%s&state=%s", name, u, claude.RedirectURIs[0], tc.wantError, tc.wantState)
		}
	}
}

func TestARegistrationNotApprovedWithinADayIsForgotten(t *testing.T) {
	n := newNando(t)
	client := n.mustRegister(claude)

	n.clock.Advance(23 * time.Hour)
	redirectOf(t, n.authorize(authorizeQuery(client, "xyz")))

	n.clock.Advance(2 * time.Hour)
	if rec := n.authorize(authorizeQuery(client, "xyz")); rec.Code != http.StatusBadRequest {
		t.Errorf("authorize a day after registering: status = %d, Location %q; want %d, an unknown client",
			rec.Code, rec.Header().Get("Location"), http.StatusBadRequest)
	}
}

// pendingRequestJSON is a Connector's pending request as the consent page
// shows it.
type pendingRequestJSON struct {
	ClientName   string `json:"clientName"`
	RedirectHost string `json:"redirectHost"`
}

// redirectJSON is where the consent page sends the browser next.
type redirectJSON struct {
	Redirect string `json:"redirect"`
}

// requestOf is the pending request id an authorization sent the browser to
// the consent page with.
func requestOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return redirectOf(t, rec).Query().Get("request")
}

// mustAuthorize registers claude, has it authorize with state "xyz" and
// returns its registration and the pending request.
func (n *nando) mustAuthorize() (clientJSON, string) {
	n.t.Helper()
	client := n.mustRegister(claude)
	return client, requestOf(n.t, n.authorize(authorizeQuery(client, "xyz")))
}

// approve approves request on the consent page with this browser's Passkey,
// returning the response that ends the ceremony (or the first one that fails).
func (d *browser) approve(request string) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	rec := d.do(http.MethodPost, "/auth/connect/"+request+"/begin", nil)
	if rec.Code != http.StatusOK {
		return rec
	}
	return d.do(http.MethodPost, "/auth/connect/"+request+"/finish", d.passkey.Get(d.nando.t, d.origin, rec.Body.Bytes()))
}

// redirectIn is the URL a consent page response sends the browser to.
func redirectIn(t *testing.T, rec *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusOK)
	}
	var r redirectJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("%v in %q", err, rec.Body)
	}
	u, err := url.Parse(r.Redirect)
	if err != nil {
		t.Fatalf("redirect %q: %v", r.Redirect, err)
	}
	return u
}

// withoutQuery is u without its query, to compare with a redirect URI.
func withoutQuery(u *url.URL) string {
	bare := *u
	bare.RawQuery = ""
	return bare.String()
}

func TestTheConsentPageShowsWhichAppAsks(t *testing.T) {
	n := newNando(t)
	_, request := n.mustAuthorize()
	mac := n.newBrowser("Mac")

	rec := mac.do(http.MethodGet, "/auth/connect/"+request, nil)

	var got pendingRequestJSON
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("GET the pending request: status = %d, body %q", rec.Code, rec.Body)
	}
	if want := (pendingRequestJSON{ClientName: "Claude", RedirectHost: "claude.ai"}); got != want {
		t.Errorf("pending request = %+v, want %+v", got, want)
	}
	if rec := mac.do(http.MethodGet, "/auth/connect/unknown", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET an unknown request: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestOwnerApprovesAConnectorWithAPasskey(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	_, request := n.mustAuthorize()
	// The Mac has the Passkey through iCloud Keychain, but no Session.
	mac := n.newBrowser("Mac").syncedFrom(phone)

	u := redirectIn(t, mac.approve(request))

	if withoutQuery(u) != claude.RedirectURIs[0] || u.Query().Get("code") == "" || u.Query().Get("state") != "xyz" {
		t.Errorf("approval redirects to %s, want %s?code=…&state=xyz", u, claude.RedirectURIs[0])
	}
	if mac.session != nil {
		t.Errorf("approving a Connector started a Session in the Mac")
	}
	if rec := mac.approve(request); rec.Code != http.StatusNotFound {
		t.Errorf("approve the same request again: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusNotFound)
	}
}

func TestOwnerDeniesAConnector(t *testing.T) {
	n := newNando(t)
	_, request := n.mustAuthorize()
	mac := n.newBrowser("Mac")

	u := redirectIn(t, mac.do(http.MethodPost, "/auth/connect/"+request+"/deny", nil))

	if withoutQuery(u) != claude.RedirectURIs[0] || u.Query().Get("error") != "access_denied" ||
		u.Query().Get("state") != "xyz" || u.Query().Get("code") != "" {
		t.Errorf("denial redirects to %s, want %s?error=access_denied&state=xyz", u, claude.RedirectURIs[0])
	}
	if rec := mac.do(http.MethodGet, "/auth/connect/"+request, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a denied request: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAPendingRequestExpiresAfterTenMinutes(t *testing.T) {
	n := newNando(t)
	_, request := n.mustAuthorize()
	mac := n.newBrowser("Mac")

	n.clock.Advance(11 * time.Minute)

	if rec := mac.do(http.MethodGet, "/auth/connect/"+request, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a request after 11 minutes: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAnApprovedRegistrationIsKept(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	client, request := n.mustAuthorize()
	redirectIn(t, phone.approve(request))

	n.clock.Advance(25 * time.Hour)

	requestOf(t, n.authorize(authorizeQuery(client, "abc")))
}

// tokenJSON is the token endpoint's answer (RFC 6749 section 5.1).
type tokenJSON struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// mustApprove approves request with this browser's Passkey and returns the
// authorization code the Connector is sent back with.
func (d *browser) mustApprove(request string) string {
	d.nando.t.Helper()
	code := redirectIn(d.nando.t, d.approve(request)).Query().Get("code")
	if code == "" {
		d.nando.t.Fatal("approval sent no code")
	}
	return code
}

// codeExchange is the form a Connector posts to exchange code for tokens.
func codeExchange(client clientJSON, code, verifier string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {client.RedirectURIs[0]},
		"client_id":     {client.ClientID},
		"code_verifier": {verifier},
	}
}

// token posts form to the token endpoint as a Connector does.
func (n *nando) token(form url.Values) *httptest.ResponseRecorder {
	n.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

// tokensIn decodes the tokens a token endpoint response gives, or fails the test.
func tokensIn(t *testing.T, rec *httptest.ResponseRecorder) tokenJSON {
	t.Helper()
	var tokens tokenJSON
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &tokens) != nil {
		t.Fatalf("POST /oauth/token: status = %d, body %q; want %d and tokens", rec.Code, rec.Body, http.StatusOK)
	}
	return tokens
}

// oauthErrorIn is the OAuth error a response gives, or "".
func oauthErrorIn(rec *httptest.ResponseRecorder) string {
	var got oauthErrorJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	return got.Error
}

// approvedConnector registers claude, has the Owner approve it with a
// Passkey, and returns its registration and authorization code.
func approvedConnector(t *testing.T, n *nando) (clientJSON, string) {
	t.Helper()
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	client, request := n.mustAuthorize()
	return client, phone.mustApprove(request)
}

func TestAnApprovedConnectorExchangesItsCodeForTokens(t *testing.T) {
	n := newNando(t)
	client, code := approvedConnector(t, n)

	rec := n.token(codeExchange(client, code, verifier))

	got := tokensIn(t, rec)
	if got.AccessToken == "" || got.RefreshToken == "" || got.AccessToken == got.RefreshToken {
		t.Errorf("tokens = %+v, want an access token and a different refresh token", got)
	}
	if got.TokenType != "Bearer" || got.ExpiresIn != 3600 {
		t.Errorf("token_type %q, expires_in %d; want Bearer, 3600 (an hour)", got.TokenType, got.ExpiresIn)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestACodeExchangeThatDoesNotMatchTheApprovalIsRefused(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	other := n.mustRegister(clientJSON{ClientName: "Other", RedirectURIs: []string{"https://other.example.org/callback"}})

	for name, change := range map[string]func(form url.Values){
		"wrong code_verifier":  func(form url.Values) { form.Set("code_verifier", "wrong-"+verifier) },
		"no code_verifier":     func(form url.Values) { form.Del("code_verifier") },
		"another client":       func(form url.Values) { form.Set("client_id", other.ClientID) },
		"another redirect_uri": func(form url.Values) { form.Set("redirect_uri", "https://claude.ai/elsewhere") },
		"no redirect_uri":      func(form url.Values) { form.Del("redirect_uri") },
		"unknown code":         func(form url.Values) { form.Set("code", "unknown") },
	} {
		client, request := n.mustAuthorize()
		form := codeExchange(client, phone.mustApprove(request), verifier)
		change(form)
		if rec := n.token(form); rec.Code != http.StatusBadRequest || oauthErrorIn(rec) != "invalid_grant" {
			t.Errorf("%s: status = %d, body %q; want 400 invalid_grant", name, rec.Code, rec.Body)
		}
	}
}

func TestACodeIsUsedUpByItsFirstExchange(t *testing.T) {
	n := newNando(t)
	client, code := approvedConnector(t, n)
	tokensIn(t, n.token(codeExchange(client, code, verifier)))

	if rec := n.token(codeExchange(client, code, verifier)); rec.Code != http.StatusBadRequest || oauthErrorIn(rec) != "invalid_grant" {
		t.Errorf("exchange a used code: status = %d, body %q; want 400 invalid_grant", rec.Code, rec.Body)
	}
}

func TestARefusedExchangeAlsoUsesUpTheCode(t *testing.T) {
	n := newNando(t)
	client, code := approvedConnector(t, n)
	n.token(codeExchange(client, code, "wrong-"+verifier))

	if rec := n.token(codeExchange(client, code, verifier)); rec.Code != http.StatusBadRequest || oauthErrorIn(rec) != "invalid_grant" {
		t.Errorf("exchange after a wrong verifier: status = %d, body %q; want 400 invalid_grant", rec.Code, rec.Body)
	}
}

func TestACodeExpiresAfterAMinute(t *testing.T) {
	n := newNando(t)
	client, code := approvedConnector(t, n)

	n.clock.Advance(61 * time.Second)

	if rec := n.token(codeExchange(client, code, verifier)); rec.Code != http.StatusBadRequest || oauthErrorIn(rec) != "invalid_grant" {
		t.Errorf("exchange a code after 61 seconds: status = %d, body %q; want 400 invalid_grant", rec.Code, rec.Body)
	}
}

func TestATokenRequestWithoutAKnownGrantIsRefused(t *testing.T) {
	n := newNando(t)

	for grant, want := range map[string]string{"": "invalid_request", "password": "unsupported_grant_type"} {
		form := url.Values{"grant_type": {grant}, "username": {"owner"}, "password": {"secret"}}
		if rec := n.token(form); rec.Code != http.StatusBadRequest || oauthErrorIn(rec) != want {
			t.Errorf("grant_type %q: status = %d, body %q; want 400 %s", grant, rec.Code, rec.Body, want)
		}
	}
}

// refresh is the form a Connector posts to swap refreshToken for new tokens.
func refresh(client clientJSON, refreshToken string) url.Values {
	return url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {client.ClientID},
	}
}

// mustRefresh swaps refreshToken for new tokens, or fails the test.
func (n *nando) mustRefresh(client clientJSON, refreshToken string) tokenJSON {
	n.t.Helper()
	return tokensIn(n.t, n.token(refresh(client, refreshToken)))
}

// refusedAsInvalidGrant checks that a token request was refused with
// invalid_grant.
func refusedAsInvalidGrant(t *testing.T, what string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest || oauthErrorIn(rec) != "invalid_grant" {
		t.Errorf("%s: status = %d, body %q; want 400 invalid_grant", what, rec.Code, rec.Body)
	}
}

func TestRefreshingGivesTheConnectorNewTokens(t *testing.T) {
	n := newNando(t)
	client, first := connectorWithTokens(t, n)

	second := n.mustRefresh(client, first.RefreshToken)

	if second.AccessToken == first.AccessToken || second.RefreshToken == first.RefreshToken || second.AccessToken == second.RefreshToken {
		t.Errorf("tokens after refreshing = %+v, want a new access and refresh token, not %+v", second, first)
	}
	if second.TokenType != "Bearer" || second.ExpiresIn != 3600 {
		t.Errorf("token_type %q, expires_in %d; want Bearer, 3600", second.TokenType, second.ExpiresIn)
	}
	if rec := n.mcpWithToken(second.AccessToken); rec.Code != http.StatusOK {
		t.Errorf("tools/list with the new access token: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec := n.mcpWithToken(first.AccessToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("tools/list with the replaced access token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestReusingARotatedRefreshTokenRemovesTheConnector(t *testing.T) {
	n := newNando(t)
	client, first := connectorWithTokens(t, n)
	second := n.mustRefresh(client, first.RefreshToken)

	refusedAsInvalidGrant(t, "refresh with the rotated token", n.token(refresh(client, first.RefreshToken)))

	if rec := n.mcpWithToken(second.AccessToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("tools/list once the Connector is removed: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	refusedAsInvalidGrant(t, "refresh once the Connector is removed", n.token(refresh(client, second.RefreshToken)))
}

func TestAnUnknownRefreshTokenIsRefusedWithoutRemovingTheConnector(t *testing.T) {
	n := newNando(t)
	client, tokens := connectorWithTokens(t, n)

	refusedAsInvalidGrant(t, "refresh with an unknown token", n.token(refresh(client, "not-a-token")))

	n.mustRefresh(client, tokens.RefreshToken)
}

func TestARefreshFromAnotherClientIsRefused(t *testing.T) {
	n := newNando(t)
	client, tokens := connectorWithTokens(t, n)
	other := n.mustRegister(clientJSON{ClientName: "Other", RedirectURIs: []string{"https://other.example.org/callback"}})

	refusedAsInvalidGrant(t, "refresh as another client", n.token(refresh(other, tokens.RefreshToken)))

	n.mustRefresh(client, tokens.RefreshToken)
}

func TestARefreshTokenExpiresAfter90DaysUnused(t *testing.T) {
	n := newNando(t)
	client, tokens := connectorWithTokens(t, n)

	n.clock.Advance(89 * 24 * time.Hour)
	tokens = n.mustRefresh(client, tokens.RefreshToken)
	n.clock.Advance(89 * 24 * time.Hour) // each refresh gives another 90 days
	tokens = n.mustRefresh(client, tokens.RefreshToken)

	n.clock.Advance(91 * 24 * time.Hour)
	refusedAsInvalidGrant(t, "refresh after 91 days unused", n.token(refresh(client, tokens.RefreshToken)))
}

// connectorJSON is a Connector as GET /api/connectors lists it.
type connectorJSON struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ApprovedAt time.Time  `json:"approvedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// listConnectors lists the Connectors as the Owner's signed-in browser sees them.
func (d *browser) listConnectors() []connectorJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/connectors", nil)
	var connectors []connectorJSON
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &connectors) != nil {
		d.nando.t.Fatalf("GET /api/connectors: status = %d, body %q", rec.Code, rec.Body)
	}
	return connectors
}

// connect registers client, has the Owner approve it with this browser's
// Passkey and returns its registration and tokens.
func (d *browser) connect(client clientJSON) (clientJSON, tokenJSON) {
	d.nando.t.Helper()
	registered := d.nando.mustRegister(client)
	request := requestOf(d.nando.t, d.nando.authorize(authorizeQuery(registered, "xyz")))
	code := d.mustApprove(request)
	return registered, tokensIn(d.nando.t, d.nando.token(codeExchange(registered, code, verifier)))
}

// sameTime reports whether a time the API answered is want, as MongoDB
// keeps it: to the millisecond.
func sameTime(got time.Time, want time.Time) bool {
	return got.Equal(want.Truncate(time.Millisecond))
}

func TestOwnerSeesEachConnectorAndWhenItWasLastUsed(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	claudeApproved := n.clock.Now()
	_, claudeTokens := phone.connect(claude)
	n.clock.Advance(time.Minute)
	codeApproved := n.clock.Now()
	phone.connect(clientJSON{ClientName: "Claude Code", RedirectURIs: []string{"http://localhost:53682/callback"}})
	n.clock.Advance(time.Minute)
	used := n.clock.Now()
	n.mustConnectMCP(claudeTokens.AccessToken)

	got := phone.listConnectors()

	if len(got) != 2 {
		t.Fatalf("Connectors = %+v, want Claude and Claude Code", got)
	}
	if got[0].Name != "Claude" || !sameTime(got[0].ApprovedAt, claudeApproved) || got[0].LastUsedAt == nil || !sameTime(*got[0].LastUsedAt, used) {
		t.Errorf("first Connector = %+v, want Claude, approved %v, last used %v", got[0], claudeApproved, used)
	}
	if got[1].Name != "Claude Code" || !sameTime(got[1].ApprovedAt, codeApproved) || got[1].LastUsedAt != nil {
		t.Errorf("second Connector = %+v, want Claude Code, approved %v, never used", got[1], codeApproved)
	}
	if got[0].ID == "" || got[0].ID == got[1].ID {
		t.Errorf("Connector ids %q and %q, want two different ones", got[0].ID, got[1].ID)
	}
}

func TestRemovingAConnectorCutsItOffAtOnce(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	client, tokens := phone.connect(claude)
	id := phone.listConnectors()[0].ID

	if rec := phone.do(http.MethodDelete, "/api/connectors/"+id, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the Connector: status = %d, body %q", rec.Code, rec.Body)
	}

	if rec := n.mcpWithToken(tokens.AccessToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("tools/list once removed: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	refusedAsInvalidGrant(t, "refresh once removed", n.token(refresh(client, tokens.RefreshToken)))
	if got := phone.listConnectors(); len(got) != 0 {
		t.Errorf("Connectors once removed = %+v, want none", got)
	}
	// Deleting one that doesn't exist succeeds, like Core's other deletes.
	if rec := phone.do(http.MethodDelete, "/api/connectors/"+id, nil); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE it again: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestConnectorsAreForTheOwnersSessionOnly(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	_, tokens := phone.connect(claude)

	signedOut := n.newBrowser("Mac")
	if rec := signedOut.do(http.MethodGet, "/api/connectors", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/connectors without a Session: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/connectors", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/connectors with a Connector's access token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// stillPending checks that request can still be approved: nothing approved it.
func stillPending(t *testing.T, d *browser, request string) {
	t.Helper()
	if rec := d.do(http.MethodGet, "/auth/connect/"+request, nil); rec.Code != http.StatusOK {
		t.Errorf("GET the request afterwards: status = %d, want %d: it should still be pending", rec.Code, http.StatusOK)
	}
}

func TestApprovingWithoutAPasskeyIsRefused(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll() // signed in, so it has a Session
	_, request := n.mustAuthorize()

	for name, body := range map[string][]byte{"no body": nil, "not a credential": []byte(`{"id":"x"}`)} {
		rec := phone.do(http.MethodPost, "/auth/connect/"+request+"/finish", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("finish with %s, from a browser with a Session: status = %d, body %q; want %d", name, rec.Code, rec.Body, http.StatusBadRequest)
		}
	}
	stillPending(t, phone, request)
}

func TestApprovingWithADeletedPasskeyIsRefused(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	if rec := phone.do(http.MethodDelete, "/api/passkeys/"+phone.listPasskeys()[0].ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the Passkey: status = %d, body %q", rec.Code, rec.Body)
	}
	_, request := n.mustAuthorize()

	if rec := phone.approve(request); rec.Code != http.StatusUnauthorized {
		t.Errorf("approve with the deleted Passkey: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusUnauthorized)
	}
	stillPending(t, phone, request)
}

func TestAPasskeyAnswerForAnotherCeremonyCannotApprove(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	_, request := n.mustAuthorize()
	_, other := n.mustAuthorize()

	for name, beginPath := range map[string]string{
		"a sign-in":                  "/auth/signin/begin",
		"another request's approval": "/auth/connect/" + other + "/begin",
	} {
		options := phone.do(http.MethodPost, beginPath, nil)
		answer := phone.passkey.Get(t, phone.origin, options.Body.Bytes())
		if rec := phone.do(http.MethodPost, "/auth/connect/"+request+"/finish", answer); rec.Code != http.StatusBadRequest {
			t.Errorf("finish with the answer to %s: status = %d, body %q; want %d", name, rec.Code, rec.Body, http.StatusBadRequest)
		}
	}
	stillPending(t, phone, request)
}

func TestAnApprovalCeremonyCannotSignIn(t *testing.T) {
	n := newNando(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	_, request := n.mustAuthorize()
	mac := n.newBrowser("Mac").syncedFrom(phone)

	options := mac.do(http.MethodPost, "/auth/connect/"+request+"/begin", nil)
	rec := mac.do(http.MethodPost, "/auth/signin/finish", mac.passkey.Get(t, mac.origin, options.Body.Bytes()))

	if rec.Code != http.StatusBadRequest || mac.session != nil {
		t.Errorf("sign in with an approval's answer: status = %d, body %q, Session %v; want %d and no Session",
			rec.Code, rec.Body, mac.session != nil, http.StatusBadRequest)
	}
}

func TestReusingACodeRemovesItsConnector(t *testing.T) {
	n := newNando(t)
	client, code := approvedConnector(t, n)
	tokens := tokensIn(t, n.token(codeExchange(client, code, verifier)))

	refusedAsInvalidGrant(t, "exchange the code again", n.token(codeExchange(client, code, verifier)))

	if rec := n.mcpWithToken(tokens.AccessToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("tools/list with the tokens from the reused code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	refusedAsInvalidGrant(t, "refresh with the tokens from the reused code", n.token(refresh(client, tokens.RefreshToken)))
}
