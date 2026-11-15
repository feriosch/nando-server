package tests

import "testing"

// companyJSON is what career_save_company and career_get_company answer.
type companyJSON struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	LinkedInURL   string            `json:"linkedInUrl"`
	WebsiteURL    string            `json:"websiteUrl"`
	Brief         string            `json:"brief"`
	Notes         string            `json:"notes"`
	Opportunities []opportunityJSON `json:"opportunities"`
}

func TestACompanyKeepsItsBriefAcrossItsOpportunities(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)

	saved := callTool[companyJSON](t, run, "career_save_company", map[string]any{
		"id":         o.Company.ID,
		"websiteUrl": "https://kumopay.example.com",
		"brief":      "Payments for small shops in Japan, about 200 people.",
	})
	if saved.ID != o.Company.ID || saved.Name != "Kumo Pay" || saved.Brief == "" || saved.WebsiteURL != "https://kumopay.example.com" {
		t.Errorf("career_save_company = %+v; want Kumo Pay with its brief and website", saved)
	}
	// The same name, in another case, is the same Company.
	second := callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{
		"company": map[string]any{"name": "KUMO PAY"}, "role": "SRE",
	})
	if second.Company.ID != o.Company.ID {
		t.Errorf("an Opportunity at KUMO PAY is at Company %+v; want Kumo Pay, %s", second.Company, o.Company.ID)
	}

	got := callTool[companyJSON](t, run, "career_get_company", map[string]any{"id": o.Company.ID})
	if got.Brief != saved.Brief || len(got.Opportunities) != 2 {
		t.Errorf("career_get_company = %+v; want the brief and both Opportunities", got)
	}

	created := callTool[companyJSON](t, run, "career_save_company", map[string]any{"name": "Sakura Games", "notes": "Tip from a friend."})
	if created.ID == "" || created.ID == o.Company.ID || created.Notes != "Tip from a friend." || len(created.Opportunities) != 0 {
		t.Errorf("saving a new Company = %+v; want Sakura Games, new, with no Opportunities", created)
	}
	toolFails(t, run, "career_save_company", map[string]any{"name": "kumo pay"})
	toolFails(t, run, "career_save_company", map[string]any{"brief": "No name."})
}
