package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
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
