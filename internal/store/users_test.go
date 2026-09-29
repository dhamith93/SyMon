package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestHashPassword(t *testing.T) {
	ctx := context.Background()
	salt := []byte("0123456789abcdef")
	first, err := hashPassword(ctx, "correct horse battery", salt, 1000)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := hashPassword(ctx, "correct horse battery", salt, 1000)
	other, _ := hashPassword(ctx, "correct horse battery", []byte("fedcba9876543210"), 1000)
	if len(first) != 32 || !bytes.Equal(first, again) || bytes.Equal(first, other) {
		t.Errorf("expected the same hash for the same salt only, got %x %x %x", first, again, other)
	}
}

func TestNewPassword(t *testing.T) {
	password, err := NewPassword()
	if err != nil || len(password) != 20 || checkNewPassword(password) != nil {
		t.Errorf("expected a usable 20 character password, got %q %v", password, err)
	}
	if err := checkNewPassword("short"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a short password, got %v", err)
	}
}

func TestUsersAndSessions(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	if has, err := st.HasUsers(ctx); err != nil || has {
		t.Fatalf("expected no users in a new database, got %v %v", has, err)
	}
	if err := st.AddUser(ctx, "alice", "correct horse battery", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUser(ctx, "alice", "another long password", RoleAdmin); !errors.Is(err, ErrUserExists) {
		t.Errorf("expected ErrUserExists, got %v", err)
	}
	if err := st.AddUser(ctx, "bob smith", "correct horse battery", RoleAdmin); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a name with a space, got %v", err)
	}
	if err := st.AddUser(ctx, "bob", "short", RoleAdmin); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a short password, got %v", err)
	}
	if has, err := st.HasUsers(ctx); err != nil || !has {
		t.Errorf("expected a user, got %v %v", has, err)
	}

	// an unknown user and a wrong password look the same
	if _, err := st.Login(ctx, "mallory", "correct horse battery"); !errors.Is(err, ErrBadLogin) {
		t.Errorf("expected ErrBadLogin for an unknown user, got %v", err)
	}
	if _, err := st.Login(ctx, "alice", "wrong horse battery"); !errors.Is(err, ErrBadLogin) {
		t.Errorf("expected ErrBadLogin for a wrong password, got %v", err)
	}

	session, err := st.Login(ctx, "alice", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.User != "alice" || time.Until(session.Expires) < SessionTTL-time.Minute {
		t.Errorf("unexpected session %+v", session)
	}
	found, err := st.Session(ctx, session.Token)
	if err != nil || found.User != "alice" || !found.Expires.Equal(session.Expires.Truncate(time.Microsecond)) {
		t.Errorf("expected alice's session, got %+v %v", found, err)
	}
	if _, err := st.Session(ctx, "not-a-token"); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected ErrBadSession for an unknown token, got %v", err)
	}

	users, err := st.Users(ctx)
	if err != nil || len(users) != 1 || users[0].Name != "alice" || users[0].LastLogin.IsZero() {
		t.Errorf("expected alice with a last login, got %+v %v", users, err)
	}

	// a new password ends the old sessions
	second, err := st.Login(ctx, "alice", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetPassword(ctx, "alice", "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Session(ctx, second.Token); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected the session to end with the password change, got %v", err)
	}
	if _, err := st.Login(ctx, "alice", "correct horse battery"); !errors.Is(err, ErrBadLogin) {
		t.Errorf("expected the old password to fail, got %v", err)
	}
	if err := st.SetPassword(ctx, "nobody", "a brand new password"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown user, got %v", err)
	}

	third, err := st.Login(ctx, "alice", "a brand new password")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Logout(ctx, third.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Session(ctx, third.Token); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected no session after logging out, got %v", err)
	}

	// expired sessions do not count and get purged
	fourth, err := st.Login(ctx, "alice", "a brand new password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, "UPDATE user_sessions SET expires_at = now() - INTERVAL '1 minute'"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Session(ctx, fourth.Token); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected an expired session to fail, got %v", err)
	}
	if purged, err := st.PurgeSessions(ctx); err != nil || purged != 1 {
		t.Errorf("expected one expired session purged, got %d %v", purged, err)
	}

	// removing a user ends their sessions
	fifth, err := st.Login(ctx, "alice", "a brand new password")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveUser(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Session(ctx, fifth.Token); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected no session for a removed user, got %v", err)
	}
	if err := st.RemoveUser(ctx, "alice"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound removing a user twice, got %v", err)
	}
}

func TestRolesAndPasswordChange(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddUser(ctx, "vera", "correct horse battery", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := st.AddUser(ctx, "root", "correct horse battery", "root"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for an unknown role, got %v", err)
	}

	session, err := st.Login(ctx, "vera", "correct horse battery")
	if err != nil || session.Role != RoleViewer {
		t.Fatalf("expected a viewer's session, got %+v %v", session, err)
	}
	if err := st.SetRole(ctx, "vera", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if found, err := st.Session(ctx, session.Token); err != nil || found.Role != RoleAdmin {
		t.Errorf("expected the new role on the session, got %+v %v", found, err)
	}
	if err := st.SetRole(ctx, "nobody", RoleAdmin); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown user, got %v", err)
	}
	if users, err := st.Users(ctx); err != nil || users[0].Role != RoleAdmin {
		t.Errorf("expected vera as an admin, got %+v %v", users, err)
	}

	// changing a password keeps this session and ends the others
	other, err := st.Login(ctx, "vera", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ChangePassword(ctx, session.Token, "wrong horse battery", "a brand new password"); !errors.Is(err, ErrBadLogin) {
		t.Errorf("expected ErrBadLogin for a wrong current password, got %v", err)
	}
	if err := st.ChangePassword(ctx, session.Token, "correct horse battery", "short"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a short new password, got %v", err)
	}
	if err := st.ChangePassword(ctx, "not-a-token", "correct horse battery", "a brand new password"); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected ErrBadSession without a session, got %v", err)
	}
	if err := st.ChangePassword(ctx, session.Token, "correct horse battery", "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Session(ctx, session.Token); err != nil {
		t.Errorf("expected this session to stay, got %v", err)
	}
	if _, err := st.Session(ctx, other.Token); !errors.Is(err, ErrBadSession) {
		t.Errorf("expected the other session to end, got %v", err)
	}
	if _, err := st.Login(ctx, "vera", "a brand new password"); err != nil {
		t.Errorf("expected the new password to work, got %v", err)
	}
}
