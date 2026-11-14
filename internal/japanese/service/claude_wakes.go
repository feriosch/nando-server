package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/japanese/model"
)

// ClaudeWakesService wakes Claude to work through the Word Requests, by
// firing the API trigger of the cloud scheduled task that does it (ADR 0006).
type ClaudeWakesService struct {
	wakes                    model.ClaudeWakesRepository
	triggerURL, triggerToken string
	client                   *http.Client
	now                      func() time.Time
}

// fireTimeout is how long a fire may take: the Owner's request waits on it.
const fireTimeout = 10 * time.Second

// wakeEvery is how long after waking Claude Nando waits before waking it
// again: a run already woken takes every Word Request asked for meanwhile.
const wakeEvery = 2 * time.Minute

// NewClaudeWakes returns the ClaudeWakesService firing the trigger at
// triggerURL with triggerToken, remembering in wakes when it last did, by
// the clock now. If triggerURL or triggerToken is empty, it never wakes
// Claude.
func NewClaudeWakes(wakes model.ClaudeWakesRepository, triggerURL, triggerToken string, now func() time.Time) *ClaudeWakesService {
	return &ClaudeWakesService{
		wakes:        wakes,
		triggerURL:   triggerURL,
		triggerToken: triggerToken,
		client:       &http.Client{Timeout: fireTimeout},
		now:          now,
	}
}

// Wake wakes Claude. A trigger that fails is only logged: waking Claude
// never fails what the Owner or a run asked for.
func (s *ClaudeWakesService) Wake(ctx context.Context) {
	if s.triggerURL == "" || s.triggerToken == "" {
		return
	}
	now := s.now()
	claimed, err := s.wakes.Claim(ctx, now, now.Add(-wakeEvery))
	if err != nil {
		log.Printf("wake Claude: %v", err)
		return
	}
	if !claimed {
		return
	}
	if err := s.fire(ctx); err != nil {
		log.Printf("wake Claude: %v", err)
	}
}

// fire calls the scheduled task's API trigger.
func (s *ClaudeWakesService) fire(ctx context.Context) error {
	// The Owner's request may end first; the fire still gets its own time.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fireTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.triggerURL, bytes.NewReader([]byte("{}")))
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the trigger answered %s: %.200s", resp.Status, body)
	}
	var fired struct {
		SessionURL string `json:"claude_code_session_url"`
	}
	if json.Unmarshal(body, &fired) == nil && fired.SessionURL != "" {
		log.Printf("woke Claude: %s", fired.SessionURL)
	}
	return nil
}
