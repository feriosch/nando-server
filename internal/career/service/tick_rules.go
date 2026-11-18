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
	// Reply: an answer to an older one turns late, and a follow-up to an
	// older one is dropped.
	lateAfter = 30 * 24 * time.Hour
	// followUpAfter is how long after the Owner's last message, with no
	// answer, a follow-up is due.
	followUpAfter = 14 * 24 * time.Hour
)

// TickRulesService applies, at every tick, the rules that need no model:
// it closes the Opportunities that went quiet as faded, turns answers to old
// messages late, drops follow-ups to old messages, and queues follow-ups for
// Opportunities waiting on the Recruiter. What they queue in the run's work,
// the tick then wakes the run for.
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
// message is the Owner's, at least followUpAfter and at most lateAfter old,
// with no Suggested Reply, gets a follow_up Item: one for each such message, ever, even if no
// run writes it. A Suggested Reply the Owner marked replied counts as their
// last message, sent when they marked it, until a Sync brings the real one,
// which the mark's follow_up Item, sent after it, already covers.
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
		ownerLast, ok := ownersLastWord(c)
		// Over lateAfter, makeLate would drop the follow-up at the next tick.
		if !ok || now.Sub(ownerLast) < followUpAfter || now.Sub(ownerLast) > lateAfter || !followedUpBefore(c, ownerLast) {
			continue
		}
		// The Item first: if marking the message then fails, the open Item
		// isn't added twice at the next tick.
		if err := s.work.Add(ctx, model.ItemFollowUp, c.ID, bson.ObjectID{}, now); err != nil {
			return err
		}
		if err := s.conversations.MarkFollowedUp(ctx, c.ID, ownerLast); err != nil {
			return err
		}
	}
	return nil
}

// ownersLastWord is when the Owner last wrote in c, if c is waiting on the
// Recruiter: its last message is the Owner's and it has no Suggested Reply,
// or the Owner marked its Suggested Reply replied, which counts as their
// last message, sent at the mark, until a Sync brings the real one.
func ownersLastWord(c model.Conversation) (time.Time, bool) {
	if c.SuggestedReply != nil {
		return c.SuggestedReply.MarkedRepliedAt, !c.SuggestedReply.MarkedRepliedAt.IsZero()
	}
	last, ok := lastMessage(c)
	return last.SentAt, ok && last.From == model.MessageFromOwner
}

// followedUpBefore reports whether c's last follow_up Item was queued for a
// message sent before sentAt, or none was.
func followedUpBefore(c model.Conversation, sentAt time.Time) bool {
	return c.FollowedUp.Truncate(time.Millisecond).Before(sentAt.Truncate(time.Millisecond))
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

// makeLate turns late every answer to a message older than lateAfter, and
// queues a late_reply Item for the run to rewrite it from the late templates.
// A follow-up to a message that old is dropped instead: a Late Reply
// apologises for the Owner's delay, and a follow-up waits on the Recruiter's.
func (s *TickRulesService) makeLate(ctx context.Context) error {
	now := s.now()
	stale, err := s.conversations.WithWaitingReplyBefore(ctx, now.Add(-lateAfter))
	if err != nil {
		return err
	}
	for _, c := range stale {
		if c.SuggestedReply.Kind == model.ReplyFollowUp {
			dropped, err := s.conversations.DropReply(ctx, c.ID, *c.SuggestedReply)
			if dropped {
				log.Printf("career: the follow-up in Conversation %s answered a message from %s, so it was dropped",
					c.ID.Hex(), c.SuggestedReply.Answers.Format(time.RFC3339))
			}
			if err != nil {
				return err
			}
			continue
		}
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
