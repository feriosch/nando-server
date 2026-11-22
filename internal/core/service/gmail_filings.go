package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// gmailFilingsKeptDays is how long the Filing Record keeps an entry.
const gmailFilingsKeptDays = 90

// maxGmailFilingsListed is the most entries core_list_gmail_filings answers.
const maxGmailFilingsListed = 200

// GmailFilingsService keeps the Filing Record: what each Gmail Filing changed,
// so the Owner can ask the Core Assistant to undo it (ADR 0011). Nando never
// touches Gmail itself: the run records what it did, and undoes it with
// Gmail's Tools.
type GmailFilingsService struct {
	filings  model.GmailFilingsRepository
	settings *SettingsService
	now      func() time.Time
}

// NewGmailFilings returns the GmailFilingsService over filings.
func NewGmailFilings(filings model.GmailFilingsRepository, settings *SettingsService, now func() time.Time) *GmailFilingsService {
	return &GmailFilingsService{filings: filings, settings: settings, now: now}
}

// ToolsDescription says what the Filing Record's Tools let a Connector do.
func (s *GmailFilingsService) ToolsDescription() string {
	return "record how it files your Gmail, and find what it filed so it can undo it"
}

// Tools are the Filing Record's Tools, for the Core Assistant's run.
func (s *GmailFilingsService) Tools() []model.Tool {
	return []model.Tool{
		{
			Name: "core_record_gmail_filing",
			Description: "Records what a Gmail Filing changed, one entry per thread, under today's Day. " +
				"Call it after changing the threads, with what was actually done. Several calls in one run add to the same Day.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"entries":{"type":"array","items":{"type":"object","properties":{` +
				`"threadId":{"type":"string","description":"Gmail's thread ID."},` +
				`"subject":{"type":"string"},` +
				`"from":{"type":"string","description":"The sender, as Gmail shows it."},` +
				`"added":{"type":"array","items":{"$ref":"#/$defs/label"},"description":"The labels added, STARRED excluded."},` +
				`"removed":{"type":"array","items":{"$ref":"#/$defs/label"},"description":"The labels removed: INBOX means archived."},` +
				`"starred":{"type":"boolean","description":"Whether the thread was starred."},` +
				`"reason":{"type":"string","description":"Why, in a few words, such as \"Uber Eats receipt → Food Delivery\"."}},` +
				`"required":["threadId","reason"]}}},"required":["entries"],` +
				`"$defs":{"label":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"}},"required":["id","name"]}}}`),
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Entries []struct {
						ThreadID string             `json:"threadId"`
						Subject  string             `json:"subject"`
						From     string             `json:"from"`
						Added    []model.GmailLabel `json:"added"`
						Removed  []model.GmailLabel `json:"removed"`
						Starred  bool               `json:"starred"`
						Reason   string             `json:"reason"`
					} `json:"entries"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				today, err := s.settings.Today(ctx)
				if err != nil {
					return nil, err
				}
				at := s.now()
				filings := make([]model.GmailFiling, 0, len(args.Entries))
				for i, e := range args.Entries {
					if strings.TrimSpace(e.ThreadID) == "" || strings.TrimSpace(e.Reason) == "" {
						return nil, fmt.Errorf("entry %d needs a threadId and a reason; nothing was recorded", i+1)
					}
					if !labelsHaveIDs(e.Added) || !labelsHaveIDs(e.Removed) {
						return nil, fmt.Errorf("entry %d has a label without its id; nothing was recorded", i+1)
					}
					filings = append(filings, model.GmailFiling{
						ID: bson.NewObjectID().Hex(), Day: today, At: at,
						ThreadID: e.ThreadID, Subject: e.Subject, From: e.From,
						Added: orEmpty(e.Added), Removed: orEmpty(e.Removed),
						Starred: e.Starred, Reason: e.Reason,
					})
				}
				if err := s.filings.Add(ctx, filings); err != nil {
					return nil, err
				}
				ids := make([]string, 0, len(filings))
				for _, f := range filings {
					ids = append(ids, f.ID)
				}
				return map[string]any{"recorded": ids}, nil
			},
		},
		{
			Name: "core_list_gmail_filings",
			Description: fmt.Sprintf("Lists the Filing Record's entries for a Day, or from one Day to another, oldest first: "+
				"at most the newest %d, with more true when there were others. query keeps only those whose subject, sender or reason contains it.",
				maxGmailFilingsListed),
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"day":{"type":"string","description":"One Day, YYYY-MM-DD."},` +
				`"from":{"type":"string","description":"The first Day of a range, YYYY-MM-DD."},` +
				`"to":{"type":"string","description":"The last Day of a range, YYYY-MM-DD."},` +
				`"query":{"type":"string","description":"Text to look for in the subject, sender or reason, ignoring case."}}}`),
			ReadOnly: true,
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Day   string `json:"day"`
					From  string `json:"from"`
					To    string `json:"to"`
					Query string `json:"query"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				from, to, err := filingDays(args.Day, args.From, args.To)
				if err != nil {
					return nil, err
				}
				filings, more, err := s.filings.List(ctx, model.GmailFilingsFilter{From: from, To: to, Query: strings.TrimSpace(args.Query)}, maxGmailFilingsListed)
				if err != nil {
					return nil, err
				}
				out := make([]model.GmailFilingJSON, 0, len(filings))
				for _, f := range filings {
					out = append(out, f.JSON())
				}
				return map[string]any{"filings": out, "more": more}, nil
			},
		},
		{
			Name: "core_mark_gmail_filing_undone",
			Description: "Marks a Filing Record entry undone, after reversing its change with Gmail's Tools: " +
				"removing the labels it added, adding back those it removed (INBOX moves the thread back to the inbox), and unstarring it if it was starred. " +
				"Marking it again changes nothing.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"The entry's ID, from core_list_gmail_filings."}},"required":["id"]}`),
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				filing, err := s.filings.MarkUndone(ctx, args.ID, s.now())
				if errors.Is(err, utils.ErrNotFound) {
					return nil, fmt.Errorf("no Filing Record entry %q: it may be older than %d days", args.ID, gmailFilingsKeptDays)
				}
				if err != nil {
					return nil, err
				}
				return map[string]any{"filing": filing.JSON()}, nil
			},
		},
	}
}

// RunAtTick drops the entries older than gmailFilingsKeptDays: undo is for
// recent mistakes.
func (s *GmailFilingsService) RunAtTick(ctx context.Context) error {
	return s.filings.DeleteBefore(ctx, s.now().AddDate(0, 0, -gmailFilingsKeptDays))
}

// filingDays are the first and last Days to list: day alone, or from to to.
func filingDays(day, from, to string) (model.Day, model.Day, error) {
	if day != "" {
		if from != "" || to != "" {
			return "", "", errors.New("give a day, or from and to, not both")
		}
		d, err := model.ParseDay(day)
		if err != nil {
			return "", "", errors.New("day must be a Day, YYYY-MM-DD")
		}
		return d, d, nil
	}
	first, err1 := model.ParseDay(from)
	last, err2 := model.ParseDay(to)
	switch {
	case from == "" && to == "":
		return "", "", errors.New("give a day, or from and to")
	case err1 != nil || err2 != nil:
		return "", "", errors.New("from and to must both be Days, YYYY-MM-DD")
	case last < first:
		return "", "", errors.New("to comes before from")
	}
	return first, last, nil
}

// labelsHaveIDs is whether every label has Gmail's ID, which undo needs.
func labelsHaveIDs(labels []model.GmailLabel) bool {
	for _, l := range labels {
		if strings.TrimSpace(l.ID) == "" {
			return false
		}
	}
	return true
}

// orEmpty is labels, or an empty list for none.
func orEmpty(labels []model.GmailLabel) []model.GmailLabel {
	if labels == nil {
		return []model.GmailLabel{}
	}
	return labels
}
