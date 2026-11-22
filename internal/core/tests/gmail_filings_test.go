package tests

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// labelJSON is a Gmail label as a Filing Record names it.
type labelJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// filingJSON is one entry of the Filing Record, as the Tools answer it.
type filingJSON struct {
	ID       string      `json:"id"`
	Day      string      `json:"day"`
	ThreadID string      `json:"threadId"`
	Subject  string      `json:"subject"`
	From     string      `json:"from"`
	Added    []labelJSON `json:"added"`
	Removed  []labelJSON `json:"removed"`
	Starred  bool        `json:"starred"`
	Reason   string      `json:"reason"`
	Undone   bool        `json:"undone"`
}

type filingsJSON struct {
	Filings []filingJSON `json:"filings"`
	More    bool         `json:"more"`
}

// filingNando is Nando with the Core Assistant's bot, at 09:00 on
// 2026-11-22 in the Owner Time Zone (Asia/Tokyo).
func filingNando(t *testing.T) (*nando, *run) {
	t.Helper()
	n, _, _ := messengerNandoWithTrigger(t)
	n.clock.Set(time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC))
	return n, n.startRun()
}

// uberEats is a filing entry: an Uber Eats receipt, filed and archived.
func uberEats() map[string]any {
	return map[string]any{
		"threadId": "19a1",
		"subject":  "Your Uber Eats order",
		"from":     "Uber Receipts <noreply@uber.com>",
		"added":    []map[string]any{{"id": "Label_733", "name": "Payments"}, {"id": "Label_9", "name": "Payments/Food Delivery"}},
		"removed":  []map[string]any{{"id": "INBOX", "name": "INBOX"}},
		"reason":   "Uber Eats receipt → Food Delivery",
	}
}

// landlord is a filing entry: the landlord's rent notice, starred.
func landlord() map[string]any {
	return map[string]any{
		"threadId": "19b2",
		"subject":  "Rent for December",
		"from":     "Sakura Homes <info@sakura-homes.jp>",
		"added":    []map[string]any{{"id": "Label_733", "name": "Payments"}, {"id": "Label_16", "name": "Payments/Utilities"}},
		"starred":  true,
		"reason":   "Rent due on the 27th",
	}
}

func (r *run) recordFiling(entries ...map[string]any) {
	r.n.t.Helper()
	callAs[map[string]any](r, "core_record_gmail_filing", map[string]any{"entries": entries})
}

func (r *run) listFilings(args map[string]any) filingsJSON {
	r.n.t.Helper()
	return callAs[filingsJSON](r, "core_list_gmail_filings", args)
}

// withoutIDs is filings with their IDs blanked, which Nando chooses.
func withoutIDs(filings []filingJSON) []filingJSON {
	out := []filingJSON{}
	for _, f := range filings {
		if f.ID == "" {
			continue
		}
		f.ID = ""
		out = append(out, f)
	}
	return out
}

func TestARunRecordsWhatItFiledAndListsTheDay(t *testing.T) {
	_, r := filingNando(t)

	r.recordFiling(uberEats(), landlord())

	got := r.listFilings(map[string]any{"day": "2026-11-22"})
	want := []filingJSON{
		{
			Day: "2026-11-22", ThreadID: "19a1", Subject: "Your Uber Eats order", From: "Uber Receipts <noreply@uber.com>",
			Added:   []labelJSON{{"Label_733", "Payments"}, {"Label_9", "Payments/Food Delivery"}},
			Removed: []labelJSON{{"INBOX", "INBOX"}},
			Reason:  "Uber Eats receipt → Food Delivery",
		},
		{
			Day: "2026-11-22", ThreadID: "19b2", Subject: "Rent for December", From: "Sakura Homes <info@sakura-homes.jp>",
			Added:   []labelJSON{{"Label_733", "Payments"}, {"Label_16", "Payments/Utilities"}},
			Removed: []labelJSON{},
			Starred: true,
			Reason:  "Rent due on the 27th",
		},
	}
	if g := withoutIDs(got.Filings); !reflect.DeepEqual(g, want) || len(g) != len(got.Filings) || got.More {
		t.Errorf("filings =\n%+v\nwant\n%+v, each with an ID", got, want)
	}
}

// threadsOf is the thread IDs of filings, in order.
func threadsOf(filings []filingJSON) []string {
	ids := []string{}
	for _, f := range filings {
		ids = append(ids, f.ThreadID)
	}
	return ids
}

func TestEachRunsFilingsAreItsDaysAndARangeListsSeveralDays(t *testing.T) {
	n, r := filingNando(t)

	n.clock.Advance(-24 * time.Hour) // 2026-11-21
	r.recordFiling(uberEats())
	n.clock.Advance(24 * time.Hour) // 2026-11-22: two calls in one run
	r.recordFiling(landlord())
	r.recordFiling(map[string]any{"threadId": "19c3", "subject": "Your code", "reason": "Sign-in code"})

	for _, c := range []struct {
		args map[string]any
		want []string
	}{
		{map[string]any{"day": "2026-11-21"}, []string{"19a1"}},
		{map[string]any{"day": "2026-11-22"}, []string{"19b2", "19c3"}},
		{map[string]any{"day": "2026-11-20"}, []string{}},
		{map[string]any{"from": "2026-11-21", "to": "2026-11-22"}, []string{"19a1", "19b2", "19c3"}},
		{map[string]any{"from": "2026-11-22", "to": "2026-11-30"}, []string{"19b2", "19c3"}},
	} {
		if got := threadsOf(r.listFilings(c.args).Filings); !reflect.DeepEqual(got, c.want) {
			t.Errorf("list %v = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestListingFilingsNeedsADayOrARange(t *testing.T) {
	_, r := filingNando(t)

	for _, args := range []map[string]any{
		{},
		{"day": "22 November"},
		{"from": "2026-11-21"},
		{"to": "2026-11-21"},
		{"from": "2026-11-22", "to": "2026-11-21"},
		{"day": "2026-11-22", "from": "2026-11-21", "to": "2026-11-22"},
	} {
		result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "core_list_gmail_filings", Arguments: args})
		if err != nil || !result.IsError {
			t.Errorf("list %v = %+v, %v; want a Tool error", args, result, err)
		}
	}
}

func TestAQueryFindsFilingsBySenderSubjectOrReason(t *testing.T) {
	_, r := filingNando(t)
	r.recordFiling(uberEats(), landlord())

	for query, want := range map[string][]string{
		"sakura-homes":  {"19b2"}, // the sender
		"UBER EATS ORD": {"19a1"}, // the subject, ignoring case
		"rent due":      {"19b2"}, // the reason
		"payments":      {},       // a label is not searched
		"(":             {},       // not a pattern
	} {
		args := map[string]any{"day": "2026-11-22", "query": query}
		if got := threadsOf(r.listFilings(args).Filings); !reflect.DeepEqual(got, want) {
			t.Errorf("query %q = %v, want %v", query, got, want)
		}
	}
}

func TestARunMarksAFilingItUndidAsUndone(t *testing.T) {
	_, r := filingNando(t)
	r.recordFiling(uberEats(), landlord())
	rent := r.listFilings(map[string]any{"day": "2026-11-22", "query": "rent"}).Filings[0]

	undone := callAs[struct {
		Filing filingJSON `json:"filing"`
	}](r, "core_mark_gmail_filing_undone", map[string]any{"id": rent.ID})
	if undone.Filing.ID != rent.ID || !undone.Filing.Undone {
		t.Errorf("marked = %+v, want %s undone", undone.Filing, rent.ID)
	}
	// Marking it again, as a retried call does, changes nothing.
	callAs[map[string]any](r, "core_mark_gmail_filing_undone", map[string]any{"id": rent.ID})

	got := map[string]bool{}
	for _, f := range r.listFilings(map[string]any{"day": "2026-11-22"}).Filings {
		got[f.ThreadID] = f.Undone
	}
	if want := map[string]bool{"19a1": false, "19b2": true}; !reflect.DeepEqual(got, want) {
		t.Errorf("undone = %v, want %v", got, want)
	}

	result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "core_mark_gmail_filing_undone", Arguments: map[string]any{"id": "nope"},
	})
	if err != nil || !result.IsError {
		t.Errorf("marking an unknown entry = %+v, %v; want a Tool error", result, err)
	}
}

func TestTheTickDropsFilingsOlderThan90Days(t *testing.T) {
	n, r := filingNando(t)
	r.recordFiling(uberEats())
	n.clock.Advance(24 * time.Hour)
	n.startRun().recordFiling(landlord()) // each Day's run is a new one
	all := map[string]any{"from": "2026-11-01", "to": "2027-03-31"}

	n.clock.Advance(89 * 24 * time.Hour) // 90 days after the first
	r = n.startRun()
	n.mustTick()
	if got, want := threadsOf(r.listFilings(all).Filings), []string{"19a1", "19b2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after 90 days, filings = %v, want %v", got, want)
	}

	n.clock.Advance(time.Minute)
	n.mustTick()
	if got, want := threadsOf(r.listFilings(all).Filings), []string{"19b2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after 90 days and a minute, filings = %v, want %v", got, want)
	}
}

func TestAFilingWithoutAThreadOrAReasonRecordsNothing(t *testing.T) {
	_, r := filingNando(t)

	for _, bad := range []map[string]any{
		{"subject": "No thread", "reason": "Sign-in code"},
		{"threadId": "19c3", "subject": "No reason"},
	} {
		result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "core_record_gmail_filing", Arguments: map[string]any{"entries": []map[string]any{uberEats(), bad}},
		})
		if err != nil || !result.IsError {
			t.Errorf("recording %v = %+v, %v; want a Tool error", bad, result, err)
		}
	}
	if got := r.listFilings(map[string]any{"day": "2026-11-22"}).Filings; len(got) != 0 {
		t.Errorf("filings = %+v, want none", got)
	}
}
