package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tokyo is the Owner Time Zone in these tests.
var tokyo = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		panic(err)
	}
	return loc
}()

// at is that wall-clock time on Monday 2026-11-16 in Tokyo, plus days.
func at(days, hour, minute int) time.Time {
	return time.Date(2026, 11, 16+days, hour, minute, 0, 0, tokyo)
}

// syncStatusJSON is what career_sync_status answers.
type syncStatusJSON struct {
	Due     bool         `json:"due"`
	Since   string       `json:"since"`
	Threads []threadJSON `json:"threads"`
}

// threadJSON is one LinkedIn thread Nando knows, in career_sync_status.
type threadJSON struct {
	ThreadID string    `json:"threadId"`
	Newest   time.Time `json:"newest"`
	Ignored  bool      `json:"ignored"`
}

// saveTimeZone sets the Owner Time Zone through Core's settings API.
func (d *browser) saveTimeZone(name string) {
	d.nando.t.Helper()
	d.must(http.MethodPut, "/api/settings", map[string]any{
		"timeZone":        name,
		"heightCm":        nil,
		"weighInReminder": map[string]any{"enabled": false, "weekday": "monday", "time": "08:00"},
	}, http.StatusNoContent)
}

// macAt is the Mac's Sync script at that time: the clock moves there, and the
// script connects anew, as a Connector's access token lasts only an hour.
func (d *browser) macAt(now time.Time) *mcp.ClientSession {
	d.nando.t.Helper()
	d.nando.clock.Set(now)
	return d.connect()
}

func TestNoSyncIsDueBefore7InTheOwnerTimeZone(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")

	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 6, 59)), "career_sync_status", nil); got.Due {
		t.Errorf("at 06:59: due = true, want false")
	}
	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 7, 0)), "career_sync_status", nil); !got.Due {
		t.Errorf("at 07:00: due = false, want true")
	}
}
