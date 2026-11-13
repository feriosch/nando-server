package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// WordRequestsService answers the Pending strip: the Owner's Word Requests,
// which they create, answer, retry and dismiss.
type WordRequestsService struct {
	requests model.WordRequestsRepository
	words    model.WordsRepository
	now      func() time.Time
}

// NewWordRequests returns the WordRequestsService, which reads the Words to
// improve from words and the time from now.
func NewWordRequests(requests model.WordRequestsRepository, words model.WordsRepository, now func() time.Time) *WordRequestsService {
	return &WordRequestsService{requests: requests, words: words, now: now}
}

// finishedListedFor is how long a Done or Failed Word Request the Owner
// hasn't dismissed stays in the Pending strip.
const finishedListedFor = 7 * 24 * time.Hour

// pickerGroups are the Groups the Owner can say they found a word in. The
// others (JLPT N2, JLPT N3, Basic) come from jisho.org's JLPT level, which
// Claude looks up.
var pickerGroups = []string{"literature", "gaming", "anime", "movie", "miscellaneous", "jlpt_n1"}

// badWordRequest is why a new Word Request was refused, to answer with 400.
type badWordRequest string

func (e badWordRequest) Error() string { return string(e) }

// errNoSuchWord is an improve Word Request for a Word that isn't stored.
var errNoSuchWord = errors.New("there's no such Word to improve")

// newWordRequest checks body and returns the pending Word Request it asks
// for, or an error to answer with 400, or errNoSuchWord.
func (s *WordRequestsService) newWordRequest(r *http.Request, body model.NewWordRequestJSON) (model.WordRequest, error) {
	request := model.WordRequest{Kind: model.WordRequestKind(body.Kind), State: model.Pending}
	switch request.Kind {
	case model.AddWord:
		request.Text = strings.TrimSpace(body.Text)
		if request.Text == "" {
			return request, badWordRequest("a word to add needs its text")
		}
		if !slices.Contains(pickerGroups, body.Group) {
			return request, badWordRequest("a word to add needs the Group it was found in, one of " + strings.Join(pickerGroups, ", "))
		}
		request.Group = body.Group
	case model.ImproveWord:
		word, err := s.words.GetOneByID(r.Context(), body.WordID)
		if errors.Is(err, coreutils.ErrNotFound) {
			return request, errNoSuchWord
		}
		if err != nil {
			return request, err
		}
		request.Text = word.Word
		request.WordID = word.ID
	default:
		return request, badWordRequest(`kind must be "add" or "improve"`)
	}
	return request, nil
}

// Routes adds the Word Request routes to mux, under
// /api/japanese/word-requests.
func (s *WordRequestsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/japanese/word-requests", s.list)
	mux.HandleFunc("POST /api/japanese/word-requests", s.create)
	mux.HandleFunc("POST /api/japanese/word-requests/{id}/answer", s.answer)
	mux.HandleFunc("POST /api/japanese/word-requests/{id}/retry", s.retry)
	mux.HandleFunc("POST /api/japanese/word-requests/{id}/dismiss", s.dismiss)
}

func (s *WordRequestsService) create(w http.ResponseWriter, r *http.Request) {
	var body model.NewWordRequestJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't a Word Request", http.StatusBadRequest)
		return
	}
	request, err := s.newWordRequest(r, body)
	if errors.Is(err, errNoSuchWord) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var badRequest badWordRequest
	if errors.As(err, &badRequest) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	open, err := s.requests.GetOpen(r.Context(), request.Text, request.Kind, request.WordID)
	if err == nil {
		coreutils.WriteJSON(w, http.StatusOK, wordRequestJSON(open))
		return
	}
	if !errors.Is(err, coreutils.ErrNotFound) {
		coreutils.ServerError(w, err)
		return
	}
	request.CreatedAt = s.now()
	request.UpdatedAt = request.CreatedAt
	id, err := s.requests.Insert(r.Context(), request)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	request.ID = id
	coreutils.WriteJSON(w, http.StatusCreated, wordRequestJSON(request))
}

func (s *WordRequestsService) list(w http.ResponseWriter, r *http.Request) {
	requests, err := s.requests.GetListed(r.Context(), s.now().Add(-finishedListedFor))
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	list := make([]model.WordRequestJSON, len(requests))
	for i, request := range requests {
		list[i] = wordRequestJSON(request)
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}

// wordRequestJSON is request as the API writes it.
func wordRequestJSON(request model.WordRequest) model.WordRequestJSON {
	return model.WordRequestJSON{
		ID:          request.ID,
		Text:        request.Text,
		Kind:        string(request.Kind),
		Group:       utils.OrNull(request.Group),
		WordID:      utils.OrNull(request.WordID),
		State:       string(request.State),
		Answer:      utils.OrNull(request.Answer),
		Summary:     utils.OrNull(request.Summary),
		Suggestions: utils.OrEmptyList(request.Suggestions),
		Question:    utils.OrNull(request.Question),
		Choices:     utils.OrEmptyList(request.Choices),
	}
}

func (s *WordRequestsService) answer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an answer", http.StatusBadRequest)
		return
	}
	answer := strings.TrimSpace(body.Answer)
	if answer == "" {
		http.Error(w, "the answer is blank", http.StatusBadRequest)
		return
	}
	pending := model.Pending
	s.update(w, r, []model.WordRequestState{model.NeedsAnswer}, model.WordRequestChanges{State: &pending, Answer: &answer})
}

// update applies changes to the Word Request in the path if it is in one of
// the states in from, and answers with it as it is after.
func (s *WordRequestsService) update(w http.ResponseWriter, r *http.Request, from []model.WordRequestState, changes model.WordRequestChanges) {
	changes.UpdatedAt = s.now()
	request, err := s.requests.Update(r.Context(), r.PathValue("id"), from, changes)
	switch {
	case errors.Is(err, coreutils.ErrNotFound):
		http.Error(w, "there's no such Word Request", http.StatusNotFound)
	case errors.Is(err, model.ErrWrongState):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		coreutils.ServerError(w, err)
	default:
		coreutils.WriteJSON(w, http.StatusOK, wordRequestJSON(request))
	}
}

func (s *WordRequestsService) retry(w http.ResponseWriter, r *http.Request) {
	pending := model.Pending
	s.update(w, r, []model.WordRequestState{model.Failed}, model.WordRequestChanges{State: &pending})
}

func (s *WordRequestsService) dismiss(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	s.update(w, r, []model.WordRequestState{model.Done, model.Failed}, model.WordRequestChanges{DismissedAt: &now})
}
