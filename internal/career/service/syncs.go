package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

const (
	// firstSyncHour is the hour of the Day, in the Owner Time Zone, before
	// which no Sync is due.
	firstSyncHour = 7
	// syncSince is the oldest Day whose LinkedIn messages a Sync reads.
	syncSince = "2025-01-01"
	// syncsPerDay is the most Syncs on one Day.
	syncsPerDay = 2
	// syncGap is the least time between two Syncs.
	syncGap = 6 * time.Hour
)

// SyncsService serves the Tools the Mac's Sync script calls: whether a Sync
// is due, and recording what one brought.
type SyncsService struct {
	syncs         model.SyncsRepository
	conversations model.ConversationsRepository
	core          coremodel.CoreService
}

// NewSyncs returns the SyncsService.
func NewSyncs(syncs model.SyncsRepository, conversations model.ConversationsRepository, core coremodel.CoreService) *SyncsService {
	return &SyncsService{syncs: syncs, conversations: conversations, core: core}
}

// Tools are what the Mac's Sync script calls.
func (s *SyncsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_sync_status",
		Description: "For the Owner's Mac, before a Sync reads LinkedIn: whether a Sync is due now, " +
			"the oldest Day whose messages it reads (since, YYYY-MM-DD), and every thread Nando holds " +
			"with when its newest message was sent, so the script opens only threads with newer ones. " +
			"An ignored thread isn't about a role: never send its messages. " +
			"Answers {\"due\": bool, \"since\", \"threads\": [{\"threadId\", \"newest\", \"ignored\"}]}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ReadOnly:    true,
		Handle:      s.statusTool,
	}, {
		Name: "career_record_sync",
		Description: "For the Owner's Mac: records a Sync, with the new messages of each LinkedIn thread it opened. " +
			"Each message is from the owner or the other person, with when it was sent and LinkedIn's id for it if known. " +
			"Messages Nando already holds are skipped. If the script couldn't read LinkedIn, it sends only a problem: " +
			"session_expired when LinkedIn wants the Owner to log in again. " +
			"Answers {\"sync\": id, \"stored\": how many new messages were stored}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"threads":{"type":"array","items":{"type":"object","properties":{` +
			`"threadId":{"type":"string"},` +
			`"url":{"type":"string"},` +
			`"person":{"type":"object","properties":{"name":{"type":"string"},"profileUrl":{"type":"string"}}},` +
			`"messages":{"type":"array","items":{"type":"object","properties":{` +
			`"id":{"type":"string"},` +
			`"from":{"type":"string","enum":["owner","other"]},` +
			`"sentAt":{"type":"string","format":"date-time"},` +
			`"text":{"type":"string"}},` +
			`"required":["from","sentAt","text"]}}},` +
			`"required":["threadId","messages"]}},` +
			`"problem":{"type":"object","description":"Only when the script couldn't read LinkedIn. Such a Sync doesn't count towards when the next is due.","properties":{` +
			`"kind":{"type":"string","enum":["session_expired","other"]},` +
			`"text":{"type":"string","description":"For other: what went wrong."}},` +
			`"required":["kind"]}}}`),
		Handle: s.recordTool,
	}}
}

func (s *SyncsService) statusTool(ctx context.Context, _ json.RawMessage) (any, error) {
	loc, err := s.core.OwnerTimeZone(ctx)
	if err != nil {
		return nil, err
	}
	now := s.core.Now().In(loc)
	due, err := s.due(ctx, now)
	if err != nil {
		return nil, err
	}
	conversations, err := s.conversations.Threads(ctx)
	if err != nil {
		return nil, err
	}
	threads := make([]model.ThreadJSON, 0, len(conversations))
	for _, c := range conversations {
		threads = append(threads, model.ThreadJSON{ThreadID: c.ThreadID, Newest: c.Newest, Ignored: c.Ignored})
	}
	return model.SyncStatusJSON{
		Due:     due,
		Since:   syncSince,
		Threads: threads,
	}, nil
}

// due reports whether a Sync is due at now, a time in the Owner Time Zone.
func (s *SyncsService) due(ctx context.Context, now time.Time) (bool, error) {
	if now.Hour() < firstSyncHour {
		return false, nil
	}
	// The newest syncsPerDay Syncs settle both other rules: the newest must
	// be syncGap ago, and the oldest of them must be before today.
	latest, err := s.syncs.LatestRead(ctx, syncsPerDay)
	if err != nil {
		return false, err
	}
	if len(latest) > 0 && now.Sub(latest[0].At) < syncGap {
		return false, nil
	}
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if len(latest) == syncsPerDay && !latest[syncsPerDay-1].At.Before(startOfDay) {
		return false, nil
	}
	return true, nil
}

func (s *SyncsService) recordTool(ctx context.Context, args json.RawMessage) (any, error) {
	var j model.RecordSyncJSON
	if err := json.Unmarshal(args, &j); err != nil {
		return nil, fmt.Errorf("the arguments aren't a Sync: %w", err)
	}
	// Check everything before writing anything, so a refused Sync stores
	// nothing and the script can send it again.
	updates, problem, err := fromRecordSyncJSON(j)
	if err != nil {
		return nil, err
	}
	stored := 0
	for _, update := range updates {
		n, err := s.conversations.AddMessages(ctx, update)
		if err != nil {
			return nil, err
		}
		stored += n
	}
	id, err := s.syncs.Insert(ctx, model.Sync{At: s.core.Now(), Threads: len(updates), Problem: problem})
	if err != nil {
		return nil, err
	}
	return model.RecordedSyncJSON{Sync: id.Hex(), Stored: stored}, nil
}

// fromRecordSyncJSON checks what the Sync script sent, or explains what's
// wrong with it. The Tool's input schema isn't enforced, so this is the only
// check.
func fromRecordSyncJSON(j model.RecordSyncJSON) ([]model.ThreadUpdate, *model.SyncProblem, error) {
	var problem *model.SyncProblem
	if j.Problem != nil {
		if j.Problem.Kind != model.ProblemSessionExpired && j.Problem.Kind != model.ProblemOther {
			return nil, nil, fmt.Errorf("problem.kind must be %s or %s", model.ProblemSessionExpired, model.ProblemOther)
		}
		problem = &model.SyncProblem{Kind: j.Problem.Kind, Text: j.Problem.Text}
	}
	updates := make([]model.ThreadUpdate, 0, len(j.Threads))
	for _, t := range j.Threads {
		if t.ThreadID == "" {
			return nil, nil, errors.New("every thread needs its threadId")
		}
		update := model.ThreadUpdate{
			ThreadID: t.ThreadID,
			URL:      t.URL,
			Person:   model.Person{Name: t.Person.Name, ProfileURL: t.Person.ProfileURL},
		}
		for _, m := range t.Messages {
			if m.From != model.MessageFromOwner && m.From != model.MessageFromOther {
				return nil, nil, fmt.Errorf("thread %s: a message's from must be %s or %s", t.ThreadID, model.MessageFromOwner, model.MessageFromOther)
			}
			if m.SentAt.IsZero() {
				return nil, nil, fmt.Errorf("thread %s: every message needs its sentAt", t.ThreadID)
			}
			update.Messages = append(update.Messages, model.Message{LinkedInID: m.ID, From: m.From, SentAt: m.SentAt, Text: m.Text})
		}
		updates = append(updates, update)
	}
	return updates, problem, nil
}
