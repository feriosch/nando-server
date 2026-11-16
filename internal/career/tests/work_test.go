package tests

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// workJSON is what career_list_work answers: the items this run claimed, and
// what every item needs.
type workJSON struct {
	Items     []itemJSON `json:"items"`
	Looking   bool       `json:"looking"`
	Profile   string     `json:"profile"`
	LiveSince time.Time  `json:"liveSince"`
}

// itemJSON is one item of the run's work.
type itemJSON struct {
	ID           string                 `json:"id"`
	Kind         string                 `json:"kind"`
	Conversation conversationJSON       `json:"conversation"`
	Recruiter    *recruiterWithWorkJSON `json:"recruiter"`
}

// conversationJSON is a Conversation as the Tools answer it.
type conversationJSON struct {
	ID             string              `json:"id"`
	ThreadID       string              `json:"threadId"`
	URL            string              `json:"url"`
	Person         personJSON          `json:"person"`
	Messages       []messageJSON       `json:"messages"`
	RecruiterID    *string             `json:"recruiterId"`
	SuggestedReply *suggestedReplyJSON `json:"suggestedReply"`
}

type personJSON struct {
	Name       string `json:"name"`
	ProfileURL string `json:"profileUrl"`
}

type messageJSON struct {
	ID     string    `json:"id"`
	From   string    `json:"from"`
	SentAt time.Time `json:"sentAt"`
	Text   string    `json:"text"`
}

// recruiterWithWorkJSON is the Recruiter of an item's Conversation, with
// their other Conversations and their Opportunities.
type recruiterWithWorkJSON struct {
	recruiterJSON
	Conversations []conversationJSON `json:"conversations"`
	Opportunities []opportunityJSON  `json:"opportunities"`
}

type recruiterJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProfileURL   string `json:"profileUrl"`
	Kind         string `json:"kind"`
	Organisation string `json:"organisation"`
}

type suggestedReplyJSON struct {
	Text      string             `json:"text"`
	Language  string             `json:"language"`
	Kind      string             `json:"kind"`
	Answers   time.Time          `json:"answers"`
	Notes     []languageNoteJSON `json:"notes"`
	WrittenAt time.Time          `json:"writtenAt"`
	// MarkedRepliedAt is when the Owner marked it replied, or nil.
	MarkedRepliedAt *time.Time `json:"markedRepliedAt"`
}

type languageNoteJSON struct {
	Wrote   string `json:"wrote"`
	Natural string `json:"natural"`
	Why     string `json:"why"`
}

// opportunityJSON is an Opportunity as the Tools answer it.
type opportunityJSON struct {
	ID      string `json:"id"`
	Company struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"company"`
	Role        string  `json:"role"`
	RecruiterID *string `json:"recruiterId"`
	Stage       string  `json:"stage"`
	Outcome     *string `json:"outcome"`
	Interest    string  `json:"interest"`
}

// syncTanaka records, at 08:00 on Monday, a Sync with Tanaka's pitch, and
// returns the Mac's session.
func syncTanaka(t *testing.T, phone *browser) *mcp.ClientSession {
	t.Helper()
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": []any{tanakaThread(
		message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo."),
	)}})
	return mac
}

func TestTheRunClaimsANewConversationToSort(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := syncTanaka(t, phone)

	got := callTool[workJSON](t, run, "career_list_work", nil)

	if len(got.Items) != 1 {
		t.Fatalf("claimed %d items, want 1: %+v", len(got.Items), got.Items)
	}
	item := got.Items[0]
	if item.ID == "" || item.Kind != "sort" || item.Conversation.ThreadID != "2-tanaka" || item.Recruiter != nil ||
		len(item.Conversation.Messages) != 1 || item.Conversation.Messages[0].Text != "Hello! I have a backend role at a fintech in Tokyo." ||
		item.Conversation.Person.Name != "Tanaka Yui" {
		t.Errorf("item = %+v, want Tanaka's unfiled Conversation to sort, with its message", item)
	}
	if got.Looking || got.Profile != "" || !got.LiveSince.Equal(at(0, 8, 0)) {
		t.Errorf("looking = %v, profile = %q, liveSince = %v; want false, empty, and the first Sync's time %v",
			got.Looking, got.Profile, got.LiveSince, at(0, 8, 0))
	}

	if again := callTool[workJSON](t, run, "career_list_work", nil); len(again.Items) != 0 {
		t.Errorf("listing again claimed %+v, want nothing: the item is claimed", again.Items)
	}
}

func TestALapsedClaimGoesBackAndThreeClaimsWithNoOutcomeFail(t *testing.T) {
	n, phone := signedInBrowser(t)
	syncTanaka(t, phone)
	logged := captureLog(t)
	first := callTool[workJSON](t, phone.connect(), "career_list_work", nil)
	if len(first.Items) != 1 {
		t.Fatalf("first claim got %d items, want 1", len(first.Items))
	}

	for claim := 2; claim <= 3; claim++ {
		n.clock.Set(n.clock.Now().Add(14 * time.Minute))
		if early := callTool[workJSON](t, phone.connect(), "career_list_work", nil); len(early.Items) != 0 {
			t.Fatalf("14 minutes into claim %d another run claimed %+v; want nothing", claim-1, early.Items)
		}
		n.clock.Set(n.clock.Now().Add(time.Minute))
		again := callTool[workJSON](t, phone.connect(), "career_list_work", nil)
		if len(again.Items) != 1 || again.Items[0].ID != first.Items[0].ID {
			t.Fatalf("claim %d got %+v; want the same item back once the claim lapsed", claim, again.Items)
		}
	}

	n.clock.Set(n.clock.Now().Add(15 * time.Minute))
	if after := callTool[workJSON](t, phone.connect(), "career_list_work", nil); len(after.Items) != 0 {
		t.Fatalf("after 3 claims with no outcome a run claimed %+v; want the item set aside", after.Items)
	}
	if !strings.Contains(logged.String(), first.Items[0].ID) {
		t.Errorf("the log says %q; want it to name the failed item %s", logged.String(), first.Items[0].ID)
	}
}

// captureLog has the log write to the returned buffer until the test ends.
func captureLog(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// lockedBuffer is a bytes.Buffer safe to write from several goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestAnIgnoredThreadKeepsOnlyItsIDAndLaterSyncsDropItsMessages(t *testing.T) {
	n, phone := signedInBrowser(t)
	syncTanaka(t, phone)
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]

	ignored := callTool[conversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": item.Conversation.ID})
	if ignored.ThreadID != "2-tanaka" || len(ignored.Messages) != 0 || ignored.Person != (personJSON{}) || ignored.URL != "" {
		t.Errorf("career_ignore_thread = %+v, want only the thread ID", ignored)
	}

	mac := phone.macAt(at(0, 14, 0))
	got := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": []any{tanakaThread(
		message("m2", "other", at(0, 13, 0), "Did you see my message?"),
	)}})
	if got.Stored != 0 {
		t.Errorf("a Sync after ignoring stored %d messages, want 0", got.Stored)
	}
	status := callTool[syncStatusJSON](t, mac, "career_sync_status", nil)
	if len(status.Threads) != 1 || !status.Threads[0].Ignored {
		t.Errorf("career_sync_status threads = %+v, want 2-tanaka ignored", status.Threads)
	}

	// Ignoring did the sort item, and the later Sync queued nothing.
	n.clock.Set(at(1, 8, 0))
	if left := callTool[workJSON](t, phone.connect(), "career_list_work", nil); len(left.Items) != 0 {
		t.Errorf("after ignoring, a run claimed %+v; want nothing", left.Items)
	}
}

// filedJSON is what career_file_conversation answers.
type filedJSON struct {
	Conversation conversationJSON `json:"conversation"`
	Recruiter    recruiterJSON    `json:"recruiter"`
}

func TestTheRunFilesConversationsUnderANewRecruiterAndThenTheSameOne(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	tanakaAgain := tanakaThread(message("m9", "other", at(-1, 9, 0), "Another role, at a game studio this time."))
	tanakaAgain["threadId"] = "2-tanaka-studio"
	callTool[recordedJSON](t, phone.macAt(at(0, 8, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.")),
		tanakaAgain,
	}})
	run := phone.connect()
	items := callTool[workJSON](t, run, "career_list_work", nil).Items
	if len(items) != 2 {
		t.Fatalf("claimed %d items, want both Conversations", len(items))
	}

	first := callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
		"conversationId": items[0].Conversation.ID,
		"recruiter": map[string]any{
			"name": "Tanaka Yui", "profileUrl": "https://www.linkedin.com/in/tanaka-yui/",
			"kind": "agency", "organisation": "Hays Japan",
		},
	})
	if first.Recruiter.ID == "" || first.Recruiter.Name != "Tanaka Yui" || first.Recruiter.Kind != "agency" ||
		first.Recruiter.Organisation != "Hays Japan" || first.Conversation.RecruiterID == nil || *first.Conversation.RecruiterID != first.Recruiter.ID {
		t.Fatalf("filing under a new Recruiter answered %+v; want the Conversation filed under the new Recruiter", first)
	}
	second := callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
		"conversationId": items[1].Conversation.ID,
		"recruiterId":    first.Recruiter.ID,
	})
	if second.Recruiter != first.Recruiter || second.Conversation.RecruiterID == nil || *second.Conversation.RecruiterID != first.Recruiter.ID {
		t.Errorf("filing under the existing Recruiter answered %+v; want it under %+v", second, first.Recruiter)
	}

	n.clock.Set(at(1, 8, 0))
	if left := callTool[workJSON](t, phone.connect(), "career_list_work", nil); len(left.Items) != 0 {
		t.Errorf("after filing both, a run claimed %+v; want nothing", left.Items)
	}
}

func TestFilingNeedsARecruiter(t *testing.T) {
	_, phone := signedInBrowser(t)
	syncTanaka(t, phone)
	run := phone.connect()
	id := callTool[workJSON](t, run, "career_list_work", nil).Items[0].Conversation.ID

	for _, args := range []map[string]any{
		{"conversationId": id},
		{"conversationId": id, "recruiterId": "000000000000000000000000"},
		{"conversationId": id, "recruiter": map[string]any{"name": "Tanaka Yui", "kind": "headhunter"}},
		{"conversationId": id, "recruiter": map[string]any{"name": " ", "kind": "agency"}},
		{"conversationId": "000000000000000000000000", "recruiter": map[string]any{"name": "Tanaka Yui", "kind": "agency"}},
	} {
		toolFails(t, run, "career_file_conversation", args)
	}
}

// fileTanaka has a run sort what syncTanaka brought: it files 2-tanaka under
// a new Recruiter, Tanaka, and returns the run's session and the Recruiter.
func fileTanaka(t *testing.T, phone *browser) (*mcp.ClientSession, recruiterJSON) {
	t.Helper()
	run, filed := fileTanakasConversation(t, phone)
	return run, filed.Recruiter
}

// fileTanakasConversation is fileTanaka, answering what
// career_file_conversation did.
func fileTanakasConversation(t *testing.T, phone *browser) (*mcp.ClientSession, filedJSON) {
	t.Helper()
	syncTanaka(t, phone)
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]
	return run, callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
		"conversationId": item.Conversation.ID,
		"recruiter":      map[string]any{"name": "Tanaka Yui", "kind": "agency", "organisation": "Hays Japan"},
	})
}

func TestARecruitersNewMessageInAFiledConversationQueuesAReply(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, tanaka := fileTanaka(t, phone)
	studio := tanakaThread(message("m9", "other", at(-1, 9, 0), "Another role, at a game studio this time."))
	studio["threadId"] = "2-tanaka-studio"
	mac := phone.macAt(at(0, 9, 0))
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": []any{studio}})
	run := phone.connect()
	studioID := callTool[workJSON](t, run, "career_list_work", nil).Items[0].Conversation.ID
	callTool[filedJSON](t, run, "career_file_conversation", map[string]any{"conversationId": studioID, "recruiterId": tanaka.ID})

	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", at(0, 14, 0), "Are you free for a call this week?")),
		map[string]any{"threadId": "2-tanaka-studio", "messages": []any{message("m10", "owner", at(0, 14, 30), "Thank you, I'll have a look.")}},
	}})
	got := callTool[workJSON](t, phone.connect(), "career_list_work", nil)

	if len(got.Items) != 1 {
		t.Fatalf("claimed %+v; want one reply item, for Tanaka's new message only", got.Items)
	}
	item := got.Items[0]
	if item.Kind != "reply" || item.Conversation.ThreadID != "2-tanaka" || len(item.Conversation.Messages) != 2 {
		t.Errorf("item = %+v; want a reply item for 2-tanaka with both its messages", item)
	}
	if item.Recruiter == nil || item.Recruiter.ID != tanaka.ID || len(item.Recruiter.Conversations) != 1 ||
		item.Recruiter.Conversations[0].ThreadID != "2-tanaka-studio" || len(item.Recruiter.Conversations[0].Messages) != 2 ||
		item.Recruiter.Opportunities == nil || len(item.Recruiter.Opportunities) != 0 {
		t.Errorf("recruiter = %+v; want Tanaka with the studio Conversation and no Opportunities yet", item.Recruiter)
	}
}

func TestTheRunSeesTheRecruitersOpportunities(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)

	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", at(0, 14, 0), "The fintech would like to meet you.")),
	}})
	item := callTool[workJSON](t, phone.connect(), "career_list_work", nil).Items[0]

	if item.Recruiter == nil || len(item.Recruiter.Opportunities) != 1 || item.Recruiter.Opportunities[0].ID != o.ID ||
		item.Recruiter.Opportunities[0].Company.Name != "Kumo Pay" || item.Recruiter.Opportunities[0].Stage != "introduced" {
		t.Errorf("the reply item's recruiter = %+v; want Tanaka with the Kumo Pay Opportunity", item.Recruiter)
	}
}

func TestIgnoringAFiledConversationDoesAllItsWork(t *testing.T) {
	n, phone := signedInBrowser(t)
	_, filed := fileTanakasConversation(t, phone)
	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", at(0, 14, 0), "Are you free for a call this week?")),
	}})
	// On a closer look, Tanaka was only selling a course.
	callTool[conversationJSON](t, phone.connect(), "career_ignore_thread", map[string]any{"conversationId": filed.Conversation.ID})

	n.clock.Set(at(1, 8, 0))
	if left := callTool[workJSON](t, phone.connect(), "career_list_work", nil); len(left.Items) != 0 {
		t.Errorf("after ignoring, a run claimed %+v; want nothing", left.Items)
	}
}
