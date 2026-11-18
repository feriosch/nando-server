package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// MessengerConfig is how Nando reaches the Owner in Telegram (ADR 0008).
type MessengerConfig struct {
	// TelegramURL is the Bot API's base URL; empty means
	// https://api.telegram.org.
	TelegramURL string
	// OwnerID is the Owner's Telegram user ID. Every other sender is ignored.
	OwnerID int64
	// WebhookSecret is the secret_token every bot's webhook is set with, which
	// Telegram sends back in each call.
	WebhookSecret string
	// BotTokens is each Assistant's bot token, by Assistant name. An Assistant
	// without one is off.
	BotTokens map[string]string
	// TriggerURL and TriggerToken are the API trigger of the cloud scheduled
	// task whose runs answer the Owner, and its bearer token. Without both,
	// Claude is never woken.
	TriggerURL, TriggerToken string
}

// updateMemory is how long Nando remembers each update a bot had. Telegram
// re-delivers an update for up to a day while it isn't answered 200.
const updateMemory = 7 * 24 * time.Hour

// textOnly is what a bot answers a message with no text.
const textOnly = "I can only read text for now."

// MessengerService receives the Owner's messages from Telegram and keeps them
// in Talks.
type MessengerService struct {
	// assistants is every Assistant's name, the Core Assistant's included.
	assistants map[string]bool
	talks      model.TalksRepository
	// fires is the fire record of an Assistant's runs.
	fires func(assistant string) utils.FireRecord
	// tasks wakes each Assistant's runs.
	tasks map[string]*utils.ScheduledTask
	// apiURL is the API's own URL, which Telegram reaches the webhooks at.
	apiURL   string
	cfg      MessengerConfig
	telegram telegramClient
	now      func() time.Time
}

// NewMessenger returns the MessengerService, with now as its clock.
// Telegram reaches the webhooks under apiURL, the API's own URL.
// fires gives each Assistant's fire record.
func NewMessenger(talks model.TalksRepository, fires func(assistant string) utils.FireRecord, cfg MessengerConfig, apiURL string, now func() time.Time) *MessengerService {
	baseURL := cfg.TelegramURL
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	s := &MessengerService{
		assistants: map[string]bool{},
		talks:      talks,
		fires:      fires,
		tasks:      map[string]*utils.ScheduledTask{},
		apiURL:     apiURL,
		cfg:        cfg,
		telegram:   telegramClient{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}},
		now:        now,
	}
	s.AddAssistant(model.Assistant{Name: model.CoreAssistant})
	return s
}

// Routes adds each bot's webhook to mux, under /messenger/telegram/.
func (s *MessengerService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /messenger/telegram/{assistant}", s.postUpdate)
}

// AddAssistant has the Messenger receive the Owner's messages to a. Each
// Module with an Assistant adds it once, at startup, before serving.
func (s *MessengerService) AddAssistant(a model.Assistant) {
	s.assistants[a.Name] = true
	s.tasks[a.Name] = utils.NewScheduledTask(s.fires(a.Name), talkQueue{talks: s.talks, assistant: a.Name},
		utils.Trigger{URL: s.cfg.TriggerURL, Token: s.cfg.TriggerToken, Env: "MESSENGER_TRIGGER"}, s.now)
}

// talkQueue is one Assistant's waiting messages, as the work its runs do.
type talkQueue struct {
	talks     model.TalksRepository
	assistant string
}

func (q talkQueue) AnyClaimable(ctx context.Context, now time.Time) (bool, error) {
	talk, err := q.talks.Open(ctx, q.assistant)
	return err == nil && talk.Claimable(now), err
}

func (q talkQueue) AnyClaimed(ctx context.Context, now time.Time) (bool, error) {
	talk, err := q.talks.Open(ctx, q.assistant)
	return err == nil && talk.ClaimedUntil.After(now), err
}

// bot is the token of assistant's bot, and false if assistant has none, so
// is off.
func (s *MessengerService) bot(assistant string) (string, bool) {
	token := s.cfg.BotTokens[assistant]
	return token, s.assistants[assistant] && token != ""
}

// RegisterWebhooks points every Assistant's bot at its webhook, with the
// webhook secret, for messages only. Telegram keeps the updates still waiting
// for the bot. It sets each one every time, since Telegram never says which
// secret a webhook has. A bot Telegram refuses is reported, and the others are
// still set.
func (s *MessengerService) RegisterWebhooks(ctx context.Context) error {
	var errs []error
	for _, assistant := range slices.Sorted(maps.Keys(s.assistants)) {
		token, ok := s.bot(assistant)
		if !ok {
			continue
		}
		if err := s.telegram.call(ctx, token, "setWebhook", map[string]any{
			"url":             s.apiURL + "/messenger/telegram/" + assistant,
			"secret_token":    s.cfg.WebhookSecret,
			"allowed_updates": []string{"message"},
		}); err != nil {
			errs = append(errs, fmt.Errorf("%s's bot: %w", assistant, err))
		}
	}
	return errors.Join(errs...)
}

// Talk is assistant's open Talk.
func (s *MessengerService) Talk(ctx context.Context, assistant string) (model.Talk, error) {
	return s.talks.Open(ctx, assistant)
}

// update is the part of a Telegram update Nando reads.
type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		From *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Text string `json:"text"`
		// Photo is one photo in the sizes Telegram made of it, smallest first.
		Photo []struct {
			FileID string `json:"file_id"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"photo"`
		// Caption is a photo's text.
		Caption string `json:"caption"`
	} `json:"message"`
}

// maxPhotoSide is the longest side, in pixels, of the photo size Nando keeps:
// enough to read a sign or a screenshot, at about 1,500 tokens.
const maxPhotoSide = 1280

// photo is the size of u's photo that Nando keeps: the largest no longer than
// maxPhotoSide on its long side, or the smallest if all are longer. It is
// false if u has no photo.
func (u update) photo() (model.Photo, bool) {
	sizes := u.Message.Photo
	if len(sizes) == 0 {
		return model.Photo{}, false
	}
	best := sizes[0]
	for _, size := range sizes {
		if max(size.Width, size.Height) <= maxPhotoSide && size.Width*size.Height > best.Width*best.Height {
			best = size
		}
	}
	return model.Photo{ID: best.FileID, Width: best.Width, Height: best.Height}, true
}

// postUpdate takes one update from assistant's bot. It always answers 200
// once the update is dealt with, since Telegram re-delivers anything else.
func (s *MessengerService) postUpdate(w http.ResponseWriter, r *http.Request) {
	// The secret first, so that no one without it learns which bots exist.
	secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if s.cfg.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(s.cfg.WebhookSecret)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	assistant := r.PathValue("assistant")
	token, ok := s.bot(assistant)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var u update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil || u.Message == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	m := u.Message
	if !s.fromOwner(u) || isStart(m.Text) {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Telegram may give up on the request midway: once an update is marked,
	// it must be kept or unmarked, or its re-delivery would be dropped.
	ctx := context.WithoutCancel(r.Context())
	isNew, err := s.talks.MarkUpdate(ctx, assistant, u.UpdateID, s.now().Add(updateMemory))
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	if !isNew {
		w.WriteHeader(http.StatusOK)
		return
	}
	text, photos := m.Text, []model.Photo(nil)
	if p, ok := u.photo(); ok {
		text, photos = m.Caption, []model.Photo{p}
	}
	if text == "" && len(photos) == 0 {
		// A sticker, voice note, file or anything else without text.
		if err := s.telegram.call(ctx, token, "sendMessage", map[string]any{
			"chat_id": m.Chat.ID,
			"text":    textOnly,
		}); err != nil {
			log.Printf("messenger: tell the Owner %s reads only text: %v", assistant, err)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.talks.AddOwnerMessage(ctx, assistant, model.TalkMessage{
		From:              model.FromOwner,
		At:                s.now(),
		Text:              text,
		Photos:            photos,
		TelegramMessageID: m.MessageID,
		State:             model.MessageWaiting,
	}); err != nil {
		// Forget the update, so that Telegram's re-delivery is kept.
		if err := s.talks.UnmarkUpdate(ctx, assistant, u.UpdateID); err != nil {
			log.Printf("messenger: forget update %d to %s: %v", u.UpdateID, assistant, err)
		}
		utils.ServerError(w, err)
		return
	}
	s.received(ctx, token, m.Chat.ID, m.MessageID)
	s.tasks[assistant].Wake(ctx)
	w.WriteHeader(http.StatusOK)
}

// fromOwner reports whether u is a message from the Owner, in their private
// chat with the bot.
func (s *MessengerService) fromOwner(u update) bool {
	m := u.Message
	return m.Chat.Type == "private" && m.Chat.ID == s.cfg.OwnerID && m.From != nil && m.From.ID == s.cfg.OwnerID
}

// isStart reports whether text is Telegram's /start, which the Owner sends
// once to open each bot's chat.
func isStart(text string) bool {
	return text == "/start" || strings.HasPrefix(text, "/start ")
}

// received shows the Owner that their message arrived: a 👀 reaction on it,
// and the typing status. Neither is worth failing for.
// Both are asked for at once, so a slow Telegram holds the webhook's answer
// no longer than one call.
func (s *MessengerService) received(ctx context.Context, token string, chatID, messageID int64) {
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := s.telegram.call(ctx, token, "setMessageReaction", map[string]any{
			"chat_id":    chatID,
			"message_id": messageID,
			"reaction":   []map[string]string{{"type": "emoji", "emoji": "👀"}},
		}); err != nil {
			log.Printf("messenger: react to message %d: %v", messageID, err)
		}
	})
	wg.Go(func() { s.typing(ctx, token) })
	wg.Wait()
}

// telegramClient calls the Telegram Bot API.
type telegramClient struct {
	baseURL string
	http    *http.Client
}

// maxPhotoBytes is the most of a photo Nando downloads. A photo of up to
// maxPhotoSide pixels is far smaller.
const maxPhotoBytes = 5 << 20

// download downloads the file with fileID as the bot with token. The bot
// token never appears in an error.
func (c telegramClient) download(ctx context.Context, token, fileID string) ([]byte, error) {
	var file struct {
		FilePath string `json:"file_path"`
	}
	if err := c.callFor(ctx, token, "getFile", map[string]any{"file_id": fileID}, &file); err != nil {
		return nil, err
	}
	if file.FilePath == "" {
		return nil, errors.New("getFile: Telegram gave no file path")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/file/bot"+token+"/"+file.FilePath, nil)
	if err != nil {
		return nil, errors.New("download: the bot token or the API URL isn't usable")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL holds the bot's token, and *url.Error repeats it.
		return nil, errors.New("download: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxPhotoBytes+1))
	if err != nil {
		return nil, errors.New("download: reading the file failed")
	}
	if len(data) > maxPhotoBytes {
		return nil, errors.New("download: the file is too large")
	}
	return data, nil
}

// call calls method as the bot with token, with params as its JSON body.
func (c telegramClient) call(ctx context.Context, token, method string, params any) error {
	return c.callFor(ctx, token, method, params, nil)
}

// callFor is call, decoding the call's result into result if it isn't nil.
func (c telegramClient) callFor(ctx context.Context, token, method string, params, result any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+token+"/"+method, bytes.NewReader(body))
	if err != nil {
		// As below: a URL error repeats the URL, token and all.
		return fmt.Errorf("%s: the bot token or the API URL isn't usable", method)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL holds the bot's token, and *url.Error repeats it.
		return fmt.Errorf("%s: request failed", method)
	}
	defer resp.Body.Close()
	var answer struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil || !answer.OK {
		return fmt.Errorf("%s: %d %s", method, resp.StatusCode, answer.Description)
	}
	if result != nil {
		if err := json.Unmarshal(answer.Result, result); err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
	}
	return nil
}
