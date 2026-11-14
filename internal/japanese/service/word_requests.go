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

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
	"github.com/feriosch/nando-server/internal/japanese/utils"
)

// WordRequestsService answers the Pending strip: the Owner's Word Requests,
// which they create, answer, retry and dismiss.
type WordRequestsService struct {
	requests model.WordRequestsRepository
	words    model.WordsRepository
	// scheduledTask wakes Claude when a Word Request is waiting for it.
	scheduledTask *ScheduledTaskService
	now           func() time.Time
}

// NewWordRequests returns the WordRequestsService, which reads the Words to
// improve from words, wakes Claude with scheduledTask and reads the time
// from now.
func NewWordRequests(requests model.WordRequestsRepository, words model.WordsRepository, scheduledTask *ScheduledTaskService, now func() time.Time) *WordRequestsService {
	return &WordRequestsService{requests: requests, words: words, scheduledTask: scheduledTask, now: now}
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
	request.CreatedAt = s.now()
	request.UpdatedAt = request.CreatedAt
	stored, created, err := s.requests.InsertUnlessOpen(r.Context(), request)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	// Asking again wakes Claude too, so retrying after a failed wake works.
	s.scheduledTask.Wake(r.Context())
	coreutils.WriteJSON(w, status, wordRequestJSON(stored))
}

func (s *WordRequestsService) list(w http.ResponseWriter, r *http.Request) {
	// A Word Request no run finished is failed by the clock, not by the
	// next run: the Owner shouldn't see "working" until Claude runs again.
	if err := s.failUnfinished(r.Context()); err != nil {
		coreutils.ServerError(w, err)
		return
	}
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
		Attempts:    request.Attempts,
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
	// The answer is new to work with, so it gets fresh claims, as a retry does.
	pending, fresh := model.Pending, 0
	s.update(w, r, []model.WordRequestState{model.NeedsAnswer}, model.WordRequestChanges{State: &pending, Answer: &answer, Attempts: &fresh})
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
		if request.State == model.Pending {
			s.scheduledTask.Wake(r.Context())
		}
		coreutils.WriteJSON(w, http.StatusOK, wordRequestJSON(request))
	}
}

func (s *WordRequestsService) retry(w http.ResponseWriter, r *http.Request) {
	pending := model.Pending
	// A retried Word Request gets fresh claims, as a new one would.
	fresh := 0
	s.update(w, r, []model.WordRequestState{model.Failed}, model.WordRequestChanges{State: &pending, Attempts: &fresh})
}

func (s *WordRequestsService) dismiss(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	s.update(w, r, []model.WordRequestState{model.Done, model.Failed}, model.WordRequestChanges{DismissedAt: &now})
}

// claimAtOnce is how many Word Requests one list claims: one, so that each
// claim's 15 minutes start when a run takes that word, not while it is still
// working on an earlier one.
const claimAtOnce = 1

// maxClaims is how many times a Word Request is claimed before it fails:
// that many runs took it and none finished it.
const maxClaims = 3

// claimFor is how long a Claude run holds the Word Requests it claims before
// another run may take them.
const claimFor = 15 * time.Minute

// Tools are what a Claude run uses to work through the Word Requests.
func (s *WordRequestsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "japanese_list_word_requests",
		Description: "Claims the oldest Word Request waiting for Claude, for this run: the Owner's ask to add a word " +
			"(text as they typed it, maybe conjugated or in kana, with the Group they found it in) or to improve a stored Word (wordId). " +
			"A Word Request the Owner has answered comes back with Claude's earlier question, its choices and the Owner's answer. " +
			"Resolve it with japanese_resolve_word_request, then call this again for the next, until it answers none. " +
			"A claim lasts 15 minutes. " +
			"Answers {\"wordRequests\": [{\"id\", \"text\", \"kind\", \"group\", \"wordId\", \"question\", \"choices\", \"answer\"}]}, with one Word Request, or empty when none are waiting.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handle:      s.listTool,
	}, {
		Name: "japanese_resolve_word_request",
		Description: "Records what came of a Word Request this run claimed with japanese_list_word_requests. " +
			"done: the word was added or the Word improved; wordId is that Word (for an improve, the Word asked about), summary one line for the Owner " +
			"(\"Added 食べる · 3 Word Links · 1 Hint\"), and suggestions what the Owner should do in Claude Code, " +
			"such as a merge or a Group change, which a run never does itself. " +
			"needs_answer: something only the Owner can settle (which homograph, which reading gets the kanji spelling); " +
			"question is short, and choices are 2 to 4 answers the Owner can tap. The Word Request comes back with their answer. " +
			"failed: it can't be done (no such word, even unconjugated); summary says why, for the Owner. " +
			"Answers the Word Request as the Owner now sees it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"The Word Request's id."},` +
			`"outcome":{"type":"string","enum":["done","needs_answer","failed"]},` +
			`"summary":{"type":"string"},` +
			`"wordId":{"type":"string","description":"For done: the Word added or improved."},` +
			`"suggestions":{"type":"array","items":{"type":"string"}},` +
			`"question":{"type":"string","description":"For needs_answer."},` +
			`"choices":{"type":"array","items":{"type":"string"},"description":"For needs_answer: 2 to 4."}},` +
			`"required":["id","outcome"]}`),
		Handle: s.resolveTool,
	}}
}

// resolveTool answers japanese_resolve_word_request with the Word Request as
// the Owner now sees it.
func (s *WordRequestsService) resolveTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID          string   `json:"id"`
		Outcome     string   `json:"outcome"`
		Summary     string   `json:"summary"`
		WordID      string   `json:"wordId"`
		Suggestions []string `json:"suggestions"`
		Question    string   `json:"question"`
		Choices     []string `json:"choices"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	outcome := model.WordRequestState(args.Outcome)
	summary := strings.TrimSpace(args.Summary)
	question := strings.TrimSpace(args.Question)
	choices := trimmed(args.Choices)
	suggestions := trimmed(args.Suggestions)
	// An earlier round's question, choices and answer don't carry over.
	none, noList := "", []string{}
	changes := model.WordRequestChanges{State: &outcome, Question: &none, Choices: &noList, UpdatedAt: s.now()}
	switch outcome {
	case model.Done:
		if summary == "" {
			return nil, errors.New("done needs a summary for the Owner, such as \"Added 食べる · 3 Word Links\"")
		}
		if _, err := s.words.GetOneByID(ctx, args.WordID); err != nil {
			if errors.Is(err, coreutils.ErrNotFound) {
				return nil, errors.New("done needs wordId, the stored Word added or improved")
			}
			return nil, err
		}
		// An improve is done for the Word the Owner asked about, so that
		// Open word opens it; work on another Word is a suggestion.
		asked, err := s.requests.GetOneByID(ctx, args.ID)
		if err != nil && !errors.Is(err, coreutils.ErrNotFound) {
			return nil, err
		}
		if err == nil && asked.Kind == model.ImproveWord && asked.WordID != args.WordID {
			return nil, fmt.Errorf("this Word Request improves the Word %s (%s): done names that wordId; suggest changes to other Words instead", asked.WordID, asked.Text)
		}
		changes.Summary = &summary
		changes.WordID = &args.WordID
		changes.Suggestions = &suggestions
	case model.NeedsAnswer:
		if question == "" || len(choices) < 2 || len(choices) > 4 || len(choices) != len(args.Choices) {
			return nil, errors.New("needs_answer needs a question and 2 to 4 choices the Owner can tap")
		}
		changes.Question = &question
		changes.Choices = &choices
		changes.Answer = &none
	case model.Failed:
		if summary == "" {
			return nil, errors.New("failed needs a summary saying why, for the Owner")
		}
		changes.Summary = &summary
	default:
		return nil, errors.New(`outcome must be "done", "needs_answer" or "failed"`)
	}
	request, err := s.requests.Update(ctx, args.ID, []model.WordRequestState{model.Working}, changes)
	if err != nil {
		return nil, err
	}
	return wordRequestJSON(request), nil
}

// trimmed is list with each item trimmed and the blank ones left out.
func trimmed(list []string) []string {
	out := []string{}
	for _, item := range list {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// listTool answers japanese_list_word_requests: it claims the Word Requests
// waiting for Claude.
func (s *WordRequestsService) listTool(ctx context.Context, _ json.RawMessage) (any, error) {
	if err := s.failUnfinished(ctx); err != nil {
		return nil, err
	}
	claimed := []model.ClaimedWordRequestJSON{}
	for len(claimed) < claimAtOnce {
		now := s.now()
		request, err := s.requests.ClaimNext(ctx, now, now.Add(claimFor), maxClaims)
		if errors.Is(err, coreutils.ErrNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, model.ClaimedWordRequestJSON{
			ID:       request.ID,
			Text:     request.Text,
			Kind:     string(request.Kind),
			Group:    utils.OrNull(request.Group),
			WordID:   utils.OrNull(request.WordID),
			Question: utils.OrNull(request.Question),
			Choices:  utils.OrEmptyList(request.Choices),
			Answer:   utils.OrNull(request.Answer),
		})
	}
	return model.ClaimedWordRequestsJSON{WordRequests: claimed}, nil
}

// failUnfinished fails, by the clock, every Word Request whose last claim ran
// out after maxClaims claims. The Owner's list, a run's claim and the tick's
// Notifications all start with it, so they agree on which have failed.
func (s *WordRequestsService) failUnfinished(ctx context.Context) error {
	return s.requests.FailUnfinished(ctx, s.now(), maxClaims)
}

// DueNotifications returns, at every tick, a Notification for each Word
// Request in the Pending strip that needs the Owner: one waiting for an
// answer, or one that failed. Core sends each only once (see
// wordRequestNotificationID).
func (s *WordRequestsService) DueNotifications(ctx context.Context, slot time.Time) ([]coremodel.Notification, error) {
	// A Word Request no run finished fails by the clock, so the tick tells
	// the Owner even when nobody has listed Word Requests since.
	if err := s.failUnfinished(ctx); err != nil {
		return nil, err
	}
	requests, err := s.requests.GetListed(ctx, s.now().Add(-finishedListedFor))
	if err != nil {
		return nil, err
	}
	var due []coremodel.Notification
	for _, request := range requests {
		var title, body string
		switch request.State {
		case model.NeedsAnswer:
			title, body = "Claude has a question", "「"+request.Text+"」: "+request.Question
		case model.Failed:
			title, body = "Couldn't add a word", "「"+request.Text+"」: "+request.Summary
			if request.Kind == model.ImproveWord {
				title = "Couldn't improve a word"
			}
		default:
			continue
		}
		due = append(due, coremodel.Notification{
			ID:    wordRequestNotificationID(request),
			Title: title,
			Body:  body,
			URL:   "/japanese",
		})
	}
	return due, nil
}

// wordRequestNotificationID names the Notification for request as it is now,
// such as "japanese:word-request:<id>:needs_answer:<ms>". The time it entered
// its state tells one round from the next: attempts can't, since an answer
// or a retry sets them back to 0, so a second question or failure would look
// like the first. Waiting in that state doesn't change the time, so the
// Owner is notified once per round.
func wordRequestNotificationID(request model.WordRequest) string {
	return fmt.Sprintf("japanese:word-request:%s:%s:%d", request.ID, request.State, request.UpdatedAt.UnixMilli())
}
