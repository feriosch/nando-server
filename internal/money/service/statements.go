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

// StatementsService handles the Statements the Owner sends and their
// Transactions. Only the Money Assistant reads and changes them, through its
// Tools. Any period can be saved, replaced or deleted: Money has no Editable
// Window (ADR 0014).
type StatementsService struct {
	statements model.StatementsRepository
	accounts   *AccountsService
	categories *CategoriesService
	merchants  *MerchantsService
}

// NewStatements returns the StatementsService.
func NewStatements(statements model.StatementsRepository, accounts *AccountsService, categories *CategoriesService, merchants *MerchantsService) *StatementsService {
	return &StatementsService{statements: statements, accounts: accounts, categories: categories, merchants: merchants}
}

// lineJSON is one Transaction as money_save_statement is sent it.
type lineJSON struct {
	Kind         string  `json:"kind"`
	Day          string  `json:"day"`
	Amount       float64 `json:"amount"`
	Direction    string  `json:"direction"`
	RawText      string  `json:"rawText"`
	Note         string  `json:"note"`
	OtherAccount string  `json:"otherAccount"`
	Category     string  `json:"category"`
	Subcategory  string  `json:"subcategory"`
	Merchant     string  `json:"merchant"`
	Essential    *bool   `json:"essential"`
	Unconfirmed  bool    `json:"unconfirmed"`
}

// statementArgs is what money_save_statement is sent.
type statementArgs struct {
	Account        string     `json:"account"`
	From           string     `json:"from"`
	To             string     `json:"to"`
	Source         string     `json:"source"`
	FileIDs        []string   `json:"fileIds"`
	Total          *float64   `json:"total"`
	ClosingBalance *float64   `json:"closingBalance"`
	Transactions   []lineJSON `json:"transactions"`
}

// TransactionJSON is a Transaction as the Tools answer it.
type TransactionJSON struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Day          string  `json:"day"`
	Amount       float64 `json:"amount"`
	Direction    string  `json:"direction,omitempty"`
	RawText      string  `json:"rawText"`
	Note         string  `json:"note"`
	OtherAccount string  `json:"otherAccount,omitempty"`
	Category     string  `json:"category,omitempty"`
	Subcategory  string  `json:"subcategory,omitempty"`
	Merchant     string  `json:"merchant,omitempty"`
	// EssentialOverride is set when this Transaction is Essential or
	// Optional apart from its Subcategory, and Essential is what it counts
	// as: its override, else its Subcategory's mark, else null, unmarked.
	EssentialOverride *bool  `json:"essentialOverride,omitempty"`
	Essential         *bool  `json:"essential"`
	Unconfirmed       bool   `json:"unconfirmed,omitempty"`
	Statement         string `json:"statement,omitempty"`
	Account           string `json:"account,omitempty"`
}

// StatementJSON is a Statement as the Tools answer it.
type StatementJSON struct {
	ID             string            `json:"id"`
	Account        string            `json:"account"`
	From           string            `json:"from"`
	To             string            `json:"to"`
	Source         string            `json:"source"`
	FileIDs        []string          `json:"fileIds"`
	Total          *float64          `json:"total"`
	ClosingBalance *float64          `json:"closingBalance"`
	Transactions   []TransactionJSON `json:"transactions,omitempty"`
	// Late counts the Transactions saved that are dated before the period,
	// and Skipped the late ones left out because another Statement of the
	// Account already has them.
	Late    int `json:"late,omitempty"`
	Skipped int `json:"skipped,omitempty"`
}

// toTransactionJSON is t, in an Account in currency, with essentials, every
// Subcategory's Essential mark by its ID.
func toTransactionJSON(currency string, t model.Transaction, essentials map[string]bool) TransactionJSON {
	j := TransactionJSON{
		ID: t.ID, Kind: string(t.Kind), Day: t.Day, Amount: utils.FromMinor(currency, t.Amount), Direction: t.Direction,
		RawText: t.RawText, Note: t.Note, OtherAccount: t.OtherAccountID, Category: t.Category, Subcategory: t.SubcategoryID,
		Merchant: t.MerchantID, EssentialOverride: t.Essential, Unconfirmed: t.Unconfirmed,
	}
	if t.Essential != nil {
		j.Essential = t.Essential
	} else if mark, ok := essentials[t.SubcategoryID]; ok {
		j.Essential = &mark
	}
	return j
}

// fromMinorPtr is amount, in currency's minor unit, as the Tools answer it,
// or nil.
func fromMinorPtr(currency string, amount *int64) *float64 {
	if amount == nil {
		return nil
	}
	v := utils.FromMinor(currency, *amount)
	return &v
}

func toStatementJSON(currency string, s model.Statement, essentials map[string]bool) StatementJSON {
	j := StatementJSON{
		ID: s.ID, Account: s.AccountID, From: s.From, To: s.To, Source: string(s.Source), FileIDs: s.FileIDs,
		Total: fromMinorPtr(currency, s.Total), ClosingBalance: fromMinorPtr(currency, s.ClosingBalance),
	}
	if j.FileIDs == nil {
		j.FileIDs = []string{}
	}
	for _, t := range counted(s.Transactions) {
		j.Transactions = append(j.Transactions, toTransactionJSON(currency, t, essentials))
	}
	return j
}

// statementSchema is what money_save_statement takes.
const statementSchema = `{"type":"object","properties":{` +
	`"account":{"type":"string","description":"The Account's id."},` +
	`"from":{"type":"string","description":"The first Day of the period, YYYY-MM-DD."},` +
	`"to":{"type":"string","description":"The last Day of the period, YYYY-MM-DD."},` +
	`"source":{"type":"string","enum":["csv","pdf","screenshots"]},` +
	`"fileIds":{"type":"array","items":{"type":"string"},"description":"The ids of the files it was read from, from messenger_claim_talk."},` +
	`"total":{"type":"number","description":"The total it states, such as a card's billed amount."},` +
	`"closingBalance":{"type":"number","description":"The balance it states at its last Day."},` +
	`"transactions":{"type":"array","items":{"type":"object","properties":{` +
	`"kind":{"type":"string","enum":["expense","income","transfer"]},` +
	`"day":{"type":"string","description":"The purchase date, YYYY-MM-DD: on or before the period's last Day."},` +
	`"amount":{"type":"number","description":"In the Account's currency: whole yen, or pesos to centavos. Positive; negative only for a refund, an expense coming back."},` +
	`"direction":{"type":"string","enum":["in","out"],"description":"For a transfer only: whether the money came into this Account or went out of it."},` +
	`"rawText":{"type":"string","description":"The Statement's text for it, exactly as written."},` +
	`"note":{"type":"string","description":"What was bought, if known."},` +
	`"otherAccount":{"type":"string","description":"For a transfer, the Owner's other Account's id, when known."},` +
	`"category":{"type":"string","description":"For an expense or income, its Category's key, of its kind."},` +
	`"subcategory":{"type":"string","description":"Its Subcategory's id, in that Category."},` +
	`"merchant":{"type":"string","description":"Its Merchant's id."},` +
	`"essential":{"type":"boolean","description":"Only when this one is Essential or Optional apart from its Subcategory."},` +
	`"unconfirmed":{"type":"boolean","description":"True while what it is is your best guess, awaiting the Owner."}},` +
	`"required":["kind","day","amount","rawText"]}}},` +
	`"required":["account","from","to","source"]}`

// Tools are what the StatementsService offers Connectors.
func (s *StatementsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_save_statement",
		Description: "Saves a Statement of one Account and period, with all its Transactions, at once or not at all. " +
			"The same Account and period sent again replaces the earlier Statement and all its Transactions; a period " +
			"that overlaps another of the Account's is refused, naming it, so ask the Owner which to keep. A Transaction " +
			"dated before the period (one that posted late) joins its own month, and is left out if another Statement " +
			"of the Account already has it (same day, amount and raw text). Answers the Statement, with how many were late and skipped.",
		InputSchema: json.RawMessage(statementSchema),
		Handle:      s.saveStatementTool,
	}, {
		Name:        "money_get_statement",
		Description: "The Statement with id, with its Transactions.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			var args struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return nil, err
			}
			statement, account, err := s.find(ctx, args.ID)
			if err != nil {
				return nil, err
			}
			essentials, err := s.categories.essentials(ctx)
			if err != nil {
				return nil, err
			}
			return toStatementJSON(account.Currency, statement, essentials), nil
		},
	}, {
		Name: "money_list_statements",
		Description: "An Account's Statements, by period, without their Transactions: those that reach into the months " +
			"from fromMonth to toMonth (YYYY-MM), or all.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"account":{"type":"string"},` +
			`"fromMonth":{"type":"string"},"toMonth":{"type":"string"}},"required":["account"]}`),
		ReadOnly: true,
		Handle:   s.listStatementsTool,
	}, {
		Name: "money_delete_statement",
		Description: "Deletes the Statement with id and all its Transactions. Ask the Owner first, and act on their yes " +
			"in a later message.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			var args struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return nil, err
			}
			statement, _, err := s.find(ctx, args.ID)
			if err != nil {
				return nil, err
			}
			if err := s.statements.Delete(ctx, args.ID); err != nil {
				return nil, err
			}
			if err := s.countDuplicatesOf(ctx, statement.AccountID, statement.ID, counted(statement.Transactions)); err != nil {
				return nil, err
			}
			return map[string]bool{"deleted": true}, nil
		},
	}, {
		Name: "money_list_transactions",
		Description: "Transactions, by Day, with their Statement and Account: of one account, in the months from " +
			"fromMonth to toMonth (YYYY-MM), of one kind, category, subcategory or merchant, unconfirmed or not, or " +
			"whose raw text or note contains text, in any case. At most limit (200 if not given, at most 1000).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"account":{"type":"string"},` +
			`"fromMonth":{"type":"string"},"toMonth":{"type":"string"},` +
			`"kind":{"type":"string","enum":["expense","income","transfer"]},"text":{"type":"string"},` +
			`"category":{"type":"string"},"subcategory":{"type":"string"},"merchant":{"type":"string"},` +
			`"unconfirmed":{"type":"boolean"},"limit":{"type":"integer"}}}`),
		ReadOnly: true,
		Handle:   s.listTransactionsTool,
	}, {
		Name: "money_update_transaction",
		Description: "Changes the fields given of the Transaction with id: its note; for a transfer, the " +
			"otherAccount (empty for none); for an expense or income, its category (which clears its subcategory " +
			"unless one is given too), subcategory, merchant, essential (null to follow its Subcategory again) and " +
			"unconfirmed. With teachMerchant, its Merchant also learns its raw text and to usually take its category " +
			"and subcategory: do it for a correction that should stick, not a one-off. Answers it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"note":{"type":"string"},` +
			`"otherAccount":{"type":"string"},"category":{"type":"string"},"subcategory":{"type":"string"},` +
			`"merchant":{"type":"string"},"essential":{"type":["boolean","null"]},"unconfirmed":{"type":"boolean"},` +
			`"teachMerchant":{"type":"boolean"}},"required":["id"]}`),
		Handle: s.updateTransactionTool,
	}, {
		Name: "money_get_month",
		Description: "Whether month (YYYY-MM) is Complete: every Account tracked in it that holds the month has " +
			"Statements reaching its last Day (a savings or investment Account, a balance on or after it). Answers " +
			"missing, the Accounts it still waits for, and late, those that don't hold the month and haven't reached it yet.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"month":{"type":"string"}},"required":["month"]}`),
		ReadOnly:    true,
		Handle:      s.getMonthTool,
	}}
}

// find is the Statement with id and its Account, or a Refusal.
func (s *StatementsService) find(ctx context.Context, id string) (model.Statement, model.Account, error) {
	statement, err := s.statements.Find(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return statement, model.Account{}, utils.Refusal{Message: fmt.Sprintf("there's no such Statement %q", id)}
	}
	if err != nil {
		return statement, model.Account{}, err
	}
	account, err := s.accounts.find(ctx, statement.AccountID)
	return statement, account, err
}

// saveStatementTool saves a Statement and its Transactions, replacing the
// Account's Statement for the same period.
func (s *StatementsService) saveStatementTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args statementArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send account, from, to and source as strings, amounts as numbers, and transactions as a list"}
	}
	account, err := s.accounts.find(ctx, args.Account)
	if err != nil {
		return nil, err
	}
	statement, err := s.checkStatement(ctx, account, args)
	if err != nil {
		return nil, err
	}
	// The Account's other Statements this period overlaps: only the one for
	// the same period, which this replaces, may.
	overlapping, err := s.statements.ForAccount(ctx, account.ID, statement.From, statement.To)
	if err != nil {
		return nil, err
	}
	replaced := ""
	for _, other := range overlapping {
		if other.From == statement.From && other.To == statement.To {
			replaced = other.ID
			continue
		}
		return nil, utils.Refusal{Message: fmt.Sprintf("the period overlaps the Account's Statement %s (%s to %s): "+
			"ask the Owner which to keep, and delete the other first", other.ID, other.From, other.To)}
	}
	answer := StatementJSON{}
	for i, t := range statement.Transactions {
		if t.Day >= statement.From {
			continue
		}
		has, err := s.statements.HasTransaction(ctx, account.ID, replaced, t.Day, t.Amount, t.RawText)
		if err != nil {
			return nil, err
		}
		if has {
			statement.Transactions[i].Duplicate = true
			answer.Skipped++
		} else {
			answer.Late++
		}
	}
	var old model.Statement
	if replaced != "" {
		if old, err = s.statements.Find(ctx, replaced); err != nil {
			return nil, err
		}
	}
	saved, err := s.statements.Save(ctx, statement)
	if err != nil {
		return nil, err
	}
	if err := s.countDuplicatesOf(ctx, account.ID, saved.ID, gone(old.Transactions, saved.Transactions)); err != nil {
		return nil, err
	}
	essentials, err := s.categories.essentials(ctx)
	if err != nil {
		return nil, err
	}
	late, skipped := answer.Late, answer.Skipped
	answer = toStatementJSON(account.Currency, saved, essentials)
	answer.Late, answer.Skipped = late, skipped
	return answer, nil
}

// counted is the Transactions of lines that count: not Duplicates.
func counted(lines []model.Transaction) []model.Transaction {
	var list []model.Transaction
	for _, t := range lines {
		if !t.Duplicate {
			list = append(list, t)
		}
	}
	return list
}

// gone is the counted Transactions of before that after no longer counts,
// copy for copy.
func gone(before, after []model.Transaction) []model.Transaction {
	left := map[string]int{}
	for _, t := range counted(after) {
		left[t.Key()]++
	}
	var list []model.Transaction
	for _, t := range counted(before) {
		if left[t.Key()] > 0 {
			left[t.Key()]--
			continue
		}
		list = append(list, t)
	}
	return list
}

// countDuplicatesOf makes one Duplicate copy, on another Statement of the
// Account than statementID, count for each of the lines that no longer
// count, so that no Transaction is lost when the copy that counted it goes.
func (s *StatementsService) countDuplicatesOf(ctx context.Context, accountID, statementID string, lines []model.Transaction) error {
	if len(lines) == 0 {
		return nil
	}
	duplicates, err := s.statements.Duplicates(ctx, accountID, statementID)
	if err != nil || len(duplicates) == 0 {
		return err
	}
	byKey := map[string][]string{}
	for _, d := range duplicates {
		byKey[d.Key()] = append(byKey[d.Key()], d.ID)
	}
	for _, t := range lines {
		ids := byKey[t.Key()]
		if len(ids) == 0 {
			continue
		}
		byKey[t.Key()] = ids[1:]
		if err := s.statements.Count(ctx, ids[0]); err != nil {
			return err
		}
	}
	return nil
}

// checkStatement is the Statement args describe for account, or a Refusal
// saying what is wrong with it.
func (s *StatementsService) checkStatement(ctx context.Context, account model.Account, args statementArgs) (model.Statement, error) {
	if !utils.IsDay(args.From) || !utils.IsDay(args.To) {
		return model.Statement{}, utils.Refusal{Message: "from and to must be Days, YYYY-MM-DD"}
	}
	if args.From > args.To {
		return model.Statement{}, utils.Refusal{Message: "from must be on or before to"}
	}
	if !slices.Contains(model.Sources, model.Source(args.Source)) {
		return model.Statement{}, utils.Refusal{Message: fmt.Sprintf("source %q isn't csv, pdf or screenshots", args.Source)}
	}
	statement := model.Statement{
		AccountID: account.ID, From: args.From, To: args.To, Source: model.Source(args.Source),
		FileIDs: args.FileIDs, Transactions: []model.Transaction{},
	}
	if statement.FileIDs == nil {
		statement.FileIDs = []string{}
	}
	for name, amount := range map[string]*float64{"total": args.Total, "closingBalance": args.ClosingBalance} {
		if amount == nil {
			continue
		}
		minor, err := utils.ToMinor(account.Currency, *amount)
		if err != nil {
			return statement, utils.Refusal{Message: name + ": " + err.Error()}
		}
		if name == "total" {
			statement.Total = &minor
		} else {
			statement.ClosingBalance = &minor
		}
	}
	for i, line := range args.Transactions {
		t, err := s.checkLine(ctx, account, statement, line)
		if err != nil {
			return statement, utils.Refusal{Message: fmt.Sprintf("transactions[%d]: %s", i, err)}
		}
		statement.Transactions = append(statement.Transactions, t)
	}
	return statement, nil
}

// checkLine is the Transaction line describes, on a Statement of account, or
// what is wrong with it.
func (s *StatementsService) checkLine(ctx context.Context, account model.Account, statement model.Statement, line lineJSON) (model.Transaction, error) {
	t := model.Transaction{
		Kind: model.TransactionKind(line.Kind), Day: line.Day, RawText: strings.TrimSpace(line.RawText),
		Note: strings.TrimSpace(line.Note),
	}
	if !slices.Contains(model.TransactionKinds, t.Kind) {
		return t, fmt.Errorf("kind %q isn't expense, income or transfer", line.Kind)
	}
	if !utils.IsDay(line.Day) {
		return t, fmt.Errorf("day %q isn't a Day, YYYY-MM-DD", line.Day)
	}
	if line.Day > statement.To {
		return t, fmt.Errorf("day %s is after the period, which ends %s", line.Day, statement.To)
	}
	if t.RawText == "" {
		return t, errors.New("rawText is empty: give the Statement's text for it")
	}
	var err error
	if t.Amount, err = utils.ToMinor(account.Currency, line.Amount); err != nil {
		return t, err
	}
	if t.Amount == 0 || (t.Amount < 0 && t.Kind != model.Expense) {
		return t, errors.New("amount must be above 0, or below 0 only for a refund, an expense coming back")
	}
	if t.Kind != model.Transfer {
		if line.Direction != "" || line.OtherAccount != "" {
			return t, errors.New("direction and otherAccount are for a transfer only")
		}
		if err := s.categories.checkPlace(ctx, t.Kind, line.Category, line.Subcategory); err != nil {
			return t, err
		}
		if line.Merchant != "" {
			if _, err := s.merchants.find(ctx, line.Merchant); err != nil {
				return t, err
			}
		}
		t.Category, t.SubcategoryID, t.MerchantID = line.Category, line.Subcategory, line.Merchant
		t.Essential, t.Unconfirmed = line.Essential, line.Unconfirmed
		return t, nil
	}
	if line.Category != "" || line.Subcategory != "" || line.Merchant != "" || line.Essential != nil {
		return t, errors.New("a transfer has no category, subcategory, merchant or essential: it is neither spending nor income")
	}
	t.Unconfirmed = line.Unconfirmed
	if line.Direction != "in" && line.Direction != "out" {
		return t, errors.New("a transfer needs direction, in or out of this Account")
	}
	t.Direction = line.Direction
	if line.OtherAccount != "" {
		if err := s.checkOtherAccount(ctx, account.ID, line.OtherAccount); err != nil {
			return t, err
		}
		t.OtherAccountID = line.OtherAccount
	}
	return t, nil
}

// checkOtherAccount is what is wrong with other as the other Account of a
// Transfer from accountID, if anything.
func (s *StatementsService) checkOtherAccount(ctx context.Context, accountID, other string) error {
	if other == accountID {
		return errors.New("otherAccount is this Account: a transfer moves money to or from another")
	}
	_, err := s.accounts.find(ctx, other)
	return err
}

// listStatementsTool lists an Account's Statements in the months asked.
func (s *StatementsService) listStatementsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Account   string `json:"account"`
		FromMonth string `json:"fromMonth"`
		ToMonth   string `json:"toMonth"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	account, err := s.accounts.find(ctx, args.Account)
	if err != nil {
		return nil, err
	}
	fromDay, toDay, err := monthDays(args.FromMonth, args.ToMonth)
	if err != nil {
		return nil, err
	}
	list, err := s.statements.ForAccount(ctx, account.ID, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	answer := make([]StatementJSON, len(list))
	for i, statement := range list {
		answer[i] = toStatementJSON(account.Currency, statement, nil)
	}
	return answer, nil
}

// monthDays is the first Day of fromMonth and the last of toMonth, each
// empty when its month is, or a Refusal.
func monthDays(fromMonth, toMonth string) (fromDay, toDay string, err error) {
	for name, month := range map[string]string{"fromMonth": fromMonth, "toMonth": toMonth} {
		if month != "" && !utils.IsMonth(month) {
			return "", "", utils.Refusal{Message: name + " must be a month, YYYY-MM"}
		}
	}
	if fromMonth != "" {
		fromDay = fromMonth + "-01"
	}
	if toMonth != "" {
		toDay = utils.LastDay(toMonth)
	}
	return fromDay, toDay, nil
}

// defaultTransactionsLimit and maxTransactionsLimit bound one
// money_list_transactions answer.
const (
	defaultTransactionsLimit = 200
	maxTransactionsLimit     = 1000
)

// listTransactionsTool lists the Transactions asked.
func (s *StatementsService) listTransactionsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Account     string `json:"account"`
		FromMonth   string `json:"fromMonth"`
		ToMonth     string `json:"toMonth"`
		Kind        string `json:"kind"`
		Text        string `json:"text"`
		Category    string `json:"category"`
		Subcategory string `json:"subcategory"`
		Merchant    string `json:"merchant"`
		Unconfirmed *bool  `json:"unconfirmed"`
		Limit       int    `json:"limit"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if _, _, err := monthDays(args.FromMonth, args.ToMonth); err != nil {
		return nil, err
	}
	if args.Kind != "" && !slices.Contains(model.TransactionKinds, model.TransactionKind(args.Kind)) {
		return nil, utils.Refusal{Message: fmt.Sprintf("kind %q isn't expense, income or transfer", args.Kind)}
	}
	if args.Limit <= 0 {
		args.Limit = defaultTransactionsLimit
	}
	found, err := s.statements.Transactions(ctx, model.TransactionFilter{
		AccountID: args.Account, FromMonth: args.FromMonth, ToMonth: args.ToMonth, Kind: model.TransactionKind(args.Kind),
		Text: strings.TrimSpace(args.Text), Category: args.Category, SubcategoryID: args.Subcategory, MerchantID: args.Merchant,
		Unconfirmed: args.Unconfirmed, Limit: min(args.Limit, maxTransactionsLimit),
	})
	if err != nil {
		return nil, err
	}
	currencies, err := s.currencies(ctx)
	if err != nil {
		return nil, err
	}
	essentials, err := s.categories.essentials(ctx)
	if err != nil {
		return nil, err
	}
	answer := make([]TransactionJSON, len(found))
	for i, f := range found {
		answer[i] = toTransactionJSON(currencies[f.AccountID], f.Transaction, essentials)
		answer[i].Statement, answer[i].Account = f.StatementID, f.AccountID
	}
	return answer, nil
}

// currencies is every Account's currency, by its ID.
func (s *StatementsService) currencies(ctx context.Context) (map[string]string, error) {
	accounts, err := s.accounts.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, a := range accounts {
		m[a.ID] = a.Currency
	}
	return m, nil
}

// updateTransactionTool changes the fields given of one Transaction.
func (s *StatementsService) updateTransactionTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID           string          `json:"id"`
		Note         *string         `json:"note"`
		OtherAccount *string         `json:"otherAccount"`
		Category     *string         `json:"category"`
		Subcategory  *string         `json:"subcategory"`
		Merchant     *string         `json:"merchant"`
		Essential    json.RawMessage `json:"essential"`
		Unconfirmed  *bool           `json:"unconfirmed"`
		// TeachMerchant also has its Merchant usually take its Category and
		// Subcategory, and know its raw text.
		TeachMerchant bool `json:"teachMerchant"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send essential as true, false or null, unconfirmed and teachMerchant as true or false, and the rest as strings"}
	}
	found, err := s.statements.FindTransaction(ctx, args.ID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, utils.Refusal{Message: fmt.Sprintf("there's no such Transaction %q", args.ID)}
	}
	if err != nil {
		return nil, err
	}
	t := found.Transaction
	if args.Note != nil {
		t.Note = strings.TrimSpace(*args.Note)
	}
	if args.OtherAccount != nil {
		if t.Kind != model.Transfer {
			return nil, utils.Refusal{Message: "otherAccount is for a transfer only"}
		}
		if *args.OtherAccount != "" {
			if err := s.checkOtherAccount(ctx, found.AccountID, *args.OtherAccount); err != nil {
				return nil, utils.Refusal{Message: err.Error()}
			}
		}
		t.OtherAccountID = *args.OtherAccount
	}
	if err := s.changeWhatItIs(ctx, &t, args.Category, args.Subcategory, args.Merchant, args.Essential); err != nil {
		return nil, utils.Refusal{Message: err.Error()}
	}
	if args.Unconfirmed != nil {
		t.Unconfirmed = *args.Unconfirmed
	}
	if args.TeachMerchant {
		if t.MerchantID == "" {
			return nil, utils.Refusal{Message: "teachMerchant needs the Transaction's merchant"}
		}
		if err := s.merchants.teach(ctx, t.MerchantID, t.RawText, t.Category, t.SubcategoryID); err != nil {
			return nil, err
		}
	}
	if err := s.statements.UpdateTransaction(ctx, t); err != nil {
		return nil, err
	}
	account, err := s.accounts.find(ctx, found.AccountID)
	if err != nil {
		return nil, err
	}
	essentials, err := s.categories.essentials(ctx)
	if err != nil {
		return nil, err
	}
	answer := toTransactionJSON(account.Currency, t, essentials)
	answer.Statement, answer.Account = found.StatementID, found.AccountID
	return answer, nil
}

// changeWhatItIs sets on t the Category, Subcategory, Merchant and Essential
// override given (a JSON null essential clears the override), or says what
// is wrong with them. A Category given without a Subcategory clears it.
func (s *StatementsService) changeWhatItIs(ctx context.Context, t *model.Transaction, category, subcategory, merchant *string, essential json.RawMessage) error {
	if category == nil && subcategory == nil && merchant == nil && essential == nil {
		return nil
	}
	if t.Kind == model.Transfer {
		return errors.New("a transfer has no category, subcategory, merchant or essential: it is neither spending nor income")
	}
	if category != nil {
		t.Category, t.SubcategoryID = *category, ""
	}
	if subcategory != nil {
		t.SubcategoryID = *subcategory
	}
	if err := s.categories.checkPlace(ctx, t.Kind, t.Category, t.SubcategoryID); err != nil {
		return err
	}
	if merchant != nil {
		if *merchant != "" {
			if _, err := s.merchants.find(ctx, *merchant); err != nil {
				return err
			}
		}
		t.MerchantID = *merchant
	}
	if essential != nil {
		if string(essential) == "null" {
			t.Essential = nil
		} else {
			var mark bool
			if err := json.Unmarshal(essential, &mark); err != nil {
				return errors.New("essential must be true, false or null")
			}
			t.Essential = &mark
		}
	}
	return nil
}

// MonthJSON is whether a month is Complete, as money_get_month answers it.
type MonthJSON struct {
	Month    string `json:"month"`
	Complete bool   `json:"complete"`
	// Missing are the Accounts the month waits for, and Late those that
	// don't hold the month and haven't reached its end yet: each id and name.
	Missing []AccountRefJSON `json:"missing"`
	Late    []AccountRefJSON `json:"late"`
}

// AccountRefJSON names an Account.
type AccountRefJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getMonthTool answers whether a month is Complete.
func (s *StatementsService) getMonthTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Month string `json:"month"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	if !utils.IsMonth(args.Month) {
		return nil, utils.Refusal{Message: "month must be YYYY-MM"}
	}
	return s.Month(ctx, args.Month)
}

// Month answers whether month is Complete (ADR 0014).
func (s *StatementsService) Month(ctx context.Context, month string) (MonthJSON, error) {
	accounts, err := s.accounts.accounts.List(ctx)
	if err != nil {
		return MonthJSON{}, err
	}
	answer := MonthJSON{Month: month, Missing: []AccountRefJSON{}, Late: []AccountRefJSON{}}
	lastDay := utils.LastDay(month)
	for _, a := range accounts {
		if !a.TracksIn(month) {
			continue
		}
		reached, err := s.reaches(ctx, a, lastDay)
		if err != nil {
			return MonthJSON{}, err
		}
		if reached {
			continue
		}
		ref := AccountRefJSON{ID: a.ID, Name: a.Name}
		if a.HoldsMonth {
			answer.Missing = append(answer.Missing, ref)
		} else {
			answer.Late = append(answer.Late, ref)
		}
	}
	answer.Complete = len(answer.Missing) == 0
	return answer, nil
}

// reaches is whether account's Statements reach day: one covers it, or, for
// a savings or investment Account, states a balance on or after it.
func (s *StatementsService) reaches(ctx context.Context, account model.Account, day string) (bool, error) {
	after, err := s.statements.ForAccount(ctx, account.ID, day, "")
	if err != nil {
		return false, err
	}
	balanceOnly := account.Kind == model.KindSavings || account.Kind == model.KindInvestment
	for _, statement := range after {
		if statement.From <= day || (balanceOnly && statement.ClosingBalance != nil) {
			return true, nil
		}
	}
	return false, nil
}
