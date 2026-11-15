package utils

import (
	"cmp"
	"slices"

	"github.com/feriosch/nando-server/internal/career/model"
)

// stageRank is how far along each open Stage is, furthest first.
var stageRank = map[string]int{model.StageOffer: 0, model.StageInProcess: 1, model.StageIntroduced: 2}

// interestRank is each level of Interest, highest first.
var interestRank = map[string]int{model.InterestHigh: 0, model.InterestNormal: 1, model.InterestLow: 2}

// Rank sorts open Opportunities top first: by Interest, then by how far
// along they are (an Offer, then In Process by the date of their next Step,
// one with none left last, then Introduced), then by the newest activity.
// today is the current Day in the Owner Time Zone, as YYYY-MM-DD. The fit
// note is never used.
func Rank(opportunities []model.Opportunity, today string) {
	slices.SortStableFunc(opportunities, func(a, b model.Opportunity) int {
		if c := cmp.Compare(interestRank[a.Interest], interestRank[b.Interest]); c != 0 {
			return c
		}
		if c := cmp.Compare(stageRank[a.Stage], stageRank[b.Stage]); c != 0 {
			return c
		}
		if a.Stage == model.StageInProcess {
			if c := compareNextSteps(NextStep(a, today), NextStep(b, today)); c != 0 {
				return c
			}
		}
		return b.UpdatedAt.Compare(a.UpdatedAt)
	})
}

// compareNextSteps orders next Steps by date, none last.
func compareNextSteps(a, b *model.Step) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmp.Compare(a.Date, b.Date)
}

// NextStep is o's next Step: the first one, by date and time, dated today or
// later. today is YYYY-MM-DD. It is nil when none is left.
func NextStep(o model.Opportunity, today string) *model.Step {
	var next *model.Step
	for i, step := range o.Steps {
		if step.Date < today {
			continue
		}
		if next == nil || step.Date+" "+step.Time < next.Date+" "+next.Time {
			next = &o.Steps[i]
		}
	}
	return next
}
