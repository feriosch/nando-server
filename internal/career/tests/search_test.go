package tests

import (
	"testing"
	"time"
)

// searchJSON is what career_search answers.
type searchJSON struct {
	Recruiters    []foundRecruiterJSON `json:"recruiters"`
	Companies     []companyJSON        `json:"companies"`
	Opportunities []opportunityJSON    `json:"opportunities"`
}

func TestSearchFindsRecruitersCompaniesAndOpportunitiesByName(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)
	callTool[companyJSON](t, run, "career_save_company", map[string]any{"name": "Sakura Games"})

	for _, c := range []struct {
		query                                string
		recruiters, companies, opportunities int
	}{
		{"tanaka", 1, 0, 0},
		{"kumo", 0, 1, 1},
		{"backend", 0, 0, 1},
		{"Hays", 1, 0, 0},
		{"games", 0, 1, 0},
		{"nobody", 0, 0, 0},
	} {
		got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": c.query})
		if len(got.Recruiters) != c.recruiters || len(got.Companies) != c.companies || len(got.Opportunities) != c.opportunities {
			t.Errorf("career_search %q = %+v; want %d Recruiters, %d Companies and %d Opportunities",
				c.query, got, c.recruiters, c.companies, c.opportunities)
		}
	}
	if got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "kumo"}); len(got.Opportunities) == 1 && got.Opportunities[0].ID != o.ID {
		t.Errorf("career_search kumo found Opportunity %s; want %s", got.Opportunities[0].ID, o.ID)
	}
}

// foundRecruiterJSON is a Recruiter as career_search answers it: with their
// Conversations, so a chat can reach a Suggested Reply, and the Opportunities
// they brought, so a chat can tell a Lead.
type foundRecruiterJSON struct {
	recruiterJSON
	Conversations []struct {
		ID                string `json:"id"`
		HasSuggestedReply bool   `json:"hasSuggestedReply"`
	} `json:"conversations"`
	Opportunities []opportunityJSON `json:"opportunities"`
}

func TestSearchFindsARecruiterWithTheOpportunitiesTheyBrought(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	found := func() foundRecruiterJSON {
		t.Helper()
		got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "tanaka"})
		if len(got.Recruiters) != 1 {
			t.Fatalf("career_search tanaka found %+v; want Tanaka", got.Recruiters)
		}
		return got.Recruiters[0]
	}

	if lead := found(); lead.Opportunities == nil || len(lead.Opportunities) != 0 {
		t.Errorf("Tanaka, a Lead, comes with Opportunities %+v; want none, as []", lead.Opportunities)
	}
	o := newOpportunity(t, run, tanaka)
	if got := found().Opportunities; len(got) != 1 || got[0].ID != o.ID {
		t.Errorf("Tanaka comes with Opportunities %+v; want the one they brought, %s", got, o.ID)
	}
}

func TestSearchLeadsFromARecruiterToTheirSuggestedReply(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, filed := fileTanakasConversation(t, phone)
	found := func() foundRecruiterJSON {
		t.Helper()
		got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "tanaka"})
		if len(got.Recruiters) != 1 || got.Recruiters[0].ID != filed.Recruiter.ID || len(got.Recruiters[0].Conversations) != 1 ||
			got.Recruiters[0].Conversations[0].ID != filed.Conversation.ID {
			t.Fatalf("career_search tanaka found %+v; want Tanaka with Conversation %s", got.Recruiters, filed.Conversation.ID)
		}
		return got.Recruiters[0]
	}

	if found().Conversations[0].HasSuggestedReply {
		t.Errorf("before the run wrote one, Tanaka's Conversation has a Suggested Reply")
	}
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": filed.Conversation.ID, "text": "Thank you for reaching out!",
		"language": "en", "kind": "answer", "answers": pitched.Format(time.RFC3339),
	})
	if !found().Conversations[0].HasSuggestedReply {
		t.Errorf("after the run wrote one, Tanaka's Conversation has no Suggested Reply")
	}
}

func TestSearchFindsARecruiterByTheirProfileLinkWhateverTheirNameInTheInbox(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	if tanaka.ProfileURL != "https://www.linkedin.com/in/tanaka-yui/" {
		t.Fatalf("filed under a new Recruiter sent with no profile link, Tanaka is %+v; want the Conversation's link", tanaka)
	}

	for _, query := range []string{"https://www.linkedin.com/in/tanaka-yui/", " https://www.linkedin.com/in/tanaka-yui "} {
		got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": query}).Recruiters
		if len(got) != 1 || got[0].ID != tanaka.ID {
			t.Errorf("career_search %q found %+v; want Tanaka, by their profile link", query, got)
		}
	}
	if got := callTool[searchJSON](t, run, "career_search", map[string]any{"query": "https://www.linkedin.com/in/someone-else/"}).Recruiters; len(got) != 0 {
		t.Errorf("career_search for another profile link found %+v; want nobody", got)
	}
}
