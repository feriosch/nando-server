// Package utils holds helpers the Career Module's layers share.
package utils

import (
	"slices"
	"strings"

	"github.com/feriosch/nando-server/internal/career/model"
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

// ToTopOpportunityJSON returns the open Opportunity o, at company, as the
// app's lists show it, with its next Step from today (YYYY-MM-DD).
func ToTopOpportunityJSON(o model.Opportunity, company model.Company, today string) model.TopOpportunityJSON {
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

// ToPageConversationJSON returns c as its Recruiter's page shows it, with its
// last n messages by when they were sent, oldest first.
func ToPageConversationJSON(c model.Conversation, n int) model.PageConversationJSON {
	messages := slices.Clone(c.Messages)
	slices.SortStableFunc(messages, func(a, b model.Message) int { return a.SentAt.Compare(b.SentAt) })
	if len(messages) > n {
		messages = messages[len(messages)-n:]
	}
	j := model.PageConversationJSON{ID: c.ID.Hex(), URL: c.URL, Messages: []model.MessageJSON{}}
	for _, m := range messages {
		j.Messages = append(j.Messages, model.MessageJSON{ID: m.LinkedInID, From: m.From, SentAt: m.SentAt, Text: m.Text})
	}
	if r := c.SuggestedReply; r != nil {
		j.SuggestedReply = &model.PageReplyJSON{Text: r.Text, Language: r.Language, Kind: r.Kind, Notes: append([]model.LanguageNote{}, r.Notes...)}
	}
	return j
}
