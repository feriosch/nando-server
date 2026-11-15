package model

// SearchJSON is what career_search answers.
type SearchJSON struct {
	Recruiters    []RecruiterJSON   `json:"recruiters"`
	Companies     []CompanyRefJSON  `json:"companies"`
	Opportunities []OpportunityJSON `json:"opportunities"`
}
