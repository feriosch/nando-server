package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// WeighInsService handles requests and Tools about Weigh-ins (ADR 0012). It
// asks Core for Today, to know which week a Weigh-in is in.
type WeighInsService struct {
	weighIns model.WeighInsRepository
	core     coremodel.CoreService
}

// NewWeighIns returns the WeighInsService.
func NewWeighIns(weighIns model.WeighInsRepository, core coremodel.CoreService) *WeighInsService {
	return &WeighInsService{weighIns: weighIns, core: core}
}

// Routes adds the Weigh-in routes to mux, /api/calories/weigh-ins and the
// Weight Home Card's /api/calories/weight-card. The web app only reads them:
// the Calories Assistant logs Weigh-ins, through the Tools.
func (s *WeighInsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/weigh-ins", s.listWeighIns)
	mux.HandleFunc("GET /api/calories/weight-card", s.getWeightCard)
}

// round1 rounds x to 1 decimal.
func round1(x float64) float64 {
	return math.Round(x*10) / 10
}

// bmi is the body mass index for kg at heightCm, to 1 decimal. It is always
// derived, never stored.
func bmi(kg float64, heightCm int) float64 {
	m := float64(heightCm) / 100
	return round1(kg / (m * m))
}

// weightCard is the Weight Home Card: the latest Weigh-in, its BMI once the
// Owner's height is set in Core, and its change since the week before if
// that week has one.
func (s *WeighInsService) weightCard(ctx context.Context) (model.WeightCardJSON, error) {
	var card model.WeightCardJSON
	latest, err := s.weighIns.FindLatest(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return card, nil
	}
	if err != nil {
		return card, err
	}
	card.Latest = &model.WeighInJSON{Week: latest.Week, Kg: latest.Kg}
	heightCm, err := s.core.HeightCm(ctx)
	if err != nil {
		return card, err
	}
	if heightCm != 0 {
		b := bmi(latest.Kg, heightCm)
		card.BMI = &b
	}
	previous, err := s.weighIns.Find(ctx, latest.Week.AddDays(-7))
	switch {
	case err == nil:
		change := round1(latest.Kg - previous.Kg)
		card.Change = &change
	case !errors.Is(err, coreutils.ErrNotFound):
		return card, err
	}
	return card, nil
}

func (s *WeighInsService) getWeightCard(w http.ResponseWriter, r *http.Request) {
	card, err := s.weightCard(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, card)
}

// listJSON is what the Weigh-in Tools answer: every Weigh-in with the Weight
// Home Card's numbers.
func (s *WeighInsService) listJSON(ctx context.Context) (model.WeighInsJSON, error) {
	list, err := s.list(ctx)
	if err != nil {
		return model.WeighInsJSON{}, err
	}
	card, err := s.weightCard(ctx)
	return model.WeighInsJSON{WeighIns: list, WeightCardJSON: card}, err
}

// list is every Weigh-in as the API shows it, newest first.
func (s *WeighInsService) list(ctx context.Context) ([]model.WeighInJSON, error) {
	weighIns, err := s.weighIns.List(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]model.WeighInJSON, len(weighIns))
	for i, weighIn := range weighIns {
		list[i] = model.WeighInJSON{Week: weighIn.Week, Kg: weighIn.Kg}
	}
	return list, nil
}

func (s *WeighInsService) listWeighIns(w http.ResponseWriter, r *http.Request) {
	list, err := s.list(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}

// Tools are what the WeighInsService offers Connectors: reading, logging and
// deleting Weigh-ins.
func (s *WeighInsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "calories_list_weigh_ins",
		Description: "Every Weigh-in, the Owner's weight for a week (named by the Monday it starts on), newest " +
			"first, with the latest's BMI (null until the Owner's height is set in Nando's settings) and its " +
			"change in kg since the week before (null if that week has none).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			return s.listJSON(ctx)
		},
	}, {
		Name: "calories_log_weigh_in",
		Description: "Logs the Owner's weight in kg (to 1 decimal) as the Weigh-in for the week of day " +
			"(YYYY-MM-DD; today if left out). A week runs Monday to Sunday and has at most one Weigh-in: this " +
			"one replaces any it had. Only this week's and last week's can be logged. Answers every Weigh-in, " +
			"as calories_list_weigh_ins does.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"kg":{"type":"number","exclusiveMinimum":0,"maximum":500,"description":"The weight, to 1 decimal."},` +
			`"day":{"type":"string","description":"Any Day of the week weighed in, YYYY-MM-DD; today if left out."}},` +
			`"required":["kg"]}`),
		Handle: s.logWeighInTool,
	}, {
		Name: "calories_delete_weigh_in",
		Description: "Deletes the Weigh-in for the week of day (YYYY-MM-DD, any Day of that week), for one logged " +
			"by mistake. It can't be undone. Only this week's and last week's can be deleted; deleting a week " +
			"with none also succeeds. Answers the Weigh-ins left, as calories_list_weigh_ins does.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"day":{"type":"string",` +
			`"description":"Any Day of the week, YYYY-MM-DD."}},"required":["day"]}`),
		Destructive: true,
		Handle:      s.deleteWeighInTool,
	}}
}

func (s *WeighInsService) deleteWeighInTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Day string `json:"day"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil || args.Day == "" {
		return nil, utils.Invalid("day must be YYYY-MM-DD")
	}
	week, err := s.editableWeek(ctx, args.Day)
	if err != nil {
		return nil, err
	}
	if err := s.weighIns.Delete(ctx, week); err != nil {
		return nil, err
	}
	return s.listJSON(ctx)
}

func (s *WeighInsService) logWeighInTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Kg  *float64 `json:"kg"`
		Day string   `json:"day"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil || args.Kg == nil {
		return nil, utils.Invalid("kg must be a number")
	}
	week, err := s.editableWeek(ctx, args.Day)
	if err != nil {
		return nil, err
	}
	kg, err := toTenths(*args.Kg)
	if err != nil {
		return nil, utils.Invalid(err.Error())
	}
	weighIn := model.WeighIn{Week: week, Kg: kg}
	if err := s.weighIns.Save(ctx, weighIn); err != nil {
		return nil, err
	}
	return s.listJSON(ctx)
}

// editableWeek returns the week of day, the Monday it starts on (today's if
// day is ""), if its Weigh-in can be changed: only this week's and last
// week's can. Otherwise it refuses.
func (s *WeighInsService) editableWeek(ctx context.Context, day string) (coremodel.Day, error) {
	parsed, err := toolDay(ctx, s.core, day)
	if err != nil {
		return "", err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return "", err
	}
	week, thisWeek := parsed.Week(), today.Week()
	if week != thisWeek && week != thisWeek.AddDays(-7) {
		return "", utils.Invalid("only this week's and last week's Weigh-ins can be changed, Monday " +
			string(thisWeek.AddDays(-7)) + " onwards")
	}
	return week, nil
}

// toTenths checks that kg is a weight to at most 1 decimal and returns it
// rounded to exactly that, as 72.4 may arrive as 72.40000000000001.
func toTenths(kg float64) (float64, error) {
	tenths := math.Round(kg * 10)
	if math.Abs(kg*10-tenths) > 1e-6 || tenths <= 0 || tenths > 5000 {
		return 0, errors.New("kg must be more than 0 and at most 500, to at most 1 decimal")
	}
	return tenths / 10, nil
}
