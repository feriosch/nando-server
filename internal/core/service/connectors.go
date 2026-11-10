package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// ConnectorsService lets the Owner see and remove Connectors, under
// /api/connectors. The OAuthService creates them.
type ConnectorsService struct {
	connectors model.ConnectorsRepository
	clients    model.OAuthClientsRepository
	now        func() time.Time
}

// NewConnectors returns the ConnectorsService over connectors and the OAuth
// clients they were approved for, with now as its clock.
func NewConnectors(connectors model.ConnectorsRepository, clients model.OAuthClientsRepository, now func() time.Time) *ConnectorsService {
	return &ConnectorsService{connectors: connectors, clients: clients, now: now}
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
	if err := removeConnector(r.Context(), s.connectors, s.clients, r.PathValue("id"), s.now()); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeConnector removes the Connector with id and so its tokens. If no
// other Connector was approved for its registration, the registration
// expires a day from now, like one never approved. Removing a Connector that
// doesn't exist does nothing.
func removeConnector(
	ctx context.Context,
	connectors model.ConnectorsRepository,
	clients model.OAuthClientsRepository,
	id string,
	now time.Time,
) error {
	connector, err := connectors.Find(ctx, id)
	if errors.Is(err, utils.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := connectors.Delete(ctx, id); err != nil {
		return err
	}
	others, err := connectors.CountForClient(ctx, connector.ClientID)
	if err != nil || others > 0 {
		return err
	}
	return clients.Expire(ctx, connector.ClientID, now.Add(unapprovedClientLifetime))
}
