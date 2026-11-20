package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// ConversationsService serves the Tools that sort Conversations: ignoring a
// thread that isn't about a role, or filing it under its Recruiter. It also
// serves the app's mark that the Owner replied.
type ConversationsService struct {
	// scheduledTask learns when a run did an Item.
	scheduledTask *coreutils.ScheduledTask
	conversations model.ConversationsRepository
	recruiters    model.RecruitersRepository
	work          model.WorkRepository
	now           func() time.Time
}

// NewConversations returns the ConversationsService.
func NewConversations(scheduledTask *coreutils.ScheduledTask, conversations model.ConversationsRepository, recruiters model.RecruitersRepository,
	work model.WorkRepository, now func() time.Time) *ConversationsService {
	return &ConversationsService{scheduledTask: scheduledTask, conversations: conversations, recruiters: recruiters, work: work, now: now}
}

// complete does the Conversation's open Items of kinds: a run may list the
// work again after it.
func (s *ConversationsService) complete(ctx context.Context, id bson.ObjectID, kinds ...string) error {
	if err := s.work.Complete(ctx, id, kinds, s.now()); err != nil {
		return err
	}
	s.scheduledTask.RunWorking(ctx)
	return nil
}

// Routes adds the routes that mark a Conversation's Suggested Reply replied
// to mux, under /api/career/conversations.
func (s *ConversationsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/career/conversations/{id}/replied", s.markReplied)
	mux.HandleFunc("DELETE /api/career/conversations/{id}/replied", s.unmarkReplied)
}

// unmarkReplied is Undo: it clears the mark on the Conversation's Suggested
// Reply, if any. A follow_up Item the tick queued for the mark is done with
// it: the Owner hasn't sent the reply after all.
func (s *ConversationsService) unmarkReplied(w http.ResponseWriter, r *http.Request) {
	conversation, ok := careerutils.FromPath(w, r, "Conversation", s.conversations.GetByID)
	if !ok {
		return
	}
	if reply := conversation.SuggestedReply; reply != nil && !reply.MarkedRepliedAt.IsZero() {
		if err := s.conversations.UnmarkReplied(r.Context(), conversation.ID); err != nil {
			coreutils.ServerError(w, err)
			return
		}
		// Not s.complete: no run did it, so the run's activity is unchanged.
		if err := s.work.Complete(r.Context(), conversation.ID, []string{model.ItemFollowUp}, s.now()); err != nil {
			coreutils.ServerError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// markReplied marks the Conversation's Suggested Reply replied, now: the
// Owner sent it in LinkedIn.
func (s *ConversationsService) markReplied(w http.ResponseWriter, r *http.Request) {
	conversation, ok := careerutils.FromPath(w, r, "Conversation", s.conversations.GetByID)
	if !ok {
		return
	}
	if conversation.SuggestedReply == nil {
		http.Error(w, "that Conversation has no Suggested Reply", http.StatusConflict)
		return
	}
	if err := s.conversations.MarkReplied(r.Context(), conversation.ID, *conversation.SuggestedReply, s.now()); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Tools are what the run, and the Owner's chat, sort Conversations with.
func (s *ConversationsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_ignore_thread",
		Description: "Marks a Conversation as a thread that isn't about a role, such as a friend's message or a sales pitch. " +
			"Its messages, person and URL are deleted and only its LinkedIn thread ID stays, so later Syncs drop its messages. " +
			"It does the Conversation's sort Item. Answers the Conversation as it now is.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"conversationId":{"type":"string"}},` +
			`"required":["conversationId"]}`),
		Handle: s.ignoreTool,
	}, {
		Name: "career_file_conversation",
		Description: "Files a Conversation about roles under its Recruiter: an existing one (recruiterId), " +
			"or a new one (recruiter: name, LinkedIn profileUrl, kind agency or in_house, and organisation, their agency or Company). " +
			"Look for an existing Recruiter first, with career_search or the Recruiter on the run's Items. " +
			"A Recruiter with no profileUrl takes the Conversation's person.profileUrl, if it has one; one they have is kept. " +
			"It does the Conversation's sort Item. Answers {\"conversation\", \"recruiter\"}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"conversationId":{"type":"string"},` +
			`"recruiterId":{"type":"string","description":"An existing Recruiter."},` +
			`"recruiter":{"type":"object","description":"A new Recruiter, when there is no recruiterId.","properties":{` +
			`"name":{"type":"string"},` +
			`"profileUrl":{"type":"string"},` +
			`"kind":{"type":"string","enum":["agency","in_house"]},` +
			`"organisation":{"type":"string"}},` +
			`"required":["name","kind"]}},` +
			`"required":["conversationId"]}`),
		Handle: s.fileTool,
	}, {
		Name: "career_get_conversation",
		Description: "A Conversation, with its messages, the Recruiter it is filed under (recruiterId) and its current Suggested Reply. " +
			"A Suggested Reply's markedRepliedAt is when the Owner marked it replied: they already sent it in LinkedIn, and no Sync has brought it yet. " +
			"Answers the Conversation.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		ReadOnly:    true,
		Handle:      s.getTool,
	}, {
		Name: "career_set_suggested_reply",
		Description: "Sets a Conversation's Suggested Reply, Claude's draft of the Owner's next message, replacing any earlier one. " +
			"Only the Owner ever sends it. language is ja or en, the language the Recruiter wrote in. " +
			"kind is answer (to the Recruiter's last message), follow_up (after 14 days of silence following the Owner's message) " +
			"or late (to a Recruiter's message more than 30 days old). answers is the sentAt of the message it answers, as the Conversation shows it; " +
			"a follow_up may instead answer the current Suggested Reply's markedRepliedAt, which stands for the Owner's message until a Sync brings it. " +
			"notes are up to 3 real mistakes in the Owner's last message: what they wrote, what is natural, and why. " +
			"It is removed once a Sync brings a newer message from the Owner. It does the Conversation's reply, late_reply or follow_up Item. " +
			"Answers the Conversation.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"conversationId":{"type":"string"},` +
			`"text":{"type":"string"},` +
			`"language":{"type":"string","enum":["ja","en"]},` +
			`"kind":{"type":"string","enum":["answer","follow_up","late"]},` +
			`"answers":{"type":"string","format":"date-time","description":"The sentAt of the message it answers."},` +
			`"notes":{"type":"array","maxItems":3,"items":{"type":"object","properties":{` +
			`"wrote":{"type":"string"},"natural":{"type":"string"},"why":{"type":"string"}},` +
			`"required":["wrote","natural","why"]}}},` +
			`"required":["conversationId","text","language","kind","answers"]}`),
		Handle: s.setSuggestedReplyTool,
	}}
}

func (s *ConversationsService) setSuggestedReplyTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ConversationID string               `json:"conversationId"`
		Text           string               `json:"text"`
		Language       string               `json:"language"`
		Kind           string               `json:"kind"`
		Answers        time.Time            `json:"answers"`
		Notes          []model.LanguageNote `json:"notes"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	id, err := careerutils.ID("Conversation", args.ConversationID)
	if err != nil {
		return nil, err
	}
	held, err := s.conversations.GetByID(ctx, id)
	if err != nil {
		return nil, careerutils.Found(err, "Conversation", args.ConversationID)
	}
	reply := model.SuggestedReply{
		Text: strings.TrimSpace(args.Text), Language: args.Language, Kind: args.Kind, Answers: args.Answers,
		Notes: []model.LanguageNote{}, WrittenAt: s.now(),
	}
	switch {
	case reply.Text == "":
		return nil, errors.New("a Suggested Reply needs its text")
	case reply.Language != model.LanguageJA && reply.Language != model.LanguageEN:
		return nil, errors.New("a Suggested Reply's language is ja or en")
	case !slices.Contains([]string{model.ReplyAnswer, model.ReplyFollowUp, model.ReplyLate}, reply.Kind):
		return nil, errors.New("a Suggested Reply's kind is answer, follow_up or late")
	case !slices.ContainsFunc(held.Messages, func(m model.Message) bool { return sameTime(m.SentAt, args.Answers) }) &&
		!(reply.Kind == model.ReplyFollowUp && followsUpMark(held, args.Answers)):
		return nil, errors.New("answers is the sentAt of a message in the Conversation, or for a follow_up the markedRepliedAt of its Suggested Reply, " +
			"as career_get_conversation shows them")
	case reply.Kind != model.ReplyFollowUp && !sameTime(args.Answers, newestFrom(held, model.MessageFromOther)):
		return nil, fmt.Errorf("an answer or late reply answers the Recruiter's newest message, sent at %s: read the Conversation again with career_get_conversation",
			newestFrom(held, model.MessageFromOther).Format(time.RFC3339))
	case len(args.Notes) > model.MaxLanguageNotes:
		return nil, fmt.Errorf("a Suggested Reply has at most %d language notes", model.MaxLanguageNotes)
	}
	// The Sync that brought the message it answers learns it led to an
	// answer, for its Notification.
	for _, m := range held.Messages {
		if sameTime(m.SentAt, args.Answers) && m.From == model.MessageFromOther {
			reply.SyncID = m.SyncID
		}
	}
	for _, n := range args.Notes {
		n = model.LanguageNote{Wrote: strings.TrimSpace(n.Wrote), Natural: strings.TrimSpace(n.Natural), Why: strings.TrimSpace(n.Why)}
		if n.Wrote == "" || n.Natural == "" || n.Why == "" {
			return nil, errors.New("each language note needs what the Owner wrote, what is natural, and why")
		}
		reply.Notes = append(reply.Notes, n)
	}
	conversation, err := s.conversations.SetSuggestedReply(ctx, id, reply)
	if err != nil {
		if errors.Is(err, coreutils.ErrNotFound) {
			return nil, errors.New("that Conversation is ignored: it isn't about a role")
		}
		return nil, err
	}
	if err := s.complete(ctx, id, model.ItemReply, model.ItemLateReply, model.ItemFollowUp); err != nil {
		return nil, err
	}
	return careerutils.ToConversationJSON(conversation), nil
}

// followsUpMark reports whether answers is when the Owner marked c's
// Suggested Reply replied: a follow-up may answer the mark, which stands for
// the Owner's message until a Sync brings it.
func followsUpMark(c model.Conversation, answers time.Time) bool {
	return c.SuggestedReply != nil && !c.SuggestedReply.MarkedRepliedAt.IsZero() && sameTime(c.SuggestedReply.MarkedRepliedAt, answers)
}

// newestFrom is when the newest of c's messages from sender was sent, or the
// zero time if there is none.
func newestFrom(c model.Conversation, sender string) time.Time {
	var newest time.Time
	for _, m := range c.Messages {
		if m.From == sender && m.SentAt.After(newest) {
			newest = m.SentAt
		}
	}
	return newest
}

// recruiter returns the Recruiter with the id from a Tool's arguments.
func (s *ConversationsService) recruiter(ctx context.Context, hex string) (model.Recruiter, error) {
	id, err := careerutils.ID("Recruiter", hex)
	if err != nil {
		return model.Recruiter{}, err
	}
	recruiter, err := s.recruiters.GetByID(ctx, id)
	return recruiter, careerutils.Found(err, "Recruiter", hex)
}

// sameTime reports whether a and b are the same time to the millisecond, as
// MongoDB keeps times.
func sameTime(a, b time.Time) bool {
	return a.Truncate(time.Millisecond).Equal(b.Truncate(time.Millisecond))
}

func (s *ConversationsService) getTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	id, err := careerutils.ID("Conversation", args.ID)
	if err != nil {
		return nil, err
	}
	conversation, err := s.conversations.GetByID(ctx, id)
	if err != nil {
		return nil, careerutils.Found(err, "Conversation", args.ID)
	}
	return careerutils.ToConversationJSON(conversation), nil
}

func (s *ConversationsService) fileTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ConversationID string               `json:"conversationId"`
		RecruiterID    string               `json:"recruiterId"`
		Recruiter      *model.RecruiterJSON `json:"recruiter"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	id, err := careerutils.ID("Conversation", args.ConversationID)
	if err != nil {
		return nil, err
	}
	// Check everything before storing a new Recruiter.
	held, err := s.conversations.GetByID(ctx, id)
	if err != nil {
		return nil, careerutils.Found(err, "Conversation", args.ConversationID)
	}
	if held.Ignored {
		return nil, errors.New("that Conversation is ignored: it isn't about a role")
	}
	var recruiter model.Recruiter
	switch {
	case args.RecruiterID != "":
		if recruiter, err = s.recruiter(ctx, args.RecruiterID); err != nil {
			return nil, err
		}
	case args.Recruiter != nil:
		r := model.Recruiter{
			Name:         strings.TrimSpace(args.Recruiter.Name),
			ProfileURL:   strings.TrimSpace(args.Recruiter.ProfileURL),
			Kind:         args.Recruiter.Kind,
			Organisation: strings.TrimSpace(args.Recruiter.Organisation),
		}
		if r.ProfileURL == "" {
			r.ProfileURL = held.Person.ProfileURL
		}
		if r.Name == "" {
			return nil, errors.New("a new Recruiter needs a name")
		}
		if r.Kind != model.RecruiterAgency && r.Kind != model.RecruiterInHouse {
			return nil, fmt.Errorf("a Recruiter's kind is %s or %s", model.RecruiterAgency, model.RecruiterInHouse)
		}
		if recruiter, err = s.recruiters.Insert(ctx, r); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("file it under a Recruiter: recruiterId for an existing one, or recruiter for a new one")
	}
	conversation, err := s.conversations.File(ctx, id, recruiter.ID)
	if err != nil {
		return nil, careerutils.Found(err, "Conversation", args.ConversationID)
	}
	// Once it's filed: a Recruiter with no link takes the Conversation's.
	if link := conversation.Person.ProfileURL; link != "" && recruiter.ProfileURL == "" {
		if err := s.recruiters.GiveProfileURL(ctx, recruiter.ID, link); err != nil {
			return nil, err
		}
		recruiter.ProfileURL = link
	}
	if err := s.complete(ctx, id, model.ItemSort); err != nil {
		return nil, err
	}
	return model.FiledJSON{Conversation: careerutils.ToConversationJSON(conversation), Recruiter: careerutils.ToRecruiterJSON(recruiter)}, nil
}

func (s *ConversationsService) ignoreTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ConversationID string `json:"conversationId"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	id, err := careerutils.ID("Conversation", args.ConversationID)
	if err != nil {
		return nil, err
	}
	conversation, err := s.conversations.Ignore(ctx, id)
	if err != nil {
		return nil, careerutils.Found(err, "Conversation", args.ConversationID)
	}
	// Nothing is left to do for a thread that isn't about a role.
	if err := s.complete(ctx, id, model.ItemKinds...); err != nil {
		return nil, err
	}
	return careerutils.ToConversationJSON(conversation), nil
}
