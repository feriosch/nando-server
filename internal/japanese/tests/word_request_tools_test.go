package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// claimedJSON is what japanese_list_word_requests answers: the Word Requests
// this run has just claimed.
type claimedJSON struct {
	WordRequests []claimedRequestJSON `json:"wordRequests"`
}

// claimedRequestJSON is one claimed Word Request, as a Claude run gets it.
type claimedRequestJSON struct {
	ID       string   `json:"id"`
	Text     string   `json:"text"`
	Kind     string   `json:"kind"`
	Group    *string  `json:"group"`
	WordID   *string  `json:"wordId"`
	Question *string  `json:"question"`
	Choices  []string `json:"choices"`
	Answer   *string  `json:"answer"`
}

func TestARunClaimsTheWordsTheOwnerAskedFor(t *testing.T) {
	_, phone := signedInBrowser(t)
	asked := askFor(phone, map[string]any{"text": "食べた", "kind": "add", "group": "gaming"}, http.StatusCreated)
	run := phone.connect()

	got := callTool[claimedJSON](t, run, "japanese_list_word_requests", nil)

	want := claimedRequestJSON{ID: asked.ID, Text: "食べた", Kind: "add", Group: ptr("gaming"), Choices: []string{}}
	if len(got.WordRequests) != 1 || !equalJSON(t, got.WordRequests[0], want) {
		t.Fatalf("claimed %+v; want only %+v", got.WordRequests, want)
	}
	listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")
	if len(listed) != 1 || listed[0].State != "working" || listed[0].Attempts != 1 {
		t.Fatalf("the Owner sees %+v; want it working, on its first attempt", listed)
	}
}

// askAt has the Owner ask Claude to add each text, a minute apart on Core's
// clock, oldest first.
func askAt(n *nando, phone *browser, texts ...string) {
	n.t.Helper()
	for _, text := range texts {
		n.clock.Set(n.clock.Now().Add(time.Minute))
		askFor(phone, map[string]any{"text": text, "kind": "add", "group": "gaming"}, http.StatusCreated)
	}
}

// claimedTexts are the texts of the Word Requests a run claimed, in order.
func claimedTexts(c claimedJSON) []string {
	texts := []string{}
	for _, wr := range c.WordRequests {
		texts = append(texts, wr.Text)
	}
	return texts
}

func TestARunClaimsAtMostThreeWordRequestsAtATimeOldestFirst(t *testing.T) {
	n, phone := signedInBrowser(t)
	askAt(n, phone, "一", "二", "三", "四", "五")
	run := phone.connect()

	first := claimedTexts(callTool[claimedJSON](t, run, "japanese_list_word_requests", nil))
	second := claimedTexts(callTool[claimedJSON](t, run, "japanese_list_word_requests", nil))
	third := claimedTexts(callTool[claimedJSON](t, run, "japanese_list_word_requests", nil))

	if !slices.Equal(first, []string{"一", "二", "三"}) || !slices.Equal(second, []string{"四", "五"}) || len(third) != 0 {
		t.Fatalf("claimed %v, then %v, then %v; want 一 二 三, then 四 五, then nothing", first, second, third)
	}
}

func TestAWordRequestWhoseRunStoppedIsClaimedAgainAfterFifteenMinutes(t *testing.T) {
	n, phone := signedInBrowser(t)
	askAt(n, phone, "食べた")
	claimedAt := n.clock.Now()
	callTool[claimedJSON](t, phone.connect(), "japanese_list_word_requests", nil)
	// That run stopped without resolving it.
	next := phone.connect()

	n.clock.Set(claimedAt.Add(15*time.Minute - time.Second))
	early := claimedTexts(callTool[claimedJSON](t, next, "japanese_list_word_requests", nil))
	n.clock.Set(claimedAt.Add(15 * time.Minute))
	late := claimedTexts(callTool[claimedJSON](t, next, "japanese_list_word_requests", nil))

	if len(early) != 0 || !slices.Equal(late, []string{"食べた"}) {
		t.Fatalf("claimed %v a second short of 15 minutes and %v at 15; want nothing, then 食べた", early, late)
	}
	if listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); listed[0].Attempts != 2 {
		t.Fatalf("the Owner sees %d attempts; want 2", listed[0].Attempts)
	}
}

func TestAWordRequestNoRunFinishedInThreeClaimsFailsAndARetryStartsAgain(t *testing.T) {
	n, phone := signedInBrowser(t)
	stuck := storedRequest("食べた", "add", "working")
	stuck["attempts"] = 3
	stuck["claimedUntil"] = n.clock.Now().Add(-time.Minute)
	n.store("word_requests", stuck)
	run := phone.connect()

	claimed := claimedTexts(callTool[claimedJSON](t, run, "japanese_list_word_requests", nil))

	if len(claimed) != 0 {
		t.Fatalf("a fourth claim handed out %v; want nothing", claimed)
	}
	if listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); listed[0].State != "failed" {
		t.Fatalf("the Owner sees it %s; want failed", listed[0].State)
	}

	act(phone, id(stuck), "retry", nil)
	again := claimedTexts(callTool[claimedJSON](t, run, "japanese_list_word_requests", nil))

	if !slices.Equal(again, []string{"食べた"}) {
		t.Fatalf("after a retry a run claimed %v; want 食べた", again)
	}
	if listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests"); listed[0].Attempts != 1 {
		t.Fatalf("after a retry and a claim the Owner sees %d attempts; want 1", listed[0].Attempts)
	}
}

func TestRunsListingAtOnceNeverClaimTheSameWordRequest(t *testing.T) {
	n, phone := signedInBrowser(t)
	askAt(n, phone, "一", "二", "三", "四", "五", "六")
	runs := []*mcp.ClientSession{phone.connect(), phone.connect(), phone.connect()}

	claims := make(chan []string, len(runs))
	var wg sync.WaitGroup
	for _, run := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := run.CallTool(context.Background(), &mcp.CallToolParams{Name: "japanese_list_word_requests"})
			if err != nil || result.IsError {
				claims <- nil
				return
			}
			var c claimedJSON
			if json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &c) != nil {
				claims <- nil
				return
			}
			claims <- claimedTexts(c)
		}()
	}
	wg.Wait()
	close(claims)

	var all []string
	for c := range claims {
		all = append(all, c...)
	}
	slices.Sort(all)
	if want := []string{"一", "三", "二", "五", "六", "四"}; !slices.Equal(all, want) {
		t.Fatalf("three runs at once claimed %v between them; want each of the 6 once", all)
	}
}

// claimOne has a new run claim the one Word Request waiting, and returns the
// run and the Word Request's id.
func claimOne(t *testing.T, phone *browser) (*mcp.ClientSession, string) {
	t.Helper()
	run := phone.connect()
	claimed := callTool[claimedJSON](t, run, "japanese_list_word_requests", nil)
	if len(claimed.WordRequests) != 1 {
		t.Fatalf("claimed %+v; want one Word Request", claimed.WordRequests)
	}
	return run, claimed.WordRequests[0].ID
}

func TestAWordRequestResolvedAsDoneShowsTheOwnerWhatClaudeDid(t *testing.T) {
	n, phone := signedInBrowser(t)
	eat := word("食べる", "たべる", "comer", "basic")
	n.store("words", eat)
	askAt(n, phone, "食べた")
	run, wordRequestID := claimOne(t, phone)

	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":          wordRequestID,
		"outcome":     "done",
		"summary":     "Added 食べる · 3 Word Links · 1 Hint",
		"wordId":      id(eat),
		"suggestions": []string{"Move 食う to jlpt_n2"},
	})

	listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")
	want := wordRequestJSON{
		ID:          wordRequestID,
		Text:        "食べた",
		Kind:        "add",
		Group:       ptr("gaming"),
		WordID:      ptr(id(eat)),
		State:       "done",
		Attempts:    1,
		Summary:     ptr("Added 食べる · 3 Word Links · 1 Hint"),
		Suggestions: []string{"Move 食う to jlpt_n2"},
		Choices:     []string{},
	}
	if len(listed) != 1 || !equalJSON(t, listed[0], want) {
		t.Fatalf("the Owner sees %+v; want %+v", listed, want)
	}
}

func TestClaudesQuestionReachesTheOwnerAndTheirAnswerReachesTheNextRun(t *testing.T) {
	n, phone := signedInBrowser(t)
	askAt(n, phone, "かける")
	run, wordRequestID := claimOne(t, phone)

	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":       wordRequestID,
		"outcome":  "needs_answer",
		"question": "Which かける: to hang, or to call?",
		"choices":  []string{"掛ける (to hang)", "架ける (to call)"},
	})
	parked := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")[0]
	if parked.State != "needs_answer" || parked.Question == nil || *parked.Question != "Which かける: to hang, or to call?" ||
		!slices.Equal(parked.Choices, []string{"掛ける (to hang)", "架ける (to call)"}) {
		t.Fatalf("the Owner sees %+v; want Claude's question and its choices", parked)
	}

	act(phone, wordRequestID, "answer", map[string]any{"answer": "掛ける (to hang)"})
	next := callTool[claimedJSON](t, phone.connect(), "japanese_list_word_requests", nil)

	want := claimedRequestJSON{
		ID:       wordRequestID,
		Text:     "かける",
		Kind:     "add",
		Group:    ptr("gaming"),
		Question: ptr("Which かける: to hang, or to call?"),
		Choices:  []string{"掛ける (to hang)", "架ける (to call)"},
		Answer:   ptr("掛ける (to hang)"),
	}
	if len(next.WordRequests) != 1 || !equalJSON(t, next.WordRequests[0], want) {
		t.Fatalf("the next run claimed %+v; want %+v", next.WordRequests, want)
	}
}

func TestAWordRequestClaudeGivesUpOnShowsTheOwnerWhy(t *testing.T) {
	n, phone := signedInBrowser(t)
	askAt(n, phone, "たべたべ")
	run, wordRequestID := claimOne(t, phone)

	callTool[map[string]any](t, run, "japanese_resolve_word_request", map[string]any{
		"id":      wordRequestID,
		"outcome": "failed",
		"summary": "jisho.org has no word たべたべ, and it isn't a conjugation of one",
	})

	listed := get[[]wordRequestJSON](phone, "/api/japanese/word-requests")[0]
	if listed.State != "failed" || listed.Summary == nil || *listed.Summary != "jisho.org has no word たべたべ, and it isn't a conjugation of one" {
		t.Fatalf("the Owner sees %+v; want it failed, saying why", listed)
	}
}

func TestOnlyAClaimedWordRequestCanBeResolved(t *testing.T) {
	n, phone := signedInBrowser(t)
	waiting := storedRequest("食べた", "add", "pending")
	finished := storedRequest("見た", "add", "done")
	n.store("word_requests", waiting, finished)
	run := phone.connect()

	for _, wordRequestID := range []string{id(waiting), id(finished), bson.NewObjectID().Hex(), "not-an-id"} {
		toolError(t, run, "japanese_resolve_word_request", map[string]any{"id": wordRequestID, "outcome": "failed", "summary": "no"})
	}
}

func TestAResolvedWordRequestSaysWhatTheOwnerNeeds(t *testing.T) {
	n, phone := signedInBrowser(t)
	eat := word("食べる", "たべる", "comer", "basic")
	n.store("words", eat)

	cases := map[string]map[string]any{
		"an unknown outcome":           {"outcome": "skipped"},
		"done without a summary":       {"outcome": "done", "wordId": id(eat)},
		"done without its Word":        {"outcome": "done", "summary": "Added 食べる"},
		"done with a Word not stored":  {"outcome": "done", "summary": "Added 食べる", "wordId": bson.NewObjectID().Hex()},
		"a question without choices":   {"outcome": "needs_answer", "question": "Which?"},
		"a question with one choice":   {"outcome": "needs_answer", "question": "Which?", "choices": []string{"this"}},
		"a question with five choices": {"outcome": "needs_answer", "question": "Which?", "choices": []string{"1", "2", "3", "4", "5"}},
		"choices without a question":   {"outcome": "needs_answer", "choices": []string{"this", "that"}},
		"failed without saying why":    {"outcome": "failed"},
	}
	for name, args := range cases {
		// Each case gets its own claimed Word Request, so that one wrongly
		// resolved can't make the next refused for the wrong reason.
		askAt(n, phone, name)
		run, wordRequestID := claimOne(t, phone)
		args["id"] = wordRequestID
		t.Log(name)
		toolError(t, run, "japanese_resolve_word_request", args)
	}

	for _, listed := range get[[]wordRequestJSON](phone, "/api/japanese/word-requests") {
		if listed.State != "working" {
			t.Errorf("after only refused resolves, %q is %s; want still working", listed.Text, listed.State)
		}
	}
}
