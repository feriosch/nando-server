package utils

import (
	"cmp"
	"slices"

	"github.com/feriosch/nando-server/internal/career/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// stageRank is how far along each open Stage is, furthest first.
var stageRank = map[string]int{model.StageOffer: 0, model.StageInProcess: 1, model.StageIntroduced: 2}

// interestRank is each level of Interest, highest first.
var interestRank = map[string]int{model.InterestHigh: 0, model.InterestNormal: 1, model.InterestLow: 2}

// Rank sorts open Opportunities top first: by Interest, then by how far
// along they are (an Offer, then In Process by when their next Step is, one
// with none left last, then Introduced), then by the newest activity. today
// is the current Day in the Owner Time Zone. The fit note is never used.
func Rank(opportunities []model.Opportunity, today coremodel.Day) {
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

// compareNextSteps orders next Steps by when they are, none last.
func compareNextSteps(a, b *model.Step) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmp.Compare(when(*a), when(*b))
}

// when is the Step's date and time, as they sort: one with no time first on
// its date.
func when(step model.Step) string {
	return step.Date + " " + step.Time
}

// NextStep is o's next Step: the first one, by date and time, dated today or
// later. It is nil when none is left.
func NextStep(o model.Opportunity, today coremodel.Day) *model.Step {
	var next *model.Step
	for i, step := range o.Steps {
		if step.Date < string(today) {
			continue
		}
		if next == nil || when(step) < when(*next) {
			next = &o.Steps[i]
		}
	}
	return next
}
