package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// CategoriesService handles the fixed Categories and the Subcategories that
// grow inside them (ADR 0014).
type CategoriesService struct {
	subcategories model.SubcategoriesRepository
	// movers are the repositories whose documents point at Subcategories,
	// which a merge moves.
	movers []model.MovesSubcategories
}

// NewCategories returns the CategoriesService.
func NewCategories(subcategories model.SubcategoriesRepository, movers ...model.MovesSubcategories) *CategoriesService {
	return &CategoriesService{subcategories: subcategories, movers: movers}
}

// SubcategoryJSON is a Subcategory as the Tools answer it.
type SubcategoryJSON struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	Name      string `json:"name"`
	Essential bool   `json:"essential"`
}

// CategoryJSON is a Category with its Subcategories, as the Tools answer it.
type CategoryJSON struct {
	Key           string            `json:"key"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Subcategories []SubcategoryJSON `json:"subcategories"`
}

func toSubcategoryJSON(s model.Subcategory) SubcategoryJSON {
	return SubcategoryJSON{ID: s.ID, Category: s.Category, Name: s.Name, Essential: s.Essential}
}

// essentials is every Subcategory's Essential mark, by its ID.
func (s *CategoriesService) essentials(ctx context.Context) (map[string]bool, error) {
	list, err := s.subcategories.List(ctx)
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, sub := range list {
		m[sub.ID] = sub.Essential
	}
	return m, nil
}

// checkPlace is what is wrong with putting a Transaction or Merchant of kind
// in category and subcategoryID, if anything: the Category must exist and be
// for kind, and the Subcategory, if any, must be in it. An empty category
// takes no subcategory.
func (s *CategoriesService) checkPlace(ctx context.Context, kind model.TransactionKind, category, subcategoryID string) error {
	if category == "" {
		if subcategoryID != "" {
			return errors.New("a subcategory needs its category")
		}
		return nil
	}
	c, ok := model.CategoryOf(category)
	if !ok {
		return fmt.Errorf("category %q isn't one of money_list_categories' keys", category)
	}
	if kind != "" && c.Kind != kind {
		return fmt.Errorf("category %q is for %s, not %s", category, c.Kind, kind)
	}
	if subcategoryID == "" {
		return nil
	}
	sub, err := s.subcategories.Find(ctx, subcategoryID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return fmt.Errorf("there's no such subcategory %q", subcategoryID)
	}
	if err != nil {
		return err
	}
	if sub.Category != category {
		return fmt.Errorf("subcategory %q is in %s, not %s", sub.Name, sub.Category, category)
	}
	return nil
}

// Tools are what the CategoriesService offers Connectors.
func (s *CategoriesService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_list_categories",
		Description: "The fixed Categories, in order, each with its key, name, kind (expense or income) and its " +
			"Subcategories, each Essential or Optional.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle:      s.listCategoriesTool,
	}, {
		Name: "money_save_subcategory",
		Description: "Saves a Subcategory: without id, a new one in category (name and essential needed); with id, " +
			"rename it or change essential. Names are unique in a Category, in any case. Answers it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},` +
			`"category":{"type":"string","description":"A Category's key, for a new one."},"name":{"type":"string"},` +
			`"essential":{"type":"boolean","description":"Essential (a need) or Optional (a want)."}}}`),
		Handle: s.saveSubcategoryTool,
	}, {
		Name: "money_merge_subcategories",
		Description: "Moves every Transaction, Merchant and Subscription of Subcategory from into Subcategory into " +
			"(and into's Category), then removes from. Ask the Owner first.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"from":{"type":"string"},"into":{"type":"string"}},"required":["from","into"]}`),
		Handle:      s.mergeTool,
	}}
}

func (s *CategoriesService) listCategoriesTool(ctx context.Context, _ json.RawMessage) (any, error) {
	subs, err := s.subcategories.List(ctx)
	if err != nil {
		return nil, err
	}
	answer := make([]CategoryJSON, len(model.Categories))
	for i, c := range model.Categories {
		answer[i] = CategoryJSON{Key: c.Key, Name: c.Name, Kind: string(c.Kind), Subcategories: []SubcategoryJSON{}}
		for _, sub := range subs {
			if sub.Category == c.Key {
				answer[i].Subcategories = append(answer[i].Subcategories, toSubcategoryJSON(sub))
			}
		}
	}
	return answer, nil
}

// findSubcategory is the Subcategory with id, or a Refusal.
func (s *CategoriesService) findSubcategory(ctx context.Context, id string) (model.Subcategory, error) {
	sub, err := s.subcategories.Find(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return sub, utils.Refusal{Message: fmt.Sprintf("there's no such Subcategory %q", id)}
	}
	return sub, err
}

func (s *CategoriesService) saveSubcategoryTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID        *string `json:"id"`
		Category  *string `json:"category"`
		Name      *string `json:"name"`
		Essential *bool   `json:"essential"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send id, category and name as strings, and essential as true or false"}
	}
	var sub model.Subcategory
	if args.ID != nil {
		var err error
		if sub, err = s.findSubcategory(ctx, *args.ID); err != nil {
			return nil, err
		}
		if args.Category != nil && *args.Category != sub.Category {
			return nil, utils.Refusal{Message: "a Subcategory can't move to another Category: merge it into one there"}
		}
	} else {
		if args.Category == nil || args.Name == nil || args.Essential == nil {
			return nil, utils.Refusal{Message: "a new Subcategory needs category, name and essential"}
		}
		c, ok := model.CategoryOf(*args.Category)
		if !ok {
			return nil, utils.Refusal{Message: fmt.Sprintf("category %q isn't one of money_list_categories' keys", *args.Category)}
		}
		sub.Category = c.Key
	}
	if args.Name != nil {
		name := strings.TrimSpace(*args.Name)
		if name == "" {
			return nil, utils.Refusal{Message: "name can't be empty"}
		}
		same, err := s.subcategories.Named(ctx, sub.Category, name)
		if err == nil && same.ID != sub.ID {
			return nil, utils.Refusal{Message: fmt.Sprintf("%s already has %q (%s)", sub.Category, same.Name, same.ID)}
		}
		if err != nil && !errors.Is(err, coreutils.ErrNotFound) {
			return nil, err
		}
		sub.Name = name
	}
	if args.Essential != nil {
		sub.Essential = *args.Essential
	}
	if sub.ID == "" {
		id, err := s.subcategories.Insert(ctx, sub)
		if err != nil {
			return nil, err
		}
		sub.ID = id
	} else if err := s.subcategories.Replace(ctx, sub); err != nil {
		return nil, err
	}
	return toSubcategoryJSON(sub), nil
}

func (s *CategoriesService) mergeTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		From string `json:"from"`
		Into string `json:"into"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if args.From == args.Into {
		return nil, utils.Refusal{Message: "from and into are the same Subcategory"}
	}
	from, err := s.findSubcategory(ctx, args.From)
	if err != nil {
		return nil, err
	}
	into, err := s.findSubcategory(ctx, args.Into)
	if err != nil {
		return nil, err
	}
	for _, m := range s.movers {
		if err := m.MoveSubcategory(ctx, from.ID, into.ID, into.Category); err != nil {
			return nil, err
		}
	}
	if err := s.subcategories.Delete(ctx, from.ID); err != nil {
		return nil, err
	}
	return map[string]any{"merged": from.Name, "into": toSubcategoryJSON(into)}, nil
}
