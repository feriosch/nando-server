package service

import (
	"errors"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"

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

// getPage answers the Recruiter with each of their Conversations.
func (s *RecruitersService) getPage(w http.ResponseWriter, r *http.Request) {
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		http.Error(w, "there's no such Recruiter", http.StatusNotFound)
		return
	}
	recruiter, err := s.recruiters.GetByID(r.Context(), id)
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Recruiter", http.StatusNotFound)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	conversations, err := s.conversations.ByRecruiter(r.Context(), id)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	page := model.RecruiterPageJSON{RecruiterJSON: careerutils.ToRecruiterJSON(recruiter), Conversations: []model.PageConversationJSON{}}
	for _, c := range conversations {
		page.Conversations = append(page.Conversations, careerutils.ToPageConversationJSON(c, pageMessages))
	}
	coreutils.WriteJSON(w, http.StatusOK, page)
}
