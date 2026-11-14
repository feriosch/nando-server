package tests

import (
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// wordRequestJSON is a Word Request as the Japanese API answers it.
type wordRequestJSON struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Kind        string   `json:"kind"`
	Group       *string  `json:"group"`
	WordID      *string  `json:"wordId"`
	State       string   `json:"state"`
	Attempts    int      `json:"attempts"`
	Answer      *string  `json:"answer"`
	Summary     *string  `json:"summary"`
	Suggestions []string `json:"suggestions"`
	Question    *string  `json:"question"`
	Choices     []string `json:"choices"`
}

// askFor sends a new Word Request from the web app and answers the Word
// Request the server returns, which must come with status.
func askFor(phone *browser, body map[string]any, status int) wordRequestJSON {
	phone.nando.t.Helper()
	rec := phone.must(http.MethodPost, "/api/japanese/word-requests", body, status)
	var wr wordRequestJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &wr); err != nil {
		phone.nando.t.Fatalf("POST /api/japanese/word-requests: %v in %q", err, rec.Body)
	}
	return wr
}

func TestAWordTheOwnerAsksClaudeForIsListedAsWaiting(t *testing.T) {
	_, phone := signedInBrowser(t)

	asked := askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)

	got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")
	want := wordRequestJSON{
		ID:          asked.ID,
		Text:        "食べた",
		Kind:        "add",
		Group:       ptr("gaming"),
		State:       "pending",
		Suggestions: []string{},
		Choices:     []string{},
	}
	if len(got) != 1 || !equalJSON(t, got[0], want) {
		t.Fatalf("Word Requests = %+v; want only %+v", got, want)
	}
	if asked.ID == "" {
		t.Fatal("the new Word Request has no id")
	}
}

// equalJSON reports whether a and b write the same JSON.
func equalJSON(t *testing.T, a, b any) bool {
	t.Helper()
	ja, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	jb, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(ja) == string(jb)
}

// storedRequest is a Word Request as it is stored, for the states only a
// Claude run reaches: the Owner's API creates only pending ones.
func storedRequest(text, kind, state string) bson.M {
	return bson.M{
		"_id":       bson.NewObjectID(),
		"key":       kind + ":" + text,
		"text":      text,
		"kind":      kind,
		"group":     "gaming",
		"state":     state,
		"attempts":  1,
		"createdAt": time.Now(),
		"updatedAt": time.Now(),
	}
}

func TestAskingAgainForAWordStillOpenGivesTheSameWordRequest(t *testing.T) {
	_, phone := signedInBrowser(t)
	first := askFor(phone, map[string]any{"text": "かける", "kind": "add", "group": "anime"}, http.StatusCreated)

	again := askFor(phone, map[string]any{"text": "かける", "kind": "add", "group": "anime"}, http.StatusOK)

	if again.ID != first.ID {
		t.Fatalf("asking again gave Word Request %s; want the open one, %s", again.ID, first.ID)
	}
	if got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); len(got) != 1 {
		t.Fatalf("%d Word Requests listed; want 1", len(got))
	}
}

func TestAWordWaitingForAnAnswerIsStillOpen(t *testing.T) {
	n, phone := signedInBrowser(t)
	parked := storedRequest("かける", "add", "needs_answer")
	n.store("word_requests", parked)

	again := askFor(phone, map[string]any{"text": "かける", "kind": "add", "group": "anime"}, http.StatusOK)

	if again.ID != id(parked) {
		t.Fatalf("asking again gave Word Request %s; want the parked one, %s", again.ID, id(parked))
	}
}

func TestAFinishedWordRequestDoesNotStopANewOne(t *testing.T) {
	n, phone := signedInBrowser(t)
	finished := storedRequest("かける", "add", "done")
	n.store("word_requests", finished)

	asked := askFor(phone, map[string]any{"text": "かける", "kind": "add", "group": "anime"}, http.StatusCreated)

	if asked.ID == id(finished) {
		t.Fatal("asking again gave the finished Word Request; want a new one")
	}
}

func TestTheOwnerCanAskClaudeToImproveAStoredWord(t *testing.T) {
	n, phone := signedInBrowser(t)
	hang := word("掛ける", "かける", "colgar", "jlpt_n2")
	n.store("words", hang)

	asked := askFor(phone, map[string]any{"kind": "improve", "wordId": id(hang)}, http.StatusCreated)

	want := wordRequestJSON{
		ID:          asked.ID,
		Text:        "掛ける",
		Kind:        "improve",
		WordID:      ptr(id(hang)),
		State:       "pending",
		Suggestions: []string{},
		Choices:     []string{},
	}
	if !equalJSON(t, asked, want) {
		t.Fatalf("Word Request = %+v; want %+v", asked, want)
	}
}

func TestImprovingAWordThatIsNotStoredIsNotFound(t *testing.T) {
	_, phone := signedInBrowser(t)

	phone.must(http.MethodPost, "/api/japanese/word-requests", map[string]any{"kind": "improve", "wordId": bson.NewObjectID().Hex()}, http.StatusNotFound)
}

func TestAWordToAddNeedsTextAndTheGroupItWasFoundIn(t *testing.T) {
	_, phone := signedInBrowser(t)
	for name, body := range map[string]map[string]any{
		"no text":                    {"text": "  ", "kind": "add", "group": "gaming"},
		"no Group":                   {"text": "食べた", "kind": "add"},
		"a Group jisho decides":      {"text": "食べた", "kind": "add", "group": "jlpt_n2"},
		"a Group that doesn't exist": {"text": "食べた", "kind": "add", "group": "cooking"},
		"no kind":                    {"text": "食べた", "group": "gaming"},
	} {
		if rec := phone.do(http.MethodPost, "/api/japanese/word-requests", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body %q; want 400", name, rec.Code, rec.Body)
		}
	}
	if got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); len(got) != 0 {
		t.Fatalf("%d Word Requests listed after refused ones; want 0", len(got))
	}
}

func TestEveryGroupOnThePickerCanBeChosen(t *testing.T) {
	_, phone := signedInBrowser(t)
	for _, group := range []string{"literature", "gaming", "anime", "movie", "miscellaneous", "jlpt_n1"} {
		askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": group}, http.StatusCreated)
		// Asked again later in the loop, so finish it to let the next one through.
		if _, err := phone.nando.japanese.Collection("word_requests").UpdateMany(t.Context(), bson.M{}, bson.M{"$set": bson.M{"state": "done"}}); err != nil {
			t.Fatal(err)
		}
	}
}

// parkedRequest is a stored Word Request that Claude parked with a question.
func parkedRequest(text string) bson.M {
	doc := storedRequest(text, "add", "needs_answer")
	doc["question"] = "Which かける: to hang, or to call?"
	doc["choices"] = bson.A{"掛ける (to hang)", "架ける (to call)"}
	return doc
}

// act POSTs to a Word Request's action, such as answer, and answers the Word
// Request the server returns, which must come with status 200.
func act(phone *browser, wordRequestID, action string, body any) wordRequestJSON {
	phone.nando.t.Helper()
	rec := phone.must(http.MethodPost, "/api/japanese/word-requests/"+wordRequestID+"/"+action, body, http.StatusOK)
	var wr wordRequestJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &wr); err != nil {
		phone.nando.t.Fatalf("POST %s: %v in %q", action, err, rec.Body)
	}
	return wr
}

func TestAnsweringClaudesQuestionSendsTheWordBackToClaudeWithTheAnswer(t *testing.T) {
	n, phone := signedInBrowser(t)
	parked := parkedRequest("かける")
	n.store("word_requests", parked)

	answered := act(phone, id(parked), "answer", map[string]any{"answer": "掛ける (to hang)"})

	if answered.State != "pending" || answered.Answer == nil || *answered.Answer != "掛ける (to hang)" {
		t.Fatalf("answered Word Request = %+v; want pending with the answer", answered)
	}
	listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")
	if len(listed) != 1 || !equalJSON(t, listed[0], answered) {
		t.Fatalf("Word Requests = %+v; want only %+v", listed, answered)
	}
}

func TestOnlyAQuestionCanBeAnswered(t *testing.T) {
	n, phone := signedInBrowser(t)
	waiting := storedRequest("かける", "add", "pending")
	n.store("word_requests", waiting)

	phone.must(http.MethodPost, "/api/japanese/word-requests/"+id(waiting)+"/answer", map[string]any{"answer": "掛ける"}, http.StatusConflict)
	phone.must(http.MethodPost, "/api/japanese/word-requests/"+bson.NewObjectID().Hex()+"/answer", map[string]any{"answer": "掛ける"}, http.StatusNotFound)
}

func TestAnAnswerCannotBeBlank(t *testing.T) {
	n, phone := signedInBrowser(t)
	parked := parkedRequest("かける")
	n.store("word_requests", parked)

	phone.must(http.MethodPost, "/api/japanese/word-requests/"+id(parked)+"/answer", map[string]any{"answer": " "}, http.StatusBadRequest)
}

func TestRetryingAFailedWordRequestSendsItBackToClaude(t *testing.T) {
	n, phone := signedInBrowser(t)
	failed := storedRequest("かける", "add", "failed")
	failed["attempts"] = 3
	n.store("word_requests", failed)

	retried := act(phone, id(failed), "retry", nil)

	if retried.State != "pending" || retried.Attempts != 0 {
		t.Fatalf("retried Word Request is %s after %d attempts; want pending with 3 fresh ones (0 used)", retried.State, retried.Attempts)
	}
}

func TestOnlyAFailedWordRequestCanBeRetried(t *testing.T) {
	n, phone := signedInBrowser(t)
	done := storedRequest("かける", "add", "done")
	n.store("word_requests", done)

	phone.must(http.MethodPost, "/api/japanese/word-requests/"+id(done)+"/retry", nil, http.StatusConflict)
}

func TestADismissedWordRequestLeavesTheList(t *testing.T) {
	n, phone := signedInBrowser(t)
	done := storedRequest("食べる", "add", "done")
	failed := storedRequest("かける", "add", "failed")
	n.store("word_requests", done, failed)

	act(phone, id(done), "dismiss", nil)
	act(phone, id(failed), "dismiss", nil)

	if got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); len(got) != 0 {
		t.Fatalf("Word Requests = %+v; want none after dismissing both", got)
	}
}

func TestAnOpenWordRequestCannotBeDismissed(t *testing.T) {
	n, phone := signedInBrowser(t)
	waiting := storedRequest("かける", "add", "pending")
	parked := parkedRequest("掛かる")
	n.store("word_requests", waiting, parked)

	phone.must(http.MethodPost, "/api/japanese/word-requests/"+id(waiting)+"/dismiss", nil, http.StatusConflict)
	phone.must(http.MethodPost, "/api/japanese/word-requests/"+id(parked)+"/dismiss", nil, http.StatusConflict)
}

func TestFinishedWordRequestsLeaveTheListAfterAWeekButOpenOnesStay(t *testing.T) {
	n, phone := signedInBrowser(t)
	finishedAt := n.clock.Now()
	done := storedRequest("食べる", "add", "done")
	failed := storedRequest("かける", "add", "failed")
	waiting := storedRequest("見る", "add", "pending")
	parked := parkedRequest("掛かる")
	for _, doc := range []bson.M{done, failed, waiting, parked} {
		doc["updatedAt"] = finishedAt
	}
	n.store("word_requests", done, failed, waiting, parked)

	n.clock.Set(finishedAt.Add(7*24*time.Hour - time.Minute))
	if got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); len(got) != 4 {
		t.Fatalf("%d Word Requests listed a minute short of a week; want all 4", len(got))
	}

	n.clock.Set(finishedAt.Add(7 * 24 * time.Hour))
	got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")
	var texts []string
	for _, wr := range got {
		texts = append(texts, wr.Text)
	}
	if len(got) != 2 || !slices.Contains(texts, "見る") || !slices.Contains(texts, "掛かる") {
		t.Fatalf("Word Requests listed after a week = %v; want only the open 見る and 掛かる", texts)
	}
}

func TestTheListShowsHowOftenClaudeHasTriedAWordRequest(t *testing.T) {
	n, phone := signedInBrowser(t)
	working := storedRequest("かける", "add", "working")
	working["attempts"] = 2
	n.store("word_requests", working)

	got := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")

	if len(got) != 1 || got[0].Attempts != 2 {
		t.Fatalf("Word Requests = %+v; want the one Claude has tried twice, with attempts 2", got)
	}
}

func TestAskingAgainToImproveAWordGivesTheOpenWordRequestEvenIfItsSpellingChanged(t *testing.T) {
	n, phone := signedInBrowser(t)
	hang := word("掛ける", "かける", "colgar", "jlpt_n2")
	n.store("words", hang)
	// Asked for when the Word was still stored in kana; Claude has since
	// moved it to its kanji spelling, and is still working on it.
	improving := storedRequest("かける", "improve", "working")
	improving["wordId"] = id(hang)
	improving["key"] = "improve:" + id(hang)
	delete(improving, "group")
	n.store("word_requests", improving)

	again := askFor(phone, map[string]any{"kind": "improve", "wordId": id(hang)}, http.StatusOK)

	if again.ID != id(improving) {
		t.Fatalf("asking again gave Word Request %s; want the open one, %s", again.ID, id(improving))
	}
}

func TestADoubleTapAsksOnlyOnce(t *testing.T) {
	n, phone := signedInBrowser(t)
	const taps = 8
	ids := make(chan string, taps)
	var wg sync.WaitGroup
	for range taps {
		// Each tap is its own request from the same Session.
		tap := &browser{nando: n, session: phone.session}
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := tap.do(http.MethodPost, "/api/japanese/word-requests", map[string]any{"text": "食べた", "kind": "add", "group": "gaming"})
			var wr wordRequestJSON
			if json.Unmarshal(rec.Body.Bytes(), &wr) == nil {
				ids <- wr.ID
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")
	if len(seen) != 1 || len(listed) != 1 {
		t.Fatalf("%d taps answered with %d different Word Requests and %d listed; want 1 and 1", taps, len(seen), len(listed))
	}
}

func TestEachActionRefusesEveryStateItDoesNotApplyTo(t *testing.T) {
	n, phone := signedInBrowser(t)
	byState := map[string]bson.M{}
	for _, state := range []string{"pending", "working", "done", "needs_answer", "failed"} {
		byState[state] = storedRequest("語"+state, "add", state)
		n.store("word_requests", byState[state])
	}
	refused := map[string][]string{
		"answer":  {"pending", "working", "done", "failed"},
		"retry":   {"pending", "working", "done", "needs_answer"},
		"dismiss": {"pending", "working", "needs_answer"},
	}
	for action, states := range refused {
		for _, state := range states {
			path := "/api/japanese/word-requests/" + id(byState[state]) + "/" + action
			if rec := phone.do(http.MethodPost, path, map[string]any{"answer": "これ"}); rec.Code != http.StatusConflict {
				t.Errorf("%s a %s Word Request: status = %d, body %q; want 409", action, state, rec.Code, rec.Body)
			}
		}
	}
}

func TestActionsOnAWordRequestThatIsNotThereAreNotFound(t *testing.T) {
	_, phone := signedInBrowser(t)
	for _, action := range []string{"answer", "retry", "dismiss"} {
		for _, wordRequestID := range []string{bson.NewObjectID().Hex(), "not-an-id"} {
			path := "/api/japanese/word-requests/" + wordRequestID + "/" + action
			if rec := phone.do(http.MethodPost, path, map[string]any{"answer": "これ"}); rec.Code != http.StatusNotFound {
				t.Errorf("POST %s: status = %d; want 404", path, rec.Code)
			}
		}
	}
}

func TestAnAnsweredWordRequestKeepsTheQuestionForClaude(t *testing.T) {
	n, phone := signedInBrowser(t)
	parked := parkedRequest("かける")
	n.store("word_requests", parked)

	answered := act(phone, id(parked), "answer", map[string]any{"answer": "掛ける (to hang)"})

	if answered.Question == nil || *answered.Question != "Which かける: to hang, or to call?" ||
		!slices.Equal(answered.Choices, []string{"掛ける (to hang)", "架ける (to call)"}) {
		t.Fatalf("answered Word Request = %+v; want it to keep Claude's question and choices", answered)
	}
}

func TestAnAnsweredWordRequestGetsFreshTriesLikeARetriedOne(t *testing.T) {
	n, phone := signedInBrowser(t)
	// Claude asked on its 3rd claim: the Owner's answer is new to work with.
	parked := parkedRequest("かける")
	parked["attempts"] = 3
	n.store("word_requests", parked)

	answered := act(phone, id(parked), "answer", map[string]any{"answer": "掛ける (to hang)"})

	if answered.Attempts != 0 {
		t.Fatalf("answered Word Request has %d attempts; want 0, so Claude gets 3 fresh tries", answered.Attempts)
	}
}
