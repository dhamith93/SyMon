package store

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrBadLogin is returned for an unknown user and for a wrong password
	// alike, so a guess learns nothing
	ErrBadLogin = errors.New("wrong user name or password")
	// ErrBadSession is returned for a session that is unknown or expired
	ErrBadSession = errors.New("not logged in")
	ErrUserExists = errors.New("user already exists")
)

const (
	// OWASP's figure for PBKDF2-HMAC-SHA256
	passwordIterations = 600_000
	minPasswordLength  = 12
	// SessionTTL is how long a login lasts
	SessionTTL = 30 * 24 * time.Hour
)

var validUserName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$`)

// hashSlots limits how many passwords are hashed at once. Each hash takes
// a good part of a second of CPU, which a flood of logins could use up.
var hashSlots = make(chan struct{}, 4)

// dummySalt is hashed against for unknown users, so a login takes as long
// whether the user exists or not
var dummySalt = make([]byte, 16)

func hashPassword(ctx context.Context, password string, salt []byte, iterations int) ([]byte, error) {
	select {
	case hashSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-hashSlots }()
	return pbkdf2.Key(sha256.New, password, salt, iterations, 32)
}

// NewPassword returns a random password, 120 bits in 20 characters
func NewPassword() (string, error) {
	bytes := make([]byte, 15)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func checkNewPassword(password string) error {
	if len([]rune(password)) < minPasswordLength {
		return fmt.Errorf("%w: a password needs at least %d characters", ErrInvalid, minPasswordLength)
	}
	return nil
}

func newPasswordHash(ctx context.Context, password string) (hash []byte, salt []byte, err error) {
	if err := checkNewPassword(password); err != nil {
		return nil, nil, err
	}
	salt = make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, err
	}
	hash, err = hashPassword(ctx, password, salt, passwordIterations)
	return hash, salt, err
}

// AddUser creates a dashboard user
func (s *Store) AddUser(ctx context.Context, name string, password string) error {
	if !validUserName.MatchString(name) {
		return fmt.Errorf("%w: user names may use letters, digits, dots, dashes, underscores and @, up to 64 characters", ErrInvalid)
	}
	hash, salt, err := newPasswordHash(ctx, password)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO users (name, password_hash, salt, iterations) VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO NOTHING`, name, hash, salt, passwordIterations)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserExists
	}
	return nil
}

// SetPassword gives a user a new password and ends all of their sessions
func (s *Store) SetPassword(ctx context.Context, name string, password string) error {
	hash, salt, err := newPasswordHash(ctx, password)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `UPDATE users SET password_hash = $2, salt = $3, iterations = $4 WHERE name = $1 RETURNING id`,
		name, hash, salt, passwordIterations).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM user_sessions WHERE user_id = $1", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RemoveUser deletes a user and ends their sessions
func (s *Store) RemoveUser(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM users WHERE name = $1", name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type User struct {
	Name      string
	CreatedAt time.Time
	// LastLogin is zero for a user who never logged in
	LastLogin time.Time
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, "SELECT name, created_at, last_login_at FROM users ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var user User
		var lastLogin *time.Time
		if err := rows.Scan(&user.Name, &user.CreatedAt, &lastLogin); err != nil {
			return nil, err
		}
		if lastLogin != nil {
			user.LastLogin = *lastLogin
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// HasUsers is false until the first user is created. The dashboard stays
// locked until then.
func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users)").Scan(&exists)
	return exists, err
}

// CheckPassword returns the user's id when the password is theirs
func (s *Store) CheckPassword(ctx context.Context, name string, password string) (int64, error) {
	var id int64
	var hash, salt []byte
	var iterations int
	err := s.pool.QueryRow(ctx, "SELECT id, password_hash, salt, iterations FROM users WHERE name = $1", name).
		Scan(&id, &hash, &salt, &iterations)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := hashPassword(ctx, password, dummySalt, passwordIterations); err != nil {
			return 0, err
		}
		return 0, ErrBadLogin
	}
	if err != nil {
		return 0, err
	}
	got, err := hashPassword(ctx, password, salt, iterations)
	if err != nil {
		return 0, err
	}
	if subtle.ConstantTimeCompare(got, hash) != 1 {
		return 0, ErrBadLogin
	}
	return id, nil
}

type Session struct {
	// Token is only set when the session is created
	Token   string
	User    string
	Expires time.Time
}

// Login checks a password and starts a session
func (s *Store) Login(ctx context.Context, name string, password string) (Session, error) {
	id, err := s.CheckPassword(ctx, name, password)
	if err != nil {
		return Session{}, err
	}
	token, err := newSecret()
	if err != nil {
		return Session{}, err
	}
	session := Session{Token: token, User: name, Expires: time.Now().Add(SessionTTL)}
	batch := &pgx.Batch{}
	batch.Queue("INSERT INTO user_sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)", hashSecret(token), id, session.Expires)
	batch.Queue("UPDATE users SET last_login_at = now() WHERE id = $1", id)
	return session, s.sendBatch(ctx, batch)
}

// Session returns who a session token belongs to, while it is valid
func (s *Store) Session(ctx context.Context, token string) (Session, error) {
	var session Session
	err := s.pool.QueryRow(ctx, `
		SELECT u.name, s.expires_at FROM user_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hashSecret(token)).Scan(&session.User, &session.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrBadSession
	}
	return session, err
}

func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM user_sessions WHERE token_hash = $1", hashSecret(token))
	return err
}

// PurgeSessions deletes expired sessions
func (s *Store) PurgeSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, "DELETE FROM user_sessions WHERE expires_at < now()")
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
