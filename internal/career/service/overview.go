package service

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// screenTop is how many top Opportunities the Career screen shows.
const screenTop = 5

// homeCardTop is how many top Opportunities the Home Card shows.
const homeCardTop = 3

// OverviewService answers the app's overviews of the Module: the Home Card,
// the Career screen and Late Replies. A Recruiter is waiting when one of
// their Conversations has a Suggested Reply that is an answer or a
// follow-up; a late one is a Late Reply instead.
type OverviewService struct {
	conversations model.ConversationsRepository
	recruiters    model.RecruitersRepository
	opportunities *OpportunitiesService
	settings      *SettingsService
}

// NewOverview returns the OverviewService.
func NewOverview(conversations model.ConversationsRepository, recruiters model.RecruitersRepository, opportunities *OpportunitiesService, settings *SettingsService) *OverviewService {
	return &OverviewService{conversations: conversations, recruiters: recruiters, opportunities: opportunities, settings: settings}
}

// Routes adds the overviews' routes to mux.
func (s *OverviewService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/career/home-card", s.getHomeCard)
	mux.HandleFunc("GET /api/career", s.getScreen)
	mux.HandleFunc("GET /api/career/late-replies", s.listLate)
}

// listLate answers the Recruiters with a Late Reply, by how old their
// message is, oldest first.
func (s *OverviewService) listLate(w http.ResponseWriter, r *http.Request) {
	late, err := s.withReplies(r.Context(), model.ReplyLate)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	slices.SortStableFunc(late, func(a, b model.WaitingJSON) int { return a.LastMessageAt.Compare(b.LastMessageAt) })
	coreutils.WriteJSON(w, http.StatusOK, late)
}

// getHomeCard answers the Career Home Card.
func (s *OverviewService) getHomeCard(w http.ResponseWriter, r *http.Request) {
	settings, err := s.settings.Settings(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	waiting, err := s.withReplies(r.Context(), model.ReplyAnswer, model.ReplyFollowUp)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	top, err := s.opportunities.Top(r.Context(), homeCardTop)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.HomeCardJSON{Looking: settings.Looking, Waiting: len(waiting), Top: top})
}

// getScreen answers the Career screen.
func (s *OverviewService) getScreen(w http.ResponseWriter, r *http.Request) {
	waiting, err := s.withReplies(r.Context(), model.ReplyAnswer, model.ReplyFollowUp)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	slices.SortStableFunc(waiting, func(a, b model.WaitingJSON) int { return b.LastMessageAt.Compare(a.LastMessageAt) })
	late, err := s.withReplies(r.Context(), model.ReplyLate)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	open, err := s.opportunities.Top(r.Context(), -1)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.CareerScreenJSON{
		Waiting:     waiting,
		Top:         open[:min(len(open), screenTop)],
		Active:      len(open),
		LateReplies: len(late),
	})
}

// withReplies returns each Recruiter with a Suggested Reply of one of kinds,
// once, from their Conversation with the newest message of theirs.
func (s *OverviewService) withReplies(ctx context.Context, kinds ...string) ([]model.WaitingJSON, error) {
	conversations, err := s.conversations.WithSuggestedReply(ctx, kinds)
	if err != nil {
		return nil, err
	}
	newest := map[bson.ObjectID]model.Conversation{}
	for _, c := range conversations {
		if held, ok := newest[*c.RecruiterID]; !ok || lastFromRecruiter(c).After(lastFromRecruiter(held)) {
			newest[*c.RecruiterID] = c
		}
	}
	listed := []model.WaitingJSON{}
	for recruiterID, c := range newest {
		recruiter, err := s.recruiters.GetByID(ctx, recruiterID)
		if err != nil {
			return nil, err
		}
		firstLine, _, _ := strings.Cut(c.SuggestedReply.Text, "\n")
		listed = append(listed, model.WaitingJSON{
			ID: recruiter.ID.Hex(), Name: recruiter.Name, Organisation: recruiter.Organisation,
			LastMessageAt: lastFromRecruiter(c), FirstLine: strings.TrimSpace(firstLine),
		})
	}
	return listed, nil
}

// lastFromRecruiter is when c's newest message from its Recruiter was sent,
// or its newest message, if they sent none.
func lastFromRecruiter(c model.Conversation) time.Time {
	var last time.Time
	for _, m := range c.Messages {
		if m.From == model.MessageFromOther && m.SentAt.After(last) {
			last = m.SentAt
		}
	}
	if last.IsZero() {
		return c.Newest
	}
	return last
}
