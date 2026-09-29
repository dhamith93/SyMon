package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// AgentUpdate is an update an admin asked a host's agent to install
type AgentUpdate struct {
	Version string
	// URL is the dashboard's address, which the agent downloads the build from
	URL         string
	RequestedAt time.Time
}

// AgentCheckIn records an agent's ping: when the host was last heard from
// and what the agent says about itself. It returns the update waiting for
// the agent, or nil. An update clears once the agent runs the version asked
// for. Agents from before updates send no version and no arch.
func (s *Store) AgentCheckIn(ctx context.Context, host string, at time.Time, agentVersion string, arch string, updateError string) (*AgentUpdate, error) {
	var version, url *string
	var requestedAt *time.Time
	// the SET expressions all see the row as it was
	err := s.pool.QueryRow(ctx, `
		UPDATE hosts SET
			last_seen = greatest(last_seen, $2),
			agent_version = CASE WHEN $3 = '' THEN agent_version ELSE $3 END,
			agent_arch = CASE WHEN $4 = '' THEN agent_arch ELSE $4 END,
			update_error = CASE WHEN update_version = $3 THEN '' ELSE $5 END,
			update_url = CASE WHEN update_version = $3 THEN NULL ELSE update_url END,
			update_requested_at = CASE WHEN update_version = $3 THEN NULL ELSE update_requested_at END,
			update_version = CASE WHEN update_version = $3 THEN NULL ELSE update_version END
		WHERE name = $1
		RETURNING update_version, update_url, update_requested_at`,
		host, at, agentVersion, arch, updateError).Scan(&version, &url, &requestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || version == nil {
		return nil, err
	}
	update := &AgentUpdate{Version: *version, URL: *url}
	if requestedAt != nil {
		update.RequestedAt = *requestedAt
	}
	return update, nil
}

// RequestAgentUpdate asks the agents of these hosts to install a version
// from the dashboard at url. Agents that cannot update themselves, or run
// that version already, are left out. It returns how many were asked.
func (s *Store) RequestAgentUpdate(ctx context.Context, hosts []string, version string, url string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE hosts SET update_version = $2, update_url = $3, update_requested_at = now(), update_error = ''
		WHERE name = ANY($1) AND agent_arch <> '' AND agent_version <> $2`, hosts, version, url)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
