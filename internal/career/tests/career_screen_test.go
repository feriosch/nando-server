package tests

import (
	"slices"
	"testing"
	"time"
)

// careerScreenJSON is what the Career screen shows.
type careerScreenJSON struct {
	Waiting     []waitingJSON        `json:"waiting"`
	Top         []topOpportunityJSON `json:"top"`
	Active      int                  `json:"active"`
	LateReplies int                  `json:"lateReplies"`
}

// waitingJSON is a Recruiter with a Suggested Reply, as the Career screen
// and Late Replies list them.
type waitingJSON struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Organisation  string    `json:"organisation"`
	LastMessageAt time.Time `json:"lastMessageAt"`
	FirstLine     string    `json:"firstLine"`
}

// thread is a LinkedIn thread with the Recruiter name, as the Mac's script
// sends it.
func thread(name string, messages ...map[string]any) map[string]any {
	return map[string]any{
		"threadId": "2-" + name,
		"url":      "https://www.linkedin.com/messaging/thread/2-" + name + "/",
		"person":   map[string]any{"name": name, "profileUrl": "https://www.linkedin.com/in/" + name + "/"},
		"messages": messages,
	}
}

// fileThreads records, at 08:00 on Monday, a Sync with threads, and has a run
// file each under a new Recruiter named as its person, of the agency
// "<name> Agency". It returns each Conversation's id by Recruiter name, and
// each Recruiter's id.
func fileThreads(t *testing.T, phone *browser, threads ...map[string]any) (conversations, recruiters map[string]string) {
	t.Helper()
	phone.saveTimeZone("Asia/Tokyo")
	callTool[recordedJSON](t, phone.macAt(at(0, 8, 0)), "career_record_sync", map[string]any{"threads": threads})
	run := phone.connect()
	conversations, recruiters = map[string]string{}, map[string]string{}
	for len(conversations) < len(threads) {
		items := callTool[workJSON](t, run, "career_list_work", nil).Items
		if len(items) == 0 {
			t.Fatalf("the run found %d Conversations to file; want %d", len(conversations), len(threads))
		}
		for _, item := range items {
			name := item.Conversation.Person.Name
			filed := callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
				"conversationId": item.Conversation.ID,
				"recruiter":      map[string]any{"name": name, "kind": "agency", "organisation": name + " Agency"},
			})
			conversations[name], recruiters[name] = filed.Conversation.ID, filed.Recruiter.ID
		}
	}
	return conversations, recruiters
}

// suggestReply has a run write a Suggested Reply of kind in the Conversation,
// answering the message sent at answers.
func suggestReply(t *testing.T, phone *browser, conversationID, kind, text string, answers time.Time) {
	t.Helper()
	callTool[conversationJSON](t, phone.connect(), "career_set_suggested_reply", map[string]any{
		"conversationId": conversationID, "text": text, "language": "en", "kind": kind, "answers": answers.Format(time.RFC3339),
	})
}

func TestARecruiterWithAnAnswerOrFollowUpIsWaitingAndALateOneIsNot(t *testing.T) {
	_, phone := signedInBrowser(t)
	conversations, recruiters := fileThreads(t, phone,
		thread("sato",
			message("s1", "other", at(-20, 10, 0), "Any news on your side?"),
			message("s2", "owner", at(-15, 9, 0), "I'll get back to you soon."),
		),
		thread("tanaka", message("t1", "other", at(-3, 10, 15), "I have a backend role at a fintech.")),
		thread("suzuki", message("z1", "other", at(-60, 10, 0), "Are you open to new roles?")),
		thread("kato", message("k1", "other", at(-2, 10, 0), "Hello!")),
		thread("ito", message("i1", "other", at(-1, 10, 0), "Would you like a casual chat?")),
	)
	suggestReply(t, phone, conversations["sato"], "follow_up", "Hello Sato-san,\nI wanted to follow up.", at(-15, 9, 0))
	suggestReply(t, phone, conversations["tanaka"], "answer", "Thank you for reaching out!\nCould you share the job description?", at(-3, 10, 15))
	suggestReply(t, phone, conversations["suzuki"], "late", "My deepest apologies for the late response.", at(-60, 10, 0))
	suggestReply(t, phone, conversations["ito"], "answer", "Sure, I'd love to.", at(-1, 10, 0))

	screen := get[careerScreenJSON](phone, "/api/career")
	var waiting []string
	for _, w := range screen.Waiting {
		waiting = append(waiting, w.Name)
	}
	if want := []string{"ito", "tanaka", "sato"}; !slices.Equal(waiting, want) {
		t.Fatalf("waiting = %q; want the Recruiters with an answer or a follow-up, by their last message, newest first: %q", waiting, want)
	}
	if tanaka := screen.Waiting[1]; tanaka.ID != recruiters["tanaka"] || tanaka.Organisation != "tanaka Agency" ||
		!tanaka.LastMessageAt.Equal(at(-3, 10, 15)) || tanaka.FirstLine != "Thank you for reaching out!" {
		t.Errorf("Tanaka waits as %+v; want their id, agency, when their pitch came and the reply's first line", tanaka)
	}
	if sato := screen.Waiting[2]; !sato.LastMessageAt.Equal(at(-20, 10, 0)) {
		t.Errorf("Sato's last message came at %v; want %v, their own, not the Owner's after it", sato.LastMessageAt, at(-20, 10, 0))
	}
	if screen.LateReplies != 1 {
		t.Errorf("lateReplies = %d; want 1, Suzuki's", screen.LateReplies)
	}
}

func TestTheCareerScreenShowsTheTop5AndCountsEveryActiveOpportunity(t *testing.T) {
	_, phone := signedInBrowser(t)
	seed(t, phone,
		seededOpportunity{role: "Low introduced", interest: "low", stage: "introduced", activity: 1},
		seededOpportunity{role: "High offer", interest: "high", stage: "offer", activity: 2},
		seededOpportunity{role: "Normal introduced", interest: "normal", stage: "introduced", activity: 3},
		seededOpportunity{role: "Closed", interest: "high", stage: "closed", activity: 4},
		seededOpportunity{role: "High introduced", interest: "high", stage: "introduced", activity: 5},
		seededOpportunity{role: "Normal offer", interest: "normal", stage: "offer", activity: 6},
		seededOpportunity{role: "Low offer", interest: "low", stage: "offer", activity: 7},
		seededOpportunity{role: "High in process", interest: "high", stage: "in_process", steps: []string{"2026-11-20"}, activity: 8},
	)

	screen := get[careerScreenJSON](phone, "/api/career")
	var top []string
	for _, o := range screen.Top {
		top = append(top, o.Role)
	}
	if want := []string{"High offer", "High in process", "High introduced", "Normal offer", "Normal introduced"}; !slices.Equal(top, want) {
		t.Errorf("top = %q; want the top 5, %q", top, want)
	}
	if screen.Active != 7 || screen.LateReplies != 0 || len(screen.Waiting) != 0 {
		t.Errorf("active = %d, lateReplies = %d, waiting = %+v; want the 7 open Opportunities, and nothing waiting", screen.Active, screen.LateReplies, screen.Waiting)
	}
	if next := screen.Top[1].NextStep; next == nil || next.Kind != "Interview" || next.Date != "2026-11-20" || next.Time != nil {
		t.Errorf("the in-process Opportunity's next Step = %+v; want the interview on 2026-11-20, with no time", next)
	}
}
