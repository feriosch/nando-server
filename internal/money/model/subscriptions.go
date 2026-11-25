package model

import "context"

// Subscription is a charge that repeats at one Merchant, which Claude keeps
// from the Transactions and the Owner's receipts (ADR 0014).
type Subscription struct {
	ID         string `bson:"_id"`
	MerchantID string `bson:"merchantId"`
	// Amount is in Currency's minor unit (yen, or centavos).
	Amount   int64  `bson:"amount"`
	Currency string `bson:"currency"`
	// Every is how often it charges: "monthly" or "yearly".
	Every string `bson:"every"`
	// LastCharged is the Day of its last charge.
	LastCharged   string `bson:"lastCharged"`
	Category      string `bson:"category"`
	SubcategoryID string `bson:"subcategoryId"`
	Essential     bool   `bson:"essential"`
	Active        bool   `bson:"active"`
}

// SubscriptionsRepository reads and writes the subscriptions collection.
type SubscriptionsRepository interface {
	MovesSubcategories
	Insert(ctx context.Context, s Subscription) (string, error)
	// Find returns the Subscription with id, or Core's utils.ErrNotFound.
	Find(ctx context.Context, id string) (Subscription, error)
	// List returns every Subscription, active ones first, then by last charge.
	List(ctx context.Context) ([]Subscription, error)
	Replace(ctx context.Context, s Subscription) error
}

// SubscriptionJSON is a Subscription as the Tools answer it.
type SubscriptionJSON struct {
	ID          string  `json:"id"`
	Merchant    string  `json:"merchant"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Every       string  `json:"every"`
	LastCharged string  `json:"lastCharged"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Essential   bool    `json:"essential"`
	Active      bool    `json:"active"`
	// Late is whether it is active and its expected charge is more than a
	// week past.
	Late bool `json:"late"`
}

// ScreenSubscriptionJSON is a Subscription as the Money screen lists it,
// with its Merchant's name and, while active, what it costs a month in yen.
type ScreenSubscriptionJSON struct {
	SubscriptionJSON
	MerchantName string `json:"merchantName"`
	// MonthlyYen is a monthly one's amount, or a yearly one's by 12, in yen
	// at the latest Rate; nil when it isn't active.
	MonthlyYen *int64 `json:"monthlyYen"`
}

// SubscriptionsPageJSON is the Money screen's Subscriptions: every one,
// active first, and what the active ones cost a month in yen.
type SubscriptionsPageJSON struct {
	MonthlyTotal  int64                    `json:"monthlyTotal"`
	Subscriptions []ScreenSubscriptionJSON `json:"subscriptions"`
}
