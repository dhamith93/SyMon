package api

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A user name gets loginLimit wrong passwords per loginWindow. After that
// its logins are refused until the window ends, even with the right one.
const (
	loginLimit  = 10
	loginWindow = 15 * time.Minute
)

var errTooManyLogins = status.Error(codes.ResourceExhausted, "too many failed logins, try again later")

// loginFailures counts wrong passwords per user name
type loginFailures struct {
	mu     sync.Mutex
	byName map[string]failureWindow
	// now is time.Now, replaced in tests
	now func() time.Time
}

type failureWindow struct {
	count int
	since time.Time
}

func (l *loginFailures) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func (l *loginFailures) blocked(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	window, ok := l.byName[name]
	return ok && window.count >= loginLimit && l.clock().Sub(window.since) < loginWindow
}

func (l *loginFailures) fail(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	if l.byName == nil {
		l.byName = map[string]failureWindow{}
	}
	// guesses at many names would otherwise grow the map without end
	if len(l.byName) > 1000 {
		for key, window := range l.byName {
			if now.Sub(window.since) >= loginWindow {
				delete(l.byName, key)
			}
		}
	}
	window, ok := l.byName[name]
	if !ok || now.Sub(window.since) >= loginWindow {
		window = failureWindow{since: now}
	}
	window.count++
	l.byName[name] = window
}

func (l *loginFailures) clear(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byName, name)
}

// checkLogin runs check unless the user is locked out, and counts it when
// the password was wrong
func (s *Server) checkLogin(name string, check func() error) error {
	if s.logins.blocked(name) {
		return errTooManyLogins
	}
	err := check()
	if errors.Is(err, store.ErrBadLogin) {
		s.logins.fail(name)
		// the name comes from whoever is logging in, so it is quoted
		logger.Log("info", "failed login for "+strconv.Quote(name))
	}
	if err != nil {
		return toStatus(err)
	}
	s.logins.clear(name)
	return nil
}

func (s *Server) Login(ctx context.Context, in *Credentials) (*SessionInfo, error) {
	var session store.Session
	err := s.checkLogin(in.User, func() error {
		var err error
		session, err = s.Store.Login(ctx, in.User, in.Password)
		return err
	})
	if err != nil {
		return nil, err
	}
	logger.Log("info", "login by "+strconv.Quote(in.User))
	return &SessionInfo{Token: session.Token, User: session.User, Role: session.Role, Expires: session.Expires.Unix()}, nil
}

func (s *Server) CheckPassword(ctx context.Context, in *Credentials) (*Message, error) {
	err := s.checkLogin(in.User, func() error {
		_, err := s.Store.CheckPassword(ctx, in.User, in.Password)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) CheckSession(ctx context.Context, in *SessionRequest) (*SessionInfo, error) {
	session, err := s.Store.Session(ctx, in.Token)
	if err != nil {
		return nil, toStatus(err)
	}
	return &SessionInfo{User: session.User, Role: session.Role, Expires: session.Expires.Unix()}, nil
}

// ChangePassword counts a wrong current password like a failed login
func (s *Server) ChangePassword(ctx context.Context, in *ChangePasswordRequest) (*Message, error) {
	session, err := s.Store.Session(ctx, in.Token)
	if err != nil {
		return nil, toStatus(err)
	}
	err = s.checkLogin(session.User, func() error {
		return s.Store.ChangePassword(ctx, in.Token, in.Current, in.NewPassword)
	})
	if err != nil {
		return nil, err
	}
	logger.Log("info", "password changed by "+strconv.Quote(session.User))
	return &Message{Body: "ok"}, nil
}

func (s *Server) Logout(ctx context.Context, in *SessionRequest) (*Message, error) {
	if err := s.Store.Logout(ctx, in.Token); err != nil {
		return nil, toStatus(err)
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) HasUsers(ctx context.Context, in *Void) (*UserStatus, error) {
	has, err := s.Store.HasUsers(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &UserStatus{HasUsers: has}, nil
}
