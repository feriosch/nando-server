package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/testtelegram"
)

func TestACSVWaitsWithItsFile(t *testing.T) {
	n, telegram, trigger := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "last month", "movements", "movements.csv", "text/csv", 2048))

	if got := len(trigger.Received()); got != 1 {
		t.Errorf("fires = %d, want 1", got)
	}
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Errorf("sendMessage calls = %+v; want none: a CSV is read", sent)
	}
	got := n.startRun().claim("")
	if len(got.Messages) != 1 || got.Messages[0].Text != "last month" ||
		len(got.Messages[0].Files) != 1 || got.Messages[0].Files[0].ID != "movements" || got.Messages[0].Files[0].Name != "movements.csv" {
		t.Errorf("claim = %+v; want the message, with its CSV listed by name", got.Messages)
	}
}

func TestACSVIsKnownByItsNameButASpreadsheetIsNot(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "", "octet", "MEISAI.CSV", "application/octet-stream", 2048))
	// Windows sends a CSV as Excel's type, the type of a real .xls too.
	n.mustPostUpdate("career", ownerSendsFile(5002, 43, "", "windows", "statement.csv", "application/vnd.ms-excel", 2048))
	n.mustPostUpdate("career", ownerSendsFile(5003, 44, "", "xls", "statement.xls", "application/vnd.ms-excel", 2048))
	n.mustPostUpdate("career", ownerSendsFile(5004, 45, "", "xlsx", "statement.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", 2048))

	got := n.startRun().claim("")
	var kept []string
	for _, m := range got.Messages {
		for _, f := range m.Files {
			kept = append(kept, f.Name)
		}
	}
	if !slices.Equal(kept, []string{"MEISAI.CSV", "statement.csv"}) {
		t.Errorf("kept %q; want the two CSVs only", kept)
	}
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 2 {
		t.Errorf("sendMessage calls = %+v; want the two spreadsheets answered", sent)
	}
}

// csvJSON is what messenger_get_file answers about a CSV.
type csvJSON struct {
	Encoding  string `json:"encoding"`
	LineCount int    `json:"lineCount"`
	FromLine  int    `json:"fromLine"`
	ToLine    int    `json:"toLine"`
	Text      string `json:"text"`
}

// sendCSV has the Owner send the CSV in testdata/name to the Career bot, and
// returns a run that claimed it and the CSV's id.
func sendCSV(t *testing.T, n *nando, telegram *testtelegram.Telegram, name string) (*run, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	telegram.SetFile("file-"+name, data)
	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "", "file-"+name, name, "text/csv", len(data)))
	r := n.startRun()
	return r, r.claim("").Messages[0].Files[0].ID
}

// csvAnswer is messenger_get_file's answer about a CSV, and fails the test if
// it failed.
func csvAnswer(t *testing.T, result *mcp.CallToolResult) csvJSON {
	t.Helper()
	text, _ := result.Content[0].(*mcp.TextContent)
	if result.IsError || text == nil {
		t.Fatalf("messenger_get_file = %+v; want the CSV", result.Content)
	}
	var got csvJSON
	if err := json.Unmarshal([]byte(text.Text), &got); err != nil {
		t.Fatalf("messenger_get_file answered %q: %v", text.Text, err)
	}
	return got
}

// statementText is testdata's statement-sjis.csv and statement-utf8-bom.csv,
// as text: a made-up bank statement in Japanese.
const statementText = "取扱日付,入出金明細ID,受入金額（円）,払出金額（円）,詳細１,詳細２\n" +
	"20260105,1,,3000,ATM,テスト駅前\n" +
	"20260125,2,250000,,給与,テスト商事"

func TestAShiftJISCSVIsReadAsItsText(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendCSV(t, n, telegram, "statement-sjis.csv")

	got := csvAnswer(t, r.getFile("career", id, ""))

	if got.Encoding != "shift_jis" || got.LineCount != 3 || got.FromLine != 1 || got.ToLine != 3 || got.Text != statementText {
		t.Errorf("messenger_get_file = %+v; want its 3 lines as text, from Shift_JIS", got)
	}
}

func TestAUTF8CSVIsReadWithoutItsBOM(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendCSV(t, n, telegram, "statement-utf8-bom.csv")

	got := csvAnswer(t, r.getFile("career", id, ""))

	if got.Encoding != "utf-8" || got.LineCount != 3 || got.Text != statementText {
		t.Errorf("messenger_get_file = %+v; want its 3 lines, without the BOM", got)
	}
}

// getCSVLines calls messenger_get_file as the Career Assistant for lines of
// the CSV with id.
func (r *run) getCSVLines(id, lines string) *mcp.CallToolResult {
	r.n.t.Helper()
	result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "messenger_get_file", Arguments: map[string]any{"assistant": "career", "id": id, "lines": lines},
	})
	if err != nil {
		r.n.t.Fatalf("messenger_get_file: %v", err)
	}
	return result
}

func TestALongCSVIsReadInParts(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendCSV(t, n, telegram, "long.csv") // a header and 650 rows

	var parts []string
	for _, c := range []struct {
		lines    string
		from, to int
	}{
		{"", 1, 300},
		{"301-600", 301, 600},
		{"601-651", 601, 651},
	} {
		got := csvAnswer(t, r.getCSVLines(id, c.lines))
		if got.LineCount != 651 || got.FromLine != c.from || got.ToLine != c.to {
			t.Errorf("lines %q: read %d-%d of %d; want %d-%d of 651", c.lines, got.FromLine, got.ToLine, got.LineCount, c.from, c.to)
		}
		parts = append(parts, got.Text)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "long.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if whole := strings.Join(parts, "\n") + "\n"; whole != string(data) {
		t.Errorf("the parts joined differ from the file")
	}
	if got := csvAnswer(t, r.getCSVLines(id, "2-651")); got.FromLine != 2 || got.ToLine != 301 {
		t.Errorf("lines 2-651: read %d-%d; want 2-301, capped at 300 lines", got.FromLine, got.ToLine)
	}
	for _, lines := range []string{"652", "0", "5-2", "two"} {
		if result := r.getCSVLines(id, lines); !result.IsError {
			t.Errorf("lines %q: %+v; want a Tool error", lines, result.Content)
		}
	}
}

func TestABinaryFileNamedCSVCantBeRead(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	telegram.SetFile("zip", []byte("PK\x03\x04\x14\x00\x00\x00 a spreadsheet, zipped"))
	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "", "zip", "statement.csv", "application/octet-stream", 40))
	r := n.startRun()

	result := r.getFile("career", r.claim("").Messages[0].Files[0].ID, "")

	text, _ := result.Content[0].(*mcp.TextContent)
	if !result.IsError || text == nil || !strings.Contains(text.Text, "can't be read") {
		t.Errorf("messenger_get_file = %+v; want an error saying it can't be read", result.Content)
	}
}

func TestALargeCSVIsAnsweredButNotKept(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "a whole year", "year", "year.csv", "text/csv", 11_000_000))

	if got := n.talk("career").Messages; len(got) != 0 {
		t.Errorf("Career Talk = %+v; want nothing kept", got)
	}
	sent := telegram.CallsTo("sendMessage")
	want := "That file is too large: I can read PDFs and CSV files of up to 10 MB, and images of up to 5 MB."
	if len(sent) != 1 || sent[0].Params["text"] != want {
		t.Errorf("sendMessage calls = %+v; want %q", sent, want)
	}
}
