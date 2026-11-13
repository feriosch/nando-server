package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// MCPService serves the Tools of Core and every Module to Connectors over
// MCP at /mcp, for Connector access tokens only, and keeps what each Tool
// provider says its Tools let a Connector do, for the consent page. It is
// stateless, as Cloud Run may send each request to a different instance.
type MCPService struct {
	connectors model.ConnectorsRepository
	// tools are every provider's Tools, checked when added.
	tools []model.Tool
	// toolsDescriptions are what each provider with Tools says they let a
	// Connector do, in the order the providers were added.
	toolsDescriptions []string
	// handler is the MCP Go SDK's Streamable HTTP transport.
	handler http.Handler
	// resourceMetadataURL is where a Connector learns how to get a token.
	resourceMetadataURL string
	now                 func() time.Time
}

// NewMCP returns the MCPService of the API at apiURL, https://api.<domain>,
// with now as its clock.
func NewMCP(connectors model.ConnectorsRepository, apiURL string, now func() time.Time) *MCPService {
	s := &MCPService{
		connectors:          connectors,
		resourceMetadataURL: apiURL + "/.well-known/oauth-protected-resource",
		now:                 now,
	}
	s.handler = mcp.NewStreamableHTTPHandler(s.server, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return s
}

// AddToolProvider has /mcp serve p's Tools, and the consent page say what
// they let a Connector do. Adding is done at startup, so a provider that
// can't be served panics then, rather than on every request: one with Tools
// but no description of them, which the Owner would approve unawares, or a
// Tool without a name, a handler or an object schema, or whose name another
// Tool has, which would silently replace it. Nothing of p is added then.
func (s *MCPService) AddToolProvider(p model.ToolProvider) {
	tools := p.Tools()
	if len(tools) == 0 {
		return
	}
	description := strings.TrimSpace(p.ToolsDescription())
	if description == "" {
		panic(errors.New("AddToolProvider: Tools without a description of what they let a Connector do"))
	}
	names := map[string]bool{}
	for _, t := range s.tools {
		names[t.Name] = true
	}
	for _, t := range tools {
		if err := checkTool(t); err != nil {
			panic(fmt.Errorf("AddToolProvider: %w", err))
		}
		if names[t.Name] {
			panic(fmt.Errorf("AddToolProvider: two Tools are named %q", t.Name))
		}
		names[t.Name] = true
	}
	s.tools = append(s.tools, tools...)
	s.toolsDescriptions = append(s.toolsDescriptions, description)
}

// ToolsDescriptions returns what each provider with Tools says they let a
// Connector do, in the order the providers were added.
func (s *MCPService) ToolsDescriptions() []string {
	return append([]string{}, s.toolsDescriptions...)
}

// checkTool reports what makes t impossible to serve.
func checkTool(t model.Tool) error {
	var schema struct {
		Type string `json:"type"`
	}
	switch {
	case t.Name == "":
		return errors.New("a Tool has no name")
	case t.Handle == nil:
		return fmt.Errorf("Tool %q has no handler", t.Name)
	case json.Unmarshal(t.InputSchema, &schema) != nil || schema.Type != "object":
		return fmt.Errorf(`Tool %q's input schema must be a JSON Schema of type "object"`, t.Name)
	}
	return nil
}

// Routes adds /mcp to mux.
func (s *MCPService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.serveMCP)
}

// serveMCP answers only a Connector with an unexpired access token, checked
// against Core's clock, and records when it was last used. Anyone else is
// told where to get a token.
func (s *MCPService) serveMCP(w http.ResponseWriter, r *http.Request) {
	// The scheme is case-insensitive (RFC 7235 section 2.1).
	scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Bearer") || token == "" {
		s.unauthorized(w)
		return
	}
	now := s.now()
	connector, err := s.connectors.FindByAccessToken(r.Context(), sha256Of(token), now)
	if errors.Is(err, utils.ErrNotFound) {
		s.unauthorized(w)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	// Recording when it was last used is bookkeeping: failing at it doesn't
	// refuse a Connector that is let in.
	if err := s.connectors.MarkUsed(r.Context(), connector.ID, now); err != nil {
		log.Printf("record Connector %s as used: %v", connector.ID, err)
	}
	s.handler.ServeHTTP(w, r)
}

// unauthorized answers 401 with where to start OAuth (RFC 9728 section 5.1).
func (s *MCPService) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q", s.resourceMetadataURL))
	http.Error(w, "a Connector access token is required", http.StatusUnauthorized)
}

// server is the MCP server for one request, with every provider's Tools.
func (s *MCPService) server(*http.Request) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "nando", Version: "1.0"}, nil)
	for _, tool := range s.tools {
		destructive := tool.Destructive
		server.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.ReadOnly, DestructiveHint: &destructive},
		}, handleTool(tool))
	}
	return server
}

// handleTool runs tool for an MCP tools/call. Its result goes back as JSON
// text and, when it is a JSON object, as structured content too; its error
// goes back as the Tool's failure, for the Connector's model to read.
func handleTool(tool model.Tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		arguments := req.Params.Arguments
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		result, err := tool.Handle(ctx, arguments)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		var text bytes.Buffer
		encoder := json.NewEncoder(&text)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(result); err != nil {
			return nil, fmt.Errorf("tool %s: %w", tool.Name, err)
		}
		encoded := bytes.TrimSpace(text.Bytes())
		answer := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}
		if bytes.HasPrefix(encoded, []byte("{")) {
			answer.StructuredContent = json.RawMessage(encoded)
		}
		return answer, nil
	}
}
