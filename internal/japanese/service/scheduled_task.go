package service

import (
	"context"
	"encoding/json"
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
	triggerURL, triggerToken string
	client                   *http.Client
	now                      func() time.Time
}

// NewScheduledTask returns the ScheduledTaskService firing the trigger at
// triggerURL with triggerToken, recording its fires in scheduledTask and
// reading the time from now. If triggerURL or triggerToken is empty, it
// never fires.
func NewScheduledTask(scheduledTask model.ScheduledTaskRepository, triggerURL, triggerToken string, now func() time.Time) *ScheduledTaskService {
	return &ScheduledTaskService{
		scheduledTask: scheduledTask,
		triggerURL:    triggerURL,
		triggerToken:  triggerToken,
		client:        &http.Client{Timeout: 10 * time.Second},
		now:           now,
	}
}

// quietFor is how long after a fire Nando doesn't fire again: a run just
// woken re-lists the queue until it is empty, so it takes what came since.
const quietFor = 2 * time.Minute

// Wake fires the trigger, unless it was fired in the last 2 minutes. A
// trigger that fails, or a fire that can't be recorded, is only logged:
// waking Claude never fails what the Owner or a run asked for.
func (s *ScheduledTaskService) Wake(ctx context.Context) {
	if s.triggerURL == "" || s.triggerToken == "" {
		return
	}
	now := s.now()
	claimed, err := s.scheduledTask.ClaimFire(ctx, now, now.Add(-quietFor))
	if err == nil && claimed {
		err = s.fire(ctx)
	}
	if err != nil {
		log.Printf("wake the scheduled task: %v", err)
	}
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
	var fired struct {
		SessionURL string `json:"claude_code_session_url"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&fired)
	if fired.SessionURL != "" {
		log.Printf("woke the scheduled task: %s", fired.SessionURL)
	}
	return nil
}
