package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
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
	// syncBatchWindow is how long after a Sync's first batch its later
	// batches may still be added to it.
	syncBatchWindow = 10 * time.Minute
)

// SyncsService serves the Tools the Mac's Sync script calls: whether a Sync
// is due, and recording what one brought.
type SyncsService struct {
	// scheduledTask is woken when a Sync stores new messages.
	scheduledTask *coreutils.ScheduledTask
	syncs         model.SyncsRepository
	conversations model.ConversationsRepository
	work          model.WorkRepository
	core          coremodel.CoreService
}

// NewSyncs returns the SyncsService.
func NewSyncs(scheduledTask *coreutils.ScheduledTask, syncs model.SyncsRepository, conversations model.ConversationsRepository,
	work model.WorkRepository, core coremodel.CoreService) *SyncsService {
	return &SyncsService{scheduledTask: scheduledTask, syncs: syncs, conversations: conversations, work: work, core: core}
}

// Tools are what the Mac's Sync script calls.
func (s *SyncsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_sync_status",
		Description: "For the Owner's Mac, before a Sync reads LinkedIn: whether a Sync is due now, " +
			"the oldest Day whose messages it reads (since, YYYY-MM-DD), and every thread Nando holds " +
			"with when its newest message was sent, so the script opens only threads with newer ones. " +
			"An ignored thread isn't about a role: never send its messages. " +
			"readThrough is false when the newest Sync stopped early or reported a problem: then also open the held threads " +
			"after one with nothing new, as that Sync may not have reached them. " +
			"Answers {\"due\": bool, \"since\", \"threads\": [{\"threadId\", \"newest\", \"ignored\"}], \"readThrough\": bool}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		ReadOnly:    true,
		Handle:      s.statusTool,
	}, {
		Name: "career_record_sync",
		Description: "For the Owner's Mac: records a Sync, with the new messages of each LinkedIn thread it opened. " +
			"Each message is from the owner or the other person, with when it was sent and LinkedIn's id for it if known. " +
			"Messages Nando already holds are skipped. If the script couldn't read LinkedIn, it sends only a problem: " +
			"session_expired when LinkedIn wants the Owner to log in again. " +
			"A Sync sent in batches is one Sync: send each batch after the first, within 10 minutes of it, with sync, the id the first was answered with. " +
			"If it stopped before reading every thread it had to, at its cap or at a problem, send stoppedEarly with the first batch. " +
			"Answers {\"sync\": id, \"stored\": how many new messages were stored}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"sync":{"type":"string","description":"On every batch after a Sync's first, within 10 minutes of it: the id the first was answered with, so the batches count as one Sync."},` +
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
			`"required":["kind"]},` +
			`"stoppedEarly":{"type":"boolean","description":"On a Sync's first batch only: true when the script stopped before reading every thread it had to, so that the next Sync reads the threads this one may not have reached."}}}`),
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
	readThrough, err := s.readThrough(ctx)
	if err != nil {
		return nil, err
	}
	return model.SyncStatusJSON{
		Due:         due,
		Since:       syncSince,
		Threads:     threads,
		ReadThrough: readThrough,
	}, nil
}

// readThrough reports whether the newest Sync read every thread it had to. The
// script skips the held threads after one with nothing new only then: the
// inbox is in order of activity, but a Sync that stopped early may have taken
// a newer message from a thread above one it never reached.
func (s *SyncsService) readThrough(ctx context.Context) (bool, error) {
	latest, err := s.syncs.Latest(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return latest.Problem == nil && !latest.StoppedEarly, nil
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
	// A later batch of a Sync adds to it, so that a Sync sent in batches
	// counts once towards when the next is due.
	// A new Sync's id is chosen now, so that its messages and Items record
	// it, and stored after them.
	id, isNew := bson.NewObjectID(), j.Sync == ""
	if !isNew {
		if id, err = s.batchOf(ctx, j.Sync); err != nil {
			return nil, err
		}
	}
	stored := 0
	for _, update := range updates {
		for i := range update.Messages {
			update.Messages[i].SyncID = id
		}
		added, err := s.conversations.AddMessages(ctx, update)
		if err != nil {
			return nil, err
		}
		stored += added.Stored
		if err := s.queue(ctx, added, id); err != nil {
			return nil, err
		}
	}
	if isNew {
		err = s.syncs.Insert(ctx, model.Sync{ID: id, At: s.core.Now(), Threads: len(updates), Problem: problem, StoppedEarly: j.StoppedEarly})
	} else {
		err = s.syncs.AddThreads(ctx, id, len(updates))
	}
	if err != nil {
		return nil, err
	}
	if stored > 0 {
		s.scheduledTask.Wake(ctx)
	}
	return model.RecordedSyncJSON{Sync: id.Hex(), Stored: stored}, nil
}

// batchOf checks that sync, the id a later batch sends, is a Sync of the same
// run: one that read LinkedIn, started at most syncBatchWindow ago. A later
// run that sends an old id would otherwise never count.
func (s *SyncsService) batchOf(ctx context.Context, sync string) (bson.ObjectID, error) {
	id, err := careerutils.ID("Sync", sync)
	if err != nil {
		return bson.ObjectID{}, err
	}
	found, err := s.syncs.Read(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return bson.ObjectID{}, fmt.Errorf("no Sync that read LinkedIn has the id %q: send the first batch without sync", sync)
	}
	if err != nil {
		return bson.ObjectID{}, err
	}
	if s.core.Now().Sub(found.At) > syncBatchWindow {
		return bson.ObjectID{}, fmt.Errorf("sync %s started over %.0f minutes ago: a new run sends its first batch without sync", sync, syncBatchWindow.Minutes())
	}
	return found.ID, nil
}

// queue puts what the run has to do about a Conversation's new messages in
// its work, as queued by the Sync with syncID: sort a Conversation not filed
// yet, or answer its Recruiter's new message.
func (s *SyncsService) queue(ctx context.Context, added model.Added, syncID bson.ObjectID) error {
	switch {
	case added.Stored == 0 || added.Ignored:
		return nil
	case !added.Filed:
		return s.work.Add(ctx, model.ItemSort, added.ConversationID, syncID, s.core.Now())
	case added.FromOther:
		return s.work.Add(ctx, model.ItemReply, added.ConversationID, syncID, s.core.Now())
	}
	return nil
}

// fromRecordSyncJSON checks what the Sync script sent, or explains what's
// wrong with it. The Tool's input schema isn't enforced, so this is the only
// check.
func fromRecordSyncJSON(j model.RecordSyncJSON) ([]model.ThreadUpdate, *model.SyncProblem, error) {
	var problem *model.SyncProblem
	if j.StoppedEarly && j.Sync != "" {
		return nil, nil, errors.New("stoppedEarly is sent with a Sync's first batch, which starts it: send it without sync")
	}
	if j.Problem != nil {
		if j.Sync != "" {
			return nil, nil, errors.New("a problem is a Sync of its own: send it without sync")
		}
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
