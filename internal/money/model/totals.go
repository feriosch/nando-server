package model

// CategoryTotalJSON is a Category's total in yen, with its Subcategories'
// and, for spending, the month before's and the average of the 3 before.
type CategoryTotalJSON struct {
	Category      string                 `json:"category"`
	Name          string                 `json:"name"`
	Total         int64                  `json:"total"`
	Previous      *int64                 `json:"previous,omitempty"`
	Average3      *int64                 `json:"average3,omitempty"`
	Subcategories []SubcategoryTotalJSON `json:"bySubcategory,omitempty"`
}

// SubcategoryTotalJSON is a Subcategory's total in yen; an empty
// subcategory is what has none.
type SubcategoryTotalJSON struct {
	Subcategory string `json:"subcategory"`
	Name        string `json:"name"`
	Total       int64  `json:"total"`
}

// SpendingJSON is a month's spending in yen.
type SpendingJSON struct {
	Total      int64               `json:"total"`
	Previous   *int64              `json:"previous"`
	Average3   *int64              `json:"average3"`
	Essential  int64               `json:"essential"`
	Optional   int64               `json:"optional"`
	Unmarked   int64               `json:"unmarked"`
	ByCategory []CategoryTotalJSON `json:"byCategory"`
}

// IncomeJSON is a month's income in yen.
type IncomeJSON struct {
	Total      int64               `json:"total"`
	ByCategory []CategoryTotalJSON `json:"byCategory"`
}

// BalanceJSON is an Account's month-end balance: in its own currency, and
// in yen at the month-end Rate. A card's is what it owes, below 0.
type BalanceJSON struct {
	Account  string  `json:"account"`
	Name     string  `json:"name"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	Yen      int64   `json:"yen"`
}

// NetWorthJSON is the Accounts' month-end balances in yen, and their total
// against the month before's; Missing names the Accounts with no balance
// near the month's end.
type NetWorthJSON struct {
	Total    int64            `json:"total"`
	Previous *int64           `json:"previous"`
	Accounts []BalanceJSON    `json:"accounts"`
	Missing  []AccountRefJSON `json:"missing"`
}

// ReturnJSON is how much a savings or investment Account grew in a month,
// less what was Transferred in, in yen.
type ReturnJSON struct {
	Account string `json:"account"`
	Name    string `json:"name"`
	Yen     int64  `json:"yen"`
}

// UnconfirmedJSON counts a month's Unconfirmed Transactions, and their
// amounts in yen.
type UnconfirmedJSON struct {
	Count int   `json:"count"`
	Yen   int64 `json:"yen"`
}

// MonthTotalsJSON is what money_get_month_totals answers.
type MonthTotalsJSON struct {
	Month    string           `json:"month"`
	Complete bool             `json:"complete"`
	Missing  []AccountRefJSON `json:"missing"`
	// Rate is the month's Rate, when a peso amount was turned into yen.
	Rate        *RateJSON       `json:"rate"`
	Spending    SpendingJSON    `json:"spending"`
	Income      IncomeJSON      `json:"income"`
	NetWorth    NetWorthJSON    `json:"netWorth"`
	Returns     []ReturnJSON    `json:"returns"`
	SavingRate  *float64        `json:"savingRate"`
	Unconfirmed UnconfirmedJSON `json:"unconfirmed"`
}

// MonthSumJSON is one month's spending and income in yen, as
// money_get_totals answers it.
type MonthSumJSON struct {
	Month      string           `json:"month"`
	Spending   int64            `json:"spending"`
	Income     int64            `json:"income"`
	ByCategory map[string]int64 `json:"byCategory"`
}

// CardJSON is the Money Home Card: the latest Complete month, never the one
// under way, since Statements arrive after a month ends (ADR 0014).
type CardJSON struct {
	Month    string `json:"month"`
	Spending struct {
		Total     int64  `json:"total"`
		Average3  *int64 `json:"average3"`
		Essential int64  `json:"essential"`
		Optional  int64  `json:"optional"`
		Unmarked  int64  `json:"unmarked"`
	} `json:"spending"`
	NetWorth struct {
		Total    int64  `json:"total"`
		Previous *int64 `json:"previous"`
	} `json:"netWorth"`
	// Unconfirmed counts the Transactions of every month still Unconfirmed.
	Unconfirmed int `json:"unconfirmed"`
}
