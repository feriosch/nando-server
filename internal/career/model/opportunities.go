package model

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Opportunity is one role at one Company put to the Owner, from the first
// message about it until it closes.
type Opportunity struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	CompanyID bson.ObjectID `bson:"companyId"`
	Role      string        `bson:"role"`
	// RecruiterID is the Recruiter who brought it, if one did.
	RecruiterID *bson.ObjectID `bson:"recruiterId,omitempty"`
	// Stage is one of Stages. It skips ahead but never goes back.
	Stage string `bson:"stage"`
	// Outcome is how a closed Opportunity ended, one of Outcomes.
	Outcome string `bson:"outcome,omitempty"`
	// ClosedFrom is the Stage it had before it closed, so a faded one can
	// be reopened to it.
	ClosedFrom string `bson:"closedFrom,omitempty"`
	// Interest is InterestHigh, InterestNormal or InterestLow.
	Interest   string     `bson:"interest"`
	FitNote    string     `bson:"fitNote"`
	Details    string     `bson:"details"`
	Pay        Pay        `bson:"pay"`
	Conditions Conditions `bson:"conditions"`
	Perks      []string   `bson:"perks"`
	Steps      []Step     `bson:"steps"`
	CreatedAt  time.Time  `bson:"createdAt"`
	// UpdatedAt is when it last changed through the Tools.
	UpdatedAt time.Time `bson:"updatedAt"`
}

// Pay is an Opportunity's annual salary, in JPY, and its bonus.
type Pay struct {
	// FirstStated is the salary range as first stated.
	FirstStated *Salary `bson:"firstStated,omitempty"`
	// Offered is the salary range offered.
	Offered       *Salary `bson:"offered,omitempty"`
	BonusIncluded *bool   `bson:"bonusIncluded,omitempty"`
}

// Salary is an annual salary range in JPY.
type Salary struct {
	Min int `bson:"min" json:"min"`
	Max int `bson:"max" json:"max"`
}

// Conditions are an Opportunity's working conditions.
type Conditions struct {
	// Remote is RemoteFull, RemoteHybrid or RemoteOffice.
	Remote string `bson:"remote,omitempty"`
	// OfficeDays is how many days a week a hybrid role is in the office.
	OfficeDays     *int   `bson:"officeDays,omitempty"`
	OfficeLocation string `bson:"officeLocation,omitempty"`
	// WorkingLanguage is LanguageJA, LanguageEN or LanguageMixed.
	WorkingLanguage string `bson:"workingLanguage,omitempty"`
}

// Step is one dated event inside an Opportunity's process.
type Step struct {
	ID   bson.ObjectID `bson:"_id"`
	Kind string        `bson:"kind"`
	// Date is YYYY-MM-DD, and Time, if known, HH:MM, in the Owner Time Zone.
	Date  string `bson:"date"`
	Time  string `bson:"time,omitempty"`
	Notes string `bson:"notes"`
}

// The Stages, in order.
const (
	StageIntroduced = "introduced"
	StageInProcess  = "in_process"
	StageOffer      = "offer"
	StageClosed     = "closed"
)

// Stages are the Stages in order: an Opportunity only moves right.
var Stages = []string{StageIntroduced, StageInProcess, StageOffer, StageClosed}

// The Outcomes of a closed Opportunity.
const (
	OutcomeAccepted  = "accepted"
	OutcomeDeclined  = "declined"
	OutcomeWithdrawn = "withdrawn"
	OutcomeRejected  = "rejected"
	OutcomeFaded     = "faded"
)

// Outcomes are every Outcome.
var Outcomes = []string{OutcomeAccepted, OutcomeDeclined, OutcomeWithdrawn, OutcomeRejected, OutcomeFaded}

// The levels of Interest.
const (
	InterestHigh   = "high"
	InterestNormal = "normal"
	InterestLow    = "low"
)

// The remote policies.
const (
	RemoteFull   = "remote"
	RemoteHybrid = "hybrid"
	RemoteOffice = "office"
)

// The working languages.
const (
	LanguageJA    = "ja"
	LanguageEN    = "en"
	LanguageMixed = "mixed"
)

// OpportunitiesRepository reads and writes the opportunities collection.
type OpportunitiesRepository interface {
	// Insert stores a new Opportunity and returns it with its id.
	Insert(ctx context.Context, opportunity Opportunity) (Opportunity, error)
	// GetByID returns the Opportunity, or Core's utils.ErrNotFound.
	GetByID(ctx context.Context, id bson.ObjectID) (Opportunity, error)
	// Replace stores the Opportunity over the one with its id, or returns
	// Core's utils.ErrNotFound.
	Replace(ctx context.Context, opportunity Opportunity) error
	// Delete deletes the Opportunity, or returns Core's utils.ErrNotFound.
	Delete(ctx context.Context, id bson.ObjectID) error
	// ByRecruiter returns the Opportunities the Recruiter brought.
	ByRecruiter(ctx context.Context, recruiterID bson.ObjectID) ([]Opportunity, error)
	// ByCompany returns the Company's Opportunities.
	ByCompany(ctx context.Context, companyID bson.ObjectID) ([]Opportunity, error)
	// Search returns the Opportunities whose role has query in it, ignoring
	// case, or that are at one of companyIDs.
	Search(ctx context.Context, query string, companyIDs []bson.ObjectID) ([]Opportunity, error)
	// OpenWithStepOn returns the Opportunities that aren't closed and have a
	// Step on date, YYYY-MM-DD.
	OpenWithStepOn(ctx context.Context, date string) ([]Opportunity, error)
	// Open returns every Opportunity that isn't closed.
	Open(ctx context.Context) ([]Opportunity, error)
	// Fade closes the Opportunity with Outcome faded, keeping the Stage it
	// had to reopen it to, unless it changed since it was read as o. It
	// reports whether it closed it.
	Fade(ctx context.Context, o Opportunity) (bool, error)
}

// The JSON shapes below are what the Tools and the API answer with and are
// sent.

// OpportunityJSON is an Opportunity in short, as lists show it.
type OpportunityJSON struct {
	ID          string         `json:"id"`
	Company     CompanyRefJSON `json:"company"`
	Role        string         `json:"role"`
	RecruiterID *string        `json:"recruiterId"`
	Stage       string         `json:"stage"`
	Outcome     *string        `json:"outcome"`
	Interest    string         `json:"interest"`
}

// FullOpportunityJSON is everything about an Opportunity.
type FullOpportunityJSON struct {
	OpportunityJSON
	FitNote    string         `json:"fitNote"`
	Details    string         `json:"details"`
	Pay        PayJSON        `json:"pay"`
	Conditions ConditionsJSON `json:"conditions"`
	Perks      []string       `json:"perks"`
	Steps      []StepJSON     `json:"steps"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// PayJSON is Pay as the Tools answer and are sent it.
type PayJSON struct {
	FirstStated   *Salary `json:"firstStated"`
	Offered       *Salary `json:"offered"`
	BonusIncluded *bool   `json:"bonusIncluded"`
}

// ConditionsJSON is Conditions as the Tools answer and are sent them.
type ConditionsJSON struct {
	Remote          *string `json:"remote"`
	OfficeDays      *int    `json:"officeDays"`
	OfficeLocation  string  `json:"officeLocation"`
	WorkingLanguage *string `json:"workingLanguage"`
}

// OpportunityDeletedJSON is what career_delete_opportunity answers: the
// deleted Opportunity's id.
type OpportunityDeletedJSON struct {
	Deleted string `json:"deleted"`
}

// StepJSON is a Step as the Tools answer it.
type StepJSON struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind"`
	Date  string  `json:"date"`
	Time  *string `json:"time"`
	Notes string  `json:"notes"`
}

// TopOpportunityJSON is an open Opportunity as the app's lists show it, top
// first.
type TopOpportunityJSON struct {
	ID       string         `json:"id"`
	Company  CompanyRefJSON `json:"company"`
	Role     string         `json:"role"`
	Stage    string         `json:"stage"`
	Interest string         `json:"interest"`
	// NextStep is its next Step, or null when none is left.
	NextStep *NextStepJSON `json:"nextStep"`
}

// NextStepJSON is an Opportunity's next Step, as the app's lists show it.
type NextStepJSON struct {
	Kind string  `json:"kind"`
	Date string  `json:"date"`
	Time *string `json:"time"`
}

// OpportunityPageJSON is everything about an Opportunity, open or closed, as
// its page in the app shows it.
type OpportunityPageJSON struct {
	ID         string         `json:"id"`
	Role       string         `json:"role"`
	Stage      string         `json:"stage"`
	Outcome    *string        `json:"outcome"`
	Interest   string         `json:"interest"`
	FitNote    string         `json:"fitNote"`
	Details    string         `json:"details"`
	Pay        PayJSON        `json:"pay"`
	Conditions ConditionsJSON `json:"conditions"`
	Perks      []string       `json:"perks"`
	// Steps are by date, then time.
	Steps []OpportunityPageStepJSON `json:"steps"`
	// Recruiter is who brought it, or null.
	Recruiter *RecruiterRefJSON          `json:"recruiter"`
	Company   OpportunityPageCompanyJSON `json:"company"`
}

// OpportunityPageStepJSON is a Step on an Opportunity's page, marked if it is
// the next.
type OpportunityPageStepJSON struct {
	StepJSON
	Next bool `json:"next"`
}
