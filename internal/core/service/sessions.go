package service

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
)

const (
	sessionCookieName = "nando_session"
	sessionLifetime   = 90 * 24 * time.Hour
	// renewalInterval is the least time between renewals, so a session in use
	// isn't written to on every request.
	renewalInterval = 24 * time.Hour
)

// startSession signs the device that sent r in: it stores a new session and
// sets its cookie.
func (s *Service) startSession(w http.ResponseWriter, r *http.Request) error {
	token := rand.Text()
	now := s.now()
	err := s.repo.InsertSession(r.Context(), model.Session{
		ID:        rand.Text(),
		TokenHash: sha256Of(token),
		UserAgent: r.UserAgent(),
		CreatedAt: now,
		RenewedAt: now,
		ExpiresAt: now.Add(sessionLifetime),
	})
	if err != nil {
		return err
	}
	s.setSessionCookie(w, token)
	return nil
}

// setSessionCookie has the browser keep token for the next 90 days.
func (s *Service) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, s.sessionCookie(token, int(sessionLifetime/time.Second)))
}

// clearSessionCookie has the browser delete its session cookie.
func (s *Service) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, s.sessionCookie("", -1)) // sent as Max-Age=0
}

// sessionCookie scopes the cookie to the parent domain, so it covers both web.
// and api., and keeps it from scripts and other sites.
func (s *Service) sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Domain:   s.domain,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// RequireSession lets a request through to next only if it carries a valid
// session; otherwise it answers 401. A session in use is renewed for another
// 90 days, at most once a day.
func (s *Service) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "sign in first", http.StatusUnauthorized)
			return
		}
		now := s.now()
		session, err := s.repo.FindSession(r.Context(), sha256Of(cookie.Value), now)
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				http.Error(w, "sign in first", http.StatusUnauthorized)
				return
			}
			serverError(w, err)
			return
		}
		if now.Sub(session.RenewedAt) >= renewalInterval {
			if err := s.repo.RenewSession(r.Context(), session.ID, now, now.Add(sessionLifetime)); err != nil {
				serverError(w, err)
				return
			}
			s.setSessionCookie(w, cookie.Value)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentSessionKey{}, session.ID)))
	})
}

// currentSessionKey holds, in a request's context, the ID of the session
// RequireSession let it through with.
type currentSessionKey struct{}

func currentSession(r *http.Request) string {
	id, _ := r.Context().Value(currentSessionKey{}).(string)
	return id
}

// listSessions lists every signed-in device, marking the one asking.
func (s *Service) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.repo.ListSessions(r.Context(), s.now())
	if err != nil {
		serverError(w, err)
		return
	}
	type sessionJSON struct {
		ID        string    `json:"id"`
		UserAgent string    `json:"userAgent"`
		CreatedAt time.Time `json:"createdAt"`
		ExpiresAt time.Time `json:"expiresAt"`
		Current   bool      `json:"current"`
	}
	list := make([]sessionJSON, len(sessions))
	for i, session := range sessions {
		list[i] = sessionJSON{
			ID:        session.ID,
			UserAgent: session.UserAgent,
			CreatedAt: session.CreatedAt,
			ExpiresAt: session.ExpiresAt,
			Current:   session.ID == currentSession(r),
		}
	}
	writeJSON(w, list)
}

// deleteSession signs a device out. Deleting the current session signs out
// the device asking, and has its browser delete the cookie too.
func (s *Service) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.repo.DeleteSession(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	if id == currentSession(r) {
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}
