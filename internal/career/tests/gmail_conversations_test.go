package tests

import (
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/testtrigger"
)

// gmailConversationJSON is a Conversation with what a Gmail one adds.
type gmailConversationJSON struct {
	conversationJSON
	Channel  string `json:"channel"`
	Subject  string `json:"subject"`
	Ignored  bool   `json:"ignored"`
	Messages []struct {
		messageJSON
		GmailID     string   `json:"gmailId"`
		Attachments []string `json:"attachments"`
	} `json:"messages"`
}

// addedGmailJSON is what career_add_gmail_thread answers.
type addedGmailJSON struct {
	Conversation gmailConversationJSON `json:"conversation"`
	Stored       int                   `json:"stored"`
	Recruiter    *recruiterJSON        `json:"recruiter"`
}

// hays is a Gmail thread from a Recruiter at an agency, with messages.
func hays(messages ...map[string]any) map[string]any {
	return map[string]any{
		"threadId": "18c2f0a1b2c3d4e5",
		"url":      "https://mail.google.com/mail/u/0/#all/18c2f0a1b2c3d4e5",
		"subject":  "Backend role at a fintech",
		"person":   map[string]any{"name": "Yui Tanaka", "email": "yui.tanaka@hays.example"},
		"messages": messages,
	}
}

// mail is one message in a Gmail thread.
func mail(gmailID, from string, sentAt time.Time, text string, attachments ...string) map[string]any {
	m := map[string]any{"gmailId": gmailID, "from": from, "sentAt": sentAt, "text": text}
	if len(attachments) > 0 {
		m["attachments"] = attachments
	}
	return m
}

// addGmail calls career_add_gmail_thread with thread.
func addGmail(t *testing.T, run *mcp.ClientSession, thread map[string]any) addedGmailJSON {
	t.Helper()
	return callTool[addedGmailJSON](t, run, "career_add_gmail_thread", thread)
}

func TestAGmailThreadBecomesAConversationOnceAndKeepsOnlyItsNewMessages(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()

	first := addGmail(t, run, hays(
		mail("g1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.", "JD.pdf"),
	))
	if first.Stored != 1 || first.Conversation.Channel != "gmail" || first.Conversation.ThreadID != "18c2f0a1b2c3d4e5" ||
		first.Conversation.Subject != "Backend role at a fintech" || first.Recruiter != nil {
		t.Fatalf("first add = %+v; want a new, unfiled Gmail Conversation with 1 message", first)
	}

	again := addGmail(t, run, hays(
		mail("g1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.", "JD.pdf"),
		mail("g2", "owner", at(-2, 9, 0), "Thanks, I'd like to hear more."),
	))
	if again.Stored != 1 || again.Conversation.ID != first.Conversation.ID {
		t.Errorf("second add = %+v; want the same Conversation, storing only g2", again)
	}

	got := callTool[gmailConversationJSON](t, run, "career_get_conversation", map[string]any{"id": first.Conversation.ID})
	if len(got.Messages) != 2 || got.Messages[0].GmailID != "g1" || len(got.Messages[0].Attachments) != 1 ||
		got.Messages[0].Attachments[0] != "JD.pdf" || got.Messages[1].From != "owner" {
		t.Errorf("career_get_conversation = %+v; want g1 with its attachment's name, then the Owner's g2", got.Messages)
	}
}

// recruiterWithEmailsJSON is a Recruiter with the addresses Nando knows them by.
type recruiterWithEmailsJSON struct {
	recruiterJSON
	Emails []string `json:"emails"`
}

func TestAFiledGmailThreadsAddressFilesTheNextThreadFromIt(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	first := addGmail(t, run, hays(mail("g1", "other", at(-3, 10, 15), "Hello! A backend role in Tokyo.")))

	filed := callTool[struct {
		Recruiter recruiterWithEmailsJSON `json:"recruiter"`
	}](t, run, "career_file_conversation", map[string]any{
		"conversationId": first.Conversation.ID,
		"recruiter":      map[string]any{"name": "Yui Tanaka", "kind": "agency", "organisation": "Hays"},
	})
	if len(filed.Recruiter.Emails) != 1 || filed.Recruiter.Emails[0] != "yui.tanaka@hays.example" {
		t.Fatalf("filed Recruiter = %+v; want them known by the thread's address", filed.Recruiter)
	}

	// A new thread from the same address, written in another case.
	next := hays(mail("g9", "other", at(-1, 11, 0), "Another role, at a game company."))
	next["threadId"] = "18c2f0ffffffffff"
	next["person"] = map[string]any{"name": "Yui Tanaka", "email": "Yui.Tanaka@Hays.example"}
	got := addGmail(t, run, next)

	if got.Recruiter == nil || got.Recruiter.ID != filed.Recruiter.ID || got.Conversation.RecruiterID == nil ||
		*got.Conversation.RecruiterID != filed.Recruiter.ID {
		t.Errorf("next thread = %+v; want it filed under %s, whose address it came from", got, filed.Recruiter.ID)
	}
}

func TestAWrongAddressIsForgottenAndFilesNothingMore(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	first := addGmail(t, run, hays(mail("g1", "other", at(-3, 10, 15), "Hello! A backend role in Tokyo.")))
	filed := callTool[struct {
		Recruiter recruiterWithEmailsJSON `json:"recruiter"`
	}](t, run, "career_file_conversation", map[string]any{
		"conversationId": first.Conversation.ID,
		"recruiter":      map[string]any{"name": "Yui Tanaka", "kind": "agency", "organisation": "Hays"},
	})

	got := callTool[recruiterWithEmailsJSON](t, run, "career_remove_recruiter_email", map[string]any{
		"recruiterId": filed.Recruiter.ID, "email": "YUI.TANAKA@hays.example",
	})
	if len(got.Emails) != 0 {
		t.Fatalf("career_remove_recruiter_email = %+v; want no addresses left", got)
	}

	next := hays(mail("g9", "other", at(-1, 11, 0), "From a shared inbox now."))
	next["threadId"] = "18c2f0ffffffffff"
	if added := addGmail(t, run, next); added.Recruiter != nil || added.Conversation.RecruiterID != nil {
		t.Errorf("next thread = %+v; want it unfiled, its address no longer theirs", added)
	}
}

func TestAGmailThreadNeverShowsInTheLinkedInSyncsStatus(t *testing.T) {
	_, phone := signedInBrowser(t)
	mac := syncTanaka(t, phone) // one LinkedIn thread, 2-tanaka
	addGmail(t, phone.connect(), hays(mail("g1", "other", at(-1, 10, 15), "Hello! A backend role in Tokyo.")))

	got := callTool[syncStatusJSON](t, mac, "career_sync_status", nil)

	if len(got.Threads) != 1 || got.Threads[0].ThreadID != "2-tanaka" {
		t.Errorf("career_sync_status threads = %+v; want only the LinkedIn thread", got.Threads)
	}
}

// fileHays adds Hays's Gmail thread with messages and files it under a new
// Recruiter, and answers the Conversation and the Recruiter.
func fileHays(t *testing.T, run *mcp.ClientSession, messages ...map[string]any) (gmailConversationJSON, recruiterJSON) {
	t.Helper()
	added := addGmail(t, run, hays(messages...))
	filed := callTool[struct {
		Recruiter recruiterJSON `json:"recruiter"`
	}](t, run, "career_file_conversation", map[string]any{
		"conversationId": added.Conversation.ID,
		"recruiter":      map[string]any{"name": "Yui Tanaka", "kind": "agency", "organisation": "Hays"},
	})
	return added.Conversation, filed.Recruiter
}

func TestTheOwnersSentGmailClearsTheSuggestedReply(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	asked := at(-3, 10, 15)
	conversation, _ := fileHays(t, run, mail("g1", "other", asked, "Are you free for a call this week?"))
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": conversation.ID, "text": "Thursday afternoon works for me.", "language": "en", "kind": "answer", "answers": asked,
	})

	addGmail(t, run, hays(mail("g2", "owner", at(-2, 9, 0), "Thursday afternoon works for me.")))

	if got := callTool[conversationJSON](t, run, "career_get_conversation", map[string]any{"id": conversation.ID}); got.SuggestedReply != nil {
		t.Errorf("Suggested Reply after the Owner's sent mail = %+v; want it gone", got.SuggestedReply)
	}
}

func TestAnIgnoredGmailThreadStoresNothingMore(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	added := addGmail(t, run, hays(mail("g1", "other", at(-3, 10, 15), "Our newsletter for November.")))
	callTool[conversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": added.Conversation.ID})

	again := addGmail(t, run, hays(mail("g2", "other", at(-1, 10, 15), "Our newsletter for December.")))

	if again.Stored != 0 || !again.Conversation.Ignored || len(again.Conversation.Messages) != 0 {
		t.Errorf("after ignoring, add = %+v; want nothing stored and the thread still ignored", again)
	}
}

func TestAGmailConversationGetsTheSameFollowUpAsALinkedInOne(t *testing.T) {
	trigger := testtrigger.New(t)
	n, phone := signedInBrowserWith(t, config(trigger))
	phone.saveTimeZone("Asia/Tokyo")
	run := phone.connect()
	conversation, recruiter := fileHays(t, run, mail("g1", "other", at(-3, 10, 15), "Hello! A backend role in Tokyo."))
	o := newOpportunity(t, run, recruiter)
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "in_process"})
	addGmail(t, run, hays(mail("g2", "owner", ownersMessage, "Thank you, I'd be glad to talk with them.")))

	due := ownersMessage.Add(14 * day)
	n.tickAt(due)

	items := phone.claimWorkAt(due.Add(time.Minute))
	if len(items) != 1 || items[0].Kind != "follow_up" || items[0].Conversation.ID != conversation.ID {
		t.Errorf("the run claimed %+v; want one follow_up item for the Gmail Conversation", items)
	}
}

func TestARecruitersPageShowsWhichConversationsAreGmail(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, recruiter := fileHays(t, phone.connect(), mail("g1", "other", at(-3, 10, 15), "Hello! A backend role in Tokyo."))

	page := get[struct {
		Conversations []struct {
			Channel string `json:"channel"`
			Subject string `json:"subject"`
			URL     string `json:"url"`
		} `json:"conversations"`
	}](phone, "/api/career/recruiters/"+recruiter.ID)

	if len(page.Conversations) != 1 || page.Conversations[0].Channel != "gmail" ||
		page.Conversations[0].Subject != "Backend role at a fintech" || page.Conversations[0].URL == "" {
		t.Errorf("Recruiter page = %+v; want the Gmail Conversation with its channel, subject and Gmail link", page)
	}
}

func TestALinkedInThreadIDSentAsGmailIsRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	syncTanaka(t, phone) // 2-tanaka, on LinkedIn
	wrong := hays(mail("g1", "other", at(-1, 10, 15), "Hello!"))
	wrong["threadId"] = "2-tanaka"

	toolFails(t, phone.connect(), "career_add_gmail_thread", wrong)
}

func TestAnIgnoredGmailThreadKeepsNoSubject(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	added := addGmail(t, run, hays(mail("g1", "other", at(-3, 10, 15), "Our newsletter for November.")))

	got := callTool[gmailConversationJSON](t, run, "career_ignore_thread", map[string]any{"conversationId": added.Conversation.ID})

	if got.Subject != "" {
		t.Errorf("ignored Conversation's subject = %q; want only its thread ID kept", got.Subject)
	}
}

func TestAnAddressBelongsToTheRecruiterItWasLastFiledUnder(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	first := addGmail(t, run, hays(mail("g1", "other", at(-3, 10, 15), "From the shared inbox.")))
	callTool[struct{}](t, run, "career_file_conversation", map[string]any{
		"conversationId": first.Conversation.ID,
		"recruiter":      map[string]any{"name": "Yui Tanaka", "kind": "agency", "organisation": "Hays"},
	})
	second := hays(mail("g2", "other", at(-2, 10, 15), "Also from the shared inbox."))
	second["threadId"] = "18c2f0eeeeeeeeee"
	moved := addGmail(t, run, second)
	other := callTool[struct {
		Recruiter recruiterWithEmailsJSON `json:"recruiter"`
	}](t, run, "career_file_conversation", map[string]any{
		"conversationId": moved.Conversation.ID,
		"recruiter":      map[string]any{"name": "Ken Sato", "kind": "agency", "organisation": "Hays"},
	})
	// Filing the second thread under Sato isn't enough: it was filed to
	// Tanaka on arrival, so file the next one to check whose the address is.
	third := hays(mail("g3", "other", at(-1, 10, 15), "Still the shared inbox."))
	third["threadId"] = "18c2f0dddddddddd"

	if got := addGmail(t, run, third); got.Recruiter == nil || got.Recruiter.ID != other.Recruiter.ID {
		t.Errorf("third thread filed to %+v; want %s, the Recruiter the address was last filed under", got.Recruiter, other.Recruiter.ID)
	}
}

func TestARecruitersPageIdentifiesEachGmailMessage(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, recruiter := fileHays(t, phone.connect(),
		mail("g1", "other", at(-3, 10, 15), "Hello!", "JD.pdf"),
		mail("g2", "owner", at(-2, 10, 15), "Thanks!"))

	page := get[struct {
		Conversations []struct {
			Messages []struct {
				ID          string   `json:"id"`
				Attachments []string `json:"attachments"`
			} `json:"messages"`
		} `json:"conversations"`
	}](phone, "/api/career/recruiters/"+recruiter.ID)

	m := page.Conversations[0].Messages
	if len(m) != 2 || m[0].ID != "g1" || m[1].ID != "g2" || len(m[0].Attachments) != 1 {
		t.Errorf("page messages = %+v; want g1 (with its attachment) and g2, each with its id", m)
	}
}

func TestSearchFindsARecruiterByTheirEmailAddress(t *testing.T) {
	_, phone := signedInBrowser(t)
	run := phone.connect()
	_, recruiter := fileHays(t, run, mail("g1", "other", at(-3, 10, 15), "Hello!"))

	got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "Yui.Tanaka@hays.example"})

	if len(got.Recruiters) != 1 || got.Recruiters[0].ID != recruiter.ID {
		t.Errorf("career_search by address = %+v; want %s", got.Recruiters, recruiter.ID)
	}
}
