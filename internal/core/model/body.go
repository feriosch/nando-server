package model

// BodyCardJSON is the Body Home Card: the latest Weigh-in, its BMI (once the
// height is set) and its change since the week before (if that week has one).
// Each is null without it.
type BodyCardJSON struct {
	Latest *WeighInJSON `json:"latest"`
	BMI    *float64     `json:"bmi"`
	Change *float64     `json:"change"`
}
