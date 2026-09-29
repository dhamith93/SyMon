package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The dashboard's data needs a login. A browser logs in once and gets a
// session cookie. Users and sessions live in the collector's database.

const sessionCookie = "symon_session"

// A checked session or password is trusted this long before the collector
// is asked again. Logging out elsewhere or removing a user takes at most
// this long to lock a browser or scraper out.
const (
	sessionCacheTTL  = time.Minute
	passwordCacheTTL = 5 * time.Minute
)

// errNotLoggedIn is a request without a valid session. Other errors mean
// the collector could not say.
var errNotLoggedIn = errors.New("log in first")

// authCache remembers sessions and passwords the collector accepted, keyed
// by a hash so the secrets themselves are not kept
type authCache struct {
	mu        sync.Mutex
	sessions  map[[32]byte]cachedSession
	passwords map[[32]byte]time.Time
}

type cachedSession struct {
	user  string
	until time.Time
}

func (c *authCache) session(key [32]byte) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.sessions[key]
	if !ok || time.Now().After(cached.until) {
		return "", false
	}
	return cached.user, true
}

func (c *authCache) keepSession(key [32]byte, user string, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessions == nil {
		c.sessions = map[[32]byte]cachedSession{}
	}
	until := time.Now().Add(sessionCacheTTL)
	if expires.Before(until) {
		until = expires
	}
	c.sessions[key] = cachedSession{user: user, until: until}
}

func (c *authCache) forgetSession(key [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, key)
}

func (c *authCache) password(key [32]byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	checked, ok := c.passwords[key]
	return ok && time.Since(checked) < passwordCacheTTL
}

func (c *authCache) keepPassword(key [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.passwords == nil {
		c.passwords = map[[32]byte]time.Time{}
	}
	c.passwords[key] = time.Now()
}

// sessionUser returns who the request's session cookie belongs to
func (s *server) sessionUser(r *http.Request) (string, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return "", errNotLoggedIn
	}
	key := sha256.Sum256([]byte(cookie.Value))
	if user, ok := s.auth.session(key); ok {
		return user, nil
	}
	info, err := s.collector.CheckSession(r.Context(), &api.SessionRequest{Token: cookie.Value})
	if status.Code(err) == codes.Unauthenticated {
		return "", errNotLoggedIn
	}
	if err != nil {
		return "", err
	}
	s.auth.keepSession(key, info.User, time.Unix(info.Expires, 0))
	return info.User, nil
}

// requireLogin answers 401 unless the request has a valid session
func (s *server) requireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := s.sessionUser(r)
		switch {
		case errors.Is(err, errNotLoggedIn):
			writeError(w, http.StatusUnauthorized, err.Error())
		case err != nil:
			writeGRPCError(w, "session", err)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// getSession says who is logged in. Without a session it says whether
// there are any users yet, since the dashboard stays locked until there are.
func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	user, err := s.sessionUser(r)
	if err == nil {
		writeJSON(w, map[string]string{"user": user})
		return
	}
	if !errors.Is(err, errNotLoggedIn) {
		writeGRPCError(w, "session", err)
		return
	}
	users, err := s.collector.HasUsers(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "users", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]any{"error": errNotLoggedIn.Error(), "hasUsers": users.HasUsers})
}

// jsonBody is false for a form another site posted. A page elsewhere cannot
// send JSON here without a CORS preflight, which the dashboard never allows.
func jsonBody(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

// isHTTPS is true when the browser reached the dashboard over HTTPS,
// directly or through a reverse proxy
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *server) postLogin(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the login as JSON")
		return
	}
	var credentials struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&credentials); err != nil {
		writeError(w, http.StatusBadRequest, "send user and password")
		return
	}
	session, err := s.collector.Login(r.Context(), &api.Credentials{User: credentials.User, Password: credentials.Password})
	if err != nil {
		writeGRPCError(w, "login", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    session.Token,
		Path:     "/",
		Expires:  time.Unix(session.Expires, 0),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, map[string]string{"user": session.User})
}

func (s *server) postLogout(w http.ResponseWriter, r *http.Request) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the logout as JSON")
		return
	}
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		s.auth.forgetSession(sha256.Sum256([]byte(cookie.Value)))
		if _, err := s.collector.Logout(r.Context(), &api.SessionRequest{Token: cookie.Value}); err != nil {
			writeGRPCError(w, "logout", err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, map[string]string{})
}

// metricsAllowed is true when /metrics needs no login, or the request has
// a session or a user's password as HTTP basic auth, like Prometheus's
// basic_auth sends. Otherwise it answers 401 itself.
func (s *server) metricsAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !s.metricsAuth {
		return true
	}
	if _, err := s.sessionUser(r); err == nil {
		return true
	}
	if user, password, ok := r.BasicAuth(); ok {
		err := s.checkPassword(r.Context(), user, password)
		if err == nil {
			return true
		}
		if code := status.Code(err); code != codes.Unauthenticated && code != codes.ResourceExhausted {
			logger.Log("error", "cannot check a password for /metrics: "+err.Error())
		}
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="SyMon", charset="UTF-8"`)
	http.Error(w, "log in with a SyMon user", http.StatusUnauthorized)
	return false
}

// checkPassword asks the collector, and remembers a right password for a
// while, since checking one takes a good part of a second
func (s *server) checkPassword(ctx context.Context, user string, password string) error {
	key := sha256.Sum256([]byte(user + "\x00" + password))
	if s.auth.password(key) {
		return nil
	}
	if _, err := s.collector.CheckPassword(ctx, &api.Credentials{User: user, Password: password}); err != nil {
		return err
	}
	s.auth.keepPassword(key)
	return nil
}
