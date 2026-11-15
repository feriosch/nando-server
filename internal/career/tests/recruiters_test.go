package tests

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// recruiterPageJSON is a Recruiter as their page in the app shows them, with
// each of their Conversations.
type recruiterPageJSON struct {
	recruiterJSON
	Conversations []struct {
		ID             string        `json:"id"`
		URL            string        `json:"url"`
		Messages       []messageJSON `json:"messages"`
		SuggestedReply *struct {
			Text     string             `json:"text"`
			Language string             `json:"language"`
			Kind     string             `json:"kind"`
			Notes    []languageNoteJSON `json:"notes"`
		} `json:"suggestedReply"`
	} `json:"conversations"`
}

func TestTheRecruiterPageShowsTheLast5MessagesAndTheSuggestedReply(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, filed := fileTanakasConversation(t, phone)
	// After the pitch, six more, sent to Nando out of order.
	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{tanakaThread(
		message("m5", "other", at(-1, 10, 0), "The team is in Shibuya."),
		message("m2", "owner", at(-2, 9, 0), "Thank you! Could you share the job description?"),
		message("m7", "other", at(0, 14, 0), "Are you free for a call this week?"),
		message("m3", "other", at(-2, 11, 0), "Sure, here it is."),
		message("m6", "owner", at(-1, 12, 0), "Sounds interesting."),
		message("m4", "owner", at(-2, 12, 0), "What is the salary range?"),
	)}})
	callTool[conversationJSON](t, phone.connect(), "career_set_suggested_reply", map[string]any{
		"conversationId": filed.Conversation.ID, "text": "Yes! Thursday evening works for me.\nLooking forward to it.",
		"language": "en", "kind": "answer", "answers": at(0, 14, 0).Format(time.RFC3339),
		"notes": []any{map[string]any{"wrote": "Sounds interesting.", "natural": "That sounds interesting!", "why": "warmer"}},
	})

	page := get[recruiterPageJSON](phone, "/api/career/recruiters/"+filed.Recruiter.ID)
	if page.ID != filed.Recruiter.ID || page.Name != "Tanaka Yui" || page.Organisation != "Hays Japan" || page.Kind != "agency" || len(page.Conversations) != 1 {
		t.Fatalf("the Recruiter page = %+v; want Tanaka Yui of Hays Japan with one Conversation", page)
	}
	c := page.Conversations[0]
	if c.ID != filed.Conversation.ID || c.URL != "https://www.linkedin.com/messaging/thread/2-tanaka/" {
		t.Errorf("the Conversation is %s at %q; want %s, with its LinkedIn URL", c.ID, c.URL, filed.Conversation.ID)
	}
	var ids []string
	for _, m := range c.Messages {
		ids = append(ids, m.ID)
	}
	if want := []string{"m3", "m4", "m5", "m6", "m7"}; !slices.Equal(ids, want) {
		t.Errorf("its messages are %q; want the last 5 by when they were sent, %q", ids, want)
	}
	if r := c.SuggestedReply; r == nil || r.Text != "Yes! Thursday evening works for me.\nLooking forward to it." || r.Language != "en" || r.Kind != "answer" ||
		len(r.Notes) != 1 || r.Notes[0].Natural != "That sounds interesting!" {
		t.Errorf("its Suggested Reply = %+v; want the answer with its language note", r)
	}
}

func TestAnUnknownRecruiterIsNotFound(t *testing.T) {
	_, phone := signedInBrowser(t)
	for _, path := range []string{"/api/career/recruiters/" + bson.NewObjectID().Hex(), "/api/career/recruiters/not-an-id"} {
		phone.must(http.MethodGet, path, nil, http.StatusNotFound)
	}
}

func TestARecruitersConversationsAreNewestFirst(t *testing.T) {
	_, phone := signedInBrowser(t)
	conversations, recruiters := fileThreads(t, phone,
		thread("tanaka", message("t1", "other", at(-30, 10, 0), "A role at a fintech.")),
		secondThread("tanaka", message("t2", "other", at(-2, 10, 0), "A role in Osaka.")),
	)

	page := get[recruiterPageJSON](phone, "/api/career/recruiters/"+recruiters["tanaka"])
	var ids []string
	for _, c := range page.Conversations {
		ids = append(ids, c.ID)
	}
	if want := []string{conversations["3-tanaka"], conversations["2-tanaka"]}; !slices.Equal(ids, want) {
		t.Errorf("Tanaka's Conversations are %q; want the one with the newest message first, %q", ids, want)
	}
}
