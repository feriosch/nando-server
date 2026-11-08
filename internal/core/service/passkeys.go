package service

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
)

// PasskeysService handles requests about the Owner's passkeys. Registering
// one and signing in with one are the IdentityService's.
type PasskeysService struct {
	passkeys model.PasskeysRepository
}

// NewPasskeys returns the PasskeysService.
func NewPasskeys(passkeys model.PasskeysRepository) *PasskeysService {
	return &PasskeysService{passkeys: passkeys}
}

// Routes adds the passkey routes to mux, under /api/passkeys.
func (s *PasskeysService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/passkeys", s.listPasskeys)
	mux.HandleFunc("DELETE /api/passkeys/{id}", s.deletePasskey)
}

// listPasskeys lists the Owner's passkeys.
func (s *PasskeysService) listPasskeys(w http.ResponseWriter, r *http.Request) {
	passkeys, err := s.passkeys.List(r.Context())
	if err != nil {
		ServerError(w, err)
		return
	}
	type passkeyJSON struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"createdAt"`
	}
	list := make([]passkeyJSON, len(passkeys))
	for i, passkey := range passkeys {
		list[i] = passkeyJSON{ID: base64.RawURLEncoding.EncodeToString(passkey.ID), CreatedAt: passkey.CreatedAt}
	}
	WriteJSON(w, http.StatusOK, list)
}

// deletePasskey stops a passkey signing in. Devices already signed in with it
// stay signed in until their sessions are deleted.
func (s *PasskeysService) deletePasskey(w http.ResponseWriter, r *http.Request) {
	id, err := base64.RawURLEncoding.DecodeString(r.PathValue("id"))
	if err != nil {
		http.Error(w, "that is not a passkey id", http.StatusBadRequest)
		return
	}
	if err := s.passkeys.Delete(r.Context(), id); err != nil {
		ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
