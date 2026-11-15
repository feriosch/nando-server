package service

import (
	"context"
	"encoding/json"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// firstSyncHour is the hour of the Day, in the Owner Time Zone, before which
// no Sync is due.
const firstSyncHour = 7

// SyncsService serves the Tools the Mac's Sync script calls: whether a Sync
// is due.
type SyncsService struct {
	core coremodel.CoreService
}

// NewSyncs returns the SyncsService.
func NewSyncs(core coremodel.CoreService) *SyncsService {
	return &SyncsService{core: core}
}

// Tools are what the Mac's Sync script calls.
func (s *SyncsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_sync_status",
		Description: "For the Owner's Mac, before a Sync reads LinkedIn: whether a Sync is due now. " +
			"Answers {\"due\": bool}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ReadOnly:    true,
		Handle:      s.statusTool,
	}}
}

func (s *SyncsService) statusTool(ctx context.Context, _ json.RawMessage) (any, error) {
	loc, err := s.core.OwnerTimeZone(ctx)
	if err != nil {
		return nil, err
	}
	now := s.core.Now().In(loc)
	return model.SyncStatusJSON{Due: now.Hour() >= firstSyncHour}, nil
}
