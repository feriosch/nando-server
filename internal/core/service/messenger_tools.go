package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/feriosch/nando-server/internal/core/model"
)

// claimFor is how long a run's claim on a Talk holds before the Talk is
// claimable again.
const claimFor = 15 * time.Minute

// ToolsDescription says what the Messenger's Tools let a Connector do.
func (s *MessengerService) ToolsDescription() string {
	return "read what you send your Assistants in Telegram, and answer you there as them"
}

// Tools are the Messenger's Tools, for an Assistant's run (ADR 0008).
func (s *MessengerService) Tools() []model.Tool {
	return []model.Tool{
		{
			Name: "messenger_claim_talk",
			Description: "Claims a Talk whose messages from the Owner wait for a reply, for 15 minutes, and returns it: " +
				"the Assistant to be, the path of its skill, the Talk's opening summary, the Notes carried over from the Talk it replaced, its messages (the waiting ones " +
				"marked), and the other Assistants. Without assistant, it claims the Talk waiting longest; with it, only " +
				"that Assistant's, to look for messages that arrived meanwhile. An empty answer means nothing waits: stop.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"assistant":{"type":"string","description":"Only this Assistant's Talk, such as career."}}}`),
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Assistant string `json:"assistant"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				return s.claim(ctx, args.Assistant)
			},
		},
		{
			Name: "messenger_reply",
			Description: "Sends text to the Owner as the Assistant, in its chat, and marks the messages waiting when its Talk " +
				"was claimed as answered. Messages that came since stay waiting: claim the Talk again with assistant to get them. " +
				"A note is kept with the reply for later runs, which see it in messenger_claim_talk, and is never sent to the Owner.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"assistant":{"type":"string","description":"The Assistant the run is, as messenger_claim_talk said."},` +
				`"text":{"type":"string","description":"The reply, as plain text."},` +
				`"note":{"type":"string","description":"What a later run needs and the Owner shouldn't read, such as the ID of a draft shown in text; at most 1000 characters."}},` +
				`"required":["assistant","text"]}`),
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Assistant string `json:"assistant"`
					Text      string `json:"text"`
					Note      string `json:"note"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				return s.reply(ctx, args.Assistant, args.Text, strings.TrimSpace(args.Note))
			},
		},
		{
			Name: "messenger_get_photo",
			Description: "Shows a photo the Owner sent the Assistant, by the id messenger_claim_talk listed in a message's " +
				"photos. Look at the photos of every waiting message before answering it, and at older ones only when the Owner refers to them.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"assistant":{"type":"string","description":"The Assistant the run is."},` +
				`"id":{"type":"string","description":"The photo's id, from messenger_claim_talk."}},"required":["assistant","id"]}`),
			ReadOnly: true,
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Assistant string `json:"assistant"`
					ID        string `json:"id"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				return s.photo(ctx, args.Assistant, args.ID)
			},
		},
		{
			Name: "messenger_get_file",
			Description: "Reads a PDF the Owner sent the Assistant, by the id messenger_claim_talk listed in a message's " +
				"files: its page count and, for the pages asked (the first 10 if not said, never more than 10 at a time), " +
				"their text, or, for a scan with no text, the pages as images.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"assistant":{"type":"string","description":"The Assistant the run is."},` +
				`"id":{"type":"string","description":"The file's id, from messenger_claim_talk."},` +
				`"pages":{"type":"string","description":"The pages to read, such as 3 or 11-20; at most 10."}},"required":["assistant","id"]}`),
			ReadOnly: true,
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Assistant string `json:"assistant"`
					ID        string `json:"id"`
					Pages     string `json:"pages"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				return s.file(ctx, args.Assistant, args.ID, args.Pages)
			},
		},
		{
			Name: "messenger_new_talk",
			Description: "Closes the Assistant's Talk and opens a new one, telling the Owner in one line. Messages still " +
				"waiting move to the new Talk, and the run's claim goes with them. Without summary the new Talk starts " +
				"blank, as when the Owner asks for a new chat; with it, it opens with the summary, as when the Talk grew too long. " +
				"Either way, the 10 latest Notes carry over to the new Talk.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{` +
				`"assistant":{"type":"string","description":"The Assistant the run is."},` +
				`"summary":{"type":"string","description":"What the new Talk opens with: open threads, decisions, anything half done. Notes carry over on their own."}},` +
				`"required":["assistant"]}`),
			Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
				var args struct {
					Assistant string `json:"assistant"`
					Summary   string `json:"summary"`
				}
				if err := json.Unmarshal(arguments, &args); err != nil {
					return nil, err
				}
				return s.newTalk(ctx, args.Assistant, strings.TrimSpace(args.Summary))
			},
		},
	}
}

// photo downloads the photo with id, which the Owner sent assistant, from
// Telegram.
func (s *MessengerService) photo(ctx context.Context, assistant, id string) (model.Image, error) {
	token, ok := s.bot(assistant)
	if !ok {
		return model.Image{}, fmt.Errorf("there is no Assistant %q with a bot", assistant)
	}
	if strings.TrimSpace(id) == "" {
		return model.Image{}, errors.New("id is empty")
	}
	data, err := s.telegram.download(ctx, token, id)
	if errors.Is(err, errFileGone) {
		return model.Image{}, fmt.Errorf("Telegram has no such photo for this Assistant, or no longer keeps it: ask the Owner to send it again (%w)", err)
	}
	if err != nil {
		return model.Image{}, fmt.Errorf("couldn't get the photo from Telegram just now, try once more (%w)", err)
	}
	// Telegram keeps photos as JPEGs; trust the bytes if they say otherwise.
	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}
	return model.Image{MIMEType: mimeType, Data: data}, nil
}

// maxPagesRead is the most pages of a PDF one messenger_get_file reads.
const maxPagesRead = 10

// fileAnswerJSON is what messenger_get_file answers about a PDF.
type fileAnswerJSON struct {
	PageCount int            `json:"pageCount"`
	Pages     []pageTextJSON `json:"pages"`
	// Scanned is true when none of the pages read has text, as in a scan:
	// then all of them follow as images, in order.
	Scanned bool `json:"scanned,omitempty"`
}

// pageTextJSON is one page of a PDF, as text.
type pageTextJSON struct {
	Page int    `json:"page"`
	Text string `json:"text"`
	// Image is true for a page with no text, such as a scanned one: it
	// follows the answer as an image, in page order.
	Image bool `json:"image,omitempty"`
}

// file reads the pages of the PDF with id, which the Owner sent assistant.
func (s *MessengerService) file(ctx context.Context, assistant, id, pages string) (any, error) {
	token, ok := s.bot(assistant)
	if !ok {
		return nil, fmt.Errorf("there is no Assistant %q with a bot", assistant)
	}
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("id is empty")
	}
	data, err := s.telegram.download(ctx, token, id)
	if errors.Is(err, errFileGone) {
		return nil, fmt.Errorf("Telegram has no such file for this Assistant, or no longer keeps it: ask the Owner to send it again (%w)", err)
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't get the file from Telegram just now, try once more (%w)", err)
	}
	doc, err := openPDF(data)
	if err != nil {
		return nil, err
	}
	defer doc.close()
	count, err := doc.pageCount(ctx)
	if err != nil {
		return nil, err
	}
	first, last, err := pageRange(pages, count)
	if err != nil {
		return nil, err
	}
	texts, err := doc.text(ctx, first, last)
	if err != nil {
		return nil, err
	}
	answer := fileAnswerJSON{PageCount: count, Pages: []pageTextJSON{}, Scanned: true}
	anyImage := false
	for i, text := range texts {
		page := pageTextJSON{Page: first + i, Text: text, Image: text == ""}
		answer.Pages = append(answer.Pages, page)
		answer.Scanned = answer.Scanned && page.Image
		anyImage = anyImage || page.Image
	}
	if !anyImage {
		return answer, nil
	}
	// Pages without text, such as a scanned one among typed ones, are drawn,
	// so that nothing on them is lost.
	drawn, err := doc.images(ctx, first, last)
	if err != nil {
		return nil, err
	}
	with := model.WithImages{Result: answer}
	for i, page := range answer.Pages {
		if page.Image && i < len(drawn) {
			with.Images = append(with.Images, model.Image{MIMEType: "image/jpeg", Data: drawn[i]})
		}
	}
	return with, nil
}

// pageRange is the pages of a PDF of count pages that pages asks for, such
// as "3" or "11-20", or the first ones when it is empty: never more than
// maxPagesRead, so a longer range is cut short.
func pageRange(pages string, count int) (first, last int, err error) {
	pages = strings.TrimSpace(pages)
	first, last = 1, count
	if pages != "" {
		from, to, isRange := strings.Cut(pages, "-")
		if !isRange {
			to = from
		}
		var errFrom, errTo error
		first, errFrom = strconv.Atoi(strings.TrimSpace(from))
		last, errTo = strconv.Atoi(strings.TrimSpace(to))
		if errFrom != nil || errTo != nil {
			return 0, 0, fmt.Errorf("pages %q isn't a page or a range of pages, such as 3 or 11-20", pages)
		}
	}
	if first < 1 || first > last || last > count {
		return 0, 0, fmt.Errorf("pages %q isn't in the PDF, which has %d pages", pages, count)
	}
	return first, min(last, first+maxPagesRead-1), nil
}

// newTalkJSON is what messenger_new_talk answers.
type newTalkJSON struct {
	// Moved counts the waiting messages moved to the new Talk.
	Moved int `json:"moved"`
}

// newTalk closes assistant's open Talk and opens one starting from summary,
// moving the waiting messages and the run's claim to it.
func (s *MessengerService) newTalk(ctx context.Context, assistant, summary string) (newTalkJSON, error) {
	token, ok := s.bot(assistant)
	if !ok {
		return newTalkJSON{}, fmt.Errorf("there is no Assistant %q with a bot", assistant)
	}
	var next model.Talk
	for {
		old, err := s.talks.Open(ctx, assistant)
		if err != nil {
			return newTalkJSON{}, err
		}
		next = model.Talk{
			Assistant:    assistant,
			Summary:      summary,
			Notes:        carriedNotes(old),
			Messages:     []model.TalkMessage{},
			ClaimedUntil: old.ClaimedUntil,
			Claims:       old.Claims,
			ClaimedUpTo:  old.ClaimedUpTo,
		}
		if !old.ID.IsZero() {
			kept := []model.TalkMessage{}
			for _, m := range old.Messages {
				if m.State == model.MessageWaiting {
					next.Messages = append(next.Messages, m)
				} else {
					kept = append(kept, m)
				}
			}
			old.Messages, old.Open = kept, false
			if err := s.talks.Save(ctx, old); errors.Is(err, model.ErrTalkChanged) {
				continue
			} else if err != nil {
				return newTalkJSON{}, err
			}
		}
		// A message arriving now would open a Talk of its own: then old's
		// messages are lost to the run, so the new Talk takes them back.
		if err := s.talks.Insert(ctx, next); errors.Is(err, model.ErrTalkChanged) {
			if err := s.mergeInto(ctx, assistant, next); err != nil {
				return newTalkJSON{}, err
			}
		} else if err != nil {
			return newTalkJSON{}, err
		}
		break
	}
	s.tasks[assistant].RunWorking(ctx)
	notice := "Started a new chat."
	if summary != "" {
		notice = "Started a new chat (kept a short summary)."
	}
	if err := s.telegram.call(ctx, token, "sendMessage", map[string]any{"chat_id": s.cfg.OwnerID, "text": notice}); err != nil {
		log.Printf("messenger: tell the Owner of a new chat: %v", err)
	}
	if next.Waiting() {
		s.showTyping(ctx, assistant)
	}
	return newTalkJSON{Moved: len(next.Messages)}, nil
}

// maxCarriedNotes is how many Notes a new Talk takes over: what still waits,
// such as a draft for the Owner's yes, is among the latest.
const maxCarriedNotes = 10

// carriedNotes are the Notes a new Talk takes over from old: the latest
// maxCarriedNotes of those old took over itself and those of its replies,
// oldest first.
func carriedNotes(old model.Talk) []string {
	notes := slices.Clone(old.Notes)
	for _, m := range old.Messages {
		if m.Note != "" {
			notes = append(notes, m.Note)
		}
	}
	return notes[max(0, len(notes)-maxCarriedNotes):]
}

// mergeInto puts next's summary, Notes, messages and claim into the Talk a message
// opened for assistant while next replaced the old one.
func (s *MessengerService) mergeInto(ctx context.Context, assistant string, next model.Talk) error {
	for {
		opened, err := s.talks.Open(ctx, assistant)
		if err != nil {
			return err
		}
		opened.Summary, opened.Notes = next.Summary, next.Notes
		opened.Messages = append(slices.Clone(next.Messages), opened.Messages...)
		opened.ClaimedUntil, opened.Claims, opened.ClaimedUpTo = next.ClaimedUntil, next.Claims, next.ClaimedUpTo
		if err := s.talks.Save(ctx, opened); !errors.Is(err, model.ErrTalkChanged) {
			return err
		}
	}
}

// showTyping shows the typing status in assistant's chat, as the run works
// on its waiting messages. It lasts about 5 seconds, and Nando has no CPU
// between requests to keep it going, so each of the run's Tools shows it
// again.
func (s *MessengerService) showTyping(ctx context.Context, assistant string) {
	token, _ := s.bot(assistant)
	s.typing(ctx, token)
}

// typing shows the typing status in the Owner's chat with the bot with token.
// A failure is only logged.
func (s *MessengerService) typing(ctx context.Context, token string) {
	if err := s.telegram.call(ctx, token, "sendChatAction", map[string]any{
		"chat_id": s.cfg.OwnerID,
		"action":  "typing",
	}); err != nil {
		log.Printf("messenger: show typing: %v", err)
	}
}

// replyJSON is what messenger_reply answers.
type replyJSON struct {
	// Answered counts the messages the reply answered: the Owner's, and
	// Nando's that asked for work.
	Answered int `json:"answered"`
}

// maxNote is the longest Note a run may leave with a reply, in characters.
const maxNote = 1000

// reply sends text to the Owner as assistant, keeps it in assistant's Talk
// with note, which is never sent, and answers the messages waiting when the
// Talk was claimed.
func (s *MessengerService) reply(ctx context.Context, assistant, text, note string) (replyJSON, error) {
	token, ok := s.bot(assistant)
	if !ok {
		return replyJSON{}, fmt.Errorf("there is no Assistant %q with a bot", assistant)
	}
	if strings.TrimSpace(text) == "" {
		return replyJSON{}, errors.New("text is empty")
	}
	if n := utf8.RuneCountInString(note); n > maxNote {
		return replyJSON{}, fmt.Errorf("note is %d characters; it may be at most %d", n, maxNote)
	}
	var replies []model.TalkMessage
	for _, part := range splitMessage(text) {
		var sent struct {
			MessageID int64 `json:"message_id"`
		}
		if err := s.telegram.callFor(ctx, token, "sendMessage", map[string]any{
			"chat_id": s.cfg.OwnerID,
			"text":    part,
		}, &sent); err != nil {
			if len(replies) == 0 {
				return replyJSON{}, err
			}
			// Part of the reply reached the Owner: keep what did.
			log.Printf("messenger: send the rest of a reply: %v", err)
			break
		}
		reply := model.TalkMessage{
			From: model.FromAssistant, At: s.now(), Text: part, TelegramMessageID: sent.MessageID,
		}
		if len(replies) == 0 {
			// On the first part only: the reply is one answer, however it was split.
			reply.Note = note
		}
		replies = append(replies, reply)
	}
	// Recorded before the claim is released, so that a message coming in
	// between doesn't wake a second run while this one looks again.
	s.tasks[assistant].RunWorking(ctx)
	var answered, reacted []int64
	for {
		talk, err := s.talks.Open(ctx, assistant)
		if err != nil {
			return replyJSON{}, err
		}
		if talk.ID.IsZero() {
			return replyJSON{}, fmt.Errorf("the %s Assistant has no Talk", assistant)
		}
		answered, reacted = nil, nil
		// Only what the run was given: a message Telegram numbered before the
		// claim's last one can still be stored after the claim, as Nando's ask
		// is when its sending and keeping straddle the claim.
		claimedAt := talk.ClaimedUntil.Add(-claimFor)
		for i, m := range talk.Messages {
			if m.State == model.MessageWaiting && m.TelegramMessageID <= talk.ClaimedUpTo && !m.At.After(claimedAt) {
				talk.Messages[i].State = model.MessageAnswered
				answered = append(answered, m.TelegramMessageID)
				if m.From == model.FromOwner {
					reacted = append(reacted, m.TelegramMessageID)
				}
			}
		}
		talk.Messages = append(talk.Messages, replies...)
		// The run has answered: release its claim, so that what came meanwhile
		// is claimable, by it or, if it stops, by the next run.
		talk.ClaimedUntil, talk.Claims = time.Time{}, 0
		if err := s.talks.Save(ctx, talk); errors.Is(err, model.ErrTalkChanged) {
			continue
		} else if err != nil {
			return replyJSON{}, err
		}
		if talk.Waiting() {
			s.showTyping(ctx, assistant)
		}
		break
	}
	s.clearReactions(ctx, token, reacted)
	return replyJSON{Answered: len(answered)}, nil
}

// claimJSON is what messenger_claim_talk answers.
type claimJSON struct {
	Assistant string `json:"assistant,omitempty"`
	Skill     string `json:"skill,omitempty"`
	Summary   string `json:"summary,omitempty"`
	// Notes are those carried over from the Talk this one replaced.
	Notes           []string      `json:"notes,omitempty"`
	Messages        []messageJSON `json:"messages,omitempty"`
	OtherAssistants []string      `json:"otherAssistants,omitempty"`
}

// messageJSON is one message in a Talk, for a run.
type messageJSON struct {
	From    model.MessageFrom `json:"from"`
	At      time.Time         `json:"at"`
	Text    string            `json:"text"`
	Waiting bool              `json:"waiting,omitempty"`
	// Note is what an earlier run left with its reply, for later runs.
	Note string `json:"note,omitempty"`
	// Failed marks a message no run answered, which the Owner was asked to
	// send again.
	Failed bool `json:"failed,omitempty"`
	// Photos are the message's photos, for messenger_get_photo.
	Photos []photoJSON `json:"photos,omitempty"`
	// Files are the message's files, such as PDFs, for messenger_get_file.
	Files []fileJSON `json:"files,omitempty"`
}

// fileJSON is one file in a message, for a run.
type fileJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// photoJSON is one photo in a message, for a run.
type photoJSON struct {
	ID string `json:"id"`
}

// on is every Assistant with a bot, by name.
func (s *MessengerService) on() []string {
	var names []string
	for _, a := range slices.Sorted(maps.Keys(s.assistants)) {
		if _, ok := s.bot(a); ok {
			names = append(names, a)
		}
	}
	return names
}

// claim claims assistant's Talk, or, if assistant is "", the Talk waiting
// longest, and returns it for the run.
func (s *MessengerService) claim(ctx context.Context, assistant string) (claimJSON, error) {
	if assistant != "" {
		if _, ok := s.bot(assistant); !ok {
			return claimJSON{}, fmt.Errorf("there is no Assistant %q with a bot", assistant)
		}
	}
	for {
		now := s.now()
		talk, ok, err := s.claimable(ctx, assistant, now)
		if err != nil {
			return claimJSON{}, err
		}
		if !ok {
			if assistant != "" {
				s.tasks[assistant].RunStopped(ctx)
			}
			return claimJSON{}, nil
		}
		talk.ClaimedUntil = now.Add(claimFor)
		talk.Claims++
		talk.ClaimedUpTo = talk.LastWaitableMessageID()
		if err := s.talks.Save(ctx, talk); errors.Is(err, model.ErrTalkChanged) {
			continue
		} else if err != nil {
			return claimJSON{}, err
		}
		s.tasks[talk.Assistant].RunWorking(ctx)
		s.showTyping(ctx, talk.Assistant)
		return s.claimAnswer(talk), nil
	}
}

// claimable is the Talk a run may claim at now: assistant's, or the one whose
// oldest waiting message is oldest.
func (s *MessengerService) claimable(ctx context.Context, assistant string, now time.Time) (model.Talk, bool, error) {
	names := s.on()
	if assistant != "" {
		names = []string{assistant}
	}
	var best model.Talk
	var bestAt time.Time
	found := false
	for _, name := range names {
		talk, err := s.talks.Open(ctx, name)
		if err != nil {
			return model.Talk{}, false, err
		}
		if !talk.Claimable(now) {
			continue
		}
		oldest, _ := talk.OldestWaiting()
		if !found || oldest.At.Before(bestAt) {
			best, bestAt, found = talk, oldest.At, true
		}
	}
	return best, found, nil
}

// claimAnswer is talk as a run gets it.
func (s *MessengerService) claimAnswer(talk model.Talk) claimJSON {
	answer := claimJSON{
		Assistant: talk.Assistant,
		Skill:     "skills/assistants/" + talk.Assistant + "/SKILL.md",
		Summary:   talk.Summary,
		Notes:     talk.Notes,
		Messages:  []messageJSON{},
	}
	for _, m := range talk.Messages {
		message := messageJSON{
			From: m.From, At: m.At, Text: m.Text, Note: m.Note,
			Waiting: m.State == model.MessageWaiting, Failed: m.State == model.MessageFailed,
		}
		for _, p := range m.Photos {
			message.Photos = append(message.Photos, photoJSON{ID: p.ID})
		}
		for _, f := range m.Files {
			message.Files = append(message.Files, fileJSON{ID: f.ID, Name: f.Name})
		}
		answer.Messages = append(answer.Messages, message)
	}
	for _, name := range s.on() {
		if name != talk.Assistant {
			answer.OtherAssistants = append(answer.OtherAssistants, name)
		}
	}
	return answer
}

// maxMessage is the most a Telegram message holds, in UTF-16 code units, as
// Telegram counts.
const maxMessage = 4096

// splitMessage splits text into Telegram messages: each part ends at the
// last line break that lets it fit, or, if there is none, where it fills up.
func splitMessage(text string) []string {
	var parts []string
	runes := []rune(text)
	for len(runes) > 0 {
		end, units, lastBreak := 0, 0, -1
		for end < len(runes) {
			// An invalid rune is sent as U+FFFD, one unit.
			n := max(utf16.RuneLen(runes[end]), 1)
			if units+n > maxMessage {
				break
			}
			if runes[end] == '\n' {
				lastBreak = end
			}
			units += n
			end++
		}
		if end == len(runes) {
			parts = append(parts, string(runes))
			break
		}
		if lastBreak > 0 {
			parts = append(parts, string(runes[:lastBreak]))
			runes = runes[lastBreak+1:]
			continue
		}
		parts = append(parts, string(runes[:end]))
		runes = runes[end:]
	}
	// Telegram refuses a message of only spaces and line breaks.
	return slices.DeleteFunc(parts, func(p string) bool { return strings.TrimSpace(p) == "" })
}

// RunAtTick is the Messenger's work at each tick: it wakes the runs of every
// Assistant whose messages still wait, such as when a run stopped without
// answering or the Owner's usage ran out.
func (s *MessengerService) RunAtTick(ctx context.Context) error {
	var errs []error
	for _, assistant := range s.on() {
		if err := s.failStuck(ctx, assistant); err != nil {
			errs = append(errs, fmt.Errorf("fail the %s Assistant's stuck messages: %w", assistant, err))
			continue
		}
		if err := s.noticeNotStarted(ctx, assistant); err != nil {
			errs = append(errs, fmt.Errorf("tell the Owner the %s Assistant's run didn't start: %w", assistant, err))
		}
		if err := s.tasks[assistant].WakeAtTick(ctx); err != nil {
			errs = append(errs, fmt.Errorf("wake the %s Assistant's runs: %w", assistant, err))
		}
	}
	return errors.Join(errs...)
}

// stuck is what a bot tells the Owner when their messages failed.
const stuck = "Sorry, something went wrong and I couldn't answer that. Please send it again."

// stuckAsk is what a bot tells the Owner when only Nando's ask for work, such
// as the Mail Digest, failed.
const stuckAsk = "Sorry, something went wrong and I couldn't do that. Ask me if you still want it."

// failStuck fails assistant's waiting messages if MaxClaims runs claimed them
// and the last claim lapsed with no reply, and tells the Owner.
func (s *MessengerService) failStuck(ctx context.Context, assistant string) error {
	var failed []int64
	for {
		talk, err := s.talks.Open(ctx, assistant)
		if err != nil {
			return err
		}
		if !talk.Waiting() || talk.Claims < model.MaxClaims || talk.ClaimedUntil.After(s.now()) {
			return nil
		}
		failed = nil
		for i, m := range talk.Messages {
			if m.State == model.MessageWaiting {
				talk.Messages[i].State = model.MessageFailed
				if m.From == model.FromOwner {
					failed = append(failed, m.TelegramMessageID)
				}
			}
		}
		talk.ClaimedUntil, talk.Claims = time.Time{}, 0
		if err := s.talks.Save(ctx, talk); errors.Is(err, model.ErrTalkChanged) {
			continue
		} else if err != nil {
			return err
		}
		break
	}
	token, _ := s.bot(assistant)
	text := stuck
	if len(failed) == 0 {
		// Only Nando's own asks failed: there's nothing for the Owner to send.
		text = stuckAsk
	}
	if err := s.telegram.call(ctx, token, "sendMessage", map[string]any{"chat_id": s.cfg.OwnerID, "text": text}); err != nil {
		log.Printf("messenger: tell the Owner their messages failed: %v", err)
	}
	s.clearReactions(ctx, token, failed)
	return nil
}

// clearReactions removes the bot's 👀 from each of the Owner's messages ids.
// Nando's own messages never get one.
func (s *MessengerService) clearReactions(ctx context.Context, token string, ids []int64) {
	for _, id := range ids {
		if err := s.telegram.call(ctx, token, "setMessageReaction", map[string]any{
			"chat_id":    s.cfg.OwnerID,
			"message_id": id,
			"reaction":   []any{},
		}); err != nil {
			log.Printf("messenger: remove the 👀 from message %d: %v", id, err)
		}
	}
}

// notStartedAfter is how long the Owner's messages wait unclaimed before they
// hear that no run started: as long as a fire stays in flight.
const notStartedAfter = 10 * time.Minute

// notStarted is what a bot tells the Owner when no run started.
const notStarted = "Claude didn't start (maybe the usage limit). I'll keep trying."

// noticeNotStarted tells the Owner, once for each batch of waiting messages,
// when no run has claimed assistant's Talk since its oldest waiting message
// came, notStartedAfter ago. The messages keep waiting, and their 👀.
func (s *MessengerService) noticeNotStarted(ctx context.Context, assistant string) error {
	if !s.tasks[assistant].CanFire() {
		// Nothing would start a run, and nothing will retry.
		return nil
	}
	for {
		talk, err := s.talks.Open(ctx, assistant)
		if err != nil {
			return err
		}
		oldest, waiting := talk.OldestWaiting()
		if !waiting || oldest.At.After(s.now().Add(-notStartedAfter)) ||
			oldest.TelegramMessageID <= talk.ClaimedUpTo || talk.NoticedFor == oldest.TelegramMessageID {
			return nil
		}
		talk.NoticedFor = oldest.TelegramMessageID
		if err := s.talks.Save(ctx, talk); errors.Is(err, model.ErrTalkChanged) {
			continue
		} else if err != nil {
			return err
		}
		break
	}
	token, _ := s.bot(assistant)
	if err := s.telegram.call(ctx, token, "sendMessage", map[string]any{"chat_id": s.cfg.OwnerID, "text": notStarted}); err != nil {
		log.Printf("messenger: tell the Owner no run started: %v", err)
	}
	return nil
}

// On reports whether the Messenger is on: some Assistant has a bot. Then
// Notifications are sent in Telegram instead of Web Push.
func (s *MessengerService) On() bool {
	return len(s.on()) > 0
}

// Notify sends n to the Owner in the chat of the Assistant its ID's area
// names, such as career for "career:…", or the Core Assistant's when that
// Assistant has no bot: the title, a line break and the body, leaving out
// either if empty. It joins that Assistant's Talk.
func (s *MessengerService) Notify(ctx context.Context, n model.Notification) error {
	area, _, _ := strings.Cut(n.ID, ":")
	assistant, token := "", ""
	// Its area's Assistant, or the Core Assistant's, or, while bots are set
	// up one at a time, any Assistant's with a bot: never lost. One that asks
	// for work goes only to the Assistant that does it.
	candidates := append([]string{area, model.CoreAssistant}, s.on()...)
	if n.Asks {
		candidates = []string{area}
	}
	for _, candidate := range candidates {
		if t, ok := s.bot(candidate); ok {
			assistant, token = candidate, t
			break
		}
	}
	if token == "" && n.Asks {
		return fmt.Errorf("the %s Assistant has no bot to ask", area)
	}
	if token == "" {
		return errors.New("no Assistant has a bot")
	}
	var lines []string
	for _, line := range []string{n.Title, n.Body} {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	text := strings.Join(lines, "\n")
	var sent struct {
		MessageID int64 `json:"message_id"`
	}
	if err := s.telegram.callFor(ctx, token, "sendMessage", map[string]any{"chat_id": s.cfg.OwnerID, "text": text}, &sent); err != nil {
		return err
	}
	if n.Asks {
		// It waits for a run like the Owner's message, so it must be kept:
		// unkept, no run would ever answer it.
		if err := s.talks.AddMessage(ctx, assistant, model.TalkMessage{
			From: model.FromNando, At: s.now(), Text: text, TelegramMessageID: sent.MessageID,
			State: model.MessageWaiting,
		}); err != nil {
			// The Owner already has the ask, but no run will answer it: say so
			// rather than leave it promising work that never comes.
			if err := s.telegram.call(ctx, token, "sendMessage", map[string]any{"chat_id": s.cfg.OwnerID, "text": stuckAsk}); err != nil {
				log.Printf("messenger: tell the Owner Notification %s can't be answered: %v", n.ID, err)
			}
			return fmt.Errorf("keep it in the %s Talk: %w", assistant, err)
		}
		s.tasks[assistant].Wake(ctx)
		return nil
	}
	// In the Talk, so the Owner's answer has it as context. It answers
	// nothing and waits for nothing, so no run is woken. The Owner has it
	// already, so failing to keep it is only logged.
	if err := s.talks.AddMessage(ctx, assistant, model.TalkMessage{
		From: model.FromAssistant, At: s.now(), Text: text, TelegramMessageID: sent.MessageID,
	}); err != nil {
		log.Printf("messenger: keep Notification %s in the %s Talk: %v", n.ID, assistant, err)
	}
	return nil
}
