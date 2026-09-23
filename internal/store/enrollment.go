package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrBadToken is returned for an enrollment token that is unknown, used up,
// expired or meant for another host. Callers get one error for all of
// these, so a guess learns nothing.
var ErrBadToken = errors.New("invalid or expired enrollment token")

// newSecret returns 256 random bits, URL safe so it fits in a shell command
func newSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// the secrets are random, so a plain hash is enough to store them
func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// CreateEnrollmentToken returns a token that can enroll `uses` hosts until
// it expires. hostName, when set, limits it to a host with that name.
func (s *Store) CreateEnrollmentToken(ctx context.Context, hostName string, uses int, ttl time.Duration) (string, time.Time, error) {
	if uses < 1 || ttl <= 0 {
		return "", time.Time{}, fmt.Errorf("%w: uses must be at least 1 and ttl positive", ErrInvalid)
	}
	token, err := newSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	var name *string
	if hostName != "" {
		name = &hostName
	}
	expires := time.Now().Add(ttl)
	_, err = s.pool.Exec(ctx, `INSERT INTO enrollment_tokens (token_hash, host_name, uses_left, expires_at) VALUES ($1, $2, $3, $4)`,
		hashSecret(token), name, uses, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Enroll uses a token to register a host, or to re-register one that
// already exists, which keeps its history. It returns the host's new
// credential, which replaces any earlier one.
func (s *Store) Enroll(ctx context.Context, token string, hostName string, timezone string) (string, error) {
	if hostName == "" {
		return "", fmt.Errorf("%w: host name is required", ErrInvalid)
	}
	secret, err := newSecret()
	if err != nil {
		return "", err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// lock the token so two hosts cannot both use its last use
	var tokenID int64
	var boundName *string
	err = tx.QueryRow(ctx, `SELECT id, host_name FROM enrollment_tokens
		WHERE token_hash = $1 AND uses_left > 0 AND expires_at > now()
		FOR UPDATE`, hashSecret(token)).Scan(&tokenID, &boundName)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && boundName != nil && *boundName != hostName) {
		return "", ErrBadToken
	}
	if err != nil {
		return "", err
	}

	var hostID int64
	err = tx.QueryRow(ctx, `INSERT INTO hosts (name, timezone) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET timezone = excluded.timezone
		RETURNING id`, hostName, timezone).Scan(&hostID)
	if err != nil {
		return "", err
	}

	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO agent_credentials (host_id, secret_hash) VALUES ($1, $2)
		ON CONFLICT (host_id) DO UPDATE SET secret_hash = excluded.secret_hash, created_at = now()`, hostID, hashSecret(secret))
	batch.Queue(`UPDATE enrollment_tokens SET uses_left = uses_left - 1 WHERE id = $1`, tokenID)
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return secret, nil
}

// HostForCredential returns the host an agent credential belongs to
func (s *Store) HostForCredential(ctx context.Context, secret string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `SELECT h.name FROM agent_credentials c
		JOIN hosts h ON h.id = c.host_id
		WHERE c.secret_hash = $1`, hashSecret(secret)).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// PurgeEnrollmentTokens deletes used up and expired tokens
func (s *Store) PurgeEnrollmentTokens(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, "DELETE FROM enrollment_tokens WHERE uses_left <= 0 OR expires_at < now()")
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
