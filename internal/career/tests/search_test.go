package tests

import "testing"

// searchJSON is what career_search answers.
type searchJSON struct {
	Recruiters    []recruiterJSON   `json:"recruiters"`
	Companies     []companyJSON     `json:"companies"`
	Opportunities []opportunityJSON `json:"opportunities"`
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
