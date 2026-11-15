package tests

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
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

func TestAClosedOpportunityIsNotMadeFadedToReopenIt(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "closed", "outcome": "rejected"})

	toolFails(t, run, "career_save_opportunity", map[string]any{"id": o.ID, "outcome": "faded"})

	// The Owner may still correct how it ended.
	corrected := callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "outcome": "withdrawn"})
	if corrected.Outcome == nil || *corrected.Outcome != "withdrawn" {
		t.Errorf("correcting the outcome answered %v; want withdrawn", corrected.Outcome)
	}
}

// topOpportunityJSON is an open Opportunity as the app's lists show it.
type topOpportunityJSON struct {
	ID      string `json:"id"`
	Company struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"company"`
	Role     string        `json:"role"`
	Stage    string        `json:"stage"`
	Interest string        `json:"interest"`
	NextStep *nextStepJSON `json:"nextStep"`
}

// nextStepJSON is an Opportunity's next Step, as the app's lists show it.
type nextStepJSON struct {
	Kind string  `json:"kind"`
	Date string  `json:"date"`
	Time *string `json:"time"`
}

// seededOpportunity is an Opportunity the Owner's chat saves, in a ranking
// table: its Interest, Stage and Steps, and when it last changed.
type seededOpportunity struct {
	role, interest, stage string
	// steps are its Steps' dates, each YYYY-MM-DD, or YYYY-MM-DD HH:MM.
	steps []string
	// activity is the minute after 09:00 on Monday it last changed.
	activity int
}

// seed has the Owner's chat save each Opportunity, at its own minute, and
// returns their ids by role. It leaves the clock at 09:59 on Monday.
func seed(t *testing.T, phone *browser, opportunities ...seededOpportunity) map[string]string {
	t.Helper()
	phone.saveTimeZone("Asia/Tokyo")
	chat := phone.macAt(at(0, 9, 0))
	ids := map[string]string{}
	for _, o := range opportunities {
		phone.nando.clock.Set(at(0, 9, o.activity))
		saved := callTool[fullOpportunityJSON](t, chat, "career_save_opportunity", map[string]any{
			"company": map[string]any{"name": o.role + " Inc."}, "role": o.role, "interest": o.interest,
		})
		for _, when := range o.steps {
			date, clock, _ := strings.Cut(when, " ")
			callTool[fullOpportunityJSON](t, chat, "career_save_step", map[string]any{"opportunityId": saved.ID, "kind": "Interview", "date": date, "time": clock})
		}
		if o.stage != "introduced" {
			args := map[string]any{"id": saved.ID, "stage": o.stage}
			if o.stage == "closed" {
				args["outcome"] = "rejected"
			}
			callTool[fullOpportunityJSON](t, chat, "career_save_opportunity", args)
		}
		ids[o.role] = saved.ID
	}
	phone.nando.clock.Set(at(0, 9, 59))
	return ids
}

func TestOpenOpportunitiesAreRankedByInterestThenHowFarAlongThenActivity(t *testing.T) {
	_, phone := signedInBrowser(t)
	// Today is Monday 2026-11-16. Saved out of order, so that neither when
	// they were saved nor when they last changed gives the ranking.
	seed(t, phone,
		seededOpportunity{role: "Low offer", interest: "low", stage: "offer", activity: 50},
		seededOpportunity{role: "Normal introduced, older", interest: "normal", stage: "introduced", activity: 1},
		seededOpportunity{role: "High in process, no Step left", interest: "high", stage: "in_process", steps: []string{"2026-11-01"}, activity: 40},
		seededOpportunity{role: "High introduced", interest: "high", stage: "introduced", activity: 45},
		seededOpportunity{role: "High in process, the 25th at 18:00", interest: "high", stage: "in_process", steps: []string{"2026-11-10", "2026-11-25 18:00"}, activity: 30},
		seededOpportunity{role: "High in process, the 25th at 10:00", interest: "high", stage: "in_process", steps: []string{"2026-11-25 10:00"}, activity: 25},
		seededOpportunity{role: "Closed, high", interest: "high", stage: "closed", activity: 55},
		seededOpportunity{role: "Normal offer", interest: "normal", stage: "offer", activity: 2},
		seededOpportunity{role: "High in process, Step today", interest: "high", stage: "in_process", steps: []string{"2026-12-01", "2026-11-16"}, activity: 20},
		seededOpportunity{role: "Normal introduced, newer", interest: "normal", stage: "introduced", activity: 3},
		seededOpportunity{role: "High offer", interest: "high", stage: "offer", activity: 10},
	)

	var ranked []string
	for _, o := range get[[]topOpportunityJSON](phone, "/api/career/opportunities") {
		ranked = append(ranked, o.Role)
	}
	want := []string{
		"High offer",
		"High in process, Step today",
		"High in process, the 25th at 10:00",
		"High in process, the 25th at 18:00",
		"High in process, no Step left",
		"High introduced",
		"Normal offer",
		"Normal introduced, newer",
		"Normal introduced, older",
		"Low offer",
	}
	if !slices.Equal(ranked, want) {
		t.Errorf("GET /api/career/opportunities ranked\n%q\nwant\n%q", ranked, want)
	}
}

// opportunityPageJSON is everything about an Opportunity, as its page in the
// app shows it.
type opportunityPageJSON struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"`
	Stage      string         `json:"stage"`
	Outcome    *string        `json:"outcome"`
	Interest   string         `json:"interest"`
	FitNote    string         `json:"fitNote"`
	Details    string         `json:"details"`
	Pay        payJSON        `json:"pay"`
	Conditions conditionsJSON `json:"conditions"`
	Perks      []string       `json:"perks"`
	Steps      []struct {
		stepJSON
		Next bool `json:"next"`
	} `json:"steps"`
	Recruiter *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"recruiter"`
	Company struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		LinkedInURL string `json:"linkedInUrl"`
		WebsiteURL  string `json:"websiteUrl"`
		Brief       string `json:"brief"`
	} `json:"company"`
}

func TestTheOpportunityPageShowsEverythingAboutIt(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, tanaka := fileTanaka(t, phone)
	o := newOpportunity(t, run, tanaka)
	callTool[companyJSON](t, run, "career_save_company", map[string]any{
		"id": o.Company.ID, "websiteUrl": "https://kumopay.example.com", "brief": "Payments for small shops in Japan, about 200 people.",
	})
	// Today is Monday 2026-11-16.
	for _, step := range []map[string]any{
		{"kind": "Casual interview", "date": "2026-11-10", "time": "18:00"},
		{"kind": "Final interview", "date": "2026-11-27"},
		{"kind": "Take-home assignment", "date": "2026-11-16", "time": "10:00", "notes": "Due by noon."},
	} {
		step["opportunityId"] = o.ID
		callTool[fullOpportunityJSON](t, run, "career_save_step", step)
	}
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "in_process", "interest": "high", "details": "Ledger team."})

	page := get[opportunityPageJSON](phone, "/api/career/opportunities/"+o.ID)
	if page.ID != o.ID || page.Role != "Backend Engineer" || page.Stage != "in_process" || page.Outcome != nil || page.Interest != "high" ||
		page.FitNote != "Go and payments: close to what the Owner does now." || page.Details != "Ledger team." ||
		page.Pay.FirstStated == nil || page.Pay.FirstStated.Min != 9000000 || page.Pay.BonusIncluded == nil || !*page.Pay.BonusIncluded ||
		page.Conditions.Remote == nil || *page.Conditions.Remote != "hybrid" || page.Conditions.OfficeLocation != "Shibuya" ||
		!slices.Equal(page.Perks, []string{"Housing allowance"}) {
		t.Errorf("the Opportunity page = %+v; want Tanaka's in-process Backend Engineer role with its pay, conditions, Perks and fit note", page)
	}
	if page.Recruiter == nil || page.Recruiter.ID != tanaka.ID || page.Recruiter.Name != "Tanaka Yui" {
		t.Errorf("its Recruiter = %+v; want Tanaka Yui, %s", page.Recruiter, tanaka.ID)
	}
	if page.Company.ID != o.Company.ID || page.Company.Name != "Kumo Pay" || page.Company.WebsiteURL != "https://kumopay.example.com" ||
		page.Company.Brief != "Payments for small shops in Japan, about 200 people." {
		t.Errorf("its Company = %+v; want Kumo Pay with its Company Brief", page.Company)
	}
	var steps []string
	for _, step := range page.Steps {
		steps = append(steps, fmt.Sprintf("%s %s next=%t", step.Date, step.Kind, step.Next))
	}
	if want := []string{
		"2026-11-10 Casual interview next=false",
		"2026-11-16 Take-home assignment next=true",
		"2026-11-27 Final interview next=false",
	}; !slices.Equal(steps, want) {
		t.Errorf("its Steps =\n%q\nwant, by date, with today's marked next,\n%q", steps, want)
	}

	// A closed one is shown too, with how it ended.
	callTool[fullOpportunityJSON](t, run, "career_save_opportunity", map[string]any{"id": o.ID, "stage": "closed", "outcome": "declined"})
	if closed := get[opportunityPageJSON](phone, "/api/career/opportunities/"+o.ID); closed.Stage != "closed" || closed.Outcome == nil || *closed.Outcome != "declined" {
		t.Errorf("the closed Opportunity's page shows stage %q, outcome %v; want closed and declined", closed.Stage, closed.Outcome)
	}
}

func TestAnOpportunityWithNoRecruiterOrStepsLeftHasNone(t *testing.T) {
	_, phone := signedInBrowser(t)
	ids := seed(t, phone, seededOpportunity{role: "Found myself", interest: "normal", stage: "in_process", steps: []string{"2026-11-01"}})

	page := get[opportunityPageJSON](phone, "/api/career/opportunities/"+ids["Found myself"])
	if page.Recruiter != nil || len(page.Steps) != 1 || page.Steps[0].Next {
		t.Errorf("an Opportunity the Owner found, with only a past Step, shows recruiter %+v and steps %+v; want none and none next", page.Recruiter, page.Steps)
	}
}

func TestAnUnknownOpportunityIsNotFound(t *testing.T) {
	_, phone := signedInBrowser(t)
	for _, path := range []string{"/api/career/opportunities/" + bson.NewObjectID().Hex(), "/api/career/opportunities/not-an-id"} {
		phone.must(http.MethodGet, path, nil, http.StatusNotFound)
	}
}
