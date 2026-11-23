package model

import "context"

// Merchant is who a Transaction was with, cleaned up from a Statement's raw
// text, with the Category and Subcategory its Transactions usually take; none
// for one that sells everything (ADR 0014).
type Merchant struct {
	ID   string `bson:"_id"`
	Name string `bson:"name"`
	// RawTexts are the Statements' texts it has been seen as.
	RawTexts      []string `bson:"rawTexts"`
	Category      string   `bson:"category"`
	SubcategoryID string   `bson:"subcategoryId"`
}

// MerchantsRepository reads and writes the merchants collection.
type MerchantsRepository interface {
	MovesSubcategories
	Insert(ctx context.Context, m Merchant) (string, error)
	// Find returns the Merchant with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Merchant, error)
	// WithRawText returns the Merchant seen as rawText, exactly, or else the
	// one named rawText, in any case, or Core's utils.ErrNotFound.
	WithRawText(ctx context.Context, rawText string) (Merchant, error)
	// List returns every Merchant, by name.
	List(ctx context.Context) ([]Merchant, error)
	Replace(ctx context.Context, m Merchant) error
}

// MerchantJSON is a Merchant as the Tools answer it.
type MerchantJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	RawTexts    []string `json:"rawTexts"`
	Category    string   `json:"category"`
	Subcategory string   `json:"subcategory"`
}
