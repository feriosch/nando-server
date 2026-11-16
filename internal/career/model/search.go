package model

// SearchJSON is what career_search answers.
type SearchJSON struct {
	Recruiters    []FoundRecruiterJSON `json:"recruiters"`
	Companies     []CompanyRefJSON     `json:"companies"`
	Opportunities []OpportunityJSON    `json:"opportunities"`
}

// FoundRecruiterJSON is a Recruiter as career_search answers it, with their
// Conversations, so that a chat can reach a Suggested Reply.
type FoundRecruiterJSON struct {
	RecruiterJSON
	Conversations []FoundConversationJSON `json:"conversations"`
	// Opportunities are the ones the Recruiter brought, in short; none for a
	// Lead.
	Opportunities []OpportunityJSON `json:"opportunities"`
}

// FoundConversationJSON is one of a found Recruiter's Conversations, in short.
type FoundConversationJSON struct {
	ID                string `json:"id"`
	HasSuggestedReply bool   `json:"hasSuggestedReply"`
}
