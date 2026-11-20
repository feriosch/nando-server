package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/testtelegram"
)

// ownerSendsFile is the Owner's file, with caption, as Telegram posts a
// document: its file_id, name, type and size in bytes.
func ownerSendsFile(updateID, messageID int64, caption, fileID, name, mimeType string, size int) telegramUpdate {
	u := ownerSays(updateID, messageID, "")
	u.Message.Caption = caption
	u.Message.Document = map[string]any{
		"file_id": fileID, "file_unique_id": "u-" + fileID, "file_name": name, "mime_type": mimeType, "file_size": size,
	}
	return u
}

func TestAnImageSentAsAFileWaitsAsAPhoto(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	telegram.SetFile("screenshot", []byte("\x89PNG\r\n\x1a\n a screenshot"))

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "recruiter X wrote this", "screenshot", "IMG_0042.PNG", "image/png", 2_400_000))

	r := n.startRun()
	got := r.claim("")
	if len(got.Messages) != 1 || got.Messages[0].Text != "recruiter X wrote this" || len(got.Messages[0].Photos) != 1 {
		t.Fatalf("claim = %+v; want the message with its caption and the image as a photo", got.Messages)
	}
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Errorf("sendMessage calls = %+v; want none: an image sent as a file is read", sent)
	}
	result := r.getPhoto("career", got.Messages[0].Photos[0].ID)
	if image, ok := result.Content[0].(*mcp.ImageContent); !ok || image.MIMEType != "image/png" {
		t.Errorf("messenger_get_photo = %+v; want the PNG, as a PNG", result.Content[0])
	}
}

func TestAPDFWithACaptionWaitsWithItsFile(t *testing.T) {
	n, telegram, trigger := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "the JD from recruiter X", "jd", "Senior Go Engineer.pdf", "application/pdf", 1833))

	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 1 {
		t.Errorf("setMessageReaction calls = %+v; want the 👀 on the file", reactions)
	}
	if got := len(trigger.Received()); got != 1 {
		t.Errorf("fires = %d, want 1", got)
	}
	got := n.startRun().claim("")
	if len(got.Messages) != 1 || got.Messages[0].Text != "the JD from recruiter X" || !got.Messages[0].Waiting ||
		len(got.Messages[0].Files) != 1 || got.Messages[0].Files[0].ID == "" || got.Messages[0].Files[0].Name != "Senior Go Engineer.pdf" ||
		len(got.Messages[0].Photos) != 0 {
		t.Errorf("claim = %+v; want the message, its caption as its text, with its PDF listed by name", got.Messages)
	}
}

func TestALargePDFOrAnotherKindOfFileIsAnsweredButNotKept(t *testing.T) {
	n, telegram, trigger := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "the whole contract", "big", "contract.pdf", "application/pdf", 12_000_000))
	n.mustPostUpdate("career", ownerSendsFile(5002, 43, "my CV", "cv", "CV.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document", 80_000))

	if got := n.talk("career").Messages; len(got) != 0 {
		t.Errorf("Career Talk = %+v; want nothing kept", got)
	}
	if got := len(trigger.Received()); got != 0 {
		t.Errorf("fires = %d, want none", got)
	}
	var texts []string
	for _, c := range telegram.CallsTo("sendMessage") {
		texts = append(texts, c.Params["text"].(string))
	}
	want := []string{
		"That file is too large: I can read PDFs of up to 10 MB and images of up to 5 MB.",
		"I can read text, photos and PDFs, but not other files, stickers or voice notes yet.",
	}
	if !slices.Equal(texts, want) {
		t.Errorf("the bot said %q; want %q", texts, want)
	}
}

// fileJSON is what messenger_get_file answers about a PDF.
type fileJSON struct {
	Name      string `json:"name"`
	PageCount int    `json:"pageCount"`
	Pages     []struct {
		Page int    `json:"page"`
		Text string `json:"text"`
	} `json:"pages"`
	Scanned bool `json:"scanned"`
}

// getFile calls messenger_get_file as assistant for the file with id, and
// pages if it isn't "".
func (r *run) getFile(assistant, id, pages string) *mcp.CallToolResult {
	r.n.t.Helper()
	args := map[string]any{"assistant": assistant, "id": id}
	if pages != "" {
		args["pages"] = pages
	}
	result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "messenger_get_file", Arguments: args})
	if err != nil {
		r.n.t.Fatalf("messenger_get_file: %v", err)
	}
	return result
}

// fileAnswer is messenger_get_file's answer about the file, from its first,
// JSON, content, and fails the test if it failed.
func fileAnswer(t *testing.T, result *mcp.CallToolResult) fileJSON {
	t.Helper()
	text, _ := result.Content[0].(*mcp.TextContent)
	if result.IsError || text == nil {
		t.Fatalf("messenger_get_file = %+v; want the file", result.Content)
	}
	var got fileJSON
	if err := json.Unmarshal([]byte(text.Text), &got); err != nil {
		t.Fatalf("messenger_get_file answered %q: %v", text.Text, err)
	}
	return got
}

// sendPDF has the Owner send the PDF in testdata/name to the Career bot, and
// returns a run that claimed it and the PDF's id.
func sendPDF(t *testing.T, n *nando, telegram *testtelegram.Telegram, name string) (*run, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	telegram.SetFile("file-"+name, data)
	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "", "file-"+name, name, "application/pdf", len(data)))
	r := n.startRun()
	return r, r.claim("").Messages[0].Files[0].ID
}

func TestAPDFWithTextIsReadAsItsText(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendPDF(t, n, telegram, "job-description.pdf")

	got := fileAnswer(t, r.getFile("career", id, ""))

	if got.PageCount != 2 || got.Scanned || len(got.Pages) != 2 ||
		got.Pages[0].Page != 1 || got.Pages[0].Text != "Senior Go Engineer at Company Y\nTokyo, hybrid, 3 office days" ||
		got.Pages[1].Page != 2 || got.Pages[1].Text != "Salary 9-12M JPY\nApply by December 15" {
		t.Errorf("messenger_get_file = %+v; want both pages' text, of 2", got)
	}
}

func TestAScannedPDFIsReadAsPageImages(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendPDF(t, n, telegram, "scan.pdf")

	result := r.getFile("career", id, "")

	got := fileAnswer(t, result)
	if got.PageCount != 1 || !got.Scanned {
		t.Errorf("messenger_get_file = %+v; want 1 page, a scan", got)
	}
	if len(result.Content) != 2 {
		t.Fatalf("messenger_get_file content = %+v; want the answer, then the page as an image", result.Content)
	}
	image, ok := result.Content[1].(*mcp.ImageContent)
	if !ok || image.MIMEType != "image/jpeg" || !bytes.HasPrefix(image.Data, []byte("\xff\xd8\xff")) {
		t.Errorf("page = %+v; want it as a JPEG", result.Content[1])
	}
}

func TestARunReadsAtMostTenPagesAtATime(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendPDF(t, n, telegram, "twelve-pages.pdf")

	for _, c := range []struct {
		pages string
		want  []int
	}{
		{"", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{"11-12", []int{11, 12}},
		{"3", []int{3}},
		{"2-12", []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}}, // capped at 10 pages
	} {
		got := fileAnswer(t, r.getFile("career", id, c.pages))
		var read []int
		for _, p := range got.Pages {
			read = append(read, p.Page)
			if want := fmt.Sprintf("Page %d of 12", p.Page); p.Text != want {
				t.Errorf("pages %q: page %d's text = %q, want %q", c.pages, p.Page, p.Text, want)
			}
		}
		if got.PageCount != 12 || !slices.Equal(read, c.want) {
			t.Errorf("pages %q: read %v of %d; want %v of 12", c.pages, read, got.PageCount, c.want)
		}
	}

	for _, pages := range []string{"13", "0", "5-2", "two"} {
		if result := r.getFile("career", id, pages); !result.IsError {
			t.Errorf("pages %q: %+v; want a Tool error", pages, result.Content)
		}
	}
}

func TestAMissingOrBrokenPDFIsAnErrorWithoutTheBotToken(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, broken := sendPDF(t, n, telegram, "broken.pdf")

	for name, c := range map[string]struct{ id, want string }{
		"a broken PDF":                  {broken, "can't be read"},
		"a file Telegram no longer has": {"no-such-file", "send it again"},
	} {
		result := r.getFile("career", c.id, "")
		text, _ := result.Content[0].(*mcp.TextContent)
		if !result.IsError || text == nil || !strings.Contains(text.Text, c.want) || strings.Contains(text.Text, careerBotToken) {
			t.Errorf("%s: messenger_get_file = %+v; want an error saying it %s, without the bot token", name, result.Content, c.want)
		}
	}
}

func TestAPDFsScannedPagesComeAsImagesAmongItsTextPages(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	r, id := sendPDF(t, n, telegram, "offer-with-scanned-page.pdf")

	result := r.getFile("career", id, "")

	var got struct {
		PageCount int  `json:"pageCount"`
		Scanned   bool `json:"scanned"`
		Pages     []struct {
			Page  int    `json:"page"`
			Text  string `json:"text"`
			Image bool   `json:"image"`
		} `json:"pages"`
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if result.IsError || text == nil || json.Unmarshal([]byte(text.Text), &got) != nil {
		t.Fatalf("messenger_get_file = %+v; want the PDF", result.Content)
	}
	if got.PageCount != 2 || got.Scanned || len(got.Pages) != 2 ||
		got.Pages[0].Text != "Offer for the Owner" || got.Pages[0].Image ||
		got.Pages[1].Text != "" || !got.Pages[1].Image {
		t.Errorf("messenger_get_file = %+v; want page 1 as text, and page 2, with none, as an image", got)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %d parts; want the answer, then page 2 as an image", len(result.Content))
	}
	if _, ok := result.Content[1].(*mcp.ImageContent); !ok {
		t.Errorf("content[1] = %+v; want page 2 as an image", result.Content[1])
	}
}

func TestOnlyImagesClaudeCanSeeAreKeptFromFiles(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)

	// An iPhone photo shared as a file is HEIC, which Claude can't see.
	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "", "heic", "IMG_0001.HEIC", "image/heic", 1_500_000))
	// Claude takes images of up to 5 MB.
	n.mustPostUpdate("career", ownerSendsFile(5002, 43, "", "big-png", "Screenshot.png", "image/png", 6_000_000))

	if got := n.talk("career").Messages; len(got) != 0 {
		t.Errorf("Career Talk = %+v; want nothing kept", got)
	}
	var texts []string
	for _, c := range telegram.CallsTo("sendMessage") {
		texts = append(texts, c.Params["text"].(string))
	}
	want := []string{
		"I can read text, photos and PDFs, but not other files, stickers or voice notes yet.",
		"That file is too large: I can read PDFs of up to 10 MB and images of up to 5 MB.",
	}
	if !slices.Equal(texts, want) {
		t.Errorf("the bot said %q; want %q", texts, want)
	}
}

func TestAPDFIsKnownByItsNameWhenItsTypeIsntSaid(t *testing.T) {
	n, _, _ := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsFile(5001, 42, "", "jd", "JD.PDF", "application/octet-stream", 1833))

	got := n.startRun().claim("")
	if len(got.Messages) != 1 || len(got.Messages[0].Files) != 1 || got.Messages[0].Files[0].Name != "JD.PDF" {
		t.Errorf("claim = %+v; want the PDF kept by its name", got.Messages)
	}
}
