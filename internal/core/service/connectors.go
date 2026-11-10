package service

import (
	"net/http"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// ConnectorsService lets the Owner see and remove Connectors, under
// /api/connectors. The OAuthService creates them.
type ConnectorsService struct {
	connectors model.ConnectorsRepository
}

// NewConnectors returns the ConnectorsService over connectors.
func NewConnectors(connectors model.ConnectorsRepository) *ConnectorsService {
	return &ConnectorsService{connectors: connectors}
}

// Routes adds the Connectors routes to mux, under /api/.
func (s *ConnectorsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/connectors", s.listConnectors)
	mux.HandleFunc("DELETE /api/connectors/{id}", s.deleteConnector)
}

// listConnectors answers every Connector, oldest approved first.
func (s *ConnectorsService) listConnectors(w http.ResponseWriter, r *http.Request) {
	connectors, err := s.connectors.List(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	list := make([]model.ConnectorJSON, len(connectors))
	for i, c := range connectors {
		list[i] = model.ConnectorJSON{ID: c.ID, Name: c.Name, ApprovedAt: c.ApprovedAt, LastUsedAt: c.LastUsedAt}
	}
	utils.WriteJSON(w, http.StatusOK, list)
}

// deleteConnector removes a Connector and its tokens, so its next call to
// /mcp is unauthorized. Deleting one that doesn't exist also succeeds.
func (s *ConnectorsService) deleteConnector(w http.ResponseWriter, r *http.Request) {
	if err := s.connectors.Delete(r.Context(), r.PathValue("id")); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
