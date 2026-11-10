package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/core/model"
)

// testTools is a Module's Tool provider, as a test Module supplies one.
type testTools struct{}

func (testTools) Tools() []model.Tool {
	return []model.Tool{
		{
			Name:        "test_echo",
			Description: "Repeats the text it is given.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
			ReadOnly:    true,
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				return map[string]string{"echoed": args.Text}, nil
			},
		},
		{
			Name:        "test_forget",
			Description: "Always fails to forget.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Destructive: true,
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				return nil, errors.New("nothing to forget")
			},
		},
	}
}

// connectorTokens has the Owner approve claude with a Passkey and returns the
// tokens it exchanges its code for.
func connectorTokens(t *testing.T, n *nando) tokenJSON {
	t.Helper()
	_, tokens := connectorWithTokens(t, n)
	return tokens
}

// connectorWithTokens has the Owner approve claude with a Passkey and returns
// its registration and the tokens it exchanges its code for.
func connectorWithTokens(t *testing.T, n *nando) (clientJSON, tokenJSON) {
	t.Helper()
	client, code := approvedConnector(t, n)
	return client, tokensIn(t, n.token(codeExchange(client, code, verifier)))
}

// bearer sends every request with a Connector's access token.
type bearer struct{ token string }

func (b bearer) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

// connectMCP connects to /mcp with accessToken over real HTTP, as the Claude
// app does, using the official MCP client.
func (n *nando) connectMCP(accessToken string) (*mcp.ClientSession, error) {
	n.t.Helper()
	srv := httptest.NewServer(n.handler)
	n.t.Cleanup(srv.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-connector", Version: "1.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             srv.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearer{accessToken}},
		DisableStandaloneSSE: true,
	}, nil)
	if err == nil {
		n.t.Cleanup(func() { _ = session.Close() })
	}
	return session, err
}

// mustConnectMCP connects to /mcp with accessToken, or fails the test.
func (n *nando) mustConnectMCP(accessToken string) *mcp.ClientSession {
	n.t.Helper()
	session, err := n.connectMCP(accessToken)
	if err != nil {
		n.t.Fatalf("connect to /mcp: %v", err)
	}
	return session
}

// mcpWithToken posts an MCP tools/list to /mcp with accessToken, returning the
// raw response.
func (n *nando) mcpWithToken(accessToken string) *httptest.ResponseRecorder {
	n.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

func TestAConnectorListsTheToolsModulesSupply(t *testing.T) {
	n := newNando(t)
	n.core.AddToolProvider(testTools{})
	session := n.mustConnectMCP(connectorTokens(t, n).AccessToken)

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	type tool struct {
		Name, Description string
		InputSchema       string
		ReadOnly          bool
		Destructive       *bool
	}
	var got []tool
	for _, t := range listed.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		got = append(got, tool{t.Name, t.Description, string(schema), t.Annotations.ReadOnlyHint, t.Annotations.DestructiveHint})
	}
	no, yes := false, true
	want := []tool{
		{"test_echo", "Repeats the text it is given.",
			`{"properties":{"text":{"type":"string"}},"required":["text"],"type":"object"}`, true, &no},
		{"test_forget", "Always fails to forget.", `{"type":"object"}`, false, &yes},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tools =\n%+v\nwant\n%+v", got, want)
	}
}

func TestAConnectorCallsATool(t *testing.T) {
	n := newNando(t)
	n.core.AddToolProvider(testTools{})
	session := n.mustConnectMCP(connectorTokens(t, n).AccessToken)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "test_echo",
		Arguments: map[string]any{"text": "こんにちは"},
	})
	if err != nil {
		t.Fatalf("tools/call test_echo: %v", err)
	}

	if result.IsError {
		t.Errorf("test_echo failed: %+v", result.Content)
	}
	if want := map[string]any{"echoed": "こんにちは"}; !reflect.DeepEqual(result.StructuredContent, want) {
		t.Errorf("structuredContent = %#v, want %#v", result.StructuredContent, want)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v, want one text", result.Content)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || text.Text != `{"echoed":"こんにちは"}` {
		t.Errorf("content = %#v, want the result as JSON text", result.Content[0])
	}
}

func TestAToolThatFailsTellsTheConnectorWhy(t *testing.T) {
	n := newNando(t)
	n.core.AddToolProvider(testTools{})
	session := n.mustConnectMCP(connectorTokens(t, n).AccessToken)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "test_forget", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("tools/call test_forget: %v", err)
	}

	if !result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %+v, want an error with one text", result)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || text.Text != "nothing to forget" {
		t.Errorf("content = %#v, want the tool's error", result.Content[0])
	}
}

func TestAnAccessTokenWorksForAnHour(t *testing.T) {
	n := newNando(t)
	tokens := connectorTokens(t, n)

	n.clock.Advance(59 * time.Minute)
	if rec := n.mcpWithToken(tokens.AccessToken); rec.Code != http.StatusOK {
		t.Errorf("tools/list after 59 minutes: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusOK)
	}

	n.clock.Advance(2 * time.Minute)
	rec := n.mcpWithToken(tokens.AccessToken)
	want := `Bearer resource_metadata="` + protectedResourceMetadata + `"`
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != want {
		t.Errorf("tools/list after 61 minutes: status = %d, WWW-Authenticate %q; want %d, %q",
			rec.Code, rec.Header().Get("WWW-Authenticate"), http.StatusUnauthorized, want)
	}
}

func TestAnUnknownTokenIsUnauthorized(t *testing.T) {
	n := newNando(t)
	tokens := connectorTokens(t, n)

	for name, token := range map[string]string{"unknown": "not-a-token", "refresh token": tokens.RefreshToken} {
		if rec := n.mcpWithToken(token); rec.Code != http.StatusUnauthorized {
			t.Errorf("tools/list with the %s: status = %d, want %d", name, rec.Code, http.StatusUnauthorized)
		}
	}
}

// TestAConnectorGoesFromRegistrationToRemoval is the whole life of a
// Connector: it registers, the Owner approves it with a Passkey, it
// exchanges its code with PKCE, lists and calls a Tool, refreshes its
// tokens, and is removed.
func TestAConnectorGoesFromRegistrationToRemoval(t *testing.T) {
	n := newNando(t)
	n.core.AddToolProvider(testTools{})
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	// Claude finds where to register, and registers.
	server := getMetadata[authorizationServerJSON](n, "/.well-known/oauth-authorization-server")
	if server.RegistrationEndpoint != "https://api.example.com/oauth/register" {
		t.Fatalf("registration_endpoint = %q", server.RegistrationEndpoint)
	}
	client := n.mustRegister(claude)

	// It sends the Owner to authorize; the Owner approves with a Passkey.
	request := requestOf(t, n.authorize(authorizeQuery(client, "xyz")))
	back := redirectIn(t, phone.approve(request))
	if back.Query().Get("state") != "xyz" {
		t.Fatalf("approval redirects to %s, want state xyz", back)
	}

	// It exchanges the code with its PKCE verifier.
	tokens := tokensIn(t, n.token(codeExchange(client, back.Query().Get("code"), verifier)))

	// It lists and calls a Tool.
	session := n.mustConnectMCP(tokens.AccessToken)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 2 {
		t.Fatalf("tools/list: %v, %+v", err, listed)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "test_echo", Arguments: map[string]any{"text": "足跡"}})
	if err != nil || result.IsError || !reflect.DeepEqual(result.StructuredContent, map[string]any{"echoed": "足跡"}) {
		t.Fatalf("tools/call test_echo: %v, %+v", err, result)
	}

	// An hour later its access token has expired, so it refreshes.
	n.clock.Advance(time.Hour)
	if rec := n.mcpWithToken(tokens.AccessToken); rec.Code != http.StatusUnauthorized {
		t.Fatalf("tools/list with an hour-old access token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	tokens = n.mustRefresh(client, tokens.RefreshToken)
	if _, err := n.mustConnectMCP(tokens.AccessToken).ListTools(context.Background(), nil); err != nil {
		t.Fatalf("tools/list after refreshing: %v", err)
	}

	// The Owner removes it, and it is cut off.
	connectors := phone.listConnectors()
	if len(connectors) != 1 || connectors[0].Name != "Claude" {
		t.Fatalf("Connectors = %+v, want Claude", connectors)
	}
	if rec := phone.do(http.MethodDelete, "/api/connectors/"+connectors[0].ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE the Connector: status = %d", rec.Code)
	}
	if _, err := n.connectMCP(tokens.AccessToken); err == nil {
		t.Errorf("connect to /mcp after removal: no error, want 401")
	}
	if rec := n.mcpWithToken(tokens.AccessToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("tools/list after removal: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestASessionCookieDoesNotOpenMCP(t *testing.T) {
	n := newNando(t)
	n.core.AddToolProvider(testTools{})
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()

	rec := phone.do(http.MethodPost, "/mcp", []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /mcp with the Session cookie: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusUnauthorized)
	}
}

func TestTheBearerSchemeIgnoresCase(t *testing.T) {
	n := newNando(t)
	tokens := connectorTokens(t, n)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "bearer "+tokens.AccessToken)
	rec := httptest.NewRecorder()

	n.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("tools/list with \"bearer\": status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusOK)
	}
}

// toolsOf is a Tool provider of exactly tools.
type toolsOf []model.Tool

func (p toolsOf) Tools() []model.Tool { return p }

// addingPanics reports whether adding p as a Tool provider panics, and with what.
func addingPanics(n *nando, p model.ToolProvider) (panicked any) {
	defer func() { panicked = recover() }()
	n.core.AddToolProvider(p)
	return nil
}

func TestAToolProviderWithABadToolIsRefusedAtStartup(t *testing.T) {
	n := newNando(t)
	n.core.AddToolProvider(testTools{})
	handle := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	object := json.RawMessage(`{"type":"object"}`)

	for name, tool := range map[string]model.Tool{
		"a name another Tool has": {Name: "test_echo", InputSchema: object, Handle: handle},
		"no name":                 {InputSchema: object, Handle: handle},
		"a schema not an object":  {Name: "test_text", InputSchema: json.RawMessage(`{"type":"string"}`), Handle: handle},
		"no schema":               {Name: "test_none", Handle: handle},
		"no handler":              {Name: "test_idle", InputSchema: object},
	} {
		if addingPanics(n, toolsOf{tool}) == nil {
			t.Errorf("adding a Tool with %s didn't panic", name)
		}
	}

	// The Tools already added are still served, unchanged.
	listed, err := n.mustConnectMCP(connectorTokens(t, n).AccessToken).ListTools(context.Background(), nil)
	if err != nil || len(listed.Tools) != 2 || listed.Tools[0].Description != "Repeats the text it is given." {
		t.Errorf("tools/list after the refused providers: %v, %+v", err, listed)
	}
}
