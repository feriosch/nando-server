package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// MerchantsService handles the Merchants Claude learns from Statements and
// the Owner's corrections (ADR 0014).
type MerchantsService struct {
	merchants  model.MerchantsRepository
	categories *CategoriesService
}

// NewMerchants returns the MerchantsService.
func NewMerchants(merchants model.MerchantsRepository, categories *CategoriesService) *MerchantsService {
	return &MerchantsService{merchants: merchants, categories: categories}
}

func toMerchantJSON(m model.Merchant) model.MerchantJSON {
	j := model.MerchantJSON{ID: m.ID, Name: m.Name, RawTexts: m.RawTexts, Category: m.Category, Subcategory: m.SubcategoryID}
	if j.RawTexts == nil {
		j.RawTexts = []string{}
	}
	return j
}

// find is the Merchant with id, or a Refusal.
func (s *MerchantsService) find(ctx context.Context, id string) (model.Merchant, error) {
	m, err := s.merchants.Find(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return m, utils.Refusal{Message: fmt.Sprintf("there's no such Merchant %q", id)}
	}
	return m, err
}

// teach has the Merchant with id usually take category and subcategoryID,
// and know rawText, after the Owner corrected one of its Transactions.
func (s *MerchantsService) teach(ctx context.Context, id, rawText, category, subcategoryID string) error {
	m, err := s.find(ctx, id)
	if err != nil {
		return err
	}
	m.Category, m.SubcategoryID = category, subcategoryID
	if rawText != "" && !slices.Contains(m.RawTexts, rawText) {
		m.RawTexts = append(m.RawTexts, rawText)
	}
	return s.merchants.Replace(ctx, m)
}

// Tools are what the MerchantsService offers Connectors.
func (s *MerchantsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_save_merchant",
		Description: "Saves a Merchant: without id, a new one (name needed); with id, only the fields given change. " +
			"rawTexts given are added to those it is known by. category and subcategory are what its Transactions " +
			"usually are; leave both empty for one that sells everything. Answers it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},` +
			`"rawTexts":{"type":"array","items":{"type":"string"},"description":"Statements' texts it has been seen as, exactly."},` +
			`"category":{"type":"string"},"subcategory":{"type":"string"}}}`),
		Handle: s.saveMerchantTool,
	}, {
		Name: "money_find_merchant",
		Description: "The Merchant a Statement's raw text means: the one seen as it exactly, or else the one named " +
			"it, in any case. An error when there is none: then save one.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"rawText":{"type":"string"}},"required":["rawText"]}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			var args struct {
				RawText string `json:"rawText"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return nil, err
			}
			m, err := s.merchants.WithRawText(ctx, strings.TrimSpace(args.RawText))
			if errors.Is(err, coreutils.ErrNotFound) {
				return nil, utils.Refusal{Message: fmt.Sprintf("there's no Merchant known as %q yet", args.RawText)}
			}
			if err != nil {
				return nil, err
			}
			return toMerchantJSON(m), nil
		},
	}, {
		Name:        "money_list_merchants",
		Description: "Every Merchant, by name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			list, err := s.merchants.List(ctx)
			if err != nil {
				return nil, err
			}
			answer := make([]model.MerchantJSON, len(list))
			for i, m := range list {
				answer[i] = toMerchantJSON(m)
			}
			return answer, nil
		},
	}}
}

func (s *MerchantsService) saveMerchantTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID          *string  `json:"id"`
		Name        *string  `json:"name"`
		RawTexts    []string `json:"rawTexts"`
		Category    *string  `json:"category"`
		Subcategory *string  `json:"subcategory"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send id, name, category and subcategory as strings, and rawTexts as a list"}
	}
	m := model.Merchant{RawTexts: []string{}}
	if args.ID != nil {
		var err error
		if m, err = s.find(ctx, *args.ID); err != nil {
			return nil, err
		}
	} else if args.Name == nil {
		return nil, utils.Refusal{Message: "a new Merchant needs name"}
	}
	if args.Name != nil {
		if m.Name = strings.TrimSpace(*args.Name); m.Name == "" {
			return nil, utils.Refusal{Message: "name can't be empty"}
		}
	}
	for _, raw := range args.RawTexts {
		if raw = strings.TrimSpace(raw); raw != "" && !slices.Contains(m.RawTexts, raw) {
			m.RawTexts = append(m.RawTexts, raw)
		}
	}
	if args.Category != nil {
		// A new Category clears the Subcategory, unless one is given too.
		m.Category, m.SubcategoryID = *args.Category, ""
	}
	if args.Subcategory != nil {
		m.SubcategoryID = *args.Subcategory
	}
	if err := s.categories.checkPlace(ctx, "", m.Category, m.SubcategoryID); err != nil {
		return nil, utils.Refusal{Message: err.Error()}
	}
	if m.ID == "" {
		id, err := s.merchants.Insert(ctx, m)
		if err != nil {
			return nil, err
		}
		m.ID = id
	} else if err := s.merchants.Replace(ctx, m); err != nil {
		return nil, err
	}
	return toMerchantJSON(m), nil
}
