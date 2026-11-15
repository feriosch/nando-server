package model

import "time"

// HomeCardJSON is the Career Home Card: whether the Owner is Looking, how
// many Recruiters are waiting, and the top Opportunities.
type HomeCardJSON struct {
	Looking bool                 `json:"looking"`
	Waiting int                  `json:"waiting"`
	Top     []TopOpportunityJSON `json:"top"`
}

// CareerScreenJSON is what the Career screen shows: the Recruiters waiting
// for the Owner, the top Opportunities, and how many open Opportunities and
// Late Replies there are.
type CareerScreenJSON struct {
	// Waiting are the Recruiters with a Suggested Reply that isn't late,
	// newest first.
	Waiting []WaitingJSON        `json:"waiting"`
	Top     []TopOpportunityJSON `json:"top"`
	// Active is how many Opportunities are open.
	Active int `json:"active"`
	// LateReplies is how many Recruiters have a Late Reply.
	LateReplies int `json:"lateReplies"`
}

// WaitingJSON is a Recruiter with a Suggested Reply, as the Career screen and
// Late Replies list them.
type WaitingJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Organisation string `json:"organisation"`
	// LastMessageAt is when the Recruiter's last message came.
	LastMessageAt time.Time `json:"lastMessageAt"`
	// FirstLine is the first line of the Suggested Reply.
	FirstLine string `json:"firstLine"`
}
