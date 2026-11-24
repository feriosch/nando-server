package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// SyncKeysService lets the Owner see and remove Sync Keys, under /api/sync-keys. The
// IdentityService creates them, once the Owner approves with a Passkey.
type SyncKeysService struct {
	keys model.SyncKeysRepository
	now  func() time.Time
	// open are the routes a Module opened to Sync Keys, as "METHOD /path".
	open map[string]bool
}

// NewSyncKeys returns the SyncKeysService over keys, with now as its clock.
func NewSyncKeys(keys model.SyncKeysRepository, now func() time.Time) *SyncKeysService {
	return &SyncKeysService{keys: keys, now: now, open: map[string]bool{}}
}

// Routes adds the Sync Keys routes to mux, under /api/.
func (s *SyncKeysService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync-keys", s.listSyncKeys)
	mux.HandleFunc("DELETE /api/sync-keys/{id}", s.deleteSyncKey)
}

// Open lets a Sync Key call route, written "METHOD /path" with no wildcards,
// such as "POST /api/calories/health-sync". Modules open their routes once,
// at startup, before serving. A route that can't be opened, a read or one
// with a wildcard, which would never match, panics, as a bad mux pattern does.
func (s *SyncKeysService) Open(route string) {
	method, path, ok := strings.Cut(route, " ")
	if !ok || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "{} ") {
		panic(fmt.Sprintf("Sync Keys can't open %q: write it \"METHOD /path\", with no wildcards", route))
	}
	if method == http.MethodGet || method == http.MethodHead {
		panic(fmt.Sprintf("Sync Keys can't open %q: a Sync Key can't read", route))
	}
	s.open[route] = true
}

// Guard lets a request to a route opened to Sync Keys through to next if it
// carries a Sync Key as its bearer token, and answers 401 if that key is
// unknown. Every other request goes to sessionGuarded, which checks for a
// Session as before.
func (s *SyncKeysService) Guard(sessionGuarded, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, bearer := bearerToken(r)
		// The escaped path, as the mux matches it: a path such as
		// /api/calories%2Fhealth-sync decodes to an open route but is
		// routed elsewhere, so it isn't open.
		if !bearer || !s.open[r.Method+" "+r.URL.EscapedPath()] {
			sessionGuarded.ServeHTTP(w, r)
			return
		}
		found, err := s.keys.FindByKey(r.Context(), sha256Of(key))
		if errors.Is(err, utils.ErrNotFound) {
			http.Error(w, "that Sync Key is unknown or was removed", http.StatusUnauthorized)
			return
		}
		if err != nil {
			utils.ServerError(w, err)
			return
		}
		// Recording when it was last used is bookkeeping, as for a
		// Connector: failing at it doesn't refuse a Sync Key that is let in.
		if err := s.keys.MarkUsed(r.Context(), found.ID, s.now()); err != nil {
			log.Printf("record Sync Key %s as used: %v", found.ID, err)
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken is r's bearer token, and whether it has one. The scheme's case
// doesn't matter (RFC 9110 section 11.1).
func bearerToken(r *http.Request) (string, bool) {
	const scheme = "bearer "
	header := r.Header.Get("Authorization")
	if len(header) < len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	return header[len(scheme):], true
}

// maxSyncKeyName is the longest a Sync Key's name may be, in characters.
const maxSyncKeyName = 50

// syncKeyName is name trimmed, or an error unless it is 1 to 50 characters.
func syncKeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxSyncKeyName {
		return "", fmt.Errorf("a Sync Key's name must be 1 to %d characters", maxSyncKeyName)
	}
	return name, nil
}

// create stores a new Sync Key named name and answers it with its key, the
// only time the key is ever answered.
func (s *SyncKeysService) create(ctx context.Context, name string) (model.SyncKeyJSON, error) {
	secret := rand.Text() + rand.Text()
	key := model.SyncKey{ID: rand.Text(), Name: name, KeyHash: sha256Of(secret), CreatedAt: s.now()}
	if err := s.keys.Insert(ctx, key); err != nil {
		return model.SyncKeyJSON{}, err
	}
	created := syncKeyJSON(key)
	created.Key = secret
	return created, nil
}

// listSyncKeys answers every Sync Key, oldest first, without their keys.
func (s *SyncKeysService) listSyncKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.keys.List(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	list := make([]model.SyncKeyJSON, len(keys))
	for i, key := range keys {
		list[i] = syncKeyJSON(key)
	}
	utils.WriteJSON(w, http.StatusOK, list)
}

func syncKeyJSON(key model.SyncKey) model.SyncKeyJSON {
	return model.SyncKeyJSON{ID: key.ID, Name: key.Name, CreatedAt: key.CreatedAt, LastUsedAt: key.LastUsedAt}
}

// deleteSyncKey removes a Sync Key, so its next call is unauthorized.
// Deleting one that doesn't exist also succeeds.
func (s *SyncKeysService) deleteSyncKey(w http.ResponseWriter, r *http.Request) {
	if err := s.keys.Delete(r.Context(), r.PathValue("id")); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
