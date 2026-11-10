package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// MCPService serves the Tools of Core and every Module to Connectors over
// MCP at /mcp, for Connector access tokens only. It is stateless, as Cloud
// Run may send each request to a different instance.
type MCPService struct {
	connectors model.ConnectorsRepository
	providers  []model.ToolProvider
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

// AddToolProvider has /mcp serve p's Tools.
func (s *MCPService) AddToolProvider(p model.ToolProvider) {
	s.providers = append(s.providers, p)
}

// Routes adds /mcp to mux.
func (s *MCPService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.serveMCP)
}

// serveMCP answers only a Connector with an unexpired access token, checked
// against Core's clock. Anyone else is told where to get one.
func (s *MCPService) serveMCP(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		s.unauthorized(w)
		return
	}
	_, err := s.connectors.FindByAccessToken(r.Context(), sha256Of(token), s.now())
	if errors.Is(err, utils.ErrNotFound) {
		s.unauthorized(w)
		return
	}
	if err != nil {
		utils.ServerError(w, err)
		return
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
	for _, p := range s.providers {
		for _, tool := range p.Tools() {
			destructive := tool.Destructive
			server.AddTool(&mcp.Tool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
				Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.ReadOnly, DestructiveHint: &destructive},
			}, handleTool(tool))
		}
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
