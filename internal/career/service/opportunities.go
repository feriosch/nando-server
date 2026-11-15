package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	careerutils "github.com/feriosch/nando-server/internal/career/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// OpportunitiesService serves the Tools for Opportunities and their Steps,
// and the app's routes that read them.
type OpportunitiesService struct {
	opportunities model.OpportunitiesRepository
	companies     model.CompaniesRepository
	recruiters    model.RecruitersRepository
	core          coremodel.CoreService
}

// NewOpportunities returns the OpportunitiesService.
func NewOpportunities(opportunities model.OpportunitiesRepository, companies model.CompaniesRepository, recruiters model.RecruitersRepository, core coremodel.CoreService) *OpportunitiesService {
	return &OpportunitiesService{opportunities: opportunities, companies: companies, recruiters: recruiters, core: core}
}

// Routes adds the routes that read Opportunities to mux, under
// /api/career/opportunities.
func (s *OpportunitiesService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/career/opportunities", s.listOpen)
	mux.HandleFunc("GET /api/career/opportunities/{id}", s.getPage)
}

// listOpen answers every open Opportunity, top first.
func (s *OpportunitiesService) listOpen(w http.ResponseWriter, r *http.Request) {
	ranked, err := s.Ranked(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, ranked)
}

// Ranked is every open Opportunity, top first, as the app's lists show them,
// for the Module's other services too.
func (s *OpportunitiesService) Ranked(ctx context.Context) ([]model.TopOpportunityJSON, error) {
	open, err := s.opportunities.Open(ctx)
	if err != nil {
		return nil, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	careerutils.Rank(open, today)
	ranked := []model.TopOpportunityJSON{}
	for _, o := range open {
		company, err := s.companies.GetByID(ctx, o.CompanyID)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, careerutils.ToTopOpportunityJSON(o, company, today))
	}
	return ranked, nil
}

// getPage answers everything about an Opportunity, open or closed, with its
// Recruiter and Company.
func (s *OpportunitiesService) getPage(w http.ResponseWriter, r *http.Request) {
	o, ok := careerutils.FromPath(w, r, "Opportunity", s.opportunities.GetByID)
	if !ok {
		return
	}
	company, err := s.companies.GetByID(r.Context(), o.CompanyID)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	var recruiter *model.Recruiter
	if o.RecruiterID != nil {
		brought, err := s.recruiters.GetByID(r.Context(), *o.RecruiterID)
		if err != nil {
			coreutils.ServerError(w, err)
			return
		}
		recruiter = &brought
	}
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, careerutils.ToOpportunityPageJSON(o, company, recruiter, today))
}

// salarySchema is the JSON Schema of a salary range.
const salarySchema = `{"type":"object","description":"Annual, in JPY.","properties":{"min":{"type":"integer"},"max":{"type":"integer"}}}`

// Tools are what the run and the Owner's chat keep Opportunities with.
func (s *OpportunitiesService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "career_save_opportunity",
		Description: "Creates an Opportunity, one role at one Company, or, with id, updates one: only the fields sent change. " +
			"company is an existing one ({\"id\"}) or a new one by name ({\"name\"}); a name the Tools already hold is that Company. " +
			"A new Opportunity needs company and role, starts introduced with Normal Interest, and has the Recruiter who brought it, if any. " +
			"The stage skips ahead (introduced, in_process, offer, closed) but never goes back. closed needs an outcome: " +
			"accepted, declined (the Owner turned down an Offer), withdrawn (the Owner stepped away before an Offer), " +
			"rejected (the Company said no) or faded. pay and conditions replace what was there. " +
			"Answers the Opportunity.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"The Opportunity to update; leave out to create one."},` +
			`"company":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"}}},` +
			`"role":{"type":"string"},` +
			`"recruiterId":{"type":["string","null"],"description":"The Recruiter who brought it; null for none."},` +
			`"stage":{"type":"string","enum":["introduced","in_process","offer","closed"]},` +
			`"outcome":{"type":"string","enum":["accepted","declined","withdrawn","rejected","faded"]},` +
			`"interest":{"type":"string","enum":["high","normal","low"]},` +
			`"fitNote":{"type":"string","description":"How well it fits the Career Profile."},` +
			`"details":{"type":"string","description":"Free text, such as a job-description summary or what the Recruiter said."},` +
			`"pay":{"type":"object","properties":{` +
			`"firstStated":` + salarySchema + `,` +
			`"offered":` + salarySchema + `,` +
			`"bonusIncluded":{"type":"boolean"}}},` +
			`"conditions":{"type":"object","properties":{` +
			`"remote":{"type":"string","enum":["remote","hybrid","office"]},` +
			`"officeDays":{"type":"integer","description":"For hybrid: days a week in the office."},` +
			`"officeLocation":{"type":"string"},` +
			`"workingLanguage":{"type":"string","enum":["ja","en","mixed"]}}},` +
			`"perks":{"type":"array","items":{"type":"string"},"description":"What it offers beyond pay and conditions, such as a housing allowance."}}}`),
		Handle: s.saveTool,
	}, {
		Name: "career_reopen_opportunity",
		Description: "Reopens a faded Opportunity, one closed because 30 days passed with no message about it, " +
			"back to the Stage it had. No other closed Opportunity is reopened. Answers the Opportunity.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Handle:      s.reopenTool,
	}, {
		Name:        "career_get_opportunity",
		Description: "Everything about an Opportunity, open or closed, with its Steps by date. Answers the Opportunity.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		ReadOnly:    true,
		Handle:      s.getTool,
	}, {
		Name: "career_delete_opportunity",
		Description: "Deletes an Opportunity and its Steps for good, for an Opportunity saved by mistake. " +
			"It can't be undone: ask the Owner first. An Opportunity that ended is closed with an outcome instead. Answers {}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Destructive: true,
		Handle:      s.deleteTool,
	}, {
		Name: "career_save_step",
		Description: "Adds a Step to an Opportunity, one dated event in its process of any kind (casual interview, take-home assignment, final interview…), " +
			"or, with stepId, replaces one. date is YYYY-MM-DD and time, if known, HH:MM, in the Owner Time Zone. Answers the Opportunity.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"opportunityId":{"type":"string"},` +
			`"stepId":{"type":"string","description":"The Step to replace; leave out to add one."},` +
			`"kind":{"type":"string"},` +
			`"date":{"type":"string","format":"date"},` +
			`"time":{"type":"string","description":"HH:MM"},` +
			`"notes":{"type":"string"}},` +
			`"required":["opportunityId","kind","date"]}`),
		Handle: s.saveStepTool,
	}, {
		Name:        "career_delete_step",
		Description: "Deletes a Step from an Opportunity for good, for a Step saved by mistake. It can't be undone: ask the Owner first. Answers the Opportunity.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"opportunityId":{"type":"string"},"stepId":{"type":"string"}},"required":["opportunityId","stepId"]}`),
		Destructive: true,
		Handle:      s.deleteStepTool,
	}}
}

// idArgs are the arguments of a Tool that takes only an id.
type idArgs struct {
	ID string `json:"id"`
}

func (s *OpportunitiesService) getTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args idArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	o, err := s.opportunity(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return s.fullJSON(ctx, o)
}

func (s *OpportunitiesService) deleteTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args idArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	o, err := s.opportunity(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if err := s.opportunities.Delete(ctx, o.ID); err != nil {
		return nil, err
	}
	return model.OpportunityDeletedJSON{Deleted: o.ID.Hex()}, nil
}

func (s *OpportunitiesService) saveStepTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		OpportunityID string `json:"opportunityId"`
		StepID        string `json:"stepId"`
		Kind          string `json:"kind"`
		Date          string `json:"date"`
		Time          string `json:"time"`
		Notes         string `json:"notes"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	o, err := s.opportunity(ctx, args.OpportunityID)
	if err != nil {
		return nil, err
	}
	step := model.Step{Kind: strings.TrimSpace(args.Kind), Date: args.Date, Time: args.Time, Notes: strings.TrimSpace(args.Notes)}
	if step.Kind == "" {
		return nil, errors.New("a Step needs its kind, such as \"Casual interview\"")
	}
	if _, err := time.Parse(time.DateOnly, step.Date); err != nil {
		return nil, errors.New("a Step's date is YYYY-MM-DD")
	}
	if _, err := time.Parse("15:04", step.Time); step.Time != "" && err != nil {
		return nil, errors.New("a Step's time is HH:MM, or left out")
	}
	if args.StepID == "" {
		step.ID = bson.NewObjectID()
		o.Steps = append(o.Steps, step)
	} else {
		i := stepIndex(o, args.StepID)
		if i < 0 {
			return nil, fmt.Errorf("the Opportunity has no Step with the id %q", args.StepID)
		}
		step.ID = o.Steps[i].ID
		o.Steps[i] = step
	}
	sortSteps(o.Steps)
	return s.replace(ctx, o)
}

func (s *OpportunitiesService) deleteStepTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		OpportunityID string `json:"opportunityId"`
		StepID        string `json:"stepId"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	o, err := s.opportunity(ctx, args.OpportunityID)
	if err != nil {
		return nil, err
	}
	i := stepIndex(o, args.StepID)
	if i < 0 {
		return nil, fmt.Errorf("the Opportunity has no Step with the id %q", args.StepID)
	}
	o.Steps = slices.Delete(o.Steps, i, i+1)
	return s.replace(ctx, o)
}

// stepIndex is where the Step with the id is in o's Steps, or -1.
func stepIndex(o model.Opportunity, hex string) int {
	return slices.IndexFunc(o.Steps, func(step model.Step) bool { return step.ID.Hex() == hex })
}

// sortSteps sorts Steps by date, then time, a Step with no time first.
func sortSteps(steps []model.Step) {
	slices.SortStableFunc(steps, func(a, b model.Step) int {
		return strings.Compare(a.Date+" "+a.Time, b.Date+" "+b.Time)
	})
}

// replace stores o, changed now through the Tools, and answers it.
func (s *OpportunitiesService) replace(ctx context.Context, o model.Opportunity) (any, error) {
	o.UpdatedAt = s.core.Now()
	if err := s.opportunities.Replace(ctx, o); err != nil {
		return nil, err
	}
	return s.fullJSON(ctx, o)
}

func (s *OpportunitiesService) reopenTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args idArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	o, err := s.opportunity(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if o.Stage != model.StageClosed || o.Outcome != model.OutcomeFaded {
		return nil, errors.New("only a faded Opportunity is reopened")
	}
	o.Stage, o.Outcome, o.ClosedFrom = o.ClosedFrom, "", ""
	return s.replace(ctx, o)
}

// optional is a Tool argument that may be left out, sent as null, or sent
// with a value.
type optional[T any] struct {
	Sent  bool
	Value *T
}

func (o *optional[T]) UnmarshalJSON(b []byte) error {
	o.Sent = true
	if string(b) == "null" {
		return nil
	}
	o.Value = new(T)
	return json.Unmarshal(b, o.Value)
}

// saveOpportunityArgs are career_save_opportunity's arguments. A field left
// out is nil, and keeps what is stored.
type saveOpportunityArgs struct {
	ID          string                `json:"id"`
	Company     *model.CompanyRefJSON `json:"company"`
	Role        *string               `json:"role"`
	RecruiterID optional[string]      `json:"recruiterId"`
	Stage       *string               `json:"stage"`
	Outcome     *string               `json:"outcome"`
	Interest    *string               `json:"interest"`
	FitNote     *string               `json:"fitNote"`
	Details     *string               `json:"details"`
	Pay         *model.PayJSON        `json:"pay"`
	Conditions  *model.ConditionsJSON `json:"conditions"`
	Perks       *[]string             `json:"perks"`
}

func (s *OpportunitiesService) saveTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args saveOpportunityArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	now := s.core.Now()
	var o model.Opportunity
	if args.ID == "" {
		if args.Company == nil || args.Role == nil || strings.TrimSpace(*args.Role) == "" {
			return nil, errors.New("a new Opportunity needs its company and role")
		}
		o = model.Opportunity{Stage: model.StageIntroduced, Interest: model.InterestNormal, Perks: []string{}, Steps: []model.Step{}, CreatedAt: now}
	} else {
		var err error
		if o, err = s.opportunity(ctx, args.ID); err != nil {
			return nil, err
		}
	}
	if err := s.apply(ctx, &o, args); err != nil {
		return nil, err
	}
	o.UpdatedAt = now
	if args.ID == "" {
		var err error
		if o, err = s.opportunities.Insert(ctx, o); err != nil {
			return nil, err
		}
	} else if err := s.opportunities.Replace(ctx, o); err != nil {
		return nil, err
	}
	return s.fullJSON(ctx, o)
}

// apply checks args and makes their changes to o, storing a new Company if
// args name one. Every check comes before the Company is stored.
func (s *OpportunitiesService) apply(ctx context.Context, o *model.Opportunity, args saveOpportunityArgs) error {
	if args.Role != nil {
		if o.Role = strings.TrimSpace(*args.Role); o.Role == "" {
			return errors.New("role can't be empty")
		}
	}
	if args.RecruiterID.Sent {
		o.RecruiterID = nil
		if args.RecruiterID.Value != nil {
			id, err := careerutils.ID("Recruiter", *args.RecruiterID.Value)
			if err != nil {
				return err
			}
			recruiter, err := s.recruiters.GetByID(ctx, id)
			if err != nil {
				return careerutils.Found(err, "Recruiter", *args.RecruiterID.Value)
			}
			o.RecruiterID = &recruiter.ID
		}
	}
	if err := applyStage(o, args.Stage, args.Outcome); err != nil {
		return err
	}
	if args.Interest != nil {
		if !slices.Contains([]string{model.InterestHigh, model.InterestNormal, model.InterestLow}, *args.Interest) {
			return errors.New("interest is high, normal or low")
		}
		o.Interest = *args.Interest
	}
	if args.FitNote != nil {
		o.FitNote = strings.TrimSpace(*args.FitNote)
	}
	if args.Details != nil {
		o.Details = strings.TrimSpace(*args.Details)
	}
	if args.Pay != nil {
		o.Pay = model.Pay{FirstStated: args.Pay.FirstStated, Offered: args.Pay.Offered, BonusIncluded: args.Pay.BonusIncluded}
	}
	if args.Conditions != nil {
		c := args.Conditions
		o.Conditions = model.Conditions{OfficeDays: c.OfficeDays, OfficeLocation: strings.TrimSpace(c.OfficeLocation)}
		if c.Remote != nil {
			if !slices.Contains([]string{model.RemoteFull, model.RemoteHybrid, model.RemoteOffice}, *c.Remote) {
				return errors.New("conditions.remote is remote, hybrid or office")
			}
			o.Conditions.Remote = *c.Remote
		}
		if c.WorkingLanguage != nil {
			if !slices.Contains([]string{model.LanguageJA, model.LanguageEN, model.LanguageMixed}, *c.WorkingLanguage) {
				return errors.New("conditions.workingLanguage is ja, en or mixed")
			}
			o.Conditions.WorkingLanguage = *c.WorkingLanguage
		}
	}
	if args.Perks != nil {
		o.Perks = careerutils.Trimmed(*args.Perks)
	}
	if args.Company != nil {
		company, err := s.company(ctx, *args.Company)
		if err != nil {
			return err
		}
		o.CompanyID = company.ID
	}
	return nil
}

// applyStage moves o to stage, which may only be ahead of its Stage, with
// outcome, which only a closed Opportunity has.
func applyStage(o *model.Opportunity, stage, outcome *string) error {
	// Faded is for an Opportunity that went quiet while open: a closed one
	// made faded could be reopened, which only a faded one may be.
	if o.Stage == model.StageClosed && outcome != nil && *outcome == model.OutcomeFaded && o.Outcome != model.OutcomeFaded {
		return errors.New("a closed Opportunity can't become faded: only one that went quiet while open is")
	}
	if stage != nil && *stage != o.Stage {
		from, to := slices.Index(model.Stages, o.Stage), slices.Index(model.Stages, *stage)
		if to < 0 {
			return errors.New("stage is introduced, in_process, offer or closed")
		}
		if to < from {
			return fmt.Errorf("the Stage never goes back, from %s to %s; only a faded Opportunity is reopened, with career_reopen_opportunity", o.Stage, *stage)
		}
		if *stage == model.StageClosed {
			o.ClosedFrom = o.Stage
		}
		o.Stage = *stage
	}
	if outcome != nil {
		if !slices.Contains(model.Outcomes, *outcome) {
			return errors.New("outcome is accepted, declined, withdrawn, rejected or faded")
		}
		o.Outcome = *outcome
	}
	switch {
	case o.Stage == model.StageClosed && o.Outcome == "":
		return errors.New("closing an Opportunity needs its outcome: accepted, declined, withdrawn, rejected or faded")
	case o.Stage != model.StageClosed && o.Outcome != "":
		return errors.New("only a closed Opportunity has an outcome")
	}
	return nil
}

// company returns the Company ref names: an existing one by id, or by name,
// ignoring case, or a new one it stores.
func (s *OpportunitiesService) company(ctx context.Context, ref model.CompanyRefJSON) (model.Company, error) {
	if ref.ID != "" {
		id, err := careerutils.ID("Company", ref.ID)
		if err != nil {
			return model.Company{}, err
		}
		company, err := s.companies.GetByID(ctx, id)
		return company, careerutils.Found(err, "Company", ref.ID)
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return model.Company{}, errors.New("company is {\"id\"} for an existing one or {\"name\"} for a new one")
	}
	company, err := s.companies.GetByName(ctx, name)
	if errors.Is(err, coreutils.ErrNotFound) {
		return s.companies.Insert(ctx, model.Company{Name: name})
	}
	return company, err
}

// opportunity returns the Opportunity with the id from a Tool's arguments.
func (s *OpportunitiesService) opportunity(ctx context.Context, hex string) (model.Opportunity, error) {
	id, err := careerutils.ID("Opportunity", hex)
	if err != nil {
		return model.Opportunity{}, err
	}
	o, err := s.opportunities.GetByID(ctx, id)
	return o, careerutils.Found(err, "Opportunity", hex)
}

// fullJSON is everything about o, with its Company's name.
func (s *OpportunitiesService) fullJSON(ctx context.Context, o model.Opportunity) (model.FullOpportunityJSON, error) {
	short, err := s.Short(ctx, o)
	if err != nil {
		return model.FullOpportunityJSON{}, err
	}
	j := model.FullOpportunityJSON{
		OpportunityJSON: short,
		FitNote:         o.FitNote,
		Details:         o.Details,
		Pay:             careerutils.ToPayJSON(o.Pay),
		Conditions:      careerutils.ToConditionsJSON(o.Conditions),
		Perks:           append([]string{}, o.Perks...),
		Steps:           []model.StepJSON{},
		CreatedAt:       o.CreatedAt,
		UpdatedAt:       o.UpdatedAt,
	}
	for _, step := range o.Steps {
		j.Steps = append(j.Steps, careerutils.ToStepJSON(step))
	}
	return j, nil
}

// Short is o in short, with its Company's name, for the Module's other
// services too.
func (s *OpportunitiesService) Short(ctx context.Context, o model.Opportunity) (model.OpportunityJSON, error) {
	company, err := s.companies.GetByID(ctx, o.CompanyID)
	if err != nil {
		return model.OpportunityJSON{}, err
	}
	return careerutils.ToOpportunityJSON(o, company), nil
}

// ByRecruiter is the Opportunities the Recruiter brought, in short.
func (s *OpportunitiesService) ByRecruiter(ctx context.Context, recruiterID bson.ObjectID) ([]model.OpportunityJSON, error) {
	brought, err := s.opportunities.ByRecruiter(ctx, recruiterID)
	if err != nil {
		return nil, err
	}
	return s.shortAll(ctx, brought)
}

// shortAll is every Opportunity in opportunities, in short.
func (s *OpportunitiesService) shortAll(ctx context.Context, opportunities []model.Opportunity) ([]model.OpportunityJSON, error) {
	all := []model.OpportunityJSON{}
	for _, o := range opportunities {
		short, err := s.Short(ctx, o)
		if err != nil {
			return nil, err
		}
		all = append(all, short)
	}
	return all, nil
}
