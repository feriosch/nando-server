package tests

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fullOpportunityJSON is what career_save_opportunity and
// career_get_opportunity answer.
type fullOpportunityJSON struct {
	opportunityJSON
	FitNote    string         `json:"fitNote"`
	Details    string         `json:"details"`
	Pay        payJSON        `json:"pay"`
	Conditions conditionsJSON `json:"conditions"`
	Perks      []string       `json:"perks"`
	Steps      []stepJSON     `json:"steps"`
}

type payJSON struct {
	FirstStated   *salaryJSON `json:"firstStated"`
	Offered       *salaryJSON `json:"offered"`
	BonusIncluded *bool       `json:"bonusIncluded"`
}

type salaryJSON struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type conditionsJSON struct {
	Remote          *string `json:"remote"`
	OfficeDays      *int    `json:"officeDays"`
	OfficeLocation  string  `json:"officeLocation"`
	WorkingLanguage *string `json:"workingLanguage"`
}

type stepJSON struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Date  string  `json:"date"`
	Time  *string `json:"time"`
	Notes string  `json:"notes"`
}

// newOpportunity has the run save Tanaka's fintech Opportunity, at a new
// Company, and returns it.
func newOpportunity(t *testing.T, run *mcp.ClientSession, tanaka recruiterJSON) fullOpportunityJSON {
	t.Helper()
	return callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{
		"company":     map[string]any{"name": "Kumo Pay"},
		"role":        "Backend Engineer",
		"recruiterId": tanaka.ID,
		"fitNote":     "Go and payments: close to what the Owner does now.",
		"pay":         map[string]any{"firstStated": map[string]any{"min": 9000000, "max": 12000000}, "bonusIncluded": true},
		"conditions":  map[string]any{"remote": "hybrid", "officeDays": 2, "officeLocation": "Shibuya", "workingLanguage": "en"},
		"perks":       []string{"Housing allowance"},
	})
}

func TestAnOpportunitysStageSkipsAheadButNeverGoesBack(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)

	created := newOpportunity(t, run, tanaka)
	if created.ID == "" || created.Company.ID == "" || created.Company.Name != "Kumo Pay" || created.Role != "Backend Engineer" ||
		created.Stage != "introduced" || created.Outcome != nil || created.Interest != "normal" ||
		created.RecruiterID == nil || *created.RecruiterID != tanaka.ID ||
		created.Pay.FirstStated == nil || created.Pay.FirstStated.Max != 12000000 ||
		created.Conditions.OfficeDays == nil || *created.Conditions.OfficeDays != 2 || len(created.Perks) != 1 {
		t.Fatalf("career_save_opportunity = %+v; want a new introduced Opportunity at Kumo Pay with Normal Interest", created)
	}

	offer := callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": created.ID, "stage": "offer"})
	if offer.Stage != "offer" || offer.Role != "Backend Engineer" || offer.Pay.FirstStated == nil {
		t.Errorf("skipping to offer answered %+v; want the offer Stage, the rest unchanged", offer)
	}
	toolFails(t, run, "career_save_opportunity", map[string]any{"id": created.ID, "stage": "in_process"})
}

func TestClosingAnOpportunityNeedsAnOutcome(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)

	toolFails(t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "closed"})
	toolFails(t, run, "career_save_opportunity", map[string]any{"id": o.ID, "outcome": "rejected"})

	closed := callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "closed", "outcome": "rejected"})
	if closed.Stage != "closed" || closed.Outcome == nil || *closed.Outcome != "rejected" {
		t.Errorf("closing with an outcome answered stage %q, outcome %v; want closed and rejected", closed.Stage, closed.Outcome)
	}
}

func TestOnlyAFadedOpportunityIsReopenedToTheStageItHad(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	faded := newOpportunity(t, run, tanaka)
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": faded.ID, "stage": "in_process"})
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": faded.ID, "stage": "closed", "outcome": "faded"})

	reopened := callTool[fullOpportunityJSON](t, run, "career_reopen_opportunity", map[string]any{"id": faded.ID})
	if reopened.Stage != "in_process" || reopened.Outcome != nil {
		t.Errorf("reopening a faded Opportunity answered stage %q, outcome %v; want in_process, and no outcome", reopened.Stage, reopened.Outcome)
	}

	for _, outcome := range []string{"accepted", "declined", "withdrawn", "rejected"} {
		o := newOpportunity(t, run, tanaka)
		callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "closed", "outcome": outcome})
		toolFails(t, run, "career_reopen_opportunity", map[string]any{"id": o.ID})
	}
	toolFails(t, run, "career_reopen_opportunity", map[string]any{"id": newOpportunity(t, run, tanaka).ID})
}

func TestStepsAreAddedChangedAndDeleted(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)

	casual := callTool[fullOpportunityJSON](t, run, "career_save_step", map[string]any{
		"opportunityId": o.ID, "kind": "Casual interview", "date": "2026-11-20", "time": "18:00", "notes": "With the CTO.",
	})
	if len(casual.Steps) != 1 || casual.Steps[0].ID == "" || casual.Steps[0].Kind != "Casual interview" ||
		casual.Steps[0].Date != "2026-11-20" || casual.Steps[0].Time == nil || *casual.Steps[0].Time != "18:00" {
		t.Fatalf("adding a Step answered steps %+v; want the casual interview on 2026-11-20 at 18:00", casual.Steps)
	}
	stepID := casual.Steps[0].ID
	callTool[fullOpportunityJSON](t, run, "career_save_step", map[string]any{
		"opportunityId": o.ID, "kind": "Take-home assignment", "date": "2026-11-18",
	})
	moved := callTool[fullOpportunityJSON](t, run, "career_save_step", map[string]any{
		"opportunityId": o.ID, "stepId": stepID, "kind": "Casual interview", "date": "2026-11-21", "notes": "Moved by a day.",
	})
	if len(moved.Steps) != 2 || moved.Steps[0].Kind != "Take-home assignment" || moved.Steps[1].ID != stepID ||
		moved.Steps[1].Date != "2026-11-21" || moved.Steps[1].Time != nil {
		t.Fatalf("changing a Step answered steps %+v; want both, by date, the interview moved to 2026-11-21 with no time", moved.Steps)
	}
	for _, bad := range []map[string]any{
		{"opportunityId": o.ID, "kind": "Final interview", "date": "21 Nov"},
		{"opportunityId": o.ID, "kind": "Final interview", "date": "2026-11-21", "time": "6pm"},
		{"opportunityId": o.ID, "kind": " ", "date": "2026-11-21"},
		{"opportunityId": o.ID, "stepId": "000000000000000000000000", "kind": "Final interview", "date": "2026-11-21"},
	} {
		toolFails(t, run, "career_save_step", bad)
	}

	callTool[fullOpportunityJSON](t, run, "career_delete_step", map[string]any{"opportunityId": o.ID, "stepId": stepID})
	got := callTool[fullOpportunityJSON](t, run, "career_get_opportunity", map[string]any{"id": o.ID})
	if len(got.Steps) != 1 || got.Steps[0].Kind != "Take-home assignment" || got.FitNote != o.FitNote {
		t.Errorf("career_get_opportunity after deleting the interview = %+v; want only the assignment left", got)
	}
}

func TestAnOpportunityIsDeleted(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)

	callTool[map[string]any](t, run, "career_delete_opportunity", map[string]any{"id": o.ID})

	toolFails(t, run, "career_get_opportunity", map[string]any{"id": o.ID})
	toolFails(t, run, "career_delete_opportunity", map[string]any{"id": o.ID})
}

func TestOnlyDeletingIsDestructive(t *testing.T) {
	_, phone := signedInBrowser(t)
	tools, err := phone.connect().ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	destructive := []string{}
	for _, tool := range tools.Tools {
		if strings.HasPrefix(tool.Name, "career_") && tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			destructive = append(destructive, tool.Name)
		}
	}
	slices.Sort(destructive)
	if want := []string{"career_delete_opportunity", "career_delete_step"}; !slices.Equal(destructive, want) {
		t.Errorf("destructive Career Tools = %v; want %v", destructive, want)
	}
}
