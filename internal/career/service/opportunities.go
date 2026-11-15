package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// OpportunitiesService serves the Tools for Opportunities, their Steps and
// their Companies.
type OpportunitiesService struct {
	opportunities model.OpportunitiesRepository
	companies     model.CompaniesRepository
	recruiters    model.RecruitersRepository
	now           func() time.Time
}

// NewOpportunities returns the OpportunitiesService.
func NewOpportunities(opportunities model.OpportunitiesRepository, companies model.CompaniesRepository, recruiters model.RecruitersRepository, now func() time.Time) *OpportunitiesService {
	return &OpportunitiesService{opportunities: opportunities, companies: companies, recruiters: recruiters, now: now}
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
	}, {
		Name: "career_save_company",
		Description: "Creates a Company, or, with id, updates one: only the fields sent change. " +
			"brief is the Company Brief, a summary from public information only (what they do, size, tech, reputation, red flags), " +
			"stopping where only the Owner could add more; notes are anything else worth keeping. " +
			"A new Company needs a name no Company has. Answers the Company with its Opportunities.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"The Company to update; leave out to create one."},` +
			`"name":{"type":"string"},` +
			`"linkedInUrl":{"type":"string"},` +
			`"websiteUrl":{"type":"string"},` +
			`"brief":{"type":"string"},` +
			`"notes":{"type":"string"}}}`),
		Handle: s.saveCompanyTool,
	}, {
		Name:        "career_get_company",
		Description: "A Company, with its Company Brief, notes and Opportunities. Answers the Company.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		ReadOnly:    true,
		Handle:      s.getCompanyTool,
	}}
}

func (s *OpportunitiesService) saveCompanyTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID          string  `json:"id"`
		Name        *string `json:"name"`
		LinkedInURL *string `json:"linkedInUrl"`
		WebsiteURL  *string `json:"websiteUrl"`
		Brief       *string `json:"brief"`
		Notes       *string `json:"notes"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	var company model.Company
	if args.ID != "" {
		var err error
		if company, err = findCompany(ctx, s.companies, args.ID); err != nil {
			return nil, err
		}
	} else if args.Name == nil {
		return nil, badOpportunity("a new Company needs its name")
	}
	if args.Name != nil {
		name := strings.TrimSpace(*args.Name)
		if name == "" {
			return nil, badOpportunity("a Company's name can't be empty")
		}
		other, err := s.companies.GetByName(ctx, name)
		if err == nil && other.ID != company.ID {
			return nil, fmt.Errorf("the Company %s (%s) already has that name", other.ID.Hex(), other.Name)
		}
		if err != nil && !errors.Is(err, coreutils.ErrNotFound) {
			return nil, err
		}
		company.Name = name
	}
	for field, value := range map[*string]*string{
		&company.LinkedInURL: args.LinkedInURL, &company.WebsiteURL: args.WebsiteURL,
		&company.Brief: args.Brief, &company.Notes: args.Notes,
	} {
		if value != nil {
			*field = strings.TrimSpace(*value)
		}
	}
	var err error
	if args.ID == "" {
		company, err = s.companies.Insert(ctx, company)
	} else {
		err = s.companies.Replace(ctx, company)
	}
	if err != nil {
		return nil, err
	}
	return s.companyJSON(ctx, company)
}

func (s *OpportunitiesService) getCompanyTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args idArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	company, err := findCompany(ctx, s.companies, args.ID)
	if err != nil {
		return nil, err
	}
	return s.companyJSON(ctx, company)
}

// companyJSON is the Company with its Opportunities.
func (s *OpportunitiesService) companyJSON(ctx context.Context, c model.Company) (model.CompanyJSON, error) {
	opportunities, err := s.opportunities.ByCompany(ctx, c.ID)
	if err != nil {
		return model.CompanyJSON{}, err
	}
	j := model.CompanyJSON{
		ID: c.ID.Hex(), Name: c.Name, LinkedInURL: c.LinkedInURL, WebsiteURL: c.WebsiteURL, Brief: c.Brief, Notes: c.Notes,
		Opportunities: []model.OpportunityJSON{},
	}
	for _, o := range opportunities {
		short, err := opportunityJSON(ctx, s.companies, o)
		if err != nil {
			return model.CompanyJSON{}, err
		}
		j.Opportunities = append(j.Opportunities, short)
	}
	return j, nil
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
	return struct{}{}, s.opportunities.Delete(ctx, o.ID)
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
		return nil, badOpportunity("a Step needs its kind, such as \"Casual interview\"")
	}
	if _, err := time.Parse(time.DateOnly, step.Date); err != nil {
		return nil, badOpportunity("a Step's date is YYYY-MM-DD")
	}
	if _, err := time.Parse("15:04", step.Time); step.Time != "" && err != nil {
		return nil, badOpportunity("a Step's time is HH:MM, or left out")
	}
	if args.StepID == "" {
		step.ID = bson.NewObjectID()
		o.Steps = append(o.Steps, step)
	} else {
		i := s.stepIndex(o, args.StepID)
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
	i := s.stepIndex(o, args.StepID)
	if i < 0 {
		return nil, fmt.Errorf("the Opportunity has no Step with the id %q", args.StepID)
	}
	o.Steps = slices.Delete(o.Steps, i, i+1)
	return s.replace(ctx, o)
}

// stepIndex is where the Step with the id is in o's Steps, or -1.
func (s *OpportunitiesService) stepIndex(o model.Opportunity, hex string) int {
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
	o.UpdatedAt = s.now()
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
		return nil, badOpportunity("only a faded Opportunity is reopened")
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

// badOpportunity is a Tool argument that can't be saved, said for the model.
type badOpportunity string

func (e badOpportunity) Error() string { return string(e) }

func (s *OpportunitiesService) saveTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args saveOpportunityArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	now := s.now()
	var o model.Opportunity
	if args.ID == "" {
		if args.Company == nil || args.Role == nil || strings.TrimSpace(*args.Role) == "" {
			return nil, badOpportunity("a new Opportunity needs its company and role")
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
			return badOpportunity("role can't be empty")
		}
	}
	if args.RecruiterID.Sent {
		o.RecruiterID = nil
		if args.RecruiterID.Value != nil {
			recruiter, err := findRecruiter(ctx, s.recruiters, *args.RecruiterID.Value)
			if err != nil {
				return err
			}
			o.RecruiterID = &recruiter.ID
		}
	}
	if err := applyStage(o, args.Stage, args.Outcome); err != nil {
		return err
	}
	if args.Interest != nil {
		if !slices.Contains([]string{model.InterestHigh, model.InterestNormal, model.InterestLow}, *args.Interest) {
			return badOpportunity("interest is high, normal or low")
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
				return badOpportunity("conditions.remote is remote, hybrid or office")
			}
			o.Conditions.Remote = *c.Remote
		}
		if c.WorkingLanguage != nil {
			if !slices.Contains([]string{model.LanguageJA, model.LanguageEN, model.LanguageMixed}, *c.WorkingLanguage) {
				return badOpportunity("conditions.workingLanguage is ja, en or mixed")
			}
			o.Conditions.WorkingLanguage = *c.WorkingLanguage
		}
	}
	if args.Perks != nil {
		o.Perks = trimmed(*args.Perks)
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
	if stage != nil && *stage != o.Stage {
		from, to := slices.Index(model.Stages, o.Stage), slices.Index(model.Stages, *stage)
		if to < 0 {
			return badOpportunity("stage is introduced, in_process, offer or closed")
		}
		if to < from {
			return badOpportunity(fmt.Sprintf("the Stage never goes back, from %s to %s; only a faded Opportunity is reopened, with career_reopen_opportunity", o.Stage, *stage))
		}
		if *stage == model.StageClosed {
			o.ClosedFrom = o.Stage
		}
		o.Stage = *stage
	}
	if outcome != nil {
		if !slices.Contains(model.Outcomes, *outcome) {
			return badOpportunity("outcome is accepted, declined, withdrawn, rejected or faded")
		}
		o.Outcome = *outcome
	}
	switch {
	case o.Stage == model.StageClosed && o.Outcome == "":
		return badOpportunity("closing an Opportunity needs its outcome: accepted, declined, withdrawn, rejected or faded")
	case o.Stage != model.StageClosed && o.Outcome != "":
		return badOpportunity("only a closed Opportunity has an outcome")
	}
	return nil
}

// company returns the Company ref names: an existing one by id, or by name,
// ignoring case, or a new one it stores.
func (s *OpportunitiesService) company(ctx context.Context, ref model.CompanyRefJSON) (model.Company, error) {
	if ref.ID != "" {
		return findCompany(ctx, s.companies, ref.ID)
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return model.Company{}, badOpportunity("company is {\"id\"} for an existing one or {\"name\"} for a new one")
	}
	company, err := s.companies.GetByName(ctx, name)
	if errors.Is(err, coreutils.ErrNotFound) {
		return s.companies.Insert(ctx, model.Company{Name: name})
	}
	return company, err
}

// opportunity returns the Opportunity with the id from a Tool's arguments.
func (s *OpportunitiesService) opportunity(ctx context.Context, hex string) (model.Opportunity, error) {
	missing := fmt.Errorf("no Opportunity has the id %q", hex)
	id, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return model.Opportunity{}, missing
	}
	o, err := s.opportunities.GetByID(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.Opportunity{}, missing
	}
	return o, err
}

// fullJSON is everything about o, with its Company's name.
func (s *OpportunitiesService) fullJSON(ctx context.Context, o model.Opportunity) (model.FullOpportunityJSON, error) {
	short, err := opportunityJSON(ctx, s.companies, o)
	if err != nil {
		return model.FullOpportunityJSON{}, err
	}
	j := model.FullOpportunityJSON{
		OpportunityJSON: short,
		FitNote:         o.FitNote,
		Details:         o.Details,
		Pay:             model.PayJSON{FirstStated: o.Pay.FirstStated, Offered: o.Pay.Offered, BonusIncluded: o.Pay.BonusIncluded},
		Conditions: model.ConditionsJSON{
			Remote:          orNull(o.Conditions.Remote),
			OfficeDays:      o.Conditions.OfficeDays,
			OfficeLocation:  o.Conditions.OfficeLocation,
			WorkingLanguage: orNull(o.Conditions.WorkingLanguage),
		},
		Perks:     append([]string{}, o.Perks...),
		Steps:     []model.StepJSON{},
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
	for _, step := range o.Steps {
		j.Steps = append(j.Steps, model.StepJSON{ID: step.ID.Hex(), Kind: step.Kind, Date: step.Date, Time: orNull(step.Time), Notes: step.Notes})
	}
	return j, nil
}

// opportunityJSON is o in short, with its Company's name from companies.
func opportunityJSON(ctx context.Context, companies model.CompaniesRepository, o model.Opportunity) (model.OpportunityJSON, error) {
	company, err := companies.GetByID(ctx, o.CompanyID)
	if err != nil {
		return model.OpportunityJSON{}, err
	}
	j := model.OpportunityJSON{
		ID:       o.ID.Hex(),
		Company:  model.CompanyRefJSON{ID: company.ID.Hex(), Name: company.Name},
		Role:     o.Role,
		Stage:    o.Stage,
		Outcome:  orNull(o.Outcome),
		Interest: o.Interest,
	}
	if o.RecruiterID != nil {
		id := o.RecruiterID.Hex()
		j.RecruiterID = &id
	}
	return j, nil
}

// findCompany returns the Company with the id from a Tool's arguments.
func findCompany(ctx context.Context, companies model.CompaniesRepository, hex string) (model.Company, error) {
	missing := fmt.Errorf("no Company has the id %q", hex)
	id, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return model.Company{}, missing
	}
	company, err := companies.GetByID(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.Company{}, missing
	}
	return company, err
}

// orNull is s, or nil if it is empty.
func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// trimmed is list with each item trimmed and the blank ones left out.
func trimmed(list []string) []string {
	out := []string{}
	for _, item := range list {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
