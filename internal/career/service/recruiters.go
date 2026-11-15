package service

import (
	"net/http"
	"slices"
	"strings"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// pageMessages is how many of a Conversation's last messages a Recruiter's
// page shows.
const pageMessages = 5

// RecruitersService serves the app's Recruiter page.
type RecruitersService struct {
	recruiters    model.RecruitersRepository
	conversations model.ConversationsRepository
}

// NewRecruiters returns the RecruitersService.
func NewRecruiters(recruiters model.RecruitersRepository, conversations model.ConversationsRepository) *RecruitersService {
	return &RecruitersService{recruiters: recruiters, conversations: conversations}
}

// Routes adds the Recruiter page's route to mux, under
// /api/career/recruiters.
func (s *RecruitersService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/career/recruiters/{id}", s.getPage)
}

// getPage answers the Recruiter with each of their Conversations, the one
// with the newest message first.
func (s *RecruitersService) getPage(w http.ResponseWriter, r *http.Request) {
	recruiter, ok := careerutils.FromPath(w, r, "Recruiter", s.recruiters.GetByID)
	if !ok {
		return
	}
	conversations, err := s.conversations.ByRecruiter(r.Context(), recruiter.ID)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	slices.SortFunc(conversations, func(a, b model.Conversation) int {
		if c := b.Newest.Compare(a.Newest); c != 0 {
			return c
		}
		return strings.Compare(a.ID.Hex(), b.ID.Hex())
	})
	page := model.RecruiterPageJSON{RecruiterJSON: careerutils.ToRecruiterJSON(recruiter), Conversations: []model.RecruiterPageConversationJSON{}}
	for _, c := range conversations {
		page.Conversations = append(page.Conversations, careerutils.ToRecruiterPageConversationJSON(c, pageMessages))
	}
	coreutils.WriteJSON(w, http.StatusOK, page)
}
