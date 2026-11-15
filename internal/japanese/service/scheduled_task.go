package service

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

	"github.com/feriosch/nando-server/internal/japanese/model"
)

// ScheduledTaskService wakes Claude: it fires the API trigger of the cloud
// scheduled task whose runs work through the Word Requests (ADR 0006). It
// serves no route.
type ScheduledTaskService struct {
	scheduledTask            model.ScheduledTaskRepository
	requests                 model.WordRequestsRepository
	triggerURL, triggerToken string
	client                   *http.Client
	now                      func() time.Time
}

// NewScheduledTask returns the ScheduledTaskService firing the trigger at
// triggerURL with triggerToken, recording its fires in scheduledTask,
// reading the Word Requests from requests and the time from now. If
// triggerURL or triggerToken is empty, it never fires; see checkTrigger for
// what a paste can get wrong.
func NewScheduledTask(scheduledTask model.ScheduledTaskRepository, requests model.WordRequestsRepository, triggerURL, triggerToken string, now func() time.Time) *ScheduledTaskService {
	triggerURL, triggerToken = checkTrigger(triggerURL, triggerToken)
	return &ScheduledTaskService{
		scheduledTask: scheduledTask,
		requests:      requests,
		triggerURL:    triggerURL,
		triggerToken:  triggerToken,
		client:        &http.Client{Timeout: 10 * time.Second},
		now:           now,
	}
}

// checkTrigger returns the trigger's URL and token as pasted, without the
// spaces and newlines around them, or both empty if they can't be used. What
// is wrong is logged once, at startup, rather than at the Owner's first ask.
// Neither set is fine: Claude is simply never woken, as in local development.
func checkTrigger(triggerURL, triggerToken string) (string, string) {
	triggerURL, triggerToken = strings.TrimSpace(triggerURL), strings.TrimSpace(triggerToken)
	unusable := func(why string) (string, string) {
		log.Printf("%s, so Claude is never woken: store the scheduled task's API trigger again", why)
		return "", ""
	}
	switch {
	case triggerURL == "" && triggerToken == "":
		return "", ""
	case triggerToken == "":
		return unusable("WORD_REQUESTS_TRIGGER_TOKEN is empty")
	case triggerURL == "":
		return unusable("WORD_REQUESTS_TRIGGER_URL is empty")
	case strings.ContainsFunc(triggerToken, unicode.IsControl):
		return unusable("WORD_REQUESTS_TRIGGER_TOKEN has control characters, such as pasted arrow keys")
	}
	u, err := url.Parse(triggerURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		strings.ContainsFunc(triggerURL, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return unusable("WORD_REQUESTS_TRIGGER_URL isn't a URL")
	}
	return triggerURL, triggerToken
}

// quietFor is how long after a fire the Owner's ask doesn't fire again: a run
// just woken re-lists the queue until it is empty, so it takes what came since.
const quietFor = 2 * time.Minute

// quietAtTickFor is how long after a fire the tick doesn't fire again. A run
// takes a while to start and claim its first Word Request, and the tick runs
// every 5 minutes: with only quietFor, a tick in that while would start a
// second run to share the queue. It also spaces the tick's retries while the
// Owner's usage has run out. The Owner's asks keep quietFor, since a word
// added after a quick run stopped shouldn't wait for nothing.
const quietAtTickFor = 10 * time.Minute

// Wake fires the trigger if a Word Request is waiting for a run: Pending,
// or Working with its claim run out and claims left. It doesn't while a run
// holds a Word Request, as that run lists the rest before it stops, nor if
// the trigger was fired in the last 2 minutes (quietFor). Any error is only
// logged: waking Claude never fails what the Owner asked for.
func (s *ScheduledTaskService) Wake(ctx context.Context) {
	// The Owner may leave Nando the moment they've asked: their request's
	// context ending must not cancel a fire whose 2-minute slot is already
	// taken. The client's timeout still bounds it.
	ctx = context.WithoutCancel(ctx)
	if err := s.wake(ctx, quietFor); err != nil {
		log.Printf("wake the scheduled task: %v", err)
	}
}

// WakeAtTick is Wake for Core's tick, which also wakes the scheduled task for
// Word Requests whose run stopped, such as when the Owner's usage ran out. It
// returns the error of reading the Word Requests or recording the fire, so
// that the tick is retried; a trigger that fails is only logged, as the next
// tick fires again. It doesn't fire within 10 minutes of the last fire
// (quietAtTickFor).
func (s *ScheduledTaskService) WakeAtTick(ctx context.Context) error {
	return s.wake(ctx, quietAtTickFor)
}

// wake fires the trigger if a Word Request is waiting for a run, no run holds
// one, and it wasn't fired in the last quiet.
func (s *ScheduledTaskService) wake(ctx context.Context, quiet time.Duration) error {
	if s.triggerURL == "" || s.triggerToken == "" {
		return nil
	}
	now := s.now()
	// Nothing waiting is the usual answer at a tick, so it is asked first.
	if waiting, err := s.requests.AnyClaimable(ctx, now, maxClaims); err != nil || !waiting {
		return err
	}
	if claimed, err := s.requests.AnyClaimed(ctx, now); err != nil || claimed {
		return err
	}
	if claimed, err := s.scheduledTask.ClaimFire(ctx, now, now.Add(-quiet)); err != nil || !claimed {
		return err
	}
	if err := s.fire(ctx); err != nil {
		log.Printf("fire the scheduled task's trigger: %v", err)
	}
	return nil
}

// fire calls the trigger, which starts a run, and logs the run's session.
func (s *ScheduledTaskService) fire(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.triggerURL, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.triggerToken)
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
