package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

const (
	// claimAtOnce is the most Items one career_list_work claims.
	claimAtOnce = 5
	// claimFor is how long a run holds the Items it claims before another
	// run may take them.
	claimFor = 15 * time.Minute
	// maxClaims is how many times an Item is claimed before it is set aside
	// as failed: that many runs took it and none did it.
	maxClaims = 3
)

// WorkService serves the Tools the cloud run works through its Items with.
type WorkService struct {
	// scheduledTask learns when a run works or stops.
	scheduledTask *coreutils.ScheduledTask
	work          model.WorkRepository
	conversations model.ConversationsRepository
	recruiters    model.RecruitersRepository
	opportunities model.OpportunitiesRepository
	companies     model.CompaniesRepository
	settings      model.SettingsRepository
	profile       model.ProfileRepository
	syncs         model.SyncsRepository
	now           func() time.Time
}

// NewWork returns the WorkService.
func NewWork(scheduledTask *coreutils.ScheduledTask, work model.WorkRepository, conversations model.ConversationsRepository, recruiters model.RecruitersRepository,
	opportunities model.OpportunitiesRepository, companies model.CompaniesRepository,
	settings model.SettingsRepository, profile model.ProfileRepository, syncs model.SyncsRepository, now func() time.Time) *WorkService {
	return &WorkService{
		scheduledTask: scheduledTask, work: work, conversations: conversations, recruiters: recruiters, opportunities: opportunities, companies: companies,
		settings: settings, profile: profile, syncs: syncs, now: now,
	}
}

// Tools are what the cloud run uses to take its work.
func (s *WorkService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_list_work",
		Description: "For the cloud run: claims up to 5 Items of work for 15 minutes, oldest first. " +
			"A sort Item is a Conversation not sorted yet: ignore it with career_ignore_thread if it isn't about a role, " +
			"or file it under its Recruiter with career_file_conversation. " +
			"A reply Item is a filed Conversation with a new message from its Recruiter: update their Opportunities, " +
			"and write the Owner's answer with career_set_suggested_reply. " +
			"Each Item comes with its Conversation's messages, and its Recruiter, if filed. " +
			"The answer also has whether the Owner is Looking, the Career Profile, and liveSince, the time of the first Sync: " +
			"nothing from before it becomes an Opportunity. Call this again until it answers no Items. " +
			"Answers {\"items\": [{\"id\", \"kind\", \"conversation\"}], \"looking\", \"profile\", \"liveSince\"}.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handle:      s.listTool,
	}}
}

func (s *WorkService) listTool(ctx context.Context, _ json.RawMessage) (any, error) {
	if err := s.failUnfinished(ctx); err != nil {
		return nil, err
	}
	items := []model.ItemJSON{}
	for len(items) < claimAtOnce {
		now := s.now()
		item, err := s.work.ClaimNext(ctx, now, now.Add(claimFor), maxClaims)
		if errors.Is(err, coreutils.ErrNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		conversation, err := s.conversations.GetByID(ctx, item.ConversationID)
		if err != nil {
			return nil, err
		}
		j := model.ItemJSON{ID: item.ID.Hex(), Kind: item.Kind, Conversation: conversationJSON(conversation)}
		if conversation.RecruiterID != nil {
			if j.Recruiter, err = s.recruiterWithWork(ctx, *conversation.RecruiterID, conversation.ID); err != nil {
				return nil, err
			}
		}
		items = append(items, j)
	}
	if len(items) == 0 {
		s.scheduledTask.RunStopped(ctx)
	} else {
		s.scheduledTask.RunWorking(ctx)
	}
	settings, err := savedOrDefault(ctx, s.settings)
	if err != nil {
		return nil, err
	}
	profile, err := s.profile.Find(ctx)
	if err != nil {
		return nil, err
	}
	var liveSince *time.Time
	first, err := s.syncs.FirstRead(ctx)
	if err == nil {
		liveSince = &first.At
	} else if !errors.Is(err, coreutils.ErrNotFound) {
		return nil, err
	}
	return model.WorkJSON{Items: items, Looking: settings.Looking, Profile: profile, LiveSince: liveSince}, nil
}

// recruiterWithWork is the Recruiter, with their Conversations other than
// the one with the id item, and their Opportunities.
func (s *WorkService) recruiterWithWork(ctx context.Context, id, item bson.ObjectID) (*model.RecruiterWithWorkJSON, error) {
	recruiter, err := s.recruiters.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	filed, err := s.conversations.ByRecruiter(ctx, id)
	if err != nil {
		return nil, err
	}
	brought, err := s.opportunities.ByRecruiter(ctx, id)
	if err != nil {
		return nil, err
	}
	j := &model.RecruiterWithWorkJSON{RecruiterJSON: recruiterJSON(recruiter), Conversations: []model.ConversationJSON{}, Opportunities: []model.OpportunityJSON{}}
	for _, c := range filed {
		if c.ID != item {
			j.Conversations = append(j.Conversations, conversationJSON(c))
		}
	}
	for _, o := range brought {
		short, err := opportunityJSON(ctx, s.companies, o)
		if err != nil {
			return nil, err
		}
		j.Opportunities = append(j.Opportunities, short)
	}
	return j, nil
}

// RunAtTick is the Module's work at every tick: it sets aside the Items no
// run could do, and wakes the scheduled task while others wait, such as
// those whose run stopped when the Owner's usage ran out.
func (s *WorkService) RunAtTick(ctx context.Context) error {
	if err := s.failUnfinished(ctx); err != nil {
		return err
	}
	return s.scheduledTask.WakeAtTick(ctx)
}

// WorkQueue is the run's Items as the queue the scheduled task works
// through, for Core's utils.NewScheduledTask.
func WorkQueue(work model.WorkRepository) coreutils.Queue {
	return workQueue{work}
}

// workQueue is the Items waiting for, or held by, a run.
type workQueue struct {
	work model.WorkRepository
}

func (q workQueue) AnyClaimable(ctx context.Context, now time.Time) (bool, error) {
	return q.work.AnyClaimable(ctx, now, maxClaims)
}

func (q workQueue) AnyClaimed(ctx context.Context, now time.Time) (bool, error) {
	return q.work.AnyClaimed(ctx, now)
}

// failUnfinished sets aside, by the clock, every Item whose last claim lapsed
// after maxClaims claims, and logs each: no run could do it.
func (s *WorkService) failUnfinished(ctx context.Context) error {
	failed, err := s.work.FailUnfinished(ctx, s.now(), maxClaims)
	for _, item := range failed {
		log.Printf("career: no run did the %s item %s for the Conversation %s in %d claims, so it is set aside as failed",
			item.Kind, item.ID.Hex(), item.ConversationID.Hex(), item.Claims)
	}
	return err
}

// conversationJSON is c as the Tools answer it.
func conversationJSON(c model.Conversation) model.ConversationJSON {
	j := model.ConversationJSON{
		ID:       c.ID.Hex(),
		ThreadID: c.ThreadID,
		URL:      c.URL,
		Person:   model.PersonJSON{Name: c.Person.Name, ProfileURL: c.Person.ProfileURL},
		Messages: []model.MessageJSON{},
		Ignored:  c.Ignored,
	}
	for _, m := range c.Messages {
		j.Messages = append(j.Messages, model.MessageJSON{ID: m.LinkedInID, From: m.From, SentAt: m.SentAt, Text: m.Text})
	}
	if c.RecruiterID != nil {
		id := c.RecruiterID.Hex()
		j.RecruiterID = &id
	}
	if r := c.SuggestedReply; r != nil {
		j.SuggestedReply = &model.SuggestedReplyJSON{
			Text: r.Text, Language: r.Language, Kind: r.Kind, Answers: r.Answers,
			Notes: append([]model.LanguageNote{}, r.Notes...), WrittenAt: r.WrittenAt,
		}
	}
	return j
}
