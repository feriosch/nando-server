package service

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

const (
	sessionCookieName = "nando_session"
	sessionLifetime   = 90 * 24 * time.Hour
	// renewalInterval is the least time between renewals, so a Session in use
	// isn't written to on every request.
	renewalInterval = 24 * time.Hour
)

// SessionsService handles requests about Sessions, and guards every /api/
// route with RequireSession. Signing in ends by starting a Session here.
type SessionsService struct {
	sessions model.SessionsRepository
	domain   string
	now      func() time.Time
}

// NewSessions returns the SessionsService, setting its cookie for domain and
// with now as its clock.
func NewSessions(sessions model.SessionsRepository, domain string, now func() time.Time) *SessionsService {
	return &SessionsService{sessions: sessions, domain: domain, now: now}
}

// Routes adds the Session routes to mux, under /api/sessions.
func (s *SessionsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
}

// startSession starts a Session for the browser that sent r: it stores the
// Session and sets its cookie.
func (s *SessionsService) startSession(w http.ResponseWriter, r *http.Request) error {
	token := rand.Text()
	now := s.now()
	err := s.sessions.Insert(r.Context(), model.Session{
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
func (s *SessionsService) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, s.sessionCookie(token, int(sessionLifetime/time.Second)))
}

// clearSessionCookie has the browser delete its Session cookie.
func (s *SessionsService) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, s.sessionCookie("", -1)) // sent as Max-Age=0
}

// sessionCookie scopes the cookie to the parent domain, so it covers both web.
// and api., and keeps it from scripts and other sites.
func (s *SessionsService) sessionCookie(value string, maxAge int) *http.Cookie {
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
// Session; otherwise it answers 401. A Session in use is renewed for another
// 90 days, at most once a day.
func (s *SessionsService) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "sign in first", http.StatusUnauthorized)
			return
		}
		now := s.now()
		session, err := s.sessions.Find(r.Context(), sha256Of(cookie.Value), now)
		if err != nil {
			if errors.Is(err, utils.ErrNotFound) {
				http.Error(w, "sign in first", http.StatusUnauthorized)
				return
			}
			utils.ServerError(w, err)
			return
		}
		if now.Sub(session.RenewedAt) >= renewalInterval {
			if err := s.sessions.Renew(r.Context(), session.ID, now, now.Add(sessionLifetime)); err != nil {
				utils.ServerError(w, err)
				return
			}
			s.setSessionCookie(w, cookie.Value)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentSessionKey{}, session.ID)))
	})
}

// currentSessionKey holds, in a request's context, the ID of the Session
// RequireSession let it through with.
type currentSessionKey struct{}

func currentSession(r *http.Request) string {
	id, _ := r.Context().Value(currentSessionKey{}).(string)
	return id
}

// listSessions lists every Session, marking the one asking.
func (s *SessionsService) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.sessions.List(r.Context(), s.now())
	if err != nil {
		utils.ServerError(w, err)
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
	utils.WriteJSON(w, http.StatusOK, list)
}

// deleteSession deletes a Session, which signs its browser out. Deleting the
// current Session also has the browser asking delete its cookie.
func (s *SessionsService) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.sessions.Delete(r.Context(), id); err != nil {
		utils.ServerError(w, err)
		return
	}
	if id == currentSession(r) {
		s.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}
