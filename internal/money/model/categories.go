package model

import "context"

// Category is where an Expense or Income belongs, from one fixed list so
// that months always compare (ADR 0014).
type Category struct {
	// Key is how the Tools and the database name it; Name is how the Owner
	// reads it.
	Key  string
	Name string
	// Kind is the kind of Transaction it is for: Expense or Income.
	Kind TransactionKind
}

// Categories are every Category, in the order they are shown.
var Categories = []Category{
	{"rent", "Rent", Expense},
	{"utilities", "Water, Gas & Electricity", Expense},
	{"phone_internet", "Phone & Internet", Expense},
	{"groceries", "Groceries", Expense},
	{"eating_out", "Eating Out", Expense},
	{"convenience_store", "Convenience Store", Expense},
	{"public_transport", "Public Transport", Expense},
	{"travel", "Travel", Expense},
	{"subscriptions", "Subscriptions", Expense},
	{"shopping", "Shopping", Expense},
	{"personal_care", "Personal Care", Expense},
	{"beauty", "Beauty", Expense},
	{"health", "Health", Expense},
	{"entertainment", "Entertainment", Expense},
	{"education", "Education", Expense},
	{"gifts", "Gifts", Expense},
	{"fees_interest", "Fees & Interest", Expense},
	{"cash_paypay", "Cash & PayPay (unitemized)", Expense},
	{"other", "Other", Expense},
	{"salary", "Salary", Income},
	{"interest", "Interest", Income},
	{"other_income", "Other income", Income},
}

// CategoryOf is the Category with key, and whether there is one.
func CategoryOf(key string) (Category, bool) {
	for _, c := range Categories {
		if c.Key == key {
			return c, true
		}
	}
	return Category{}, false
}

// Subcategory is a finer place inside a Category, which grows as needed, and
// is Essential or Optional (ADR 0014).
type Subcategory struct {
	ID        string `bson:"_id"`
	Category  string `bson:"category"`
	Name      string `bson:"name"`
	Essential bool   `bson:"essential"`
}

// SubcategoriesRepository reads and writes the subcategories collection.
type SubcategoriesRepository interface {
	Insert(ctx context.Context, s Subcategory) (string, error)
	// Find returns the Subcategory with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Subcategory, error)
	// Named returns the Category's Subcategory called name, in any case, or
	// Core's utils.ErrNotFound.
	Named(ctx context.Context, category, name string) (Subcategory, error)
	// List returns every Subcategory, by name.
	List(ctx context.Context) ([]Subcategory, error)
	Replace(ctx context.Context, s Subcategory) error
	Delete(ctx context.Context, id string) error
}

// MovesSubcategories is a repository whose documents point at Subcategories,
// and moves them all from one to another.
type MovesSubcategories interface {
	// MoveSubcategory points everything at Subcategory from at into, in
	// intoCategory, instead.
	MoveSubcategory(ctx context.Context, from, into, intoCategory string) error
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
