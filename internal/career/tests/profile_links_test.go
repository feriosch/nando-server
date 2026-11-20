package tests

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tanakasLink is Tanaka-san's LinkedIn profile, as the Sync script reads it
// from a thread's page.
const tanakasLink = "https://www.linkedin.com/in/ACoAATanaka/"

// fileTanakaWithoutALink files 2-tanaka, which a Sync sent with no profile
// link as the first Syncs did, under a new Recruiter, Tanaka, sent with none.
// It returns the run's session and Tanaka.
func fileTanakaWithoutALink(t *testing.T, phone *browser) (*mcp.ClientSession, recruiterJSON) {
	t.Helper()
	phone.saveTimeZone("Asia/Tokyo")
	unlinked := tanakaThread(message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo."))
	unlinked["person"] = map[string]any{"name": "Tanaka Yui", "profileUrl": ""}
	callTool[recordedJSON](t, phone.macAt(at(0, 8, 0)), "career_record_sync", map[string]any{"threads": []any{unlinked}})
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]
	tanaka := callTool[filedJSON](t, run, "career_file_conversation", map[string]any{
		"conversationId": item.Conversation.ID,
		"recruiter":      map[string]any{"name": "Tanaka Yui", "kind": "agency", "organisation": "Hays Japan"},
	}).Recruiter
	if tanaka.ProfileURL != "" {
		t.Fatalf("Tanaka, filed from a thread with no link, has profileUrl %q; want none", tanaka.ProfileURL)
	}
	return run, tanaka
}

// tanakasLinkIs fails the test unless career_search finds Tanaka by name with
// that profileUrl.
func tanakasLinkIs(t *testing.T, run *mcp.ClientSession, tanaka recruiterJSON, want string) {
	t.Helper()
	got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "Tanaka"})
	if len(got.Recruiters) != 1 || got.Recruiters[0].ID != tanaka.ID {
		t.Fatalf("career_search Tanaka found %+v; want Tanaka alone, %s", got.Recruiters, tanaka.ID)
	}
	if got.Recruiters[0].ProfileURL != want {
		t.Errorf("Tanaka's profileUrl = %q; want %q", got.Recruiters[0].ProfileURL, want)
	}
}

func TestFilingAThreadWithALinkUnderARecruiterWithNoneGivesThemTheLink(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, tanaka := fileTanakaWithoutALink(t, phone)
	studio := tanakaThread(message("s1", "other", at(0, 9, 0), "Another role, at a game studio this time."))
	studio["threadId"] = "2-tanaka-studio"
	studio["person"] = map[string]any{"name": "Tanaka Yui (田中 結)", "profileUrl": tanakasLink}
	callTool[recordedJSON](t, phone.macAt(at(0, 10, 0)), "career_record_sync", map[string]any{"threads": []any{studio}})
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]

	filed := callTool[filedJSON](t, run, "career_file_conversation", map[string]any{"conversationId": item.Conversation.ID, "recruiterId": tanaka.ID})

	if filed.Recruiter.ProfileURL != tanakasLink {
		t.Errorf("career_file_conversation answered Tanaka with profileUrl %q; want %q", filed.Recruiter.ProfileURL, tanakasLink)
	}
	tanakasLinkIs(t, run, tanaka, tanakasLink)
}

func TestANewRecruiterSentWithNoLinkTakesTheirThreadsLink(t *testing.T) {
	_, phone := signedInBrowser(t)
	// 2-tanaka comes with Tanaka-san's link, but the run files it without one.
	run, filed := fileTanakasConversation(t, phone)

	if filed.Recruiter.ProfileURL != "https://www.linkedin.com/in/tanaka-yui/" {
		t.Errorf("career_file_conversation answered Tanaka with profileUrl %q; want their thread's link", filed.Recruiter.ProfileURL)
	}
	tanakasLinkIs(t, run, filed.Recruiter, "https://www.linkedin.com/in/tanaka-yui/")
}

func TestASyncThatBringsAFiledThreadsLinkGivesItToItsRecruiter(t *testing.T) {
	_, phone := signedInBrowser(t)
	_, tanaka := fileTanakaWithoutALink(t, phone)
	linked := tanakaThread(message("m2", "other", at(0, 14, 0), "Are you free for a call this week?"))
	linked["person"] = map[string]any{"name": "Tanaka Yui", "profileUrl": tanakasLink}

	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{linked}})

	tanakasLinkIs(t, phone.connect(), tanaka, tanakasLink)
}

func TestARecruitersLinkIsNeverReplaced(t *testing.T) {
	_, phone := signedInBrowser(t)
	// Tanaka takes 2-tanaka's link, https://www.linkedin.com/in/tanaka-yui/.
	_, filed := fileTanakasConversation(t, phone)
	tanaka := filed.Recruiter
	other := map[string]any{"name": "Tanaka Yui", "profileUrl": "https://www.linkedin.com/in/ACoAAOther/"}
	relinked := tanakaThread(message("m2", "other", at(0, 9, 0), "Are you free for a call this week?"))
	relinked["person"] = other
	studio := tanakaThread(message("s1", "other", at(0, 9, 30), "Another role, at a game studio this time."))
	studio["threadId"], studio["person"] = "2-tanaka-studio", other

	callTool[recordedJSON](t, phone.macAt(at(0, 10, 0)), "career_record_sync", map[string]any{"threads": []any{relinked, studio}})
	run := phone.connect()
	var studioID string
	for _, item := range callTool[workJSON](t, run, "career_list_work", nil).Items {
		if item.Conversation.ThreadID == "2-tanaka-studio" {
			studioID = item.Conversation.ID
		}
	}
	callTool[filedJSON](t, run, "career_file_conversation", map[string]any{"conversationId": studioID, "recruiterId": tanaka.ID})

	tanakasLinkIs(t, run, tanaka, "https://www.linkedin.com/in/tanaka-yui/")
}

func TestANewThreadFromARecruiterUnderAnotherNameIsFoundByTheirLink(t *testing.T) {
	_, phone := signedInBrowser(t)
	// The first Syncs sent no links: Tanaka was filed without one.
	_, tanaka := fileTanakaWithoutALink(t, phone)
	linked := tanakaThread(message("m2", "other", at(0, 9, 0), "Are you free for a call this week?"))
	linked["person"] = map[string]any{"name": "Tanaka Yui", "profileUrl": tanakasLink}
	callTool[recordedJSON](t, phone.macAt(at(0, 10, 0)), "career_record_sync", map[string]any{"threads": []any{linked}})

	// Weeks later, Tanaka-san writes in a new thread, named another way.
	studio := tanakaThread(message("s1", "other", at(20, 9, 0), "Another role, at a game studio this time."))
	studio["threadId"] = "2-tanaka-studio"
	studio["person"] = map[string]any{"name": "結 🌸 Hays", "profileUrl": tanakasLink}
	callTool[recordedJSON](t, phone.macAt(at(20, 10, 0)), "career_record_sync", map[string]any{"threads": []any{studio}})
	run := phone.connect()
	var item itemJSON
	for _, i := range callTool[workJSON](t, run, "career_list_work", nil).Items {
		if i.Kind == "sort" {
			item = i
		}
	}

	if got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "結 🌸 Hays"}); len(got.Recruiters) != 0 {
		t.Fatalf("career_search by the new name found %+v; this test wants a name that doesn't find Tanaka", got.Recruiters)
	}
	got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": item.Conversation.Person.ProfileURL})
	if len(got.Recruiters) != 1 || got.Recruiters[0].ID != tanaka.ID {
		t.Errorf("career_search by the new thread's link %q found %+v; want Tanaka, %s", item.Conversation.Person.ProfileURL, got.Recruiters, tanaka.ID)
	}
}
