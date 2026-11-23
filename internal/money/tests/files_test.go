package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/money"
)

// fakeBucket is the Statements bucket, in memory.
type fakeBucket struct {
	mu      sync.Mutex
	objects map[string][]byte
	// failing makes every Put fail, as a bucket out of reach.
	failing bool
}

func (b *fakeBucket) Put(_ context.Context, name string, data []byte, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failing {
		return errors.New("bucket unreachable")
	}
	if _, ok := b.objects[name]; ok {
		return errors.New("Nando may not overwrite an object")
	}
	b.objects[name] = data
	return nil
}

func (b *fakeBucket) Get(_ context.Context, name string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.objects[name]
	if !ok {
		return nil, errors.New("no such object")
	}
	return data, nil
}

var _ money.FileStore = (*fakeBucket)(nil)

// withBucket returns Nando with the Owner signed in and Statements' files
// kept in a fake bucket.
func withBucket(t *testing.T) (*nando, *browser, *fakeBucket) {
	t.Helper()
	bucket := &fakeBucket{objects: map[string][]byte{}}
	n, phone := signedInBrowserWith(t, func(cfg *money.Config) { cfg.Files = bucket })
	return n, phone, bucket
}

// keptStatementJSON is a Statement with what was done with its files.
type keptStatementJSON struct {
	ID           string   `json:"id"`
	Files        []string `json:"files"`
	MissingFiles []string `json:"missingFiles"`
	FilesNote    string   `json:"filesNote"`
}

// testPDF is a PDF with text, from Core's test data.
func testPDF(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../core/tests/testdata/job-description.pdf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAStatementsFilesAreKeptInTheBucket(t *testing.T) {
	n, phone, bucket := withBucket(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)
	n.telegram.SetFile("page-1", testPDF(t))
	n.telegram.SetFile("page-2", []byte("\x89PNG\r\n\x1a\n a screenshot"))

	got := callTool[keptStatementJSON](t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"page-1", "page-2"},
	})

	if len(got.Files) != 2 || len(bucket.objects) != 2 {
		t.Fatalf("files %q, bucket %d objects; want both files kept", got.Files, len(bucket.objects))
	}
	for i, name := range got.Files {
		if !strings.HasPrefix(name, card.ID+"/2026-10-01_2026-10-31/") {
			t.Errorf("file %d is %q; want it under the Account and period", i, name)
		}
	}
	if !bytes.Equal(bucket.objects[got.Files[0]], testPDF(t)) {
		t.Errorf("the first file kept differs from the PDF sent")
	}
	if stored := callTool[keptStatementJSON](t, session, "money_get_statement", map[string]any{"id": got.ID}); len(stored.Files) != 2 {
		t.Errorf("money_get_statement files = %q; want both recorded", stored.Files)
	}

	// Sent again, its files go beside the earlier ones: Nando can't overwrite.
	again := callTool[keptStatementJSON](t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"page-1"},
	})
	if len(again.Files) != 1 || len(bucket.objects) != 3 {
		t.Errorf("sent again: files %q, bucket %d objects; want a third object", again.Files, len(bucket.objects))
	}
}

func TestNothingIsSavedWhenTheBucketFails(t *testing.T) {
	n, phone, bucket := withBucket(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)
	n.telegram.SetFile("page-1", testPDF(t))
	bucket.failing = true

	msg := toolError(t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"page-1"},
		"transactions": []any{line("expense", "2026-10-03", 100, "SHOP")},
	})

	if !strings.Contains(msg, "bucket") {
		t.Errorf("money_save_statement = %q; want it to say the file couldn't be kept", msg)
	}
	if got := callTool[[]transactionJSON](t, session, "money_list_transactions", nil); len(got) != 0 {
		t.Errorf("Transactions = %+v; want nothing saved", got)
	}
}

func TestAFileTelegramDroppedIsReportedAndTheStatementSaved(t *testing.T) {
	n, phone, _ := withBucket(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)
	n.telegram.SetFile("page-1", testPDF(t))

	got := callTool[keptStatementJSON](t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"page-1", "gone"},
	})

	if got.ID == "" || len(got.Files) != 1 || len(got.MissingFiles) != 1 || got.MissingFiles[0] != "gone" {
		t.Errorf("money_save_statement = %+v; want it saved with the one file, and the dropped one named", got)
	}
}

func TestAStatementIsSavedWithoutFilesWhenThereIsNoBucket(t *testing.T) {
	n, phone := signedInBrowser(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)
	n.telegram.SetFile("page-1", testPDF(t))

	got := callTool[keptStatementJSON](t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"page-1"},
	})

	if got.ID == "" || len(got.Files) != 0 || !strings.Contains(got.FilesNote, "not kept") {
		t.Errorf("money_save_statement = %+v; want it saved, saying its files were not kept", got)
	}
}

func TestAKeptFileIsReadBack(t *testing.T) {
	n, phone, _ := withBucket(t)
	session := phone.tools()
	bank, _ := twoAccounts(t, session)
	n.telegram.SetFile("pdf", testPDF(t))
	n.telegram.SetFile("csv", []byte("date,amount\n2026-10-03,500\n"))
	n.telegram.SetFile("png", []byte("\x89PNG\r\n\x1a\n a screenshot"))
	s := callTool[keptStatementJSON](t, session, "money_save_statement", map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "fileIds": []string{"pdf", "csv", "png"},
	})
	read := func(n int) *mcp.CallToolResult {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "money_get_statement_file", Arguments: map[string]any{"statement": s.ID, "n": n},
		})
		if err != nil || result.IsError {
			t.Fatalf("money_get_statement_file %d: %v %+v", n, err, result)
		}
		return result
	}

	var pdf struct {
		PageCount int `json:"pageCount"`
		Pages     []struct {
			Text string `json:"text"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(read(1).Content[0].(*mcp.TextContent).Text), &pdf); err != nil || pdf.PageCount != 2 ||
		!strings.HasPrefix(pdf.Pages[0].Text, "Senior Go Engineer") {
		t.Errorf("file 1 = %+v (%v); want the PDF's pages", pdf, err)
	}
	var csv struct {
		LineCount int    `json:"lineCount"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal([]byte(read(2).Content[0].(*mcp.TextContent).Text), &csv); err != nil || csv.LineCount != 2 {
		t.Errorf("file 2 = %+v (%v); want the CSV's lines", csv, err)
	}
	if image, ok := read(3).Content[0].(*mcp.ImageContent); !ok || image.MIMEType != "image/png" {
		t.Errorf("file 3 = %+v; want the screenshot as an image", read(3).Content[0])
	}
	if msg := toolError(t, session, "money_get_statement_file", map[string]any{"statement": s.ID, "n": 4}); !strings.Contains(msg, "3 files") {
		t.Errorf("file 4: %q; want it refused, saying there are 3", msg)
	}
}
