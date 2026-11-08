package tests

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// pushService is a browser's push service (Apple's, Google's, ...) holding one
// Push Subscription: an endpoint plus the keys only the browser knows. It
// decrypts each push message the way the browser would (RFC 8291) and records
// it, answering with status (201 Created unless a test changes it).
type pushService struct {
	t       *testing.T
	server  *httptest.Server
	private *ecdh.PrivateKey
	auth    []byte

	mu       sync.Mutex
	status   int
	requests int
	messages []pushMessage
	headers  []http.Header
}

// pushMessage is the Notification the web app's service worker shows.
type pushMessage struct {
	Title string
	Body  string
	URL   string
}

// shownBy reads a push message's JSON the way nando-web's service worker does
// (src/core/sw/notifications.ts): a field that is missing or null falls back
// to its default.
func shownBy(t *testing.T, payload []byte) pushMessage {
	var sent struct{ Title, Body, URL *string }
	if err := json.Unmarshal(payload, &sent); err != nil {
		t.Errorf("push message isn't JSON: %v", err)
	}
	or := func(s *string, fallback string) string {
		if s == nil {
			return fallback
		}
		return *s
	}
	return pushMessage{Title: or(sent.Title, "Nando"), Body: or(sent.Body, ""), URL: or(sent.URL, "/")}
}

func newPushService(t *testing.T) *pushService {
	t.Helper()
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &pushService{t: t, private: private, auth: make([]byte, 16), status: http.StatusCreated}
	_, _ = rand.Read(p.auth)
	p.server = httptest.NewServer(http.HandlerFunc(p.receive))
	t.Cleanup(p.server.Close)
	return p
}

// subscriptionJSON is the Push Subscription as PushSubscription.toJSON()
// returns it in the browser.
func (p *pushService) subscriptionJSON() map[string]any {
	enc := base64.RawURLEncoding
	return map[string]any{
		"endpoint":       p.server.URL + "/push/" + enc.EncodeToString(p.auth),
		"expirationTime": nil,
		"keys": map[string]string{
			"p256dh": enc.EncodeToString(p.private.PublicKey().Bytes()),
			"auth":   enc.EncodeToString(p.auth),
		},
	}
}

func (p *pushService) endpoint() string { return p.subscriptionJSON()["endpoint"].(string) }

// answer makes the push service reply to every later push message with status.
func (p *pushService) answer(status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

// received returns the push messages delivered so far.
func (p *pushService) received() []pushMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]pushMessage(nil), p.messages...)
}

// vapidKeys returns the VAPID public key ("k=") of each delivered push message.
func (p *pushService) vapidKeys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var keys []string
	for _, h := range p.headers {
		auth, _ := strings.CutPrefix(h.Get("Authorization"), "vapid ")
		for part := range strings.SplitSeq(auth, ",") {
			if k, ok := strings.CutPrefix(strings.TrimSpace(part), "k="); ok {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

// requestCount is how many push messages were sent here, delivered or not.
func (p *pushService) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

func (p *pushService) receive(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests++
	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.t.Errorf("push service: read body: %v", err)
	}
	if r.Header.Get("Content-Encoding") != "aes128gcm" {
		p.t.Errorf("push message Content-Encoding = %q, want aes128gcm", r.Header.Get("Content-Encoding"))
	}
	if p.status == http.StatusCreated {
		p.messages = append(p.messages, shownBy(p.t, p.decrypt(body)))
		p.headers = append(p.headers, r.Header.Clone())
	}
	w.WriteHeader(p.status)
}

// decrypt opens an aes128gcm push message as RFC 8291 describes.
func (p *pushService) decrypt(body []byte) []byte {
	t := p.t
	if len(body) < 21 {
		t.Fatalf("push message is too short: %d bytes", len(body))
	}
	salt, idLen := body[:16], int(body[20])
	senderKey, ciphertext := body[21:21+idLen], body[21+idLen:]
	_ = binary.BigEndian.Uint32(body[16:20]) // record size: one record here

	sender, err := ecdh.P256().NewPublicKey(senderKey)
	if err != nil {
		t.Fatalf("push message sender key: %v", err)
	}
	secret, err := p.private.ECDH(sender)
	if err != nil {
		t.Fatal(err)
	}
	keyInfo := append(append([]byte("WebPush: info\x00"), p.private.PublicKey().Bytes()...), senderKey...)
	ikm, err := hkdf.Key(sha256.New, secret, p.auth, string(keyInfo), 32)
	if err != nil {
		t.Fatal(err)
	}
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("decrypt push message: %v", err)
	}
	// The last record ends with a 0x02 delimiter, then zero padding.
	for len(plain) > 0 && plain[len(plain)-1] == 0 {
		plain = plain[:len(plain)-1]
	}
	if len(plain) == 0 || plain[len(plain)-1] != 2 {
		t.Fatalf("push message has no last-record delimiter")
	}
	return plain[:len(plain)-1]
}
