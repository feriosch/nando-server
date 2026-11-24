package model

import (
	"context"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// HealthSync is one Day's steps and active kcal from Apple Health on the
// Owner's iPhone, sent by the Health Sync Shortcut with a Sync Key (ADR
// 0015). A Day has at most one; a later one replaces it. When the iPhone
// missed a Day's last sync, the Calories Assistant can correct it with the
// Health app's numbers the Owner gives.
type HealthSync struct {
	Day        coremodel.Day `bson:"_id"`
	Steps      int           `bson:"steps"`
	ActiveKcal int           `bson:"activeKcal"`
	SyncedAt   time.Time     `bson:"syncedAt"`
	// Corrected is true when the Calories Assistant set it, not the iPhone.
	Corrected bool `bson:"corrected,omitempty"`
}

// TreadmillWalk is one walk on the treadmill, which the iPhone never counts,
// logged through the Calories Assistant. Its Steps are worked out from its km
// and the Owner's height when it is logged, and kept.
type TreadmillWalk struct {
	ID  string        `bson:"_id,omitempty"`
	Day coremodel.Day `bson:"day"`
	Km  float64       `bson:"km"`
	// Steps is nil if the Owner's height wasn't set when it was logged.
	Steps *int `bson:"steps"`
	Kcal  int  `bson:"kcal"`
	// KcalEstimated is true when Kcal was estimated, not read off the
	// treadmill's screen.
	KcalEstimated bool      `bson:"kcalEstimated"`
	LoggedAt      time.Time `bson:"loggedAt"`
}

// DayActivity is what a Day's Steps and Burned Calories come from.
type DayActivity struct {
	// HealthSync is nil if none came for the Day.
	HealthSync *HealthSync
	Walks      []TreadmillWalk
}

// Steps is the Day's Health Sync steps plus its Treadmill Walks' Steps. It is
// nil if the Day has neither, which isn't 0 steps.
func (a DayActivity) Steps() *int {
	var steps *int
	add := func(n int) {
		if steps == nil {
			steps = new(int)
		}
		*steps += n
	}
	if a.HealthSync != nil {
		add(a.HealthSync.Steps)
	}
	for _, walk := range a.Walks {
		if walk.Steps != nil {
			add(*walk.Steps)
		}
	}
	return steps
}

// BurnedCalories is the Day's Health Sync active kcal plus its Treadmill
// Walks' kcal, always an estimate. It is nil if the Day has neither.
func (a DayActivity) BurnedCalories() *int {
	if a.HealthSync == nil && len(a.Walks) == 0 {
		return nil
	}
	burned := 0
	if a.HealthSync != nil {
		burned += a.HealthSync.ActiveKcal
	}
	for _, walk := range a.Walks {
		burned += walk.Kcal
	}
	return &burned
}

// HealthSyncsRepository reads and writes the health_syncs collection.
type HealthSyncsRepository interface {
	// Save stores sync, replacing any for the same Day.
	Save(ctx context.Context, sync HealthSync) error
	// ListBetween returns the Health Syncs from Day from to Day to, inclusive.
	ListBetween(ctx context.Context, from, to coremodel.Day) ([]HealthSync, error)
	// LastSyncedAt returns when the iPhone last sent a Health Sync, or
	// ErrNotFound if it never has. Corrections don't count.
	LastSyncedAt(ctx context.Context) (time.Time, error)
}

// TreadmillWalksRepository reads and writes the treadmill_walks collection.
type TreadmillWalksRepository interface {
	// Insert stores walk and returns its new ID.
	Insert(ctx context.Context, walk TreadmillWalk) (string, error)
	Find(ctx context.Context, id string) (TreadmillWalk, error)
	Update(ctx context.Context, walk TreadmillWalk) error
	Delete(ctx context.Context, id string) error
	// ListBetween returns the walks from Day from to Day to, inclusive, in
	// the order they were logged.
	ListBetween(ctx context.Context, from, to coremodel.Day) ([]TreadmillWalk, error)
}

// The JSON shapes below are what the API answers with.

// ActivityTotalsJSON is a Day's Steps and Burned Calories, each null for a
// Day with no Health Sync and no Treadmill Walk.
type ActivityTotalsJSON struct {
	Steps          *int `json:"steps"`
	BurnedCalories *int `json:"burnedCalories"`
}

// HealthSyncJSON is a Day's Health Sync as the API shows it.
type HealthSyncJSON struct {
	Steps      int    `json:"steps"`
	ActiveKcal int    `json:"activeKcal"`
	SyncedAt   string `json:"syncedAt"`
	Corrected  bool   `json:"corrected"`
}

// TreadmillWalkJSON is a Treadmill Walk as the API shows it.
type TreadmillWalkJSON struct {
	ID            string        `json:"id"`
	Day           coremodel.Day `json:"day"`
	Km            float64       `json:"km"`
	Steps         *int          `json:"steps"`
	Kcal          int           `json:"kcal"`
	KcalEstimated bool          `json:"kcalEstimated"`
	LoggedAt      string        `json:"loggedAt"`
}

// ActivityJSON is what a Day's Steps and Burned Calories come from, as the
// Day view shows it.
type ActivityJSON struct {
	HealthSync *HealthSyncJSON     `json:"healthSync"`
	Walks      []TreadmillWalkJSON `json:"walks"`
}

// HealthSyncDayJSON is one Day a Health Sync sends.
type HealthSyncDayJSON struct {
	Day        string `json:"day"`
	Steps      *int   `json:"steps"`
	ActiveKcal *int   `json:"activeKcal"`
}

// HealthSyncRequestJSON is what the Health Sync Shortcut sends.
type HealthSyncRequestJSON struct {
	Days []HealthSyncDayJSON `json:"days"`
}
