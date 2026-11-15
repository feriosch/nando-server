// Package utils holds helpers the Career Module's layers share.
package utils

import (
	"slices"
	"strings"
	"time"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// OrNull is s, or nil if s is "": an optional string as the Tools write it.
func OrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Trimmed is list with each item trimmed and the blank ones left out.
func Trimmed(list []string) []string {
	out := []string{}
	for _, item := range list {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// ToConversationJSON returns c as the Tools answer it.
func ToConversationJSON(c model.Conversation) model.ConversationJSON {
	j := model.ConversationJSON{
		ID:       c.ID.Hex(),
		ThreadID: c.ThreadID,
		URL:      c.URL,
		Person:   model.PersonJSON{Name: c.Person.Name, ProfileURL: c.Person.ProfileURL},
		Messages: []model.MessageJSON{},
		Ignored:  c.Ignored,
	}
	for _, m := range c.Messages {
		j.Messages = append(j.Messages, model.MessageJSON{ID: m.LinkedInID, From: m.From, SentAt: m.SentAt, Text: m.Text})
	}
	if c.RecruiterID != nil {
		id := c.RecruiterID.Hex()
		j.RecruiterID = &id
	}
	if r := c.SuggestedReply; r != nil {
		j.SuggestedReply = &model.SuggestedReplyJSON{
			Text: r.Text, Language: r.Language, Kind: r.Kind, Answers: r.Answers,
			Notes: append([]model.LanguageNote{}, r.Notes...), WrittenAt: r.WrittenAt,
		}
	}
	return j
}

// ToRecruiterJSON returns r as the Tools answer it.
func ToRecruiterJSON(r model.Recruiter) model.RecruiterJSON {
	return model.RecruiterJSON{ID: r.ID.Hex(), Name: r.Name, ProfileURL: r.ProfileURL, Kind: r.Kind, Organisation: r.Organisation}
}

// ToOpportunityJSON returns o in short, at company.
func ToOpportunityJSON(o model.Opportunity, company model.Company) model.OpportunityJSON {
	j := model.OpportunityJSON{
		ID:       o.ID.Hex(),
		Company:  model.CompanyRefJSON{ID: company.ID.Hex(), Name: company.Name},
		Role:     o.Role,
		Stage:    o.Stage,
		Outcome:  OrNull(o.Outcome),
		Interest: o.Interest,
	}
	if o.RecruiterID != nil {
		id := o.RecruiterID.Hex()
		j.RecruiterID = &id
	}
	return j
}

// ToPayJSON returns pay as the Tools and the API answer it.
func ToPayJSON(pay model.Pay) model.PayJSON {
	return model.PayJSON{FirstStated: pay.FirstStated, Offered: pay.Offered, BonusIncluded: pay.BonusIncluded}
}

// ToConditionsJSON returns c as the Tools and the API answer them.
func ToConditionsJSON(c model.Conditions) model.ConditionsJSON {
	return model.ConditionsJSON{
		Remote:          OrNull(c.Remote),
		OfficeDays:      c.OfficeDays,
		OfficeLocation:  c.OfficeLocation,
		WorkingLanguage: OrNull(c.WorkingLanguage),
	}
}

// ToStepJSON returns step as the Tools and the API answer it.
func ToStepJSON(step model.Step) model.StepJSON {
	return model.StepJSON{ID: step.ID.Hex(), Kind: step.Kind, Date: step.Date, Time: OrNull(step.Time), Notes: step.Notes}
}

// ToTopOpportunityJSON returns the open Opportunity o, at company, as the
// app's lists show it, with its next Step from today.
func ToTopOpportunityJSON(o model.Opportunity, company model.Company, today coremodel.Day) model.TopOpportunityJSON {
	j := model.TopOpportunityJSON{
		ID:       o.ID.Hex(),
		Company:  model.CompanyRefJSON{ID: company.ID.Hex(), Name: company.Name},
		Role:     o.Role,
		Stage:    o.Stage,
		Interest: o.Interest,
	}
	if step := NextStep(o, today); step != nil {
		j.NextStep = &model.NextStepJSON{Kind: step.Kind, Date: step.Date, Time: OrNull(step.Time)}
	}
	return j
}

// ToOpportunityPageJSON returns o, at company and brought by recruiter, if
// any, as its page shows it, with its next Step from today marked.
func ToOpportunityPageJSON(o model.Opportunity, company model.Company, recruiter *model.Recruiter, today coremodel.Day) model.OpportunityPageJSON {
	j := model.OpportunityPageJSON{
		ID:         o.ID.Hex(),
		Role:       o.Role,
		Stage:      o.Stage,
		Outcome:    OrNull(o.Outcome),
		Interest:   o.Interest,
		FitNote:    o.FitNote,
		Details:    o.Details,
		Pay:        ToPayJSON(o.Pay),
		Conditions: ToConditionsJSON(o.Conditions),
		Perks:      append([]string{}, o.Perks...),
		Steps:      []model.OpportunityPageStepJSON{},
		Company: model.OpportunityPageCompanyJSON{
			ID: company.ID.Hex(), Name: company.Name, LinkedInURL: company.LinkedInURL, WebsiteURL: company.WebsiteURL, Brief: company.Brief,
		},
	}
	next := NextStep(o, today)
	for _, step := range o.Steps {
		j.Steps = append(j.Steps, model.OpportunityPageStepJSON{StepJSON: ToStepJSON(step), Next: next != nil && step.ID == next.ID})
	}
	if recruiter != nil {
		j.Recruiter = &model.RecruiterRefJSON{ID: recruiter.ID.Hex(), Name: recruiter.Name}
	}
	return j
}

// ToRecruiterPageConversationJSON returns c as its Recruiter's page shows it,
// with its last n messages by when they were sent, oldest first.
func ToRecruiterPageConversationJSON(c model.Conversation, n int) model.RecruiterPageConversationJSON {
	messages := slices.Clone(c.Messages)
	slices.SortStableFunc(messages, func(a, b model.Message) int { return a.SentAt.Compare(b.SentAt) })
	if len(messages) > n {
		messages = messages[len(messages)-n:]
	}
	j := model.RecruiterPageConversationJSON{ID: c.ID.Hex(), URL: c.URL, Messages: []model.MessageJSON{}}
	for _, m := range messages {
		j.Messages = append(j.Messages, model.MessageJSON{ID: m.LinkedInID, From: m.From, SentAt: m.SentAt, Text: m.Text})
	}
	if r := c.SuggestedReply; r != nil {
		j.SuggestedReply = &model.RecruiterPageReplyJSON{Text: r.Text, Language: r.Language, Kind: r.Kind, Notes: append([]model.LanguageNote{}, r.Notes...)}
	}
	return j
}

// ToRecruiterReplyJSON returns recruiter with c's Suggested Reply, as the
// Career screen and Late Replies list them.
func ToRecruiterReplyJSON(recruiter model.Recruiter, c model.Conversation) model.RecruiterReplyJSON {
	firstLine, _, _ := strings.Cut(c.SuggestedReply.Text, "\n")
	return model.RecruiterReplyJSON{
		ID: recruiter.ID.Hex(), Name: recruiter.Name, Organisation: recruiter.Organisation,
		LastMessageAt: lastFromRecruiter(c), FirstLine: strings.TrimSpace(firstLine),
	}
}

// lastFromRecruiter is when c's newest message from its Recruiter was sent,
// or its newest message, if they sent none.
func lastFromRecruiter(c model.Conversation) time.Time {
	var last time.Time
	for _, m := range c.Messages {
		if m.From == model.MessageFromOther && m.SentAt.After(last) {
			last = m.SentAt
		}
	}
	if last.IsZero() {
		return c.Newest
	}
	return last
}
