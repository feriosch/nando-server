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

// AccountsService handles the Owner's Accounts. Only the Money Assistant
// reads and changes them, through its Tools.
type AccountsService struct {
	accounts model.AccountsRepository
	// statements tells whether an Account has Statements, whose amounts are
	// in its currency.
	statements model.StatementsRepository
}

// NewAccounts returns the AccountsService.
func NewAccounts(accounts model.AccountsRepository, statements model.StatementsRepository) *AccountsService {
	return &AccountsService{accounts: accounts, statements: statements}
}

// find is the Account with id, or a Refusal if there is none.
func (s *AccountsService) find(ctx context.Context, id string) (model.Account, error) {
	account, err := s.accounts.Find(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return account, utils.Refusal{Message: fmt.Sprintf("there's no such Account %q: money_list_accounts lists them", id)}
	}
	return account, err
}

func toAccountJSON(a model.Account) model.AccountJSON {
	kind := string(a.Kind)
	return model.AccountJSON{
		ID: &a.ID, Name: &a.Name, Kind: &kind, Currency: &a.Currency, Institution: &a.Institution, Last4: &a.Last4,
		Note: &a.Note, TrackedFrom: &a.TrackedFrom, ClosedFrom: &a.ClosedFrom, HoldsMonth: &a.HoldsMonth,
	}
}

// Tools are what the AccountsService offers Connectors.
func (s *AccountsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_save_account",
		Description: "Saves one of the Owner's Accounts: without id, a new one (name, kind, currency and trackedFrom " +
			"needed); with id, only the fields given change. Answers the Account. An Account is never deleted, only " +
			"closed from a month on with closedFrom, and its currency can't change once it has Statements.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"The Account to change; leave out for a new one."},` +
			`"name":{"type":"string"},` +
			`"kind":{"type":"string","enum":["bank","card","savings","investment"]},` +
			`"currency":{"type":"string","enum":["JPY","MXN"]},` +
			`"institution":{"type":"string","description":"The bank or card's issuer."},` +
			`"last4":{"type":"string","description":"At most the last 4 digits, to tell two Accounts apart; never a full number. Empty for none."},` +
			`"note":{"type":"string","description":"What you learned about it: its export format, its billing cycle."},` +
			`"trackedFrom":{"type":"string","description":"The first month tracked, YYYY-MM."},` +
			`"closedFrom":{"type":"string","description":"The first month no longer tracked, YYYY-MM; empty while open."},` +
			`"holdsMonth":{"type":"boolean","description":"Whether a month waits for its Statements to be Complete; true unless it is a card whose charge never changes and closes mid-month."}}}`),
		Handle: s.saveAccountTool,
	}, {
		Name:        "money_list_accounts",
		Description: "Every Account of the Owner's, open and closed, by name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			accounts, err := s.accounts.List(ctx)
			if err != nil {
				return nil, err
			}
			list := make([]model.AccountJSON, len(accounts))
			for i, a := range accounts {
				list[i] = toAccountJSON(a)
			}
			return list, nil
		},
	}}
}

// saveAccountTool creates an Account, or changes the fields given of one.
func (s *AccountsService) saveAccountTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.AccountJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send the Account's fields as strings, and holdsMonth as true or false"}
	}
	account := model.Account{HoldsMonth: true}
	isNew := args.ID == nil
	if !isNew {
		var err error
		if account, err = s.find(ctx, *args.ID); err != nil {
			return nil, err
		}
	} else if args.Name == nil || args.Kind == nil || args.Currency == nil || args.TrackedFrom == nil {
		return nil, utils.Refusal{Message: "a new Account needs name, kind, currency and trackedFrom"}
	}
	if args.Currency != nil && !isNew && *args.Currency != account.Currency {
		has, err := s.statements.AccountHasStatements(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		if has {
			return nil, utils.Refusal{Message: "the Account's currency can't change: it has Statements in " + account.Currency}
		}
	}
	if err := changeAccount(&account, args); err != nil {
		return nil, err
	}
	if isNew {
		id, err := s.accounts.Insert(ctx, account)
		if err != nil {
			return nil, err
		}
		account.ID = id
	} else if err := s.accounts.Replace(ctx, account); err != nil {
		return nil, err
	}
	return toAccountJSON(account), nil
}

// changeAccount sets the fields args gives on a, or refuses saying what is
// wrong with them.
func changeAccount(a *model.Account, args model.AccountJSON) error {
	if args.Name != nil {
		if a.Name = strings.TrimSpace(*args.Name); a.Name == "" {
			return utils.Refusal{Message: "name can't be empty"}
		}
	}
	if args.Kind != nil {
		if !slices.Contains(model.Kinds, model.Kind(*args.Kind)) {
			return utils.Refusal{Message: fmt.Sprintf("kind %q isn't bank, card, savings or investment", *args.Kind)}
		}
		a.Kind = model.Kind(*args.Kind)
	}
	if args.Currency != nil {
		if !slices.Contains(model.Currencies, *args.Currency) {
			return utils.Refusal{Message: fmt.Sprintf("currency %q isn't one of the Owner's, JPY or MXN", *args.Currency)}
		}
		a.Currency = *args.Currency
	}
	if args.Institution != nil {
		a.Institution = strings.TrimSpace(*args.Institution)
	}
	if args.Last4 != nil {
		last4 := strings.TrimSpace(*args.Last4)
		if last4 != "" && (len(last4) != 4 || strings.ContainsFunc(last4, func(r rune) bool { return r < '0' || r > '9' })) {
			return utils.Refusal{Message: "last4 must be exactly 4 digits, never a full number, or empty"}
		}
		a.Last4 = last4
	}
	if args.Note != nil {
		a.Note = strings.TrimSpace(*args.Note)
	}
	if args.TrackedFrom != nil {
		if !utils.IsMonth(*args.TrackedFrom) {
			return utils.Refusal{Message: "trackedFrom must be a month, YYYY-MM"}
		}
		a.TrackedFrom = *args.TrackedFrom
	}
	if args.ClosedFrom != nil {
		if *args.ClosedFrom != "" && !utils.IsMonth(*args.ClosedFrom) {
			return utils.Refusal{Message: "closedFrom must be a month, YYYY-MM, or empty while the Account is open"}
		}
		a.ClosedFrom = *args.ClosedFrom
	}
	if a.ClosedFrom != "" && a.ClosedFrom <= a.TrackedFrom {
		return utils.Refusal{Message: "closedFrom must come after trackedFrom"}
	}
	if args.HoldsMonth != nil {
		a.HoldsMonth = *args.HoldsMonth
	}
	return nil
}
