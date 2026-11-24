package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

const (
	// healthSyncRoute is the route the Health Sync Shortcut calls with a Sync
	// Key (ADR 0015).
	healthSyncRoute = "POST /api/calories/health-sync"
	// maxSteps is the most steps a Health Sync takes for a Day.
	maxSteps = 200000
	// maxWalkKm is the longest a Treadmill Walk may be.
	maxWalkKm = 100
	// strideFactor is a walking stride as a share of the Owner's height.
	strideFactor = 0.415
	// kcalPerKgKm estimates a walk's kcal from the Owner's weight and its km.
	kcalPerKgKm = 0.5
)

// ActivityService takes the Health Syncs the Owner's iPhone sends and the
// Treadmill Walks the Calories Assistant logs, and answers what a Day's
// Steps and Burned Calories come from.
type ActivityService struct {
	healthSyncs model.HealthSyncsRepository
	walks       model.TreadmillWalksRepository
	// weighIns give the Owner's weight, to estimate a walk's kcal.
	weighIns model.WeighInsRepository
	core     coremodel.CoreService
	now      func() time.Time
}

// NewActivity returns the ActivityService, with now as its clock.
func NewActivity(healthSyncs model.HealthSyncsRepository, walks model.TreadmillWalksRepository,
	weighIns model.WeighInsRepository, core coremodel.CoreService, now func() time.Time) *ActivityService {
	return &ActivityService{healthSyncs: healthSyncs, walks: walks, weighIns: weighIns, core: core, now: now}
}

// Routes adds the Health Sync route to mux, under /api/calories/.
func (s *ActivityService) Routes(mux *http.ServeMux) {
	mux.HandleFunc(healthSyncRoute, s.healthSync)
}

// SyncKeyRoutes are the routes a Sync Key may call: the Health Sync's.
func (s *ActivityService) SyncKeyRoutes() []string {
	return []string{healthSyncRoute}
}

// dayActivity returns what day's Steps and Burned Calories come from.
func (s *ActivityService) dayActivity(ctx context.Context, day coremodel.Day) (model.DayActivity, error) {
	byDay, err := s.activityBetween(ctx, day, day)
	return byDay[day], err
}

// activityBetween returns what the Steps and Burned Calories of each Day from
// from to to come from. A Day with neither a Health Sync nor a walk is left
// out.
func (s *ActivityService) activityBetween(ctx context.Context, from, to coremodel.Day) (map[coremodel.Day]model.DayActivity, error) {
	syncs, err := s.healthSyncs.ListBetween(ctx, from, to)
	if err != nil {
		return nil, err
	}
	walks, err := s.walks.ListBetween(ctx, from, to)
	if err != nil {
		return nil, err
	}
	byDay := map[coremodel.Day]model.DayActivity{}
	for _, sync := range syncs {
		a := byDay[sync.Day]
		a.HealthSync = &sync
		byDay[sync.Day] = a
	}
	for _, walk := range walks {
		a := byDay[walk.Day]
		a.Walks = append(a.Walks, walk)
		byDay[walk.Day] = a
	}
	return byDay, nil
}

// healthSync takes the Days the Health Sync Shortcut sends, each replacing
// that Day's last Health Sync. Only today and yesterday are taken; if any Day
// is wrong, none is.
func (s *ActivityService) healthSync(w http.ResponseWriter, r *http.Request) {
	var req model.HealthSyncRequestJSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `body must be {"days": [{"day", "steps", "activeKcal"}]}`, http.StatusBadRequest)
		return
	}
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	syncs, err := s.checkHealthSync(req, today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, sync := range syncs {
		if err := s.healthSyncs.Save(r.Context(), sync); err != nil {
			coreutils.ServerError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// checkHealthSync returns the Health Syncs req sends, or explains what is
// wrong with it.
func (s *ActivityService) checkHealthSync(req model.HealthSyncRequestJSON, today coremodel.Day) ([]model.HealthSync, error) {
	if len(req.Days) == 0 {
		return nil, errors.New("days must have at least one Day")
	}
	now := s.now()
	var syncs []model.HealthSync
	for _, d := range req.Days {
		day, err := coremodel.ParseDay(d.Day)
		if err != nil {
			return nil, fmt.Errorf("each day must be YYYY-MM-DD, not %q", d.Day)
		}
		if day != today && day != today.AddDays(-1) {
			return nil, fmt.Errorf("only today (%s) and yesterday can be synced, not %s", today, day)
		}
		if d.Steps == nil || *d.Steps < 0 || *d.Steps > maxSteps {
			return nil, fmt.Errorf("steps must be a whole number from 0 to %d", maxSteps)
		}
		if d.ActiveKcal == nil || *d.ActiveKcal < 0 || *d.ActiveKcal > 10000 {
			return nil, errors.New("activeKcal must be a whole number from 0 to 10000")
		}
		syncs = append(syncs, model.HealthSync{Day: day, Steps: *d.Steps, ActiveKcal: *d.ActiveKcal, SyncedAt: now})
	}
	return syncs, nil
}

// toActivityJSON is a, as the Day view shows it.
func toActivityJSON(a model.DayActivity) model.ActivityJSON {
	view := model.ActivityJSON{Walks: make([]model.TreadmillWalkJSON, len(a.Walks))}
	if a.HealthSync != nil {
		view.HealthSync = &model.HealthSyncJSON{
			Steps:      a.HealthSync.Steps,
			ActiveKcal: a.HealthSync.ActiveKcal,
			SyncedAt:   utcString(a.HealthSync.SyncedAt),
		}
	}
	for i, walk := range a.Walks {
		view.Walks[i] = toWalkJSON(walk)
	}
	return view
}

func toWalkJSON(walk model.TreadmillWalk) model.TreadmillWalkJSON {
	return model.TreadmillWalkJSON{
		ID:            walk.ID,
		Day:           walk.Day,
		Km:            walk.Km,
		Steps:         walk.Steps,
		Kcal:          walk.Kcal,
		KcalEstimated: walk.KcalEstimated,
		LoggedAt:      utcString(walk.LoggedAt),
	}
}

// toActivityTotalsJSON is a's Steps and Burned Calories.
func toActivityTotalsJSON(a model.DayActivity) model.ActivityTotalsJSON {
	return model.ActivityTotalsJSON{Steps: a.Steps(), BurnedCalories: a.BurnedCalories()}
}

// utcString writes t in UTC, as the API writes every moment.
func utcString(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// walkSteps is how many steps km takes at heightCm, at a stride of 0.415 of
// the height. It is nil without a height.
func walkSteps(km float64, heightCm int) *int {
	if heightCm <= 0 {
		return nil
	}
	steps := int(math.Round(km * 100000 / (strideFactor * float64(heightCm))))
	return &steps
}

// checkKm rounds km to 2 decimals and checks it is more than 0 and at most
// 100.
func checkKm(km *float64) (float64, error) {
	if km == nil || *km <= 0 || *km > maxWalkKm {
		return 0, utils.Invalid(fmt.Sprintf("km must be more than 0 and at most %d", maxWalkKm))
	}
	return math.Round(*km*100) / 100, nil
}

// walkKcal is kcal if it was sent, as read off the treadmill's screen, or
// else an estimate from km and the latest Weigh-in, which it says it is. With
// neither, it refuses, asking for the screen's kcal.
func (s *ActivityService) walkKcal(ctx context.Context, km float64, kcal *int) (int, bool, error) {
	if kcal != nil {
		checked, err := utils.CheckKcal("kcal", kcal)
		if err != nil {
			return 0, false, utils.Invalid(err.Error())
		}
		return checked, false, nil
	}
	latest, err := s.weighIns.FindLatest(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return 0, false, utils.Invalid("there's no Weigh-in to estimate this walk's kcal from: ask the Owner for the kcal on the treadmill's screen")
	}
	if err != nil {
		return 0, false, err
	}
	return int(math.Round(kcalPerKgKm * latest.Kg * km)), true, nil
}

// checkEditable refuses unless day is in the Editable Window.
func (s *ActivityService) checkEditable(ctx context.Context, day coremodel.Day) error {
	today, err := s.core.Today(ctx)
	if err != nil {
		return err
	}
	if !utils.InEditableWindow(day, today) {
		return utils.Refusal{Message: "only today and the 7 Days before it can be changed"}
	}
	return nil
}

// walkChangedJSON is what a Tool that changed a Treadmill Walk answers: the
// walk, and its Day's Steps and Burned Calories now.
type walkChangedJSON struct {
	Walk model.TreadmillWalkJSON `json:"walk"`
	Day  dayActivityJSON         `json:"day"`
}

// dayActivityJSON is a Day's Steps and Burned Calories.
type dayActivityJSON struct {
	Day coremodel.Day `json:"day"`
	model.ActivityTotalsJSON
}

// dayTotals answers day's Steps and Burned Calories now.
func (s *ActivityService) dayTotals(ctx context.Context, day coremodel.Day) (dayActivityJSON, error) {
	activity, err := s.dayActivity(ctx, day)
	if err != nil {
		return dayActivityJSON{}, err
	}
	return dayActivityJSON{Day: day, ActivityTotalsJSON: toActivityTotalsJSON(activity)}, nil
}

// Tools are what the ActivityService offers Connectors.
func (s *ActivityService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "calories_log_walk",
		Description: "Logs a Treadmill Walk, which the iPhone never counts: its km, and its kcal from the treadmill's screen. " +
			"Its Steps are worked out from the km and the Owner's height, and are null until the height is set in Nando's settings. " +
			"Without kcal, they are estimated from the latest Weigh-in, and the walk says so; with no Weigh-in either, it is refused. " +
			"Answers the walk, and its Day's Steps and Burned Calories.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"day":{"type":"string","description":"The Day walked on, YYYY-MM-DD in the Owner Time Zone; leave out for today."},` +
			`"km":{"type":"number","description":"How far, more than 0 and at most 100."},` +
			`"kcal":{"type":"integer","description":"The kcal on the treadmill's screen; leave out to estimate."}},` +
			`"required":["km"]}`),
		Handle: s.logWalkTool,
	}, {
		Name: "calories_update_walk",
		Description: "Corrects a Treadmill Walk: only the values sent change. A new km gives it new Steps from the Owner's height, " +
			"and a new estimate if its kcal was estimated; kcal sent is the screen's, no longer an estimate. " +
			"Its Day must be in the Editable Window, today and the 7 Days before it. Answers the walk, and its Day's Steps and Burned Calories.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string"},` +
			`"km":{"type":"number","description":"More than 0 and at most 100."},` +
			`"kcal":{"type":"integer","description":"The kcal on the treadmill's screen."}},` +
			`"required":["id"]}`),
		Handle: s.updateWalkTool,
	}, {
		Name: "calories_delete_walk",
		Description: "Deletes a Treadmill Walk logged by mistake. It can't be undone. " +
			"Its Day must be in the Editable Window, today and the 7 Days before it. Answers its Day's Steps and Burned Calories without it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Destructive: true,
		Handle:      s.deleteWalkTool,
	}}
}

// editableWalk returns the walk with id if its Day is in the Editable
// Window, or refuses.
func (s *ActivityService) editableWalk(ctx context.Context, id string) (model.TreadmillWalk, error) {
	walk, err := s.walks.Find(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return walk, utils.Refusal{Message: "there's no such Treadmill Walk"}
	} else if err != nil {
		return walk, err
	}
	return walk, s.checkEditable(ctx, walk.Day)
}

func (s *ActivityService) updateWalkTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID   string   `json:"id"`
		Km   *float64 `json:"km"`
		Kcal *int     `json:"kcal"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	walk, err := s.editableWalk(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if args.Km != nil {
		if walk.Km, err = checkKm(args.Km); err != nil {
			return nil, err
		}
		heightCm, err := s.core.HeightCm(ctx)
		if err != nil {
			return nil, err
		}
		walk.Steps = walkSteps(walk.Km, heightCm)
	}
	if args.Kcal != nil || (args.Km != nil && walk.KcalEstimated) {
		if walk.Kcal, walk.KcalEstimated, err = s.walkKcal(ctx, walk.Km, args.Kcal); err != nil {
			return nil, err
		}
	}
	if err := s.walks.Update(ctx, walk); err != nil {
		return nil, err
	}
	return s.walkChanged(ctx, walk)
}

func (s *ActivityService) deleteWalkTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	walk, err := s.editableWalk(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if err := s.walks.Delete(ctx, walk.ID); err != nil {
		return nil, err
	}
	totals, err := s.dayTotals(ctx, walk.Day)
	if err != nil {
		return nil, err
	}
	return struct {
		Day dayActivityJSON `json:"day"`
	}{totals}, nil
}

func (s *ActivityService) logWalkTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Day  string   `json:"day"`
		Km   *float64 `json:"km"`
		Kcal *int     `json:"kcal"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	day, err := toolDay(ctx, s.core, args.Day)
	if err != nil {
		return nil, err
	}
	if err := s.checkEditable(ctx, day); err != nil {
		return nil, err
	}
	km, err := checkKm(args.Km)
	if err != nil {
		return nil, err
	}
	kcal, estimated, err := s.walkKcal(ctx, km, args.Kcal)
	if err != nil {
		return nil, err
	}
	heightCm, err := s.core.HeightCm(ctx)
	if err != nil {
		return nil, err
	}
	walk := model.TreadmillWalk{Day: day, Km: km, Steps: walkSteps(km, heightCm), Kcal: kcal, KcalEstimated: estimated, LoggedAt: s.now()}
	if walk.ID, err = s.walks.Insert(ctx, walk); err != nil {
		return nil, err
	}
	return s.walkChanged(ctx, walk)
}

// walkChanged answers walk, and its Day's Steps and Burned Calories now.
func (s *ActivityService) walkChanged(ctx context.Context, walk model.TreadmillWalk) (walkChangedJSON, error) {
	totals, err := s.dayTotals(ctx, walk.Day)
	if err != nil {
		return walkChangedJSON{}, err
	}
	return walkChangedJSON{Walk: toWalkJSON(walk), Day: totals}, nil
}
