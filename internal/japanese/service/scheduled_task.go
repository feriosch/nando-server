package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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
// triggerURL or triggerToken is empty, it never fires.
func NewScheduledTask(scheduledTask model.ScheduledTaskRepository, requests model.WordRequestsRepository, triggerURL, triggerToken string, now func() time.Time) *ScheduledTaskService {
	return &ScheduledTaskService{
		scheduledTask: scheduledTask,
		requests:      requests,
		triggerURL:    triggerURL,
		triggerToken:  triggerToken,
		client:        &http.Client{Timeout: 10 * time.Second},
		now:           now,
	}
}

// quietFor is how long after a fire Nando doesn't fire again: a run just
// woken re-lists the queue until it is empty, so it takes what came since.
const quietFor = 2 * time.Minute

// Wake fires the trigger if a Word Request is waiting for a run: Pending,
// or Working with its claim run out and claims left. It doesn't while a run
// holds a Word Request, as that run lists the rest before it stops, nor if
// the trigger was fired in the last 2 minutes. Any error is only logged:
// waking Claude never fails what the Owner or a run asked for.
func (s *ScheduledTaskService) Wake(ctx context.Context) {
	// The Owner may leave Nando the moment they've asked: their request's
	// context ending must not cancel a fire whose 2-minute slot is already
	// taken. The client's timeout still bounds it.
	ctx = context.WithoutCancel(ctx)
	if err := s.WakeAtTick(ctx); err != nil {
		log.Printf("wake the scheduled task: %v", err)
	}
}

// WakeAtTick is Wake for Core's tick, which also wakes the scheduled task for
// Word Requests whose run stopped, such as when the Owner's usage ran out. It
// returns the error of reading the Word Requests or recording the fire, so
// that the tick is retried; a trigger that fails is only logged, as the next
// tick fires again.
func (s *ScheduledTaskService) WakeAtTick(ctx context.Context) error {
	if s.triggerURL == "" || s.triggerToken == "" {
		return nil
	}
	now := s.now()
	if claimed, err := s.requests.AnyClaimed(ctx, now); err != nil || claimed {
		return err
	}
	if waiting, err := s.requests.AnyClaimable(ctx, now, maxClaims); err != nil || !waiting {
		return err
	}
	if claimed, err := s.scheduledTask.ClaimFire(ctx, now, now.Add(-quietFor)); err != nil || !claimed {
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
