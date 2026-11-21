package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// GmailCareerSyncService serves the Tools that record a Gmail Career Sync's
// passes (ADR 0010).
type GmailCareerSyncService struct {
	passes        model.GmailPassesRepository
	conversations model.ConversationsRepository
	now           func() time.Time
}

// NewGmailCareerSync returns the GmailCareerSyncService.
func NewGmailCareerSync(passes model.GmailPassesRepository, conversations model.ConversationsRepository, now func() time.Time) *GmailCareerSyncService {
	return &GmailCareerSyncService{passes: passes, conversations: conversations, now: now}
}

// Tools are what the Career Assistant's run records its passes with.
func (s *GmailCareerSyncService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_start_gmail_career_sync",
		Description: "Starts a Gmail Career Sync's pass (ADR 0010). Answers {\"passId\", \"since\", \"held\"}: read every thread labeled Career that Nando " +
			"doesn't hold yet, whatever its date, and the held ones with mail since since. held lists the Gmail threads Nando holds, each " +
			"{\"threadId\", \"newest\", \"ignored\"}: skip an ignored one. since is when the last finished pass started, " +
			"or 2025-01-01 before any has finished. Finish the pass with career_finish_gmail_career_sync once every thread is read; " +
			"a pass that never finishes doesn't move since, so the next one picks up what it missed.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handle:      s.startTool,
	}, {
		Name:        "career_finish_gmail_career_sync",
		Description: "Finishes a Gmail Career Sync's pass, once every labeled thread it had to read is read. Answers {}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"passId":{"type":"string"}},"required":["passId"]}`),
		Handle:      s.finishTool,
	}}
}

func (s *GmailCareerSyncService) startTool(ctx context.Context, _ json.RawMessage) (any, error) {
	since := model.GmailBacklogStart
	last, err := s.passes.LastFinished(ctx)
	switch {
	case err == nil:
		since = last.StartedAt
	case !errors.Is(err, coreutils.ErrNotFound):
		return nil, err
	}
	held, err := s.conversations.GmailThreads(ctx)
	if err != nil {
		return nil, err
	}
	pass, err := s.passes.Start(ctx, s.now())
	if err != nil {
		return nil, err
	}
	answer := model.GmailPassJSON{PassID: pass.ID.Hex(), Since: since, Held: []model.ThreadJSON{}}
	for _, c := range held {
		answer.Held = append(answer.Held, model.ThreadJSON{ThreadID: c.ThreadID, Newest: c.Newest, Ignored: c.Ignored})
	}
	return answer, nil
}

func (s *GmailCareerSyncService) finishTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		PassID string `json:"passId"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	id, err := careerutils.ID("pass", args.PassID)
	if err != nil {
		return nil, err
	}
	if err := s.passes.Finish(ctx, id, s.now()); err != nil {
		return nil, careerutils.Found(err, "pass", args.PassID)
	}
	return struct{}{}, nil
}
