package service

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
)

const (
	// fadesAfter is how long an open Opportunity stays open with no new
	// message in its Recruiter's Conversations and no change through the
	// Tools.
	fadesAfter = 30 * 24 * time.Hour
	// lateAfter is how old a message is when its answer becomes a Late
	// Reply: an answer or follow-up to an older one turns late.
	lateAfter = 30 * 24 * time.Hour
	// followUpAfter is how long after the Owner's last message, with no
	// answer, a follow-up is due.
	followUpAfter = 14 * 24 * time.Hour
)

// TickRulesService applies, at every tick, the rules that need no model:
// it closes the Opportunities that went quiet as faded, turns Suggested
// Replies to old messages late, and queues follow-ups for Opportunities
// waiting on the Recruiter. What they queue in the run's work, the tick
// then wakes the run for.
type TickRulesService struct {
	opportunities model.OpportunitiesRepository
	conversations model.ConversationsRepository
	work          model.WorkRepository
	now           func() time.Time
}

// NewTickRules returns the TickRulesService.
func NewTickRules(opportunities model.OpportunitiesRepository, conversations model.ConversationsRepository, work model.WorkRepository,
	now func() time.Time) *TickRulesService {
	return &TickRulesService{opportunities: opportunities, conversations: conversations, work: work, now: now}
}

// Apply applies every rule once, by the clock.
func (s *TickRulesService) Apply(ctx context.Context) error {
	if err := s.makeLate(ctx); err != nil {
		return err
	}
	open, err := s.opportunities.Open(ctx)
	if err != nil {
		return err
	}
	for _, o := range open {
		if err := s.fadeOrFollowUp(ctx, o); err != nil {
			return err
		}
	}
	return nil
}

// fadeOrFollowUp closes the open Opportunity o as faded if it was quiet for
// fadesAfter: no new message in its Recruiter's Conversations and no change
// through the Tools, or, with no Recruiter, only no change. Otherwise, in
// process or at an offer, each of its Recruiter's Conversations whose last
// message is the Owner's, at least followUpAfter old, with no Suggested
// Reply, gets a follow_up Item: one for each such message, ever, even if no
// run writes it.
func (s *TickRulesService) fadeOrFollowUp(ctx context.Context, o model.Opportunity) error {
	var filed []model.Conversation
	if o.RecruiterID != nil {
		var err error
		if filed, err = s.conversations.ByRecruiter(ctx, *o.RecruiterID); err != nil {
			return err
		}
	}
	now := s.now()
	last := o.UpdatedAt
	for _, c := range filed {
		if c.Newest.After(last) {
			last = c.Newest
		}
	}
	if now.Sub(last) >= fadesAfter {
		faded, err := s.opportunities.Fade(ctx, o)
		if faded {
			log.Printf("career: the Opportunity %s had no message or change since %s, so it faded", o.ID.Hex(), last.Format(time.RFC3339))
		}
		return err
	}
	if o.Stage != model.StageInProcess && o.Stage != model.StageOffer {
		return nil
	}
	for _, c := range filed {
		last, ok := lastMessage(c)
		if !ok || last.From != model.MessageFromOwner || now.Sub(last.SentAt) < followUpAfter ||
			c.SuggestedReply != nil || sameTime(c.FollowedUp, last.SentAt) {
			continue
		}
		// The Item first: if marking the message then fails, the open Item
		// isn't added twice at the next tick.
		if err := s.work.Add(ctx, model.ItemFollowUp, c.ID, bson.ObjectID{}, now); err != nil {
			return err
		}
		if err := s.conversations.MarkFollowedUp(ctx, c.ID, last.SentAt); err != nil {
			return err
		}
	}
	return nil
}

// lastMessage is c's newest message, if it has any.
func lastMessage(c model.Conversation) (model.Message, bool) {
	if len(c.Messages) == 0 {
		return model.Message{}, false
	}
	last := c.Messages[0]
	for _, m := range c.Messages[1:] {
		if m.SentAt.After(last.SentAt) {
			last = m
		}
	}
	return last, true
}

// makeLate turns late every answer or follow-up to a message older than
// lateAfter, and queues a late_reply Item for the run to rewrite it from the
// late templates.
func (s *TickRulesService) makeLate(ctx context.Context) error {
	now := s.now()
	stale, err := s.conversations.WithWaitingReplyBefore(ctx, now.Add(-lateAfter))
	if err != nil {
		return err
	}
	for _, c := range stale {
		// Turned late first, and queued only if it was: a reply the run
		// rewrote meanwhile isn't sent back to be rewritten again.
		late, err := s.conversations.MakeReplyLate(ctx, c.ID, *c.SuggestedReply)
		if err != nil {
			return err
		}
		if !late {
			continue
		}
		if err := s.work.Add(ctx, model.ItemLateReply, c.ID, bson.ObjectID{}, now); err != nil {
			return err
		}
	}
	return nil
}
