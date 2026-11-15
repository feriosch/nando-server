package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Trigger is the API trigger of a cloud scheduled task (ADR 0006): its URL
// and bearer token, as pasted from claude.ai.
type Trigger struct {
	URL, Token string
	// Env is the start of the environment variables they come from, such
	// as WORD_REQUESTS_TRIGGER for WORD_REQUESTS_TRIGGER_URL and
	// WORD_REQUESTS_TRIGGER_TOKEN, to say which one is wrong.
	Env string
}

// Queue is the work a Module's scheduled task works through, such as Word
// Requests.
type Queue interface {
	// AnyClaimable reports whether some work waits for a run at now: never
	// claimed, or with its claim lapsed and claims left.
	AnyClaimable(ctx context.Context, now time.Time) (bool, error)
	// AnyClaimed reports whether a run holds some work at now.
	AnyClaimed(ctx context.Context, now time.Time) (bool, error)
}

// FireRecord is a Module's scheduled_task collection: when Nando last woke
// the Module's scheduled task, and when a run was last active. It is in the
// database, not in memory, as every instance of Nando wakes the same
// scheduled task. Its one document has the _id LastFireID, and its claim is
// ClaimFireFilter's.
type FireRecord interface {
	// ClaimFire records a fire at now and reports whether it did. It
	// doesn't while a fire is in flight, one after startingSince that no
	// run has worked or stopped since, nor while a run may be between
	// pieces of work, having worked after activeSince and not stopped
	// since. One write decides, so of instances claiming at once only one
	// gets it.
	ClaimFire(ctx context.Context, now, startingSince, activeSince time.Time) (bool, error)
	// ReleaseFire forgets the fire at firedAt, which started no run, so it
	// isn't in flight. A later fire is left as it is.
	ReleaseFire(ctx context.Context, firedAt time.Time) error
	// RecordWorking records that a run claimed or finished some work at now.
	RecordWorking(ctx context.Context, now time.Time) error
	// RecordStopped records that a run found nothing left to claim at now,
	// so it stops.
	RecordStopped(ctx context.Context, now time.Time) error
}

// ScheduledTask wakes Claude for one Module: it fires the API trigger of the
// cloud scheduled task whose runs work through the Module's Queue (ADR
// 0006). Each Module has its own, recording its fires in its own
// scheduled_task collection, under the same rules.
type ScheduledTask struct {
	fires   FireRecord
	queue   Queue
	trigger Trigger
	client  *http.Client
	now     func() time.Time
}

// NewScheduledTask returns the ScheduledTask firing trigger while queue has
// work waiting, recording its fires in fires and reading the time from now.
// If trigger's URL or token is empty, it never fires; see checkTrigger for
// what a paste can get wrong.
func NewScheduledTask(fires FireRecord, queue Queue, trigger Trigger, now func() time.Time) *ScheduledTask {
	trigger.URL, trigger.Token = checkTrigger(trigger)
	return &ScheduledTask{
		fires:   fires,
		queue:   queue,
		trigger: trigger,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     now,
	}
}

// checkTrigger returns the trigger's URL and token as pasted, without the
// spaces and newlines around them, or both empty if they can't be used. What
// is wrong is logged once, at startup, rather than at the Owner's first ask.
// Neither set is fine: Claude is simply never woken, as in local development.
func checkTrigger(trigger Trigger) (string, string) {
	triggerURL, triggerToken := strings.TrimSpace(trigger.URL), strings.TrimSpace(trigger.Token)
	unusable := func(why string) (string, string) {
		log.Printf("%s, so Claude is never woken: store the scheduled task's API trigger again", why)
		return "", ""
	}
	switch {
	case triggerURL == "" && triggerToken == "":
		return "", ""
	case triggerToken == "":
		return unusable(trigger.Env + "_TOKEN is empty")
	case triggerURL == "":
		return unusable(trigger.Env + "_URL is empty")
	case strings.ContainsFunc(triggerToken, unicode.IsControl):
		return unusable(trigger.Env + "_TOKEN has control characters, such as pasted arrow keys")
	}
	u, err := url.Parse(triggerURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		strings.ContainsFunc(triggerURL, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return unusable(trigger.Env + "_URL isn't a URL")
	}
	return triggerURL, triggerToken
}

// startingFor is how long a fire is in flight while its run hasn't claimed or
// finished any work yet: a run takes a while to start, and it lists the
// queue when it does, so a second fire meanwhile would only start a second
// run to share it. A run that never starts, such as when the Owner's usage
// has run out, stops blocking after it.
const startingFor = 10 * time.Minute

// betweenWorkFor is how long after a run claimed or finished some work it may
// still be working: it lists the queue again after finishing, so it takes
// work added meanwhile.
const betweenWorkFor = 2 * time.Minute

// Wake fires the trigger if work is waiting for a run. It doesn't while a run
// holds some, as that run lists the rest before it stops, nor while a fire is
// in flight (startingFor) or a run was active in the last 2 minutes
// (betweenWorkFor). Any error is only logged: waking Claude never fails what
// the Owner, or the Mac, asked for.
func (s *ScheduledTask) Wake(ctx context.Context) {
	// The caller may leave the moment it has asked: its request's context
	// ending must not cancel a fire whose claim is already taken. The
	// client's timeout still bounds it.
	ctx = context.WithoutCancel(ctx)
	if err := s.WakeAtTick(ctx); err != nil {
		log.Printf("wake the scheduled task: %v", err)
	}
}

// WakeAtTick is Wake for Core's tick, which also wakes the scheduled task for
// work whose run stopped, such as when the Owner's usage ran out. It returns
// the error of reading the queue or recording the fire, so that the tick is
// retried; a trigger that fails is only logged, as the next tick fires again.
func (s *ScheduledTask) WakeAtTick(ctx context.Context) error {
	if s.trigger.URL == "" || s.trigger.Token == "" {
		return nil
	}
	now := s.now()
	// Nothing waiting is the usual answer at a tick, so it is asked first.
	if waiting, err := s.queue.AnyClaimable(ctx, now); err != nil || !waiting {
		return err
	}
	if claimed, err := s.queue.AnyClaimed(ctx, now); err != nil || claimed {
		return err
	}
	claimed, err := s.fires.ClaimFire(ctx, now, now.Add(-startingFor), now.Add(-betweenWorkFor))
	if err != nil || !claimed {
		return err
	}
	if err := s.fire(ctx); err != nil {
		log.Printf("fire the scheduled task's trigger: %v", err)
		// No run is coming, so the fire isn't in flight: the next ask or
		// tick tries again.
		return s.fires.ReleaseFire(ctx, now)
	}
	return nil
}

// RunWorking records that a run claimed or finished some work now: its fire
// is no longer in flight, and it may list the queue again. It only logs an
// error: recording it never fails what the run did.
func (s *ScheduledTask) RunWorking(ctx context.Context) {
	if err := s.fires.RecordWorking(ctx, s.now()); err != nil {
		log.Printf("record that a run is working: %v", err)
	}
}

// RunStopped records that a run found nothing left to claim now, so it stops:
// its fire is no longer in flight, and no run is between pieces of work. It
// only logs an error.
func (s *ScheduledTask) RunStopped(ctx context.Context) {
	if err := s.fires.RecordStopped(ctx, s.now()); err != nil {
		log.Printf("record that a run stopped: %v", err)
	}
}

// fire calls the trigger, which starts a run, and logs the run's session.
func (s *ScheduledTask) fire(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.trigger.URL, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.trigger.Token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "experimental-cc-routine-2026-04-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		// 429 is the trigger's limit, 30 fires an hour.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: %s", resp.Status, body)
	}
	var fired struct {
		SessionURL string `json:"claude_code_session_url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&fired)
	if fired.SessionURL != "" {
		log.Printf("woke the scheduled task: %s", fired.SessionURL)
	}
	return nil
}

// LastFireID is the _id of the one document in a Module's scheduled_task
// collection: the last fire (firedAt), when a run last claimed or finished
// some work (activeAt), and when one last found nothing left and stopped
// (endedAt).
const LastFireID = "last_fire"

// Never stands in for a time a scheduled_task document hasn't recorded yet.
var Never = time.Unix(0, 0).UTC()

// ClaimFireFilter matches the scheduled_task document while a fire may be
// claimed: no fire is in flight, one after startingSince that no run has
// worked or stopped since, and no run may be between pieces of work, having
// worked after activeSince and not stopped since. A FireRecord's ClaimFire
// sets firedAt on the document it matches, or inserts the first document if
// there is none.
func ClaimFireFilter(startingSince, activeSince time.Time) bson.D {
	firedAt := bson.D{{Key: "$ifNull", Value: bson.A{"$firedAt", Never}}}
	activeAt := bson.D{{Key: "$ifNull", Value: bson.A{"$activeAt", Never}}}
	endedAt := bson.D{{Key: "$ifNull", Value: bson.A{"$endedAt", Never}}}
	return bson.D{
		{Key: "_id", Value: LastFireID},
		{Key: "$expr", Value: bson.D{{Key: "$and", Value: bson.A{
			// No fire in flight: the last one is old, or a run worked or
			// stopped since it.
			bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "$lte", Value: bson.A{firedAt, startingSince}}},
				bson.D{{Key: "$gte", Value: bson.A{activeAt, firedAt}}},
				bson.D{{Key: "$gte", Value: bson.A{endedAt, firedAt}}},
			}}},
			// No run between pieces of work: none worked lately, or it
			// stopped since.
			bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "$lte", Value: bson.A{activeAt, activeSince}}},
				bson.D{{Key: "$gte", Value: bson.A{endedAt, activeAt}}},
			}}},
		}}}},
	}
}
