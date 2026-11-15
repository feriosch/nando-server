package service

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// How many top Opportunities the Home Card and the Career screen show.
const (
	homeCardTop = 3
	screenTop   = 5
)

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

// getHomeCard answers the Career Home Card.
func (s *OverviewService) getHomeCard(w http.ResponseWriter, r *http.Request) {
	settings, err := s.settings.Settings(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	waiting, err := s.waiting(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	ranked, err := s.opportunities.Ranked(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.HomeCardJSON{Looking: settings.Looking, Waiting: len(waiting), Top: ranked[:min(len(ranked), homeCardTop)]})
}

// getScreen answers the Career screen.
func (s *OverviewService) getScreen(w http.ResponseWriter, r *http.Request) {
	waiting, err := s.waiting(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	late, err := s.late(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	ranked, err := s.opportunities.Ranked(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, model.CareerScreenJSON{
		Waiting:     waiting,
		Top:         ranked[:min(len(ranked), screenTop)],
		Open:        len(ranked),
		LateReplies: len(late),
	})
}

// listLate answers Late Replies.
func (s *OverviewService) listLate(w http.ResponseWriter, r *http.Request) {
	late, err := s.late(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, late)
}

// waiting is each Recruiter waiting, once, by their newest message waiting
// for an answer or a follow-up, newest first.
func (s *OverviewService) waiting(ctx context.Context) ([]model.RecruiterReplyJSON, error) {
	return s.withReplies(ctx, func(a, b model.RecruiterReplyJSON) int { return b.LastMessageAt.Compare(a.LastMessageAt) },
		model.WaitingReplyKinds...)
}

// late is each Recruiter with a Late Reply, once, by their oldest message
// waiting for one, oldest first.
func (s *OverviewService) late(ctx context.Context) ([]model.RecruiterReplyJSON, error) {
	return s.withReplies(ctx, func(a, b model.RecruiterReplyJSON) int { return a.LastMessageAt.Compare(b.LastMessageAt) },
		model.ReplyLate)
}

// withReplies lists each Recruiter with a Suggested Reply of one of kinds,
// once, from whichever of their Conversations comes first in order, and in
// that order. Ties fall back to the Recruiter's id, then the reply, so the
// list is the same on every request.
func (s *OverviewService) withReplies(ctx context.Context, order func(a, b model.RecruiterReplyJSON) int, kinds ...string) ([]model.RecruiterReplyJSON, error) {
	conversations, err := s.conversations.WithSuggestedReply(ctx, kinds)
	if err != nil {
		return nil, err
	}
	recruiters := map[bson.ObjectID]model.Recruiter{}
	listed := []model.RecruiterReplyJSON{}
	for _, c := range conversations {
		recruiter, ok := recruiters[*c.RecruiterID]
		if !ok {
			if recruiter, err = s.recruiters.GetByID(ctx, *c.RecruiterID); err != nil {
				return nil, err
			}
			recruiters[recruiter.ID] = recruiter
		}
		listed = append(listed, careerutils.ToRecruiterReplyJSON(recruiter, c))
	}
	slices.SortFunc(listed, func(a, b model.RecruiterReplyJSON) int {
		if c := order(a, b); c != 0 {
			return c
		}
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return strings.Compare(a.FirstLine, b.FirstLine)
	})
	once, seen := []model.RecruiterReplyJSON{}, map[string]bool{}
	for _, j := range listed {
		if !seen[j.ID] {
			seen[j.ID] = true
			once = append(once, j)
		}
	}
	return once, nil
}
