package tests

import (
	"net/http"
	"slices"
	"testing"
)

// homeCardJSON is the Career Home Card.
type homeCardJSON struct {
	Looking bool                 `json:"looking"`
	Waiting int                  `json:"waiting"`
	Top     []topOpportunityJSON `json:"top"`
}

func TestTheHomeCardShowsLookingHowManyRecruitersWaitAndTheTop3(t *testing.T) {
	_, phone := signedInBrowser(t)
	conversations, _ := fileThreads(t, phone,
		thread("tanaka", message("t1", "other", at(-3, 10, 15), "I have a backend role at a fintech.")),
		thread("sato", message("s1", "other", at(-2, 10, 0), "Hello!"), message("s2", "other", at(-2, 10, 5), "Any interest?")),
		thread("suzuki", message("z1", "other", at(-60, 10, 0), "Are you open to new roles?")),
	)
	suggestReply(t, phone, conversations["tanaka"], "answer", "Thank you!", at(-3, 10, 15))
	suggestReply(t, phone, conversations["sato"], "answer", "Hello Sato-san!", at(-2, 10, 5))
	suggestReply(t, phone, conversations["suzuki"], "late", "My deepest apologies.", at(-60, 10, 0))
	ids := seed(t, phone,
		seededOpportunity{role: "Normal offer", interest: "normal", stage: "offer", activity: 1},
		seededOpportunity{role: "High in process", interest: "high", stage: "in_process", steps: []string{"2026-11-10", "2026-11-19"}, activity: 2},
		seededOpportunity{role: "Low offer", interest: "low", stage: "offer", activity: 3},
		seededOpportunity{role: "High introduced", interest: "high", stage: "introduced", activity: 4},
	)
	phone.must(http.MethodPut, "/api/career/settings", map[string]any{"looking": true}, http.StatusNoContent)

	card := get[homeCardJSON](phone, "/api/career/home-card")
	if !card.Looking || card.Waiting != 2 {
		t.Errorf("the Home Card shows looking %t and %d Recruiters waiting; want Looking, and 2 waiting, not the late one", card.Looking, card.Waiting)
	}
	var top []string
	for _, o := range card.Top {
		top = append(top, o.Role)
	}
	if want := []string{"High in process", "High introduced", "Normal offer"}; !slices.Equal(top, want) {
		t.Fatalf("the Home Card's top = %q; want the top 3, %q", top, want)
	}
	first := card.Top[0]
	if first.ID != ids["High in process"] || first.Company.Name != "High in process Inc." || first.Company.ID == "" || first.Stage != "in_process" ||
		first.NextStep == nil || first.NextStep.Date != "2026-11-19" || first.NextStep.Kind != "Interview" {
		t.Errorf("the top Opportunity = %+v (next Step %+v); want its id, Company, role, Stage and the interview on 2026-11-19 next", first, first.NextStep)
	}
	if card.Top[1].NextStep != nil {
		t.Errorf("an Opportunity with no Steps has next Step %+v; want null", card.Top[1].NextStep)
	}
}

func TestTheHomeCardIsEmptyAtFirst(t *testing.T) {
	_, phone := signedInBrowser(t)
	card := get[homeCardJSON](phone, "/api/career/home-card")
	if card.Looking || card.Waiting != 0 || card.Top == nil || len(card.Top) != 0 {
		t.Errorf("the first Home Card = %+v; want Not Looking, nobody waiting and an empty top", card)
	}
}
