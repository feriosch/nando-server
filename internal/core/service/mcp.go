package service

import (
	"fmt"
	"net/http"
)

// MCPService serves the Tools of Core and every Module to Connectors over
// MCP at /mcp, for Connector access tokens only.
type MCPService struct {
	// resourceMetadataURL is where a Connector learns how to get a token.
	resourceMetadataURL string
}

// NewMCP returns the MCPService of the API at apiURL, https://api.<domain>.
func NewMCP(apiURL string) *MCPService {
	return &MCPService{
		resourceMetadataURL: apiURL + "/.well-known/oauth-protected-resource",
	}
}

// Routes adds /mcp to mux.
func (s *MCPService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", s.serveMCP)
}

// serveMCP answers only a Connector with a valid access token. No Connector
// can get one yet, so every request is unauthorized.
func (s *MCPService) serveMCP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer resource_metadata=%q", s.resourceMetadataURL))
	http.Error(w, "a Connector access token is required", http.StatusUnauthorized)
}
