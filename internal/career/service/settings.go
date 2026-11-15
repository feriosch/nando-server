package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// SettingsService handles requests about the Career settings: whether the
// Owner is Looking.
type SettingsService struct {
	settings model.SettingsRepository
}

// NewSettings returns the SettingsService.
func NewSettings(settings model.SettingsRepository) *SettingsService {
	return &SettingsService{settings: settings}
}

// Routes adds the settings routes to mux, under /api/career/settings.
func (s *SettingsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/career/settings", s.getSettings)
	mux.HandleFunc("PUT /api/career/settings", s.putSettings)
}

// savedOrDefault returns the saved settings, or the defaults if none were saved.
func (s *SettingsService) savedOrDefault(ctx context.Context) (model.Settings, error) {
	settings, err := s.settings.Find(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.DefaultSettings, nil
	}
	return settings, err
}

func (s *SettingsService) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.savedOrDefault(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.SettingsJSON{Looking: &settings.Looking})
}

// putSettings replaces all of the settings.
func (s *SettingsService) putSettings(w http.ResponseWriter, r *http.Request) {
	var j model.SettingsJSON
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "the body isn't settings", http.StatusBadRequest)
		return
	}
	if j.Looking == nil {
		http.Error(w, "looking must be true or false", http.StatusBadRequest)
		return
	}
	if err := s.settings.Save(r.Context(), model.Settings{Looking: *j.Looking}); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Tools are what a Connector uses to read the settings.
func (s *SettingsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_get_settings",
		Description: "Whether the Owner is Looking for a new role right now. " +
			"It sets the tone of every Suggested Reply. Only the Owner changes it, in Nando. " +
			"Answers {\"looking\": bool}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ReadOnly:    true,
		Handle:      s.getSettingsTool,
	}}
}

func (s *SettingsService) getSettingsTool(ctx context.Context, _ json.RawMessage) (any, error) {
	settings, err := s.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	return model.SettingsJSON{Looking: &settings.Looking}, nil
}
