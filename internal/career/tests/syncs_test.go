package tests

import (
	"context"
	"fmt"
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
	// ReadThrough is whether the newest Sync read every thread it had to.
	ReadThrough bool `json:"readThrough"`
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

// recordedJSON is what career_record_sync answers.
type recordedJSON struct {
	Sync   string `json:"sync"`
	Stored int    `json:"stored"`
}

// tanakaThread is a LinkedIn thread with a Recruiter, as the Mac's script
// sends it: two messages, the Owner's reply the newer.
func tanakaThread(messages ...map[string]any) map[string]any {
	return map[string]any{
		"threadId": "2-tanaka",
		"url":      "https://www.linkedin.com/messaging/thread/2-tanaka/",
		"person":   map[string]any{"name": "Tanaka Yui", "profileUrl": "https://www.linkedin.com/in/tanaka-yui/"},
		"messages": messages,
	}
}

func message(id, from string, sentAt time.Time, text string) map[string]any {
	return map[string]any{"id": id, "from": from, "sentAt": sentAt.Format(time.RFC3339), "text": text}
}

func TestASyncRecordsEachThreadsNewestMessage(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	pitched := at(-3, 10, 15)
	replied := at(-2, 21, 40)

	got := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": []any{tanakaThread(
		message("m1", "other", pitched, "Hello! I have a backend role at a fintech in Tokyo."),
		message("m2", "owner", replied, "Thank you, could you share the job description?"),
	)}})
	if got.Sync == "" || got.Stored != 2 {
		t.Errorf("career_record_sync = %+v, want a sync id and 2 stored", got)
	}

	status := callTool[syncStatusJSON](t, mac, "career_sync_status", nil)
	if status.Since != "2025-01-01" {
		t.Errorf("since = %q, want 2025-01-01", status.Since)
	}
	want := []threadJSON{{ThreadID: "2-tanaka", Newest: replied}}
	if len(status.Threads) != 1 || status.Threads[0].ThreadID != want[0].ThreadID ||
		!status.Threads[0].Newest.Equal(replied) || status.Threads[0].Ignored {
		t.Errorf("threads = %+v, want %+v", status.Threads, want)
	}
}

func TestMessagesASyncAlreadySentAreNotStoredAgain(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	pitch := message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.")
	// The reader gave no id for this one, so it is told apart by sender, time and text.
	reply := message("", "owner", at(-2, 21, 40), "Thank you, could you share the job description?")
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": []any{tanakaThread(pitch, reply)}})

	later := phone.macAt(at(0, 20, 0))
	jd := message("m3", "other", at(0, 12, 5), "Of course, here it is.")
	got := callTool[recordedJSON](t, later, "career_record_sync", map[string]any{"threads": []any{tanakaThread(pitch, reply, jd)}})

	if got.Stored != 1 {
		t.Errorf("stored = %d, want 1: only the new message", got.Stored)
	}
}

// recordSyncAt has the Mac record a Sync at that time, with a new message from
// Tanaka-san sent a minute before.
func (d *browser) recordSyncAt(now time.Time) {
	d.nando.t.Helper()
	sent := message("", "other", now.Add(-time.Minute), "Following up at "+now.Format(time.Kitchen))
	callTool[recordedJSON](d.nando.t, d.macAt(now), "career_record_sync", map[string]any{"threads": []any{tanakaThread(sent)}})
}

func TestAtMostTwoSyncsAreDueOnADay(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	phone.recordSyncAt(at(0, 7, 30))
	phone.recordSyncAt(at(0, 14, 0))

	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 21, 0)), "career_sync_status", nil); got.Due {
		t.Errorf("after 2 Syncs today: due = true, want false")
	}
	if got := callTool[syncStatusJSON](t, phone.macAt(at(1, 7, 0)), "career_sync_status", nil); !got.Due {
		t.Errorf("the next Day at 07:00: due = false, want true")
	}
}

func TestASyncIsDueOnly6HoursAfterTheLast(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	phone.recordSyncAt(at(0, 8, 0))

	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 13, 59)), "career_sync_status", nil); got.Due {
		t.Errorf("5 h 59 m after the last Sync: due = true, want false")
	}
	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 14, 0)), "career_sync_status", nil); !got.Due {
		t.Errorf("6 h after the last Sync: due = false, want true")
	}
}

// recruiterThread is a LinkedIn thread with one new message from a Recruiter.
func recruiterThread(n int, sent time.Time) map[string]any {
	id := fmt.Sprintf("2-recruiter-%d", n)
	return map[string]any{
		"threadId": id,
		"url":      "https://www.linkedin.com/messaging/thread/" + id + "/",
		"person":   map[string]any{"name": fmt.Sprintf("Recruiter %d", n)},
		"messages": []any{message("", "other", sent, "Hello! I have a role for you.")},
	}
}

// batch is threads first to first+n-1, as one career_record_sync call sends them.
func batch(first, n int, sent time.Time) []any {
	threads := []any{}
	for i := first; i < first+n; i++ {
		threads = append(threads, recruiterThread(i, sent))
	}
	return threads
}

func TestASyncSentInBatchesCountsOnce(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	start := at(0, 8, 0)
	sent := at(-1, 18, 0)

	mac := phone.macAt(start)
	first := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": batch(0, 10, sent)})
	phone.nando.clock.Set(start.Add(4 * time.Second))
	second := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"sync": first.Sync, "threads": batch(10, 10, sent)})
	phone.nando.clock.Set(start.Add(8 * time.Second))
	third := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"sync": first.Sync, "threads": batch(20, 5, sent)})

	if second.Sync != first.Sync || third.Sync != first.Sync {
		t.Errorf("syncs = %s, %s, %s; want later batches added to the first", first.Sync, second.Sync, third.Sync)
	}
	if first.Stored+second.Stored+third.Stored != 25 {
		t.Errorf("stored = %d + %d + %d, want 25 in all", first.Stored, second.Stored, third.Stored)
	}
	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 13, 59)), "career_sync_status", nil); got.Due {
		t.Errorf("5 h 59 m after a Sync sent in 3 batches: due = true, want false")
	}
	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 14, 0)), "career_sync_status", nil); !got.Due {
		t.Errorf("6 h after a Sync sent in 3 batches: due = false, want true: it is the Day's first Sync")
	}
}

func TestABatchIsAddedOnlyToASyncStartedMinutesAgo(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	start := at(0, 8, 0)
	mac := phone.macAt(start)
	first := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": batch(0, 10, at(-1, 18, 0))})

	phone.nando.clock.Set(start.Add(10 * time.Minute))
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"sync": first.Sync, "threads": batch(10, 1, at(-1, 18, 0))})

	// A later run that reuses the id must start a Sync of its own, or it
	// would never count towards when the next is due.
	later := phone.macAt(at(0, 14, 0))
	pitch := message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.")
	toolFails(t, later, "career_record_sync", map[string]any{"sync": first.Sync, "threads": []any{tanakaThread(pitch)}})

	status := callTool[syncStatusJSON](t, later, "career_sync_status", nil)
	for _, thread := range status.Threads {
		if thread.ThreadID == "2-tanaka" {
			t.Errorf("a batch for a Sync started 6 hours ago stored Tanaka-san's thread")
		}
	}
}

func TestABatchIsAddedOnlyToASyncThatReadLinkedIn(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	failed := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"problem": map[string]any{"kind": "session_expired"}})
	read := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": batch(0, 1, at(-1, 18, 0))})
	pitch := message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.")

	for _, args := range []map[string]any{
		{"sync": "not-a-sync", "threads": []any{tanakaThread(pitch)}},
		{"sync": "0123456789abcdef01234567", "threads": []any{tanakaThread(pitch)}},
		{"sync": failed.Sync, "threads": []any{tanakaThread(pitch)}},
		{"sync": read.Sync, "problem": map[string]any{"kind": "other", "text": "the reader quit"}},
	} {
		toolFails(t, mac, "career_record_sync", args)
	}

	status := callTool[syncStatusJSON](t, mac, "career_sync_status", nil)
	for _, thread := range status.Threads {
		if thread.ThreadID == "2-tanaka" {
			t.Errorf("a refused batch stored Tanaka-san's thread")
		}
	}
}

func TestASyncThatOnlyReportsAProblemDoesNotCount(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")

	for _, now := range []time.Time{at(0, 8, 0), at(0, 9, 0)} {
		got := callTool[recordedJSON](t, phone.macAt(now), "career_record_sync", map[string]any{"problem": map[string]any{"kind": "session_expired"}})
		if got.Sync == "" || got.Stored != 0 {
			t.Errorf("career_record_sync with a problem = %+v, want a sync id and 0 stored", got)
		}
	}

	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 9, 1)), "career_sync_status", nil); !got.Due {
		t.Errorf("after Syncs that only reported a problem: due = false, want true")
	}
}

// toolFails calls the Tool name with args and fails the test unless the Tool
// refuses it.
func toolFails(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !result.IsError {
		t.Errorf("%s %v succeeded, want it refused", name, args)
	}
}

func TestASyncThatIsRefusedStoresNothing(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	pitch := message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.")

	for _, args := range []map[string]any{
		{"threads": []any{tanakaThread(pitch)}, "problem": map[string]any{"kind": "timeout"}},
		{"threads": []any{map[string]any{"threadId": "", "messages": []any{pitch}}}},
		{"threads": []any{tanakaThread(message("m1", "recruiter", at(-3, 10, 15), "Hello!"))}},
		{"threads": []any{tanakaThread(map[string]any{"id": "m1", "from": "other", "text": "Hello!"})}},
	} {
		toolFails(t, mac, "career_record_sync", args)
	}

	status := callTool[syncStatusJSON](t, mac, "career_sync_status", nil)
	if len(status.Threads) != 0 || !status.Due {
		t.Errorf("after refused Syncs: threads = %+v, due = %v; want none, and due", status.Threads, status.Due)
	}
}

func TestAMessageSentTwiceInOneSyncIsStoredOnce(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	pitch := message("m1", "other", at(-3, 10, 15), "Hello! I have a backend role at a fintech in Tokyo.")

	got := callTool[recordedJSON](t, phone.macAt(at(0, 8, 0)), "career_record_sync", map[string]any{"threads": []any{tanakaThread(pitch, pitch)}})

	if got.Stored != 1 {
		t.Errorf("stored = %d, want 1", got.Stored)
	}
}

func TestAfterASyncThatStoppedEarlyTheNextIsToldItDidntReadThrough(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	sent := at(-1, 18, 0)

	stopped := phone.macAt(at(0, 8, 0))
	callTool[recordedJSON](t, stopped, "career_record_sync", map[string]any{"threads": batch(0, 10, sent), "stoppedEarly": true})
	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 14, 0)), "career_sync_status", nil); got.ReadThrough {
		t.Errorf("after a Sync that stopped early: readThrough = true, want false")
	}

	callTool[recordedJSON](t, phone.macAt(at(0, 14, 0)), "career_record_sync", map[string]any{"threads": []any{}})
	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 20, 0)), "career_sync_status", nil); !got.ReadThrough {
		t.Errorf("after a Sync that read through: readThrough = false, want true")
	}
}

func TestStoppedEarlyIsSentOnlyWithASyncsFirstBatch(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	first := callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": batch(0, 10, at(-1, 18, 0))})

	toolFails(t, mac, "career_record_sync", map[string]any{"sync": first.Sync, "threads": batch(10, 1, at(-1, 18, 0)), "stoppedEarly": true})
	// A problem says so already, and it is the batches before it that are marked.
	toolFails(t, mac, "career_record_sync", map[string]any{"problem": map[string]any{"kind": "other", "text": "rate limited"}, "stoppedEarly": true})
}

func TestAProblemAfterTheBatchesItCutShortLeavesThemStoppedEarly(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	mac := phone.macAt(at(0, 8, 0))
	// The script marks what it read, then sends the problem that stopped it.
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"threads": batch(0, 1, at(-1, 18, 0)), "stoppedEarly": true})
	callTool[recordedJSON](t, mac, "career_record_sync", map[string]any{"problem": map[string]any{"kind": "other", "text": "rate limited"}})

	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 9, 0)), "career_sync_status", nil); got.ReadThrough {
		t.Errorf("after a Sync that stopped at a problem: readThrough = true, want false")
	}
}

func TestASyncThatOnlyReportsAProblemLeavesReadThroughAsItWas(t *testing.T) {
	_, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	// It read nothing, so every thread Nando holds is as the last Sync left it.
	callTool[recordedJSON](t, phone.macAt(at(0, 8, 0)), "career_record_sync", map[string]any{"threads": batch(0, 1, at(-1, 18, 0))})
	callTool[recordedJSON](t, phone.macAt(at(0, 14, 0)), "career_record_sync", map[string]any{"problem": map[string]any{"kind": "session_expired"}})

	if got := callTool[syncStatusJSON](t, phone.macAt(at(0, 14, 15)), "career_sync_status", nil); !got.ReadThrough {
		t.Errorf("after a read-through Sync and a Sync that only reported a problem: readThrough = false, want true")
	}
}
