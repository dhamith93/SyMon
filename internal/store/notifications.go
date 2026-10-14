package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrChannelExists is a channel name that is taken. Rules pick channels by
// name, so names are unique.
var ErrChannelExists = errors.New("there is already a channel with that name")

// What became of a notification
const (
	NotificationPending = "pending"
	NotificationSent    = "sent"
	NotificationFailed  = "failed"
)

// Channel is somewhere alerts are sent, set up on the dashboard
type Channel struct {
	ID   int64
	Name string
	// Kind is email, slack, pagerduty or webhook
	Kind string
	// Settings is JSON and holds the secrets too. Only the collector reads
	// it as it is.
	Settings     []byte
	IsDefault    bool
	SkipWarnings bool
	Enabled      bool
	UpdatedAt    time.Time
	UpdatedBy    string
	// how the newest notification went, empty without one
	LastStatus string
	LastAt     time.Time
	LastError  string
}

// Channels lists the channels by name, each with its newest notification
func (s *Store) Channels(ctx context.Context) ([]Channel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.name, c.kind, c.settings, c.is_default, c.skip_warnings, c.enabled, c.updated_at, c.updated_by,
			coalesce(n.status, ''), coalesce(n.sent_at, n.created_at), coalesce(n.last_error, '')
		FROM notification_channels c
		LEFT JOIN LATERAL (
			SELECT status, sent_at, created_at, last_error FROM notifications
			WHERE channel_id = c.id ORDER BY id DESC LIMIT 1
		) n ON true
		ORDER BY c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := []Channel{}
	for rows.Next() {
		var channel Channel
		var lastAt *time.Time
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Kind, &channel.Settings, &channel.IsDefault, &channel.SkipWarnings,
			&channel.Enabled, &channel.UpdatedAt, &channel.UpdatedBy, &channel.LastStatus, &lastAt, &channel.LastError); err != nil {
			return nil, err
		}
		if lastAt != nil {
			channel.LastAt = *lastAt
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (s *Store) Channel(ctx context.Context, id int64) (Channel, error) {
	var channel Channel
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, kind, settings, is_default, skip_warnings, enabled, updated_at, updated_by
		FROM notification_channels WHERE id = $1`, id).Scan(&channel.ID, &channel.Name, &channel.Kind, &channel.Settings,
		&channel.IsDefault, &channel.SkipWarnings, &channel.Enabled, &channel.UpdatedAt, &channel.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return channel, err
}

// CreateChannel adds a channel and returns its id. by is the user adding it.
// The settings have to be checked before.
func (s *Store) CreateChannel(ctx context.Context, channel Channel, by string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notification_channels (name, kind, settings, is_default, skip_warnings, enabled, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		channel.Name, channel.Kind, channel.Settings, channel.IsDefault, channel.SkipWarnings, channel.Enabled, by).Scan(&id)
	if isUniqueViolation(err) {
		return 0, ErrChannelExists
	}
	return id, err
}

// UpdateChannel changes a channel's settings and switches. Its name and
// kind stay, since rules pick it by name.
func (s *Store) UpdateChannel(ctx context.Context, channel Channel, by string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE notification_channels SET settings = $2, is_default = $3, skip_warnings = $4, enabled = $5,
			updated_at = now(), updated_by = $6
		WHERE id = $1`, channel.ID, channel.Settings, channel.IsDefault, channel.SkipWarnings, channel.Enabled, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteChannel removes a channel and the notifications it got
func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM notification_channels WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Notification is one message about an alert, for one channel
type Notification struct {
	ID        int64
	AlertID   int64
	ChannelID int64
	// Event is opened, changed or resolved
	Event    string
	Severity int
	Value    float64
	Subject  string
	Body     string
	Status   string
	Attempts int
	// LastError is why the newest try failed
	LastError string
	CreatedAt time.Time
	// SentAt is zero until it went out
	SentAt time.Time

	ChannelName string
	ChannelKind string
	// ChannelOn is false for a channel switched off since
	ChannelOn bool
	// Settings are only filled in by DueNotifications
	Settings []byte
	// the alert it is about. Host is empty for endpoint alerts.
	Host      string
	Rule      string
	Metric    string
	Target    string
	StartedAt time.Time
	// PreviousRef is what the channel kept from an earlier message about
	// the same alert, like the Slack message to update
	PreviousRef string
}

// QueueNotifications queues one notification per channel
func (s *Store) QueueNotifications(ctx context.Context, n Notification, channelIDs []int64) error {
	if len(channelIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, channelID := range channelIDs {
		batch.Queue(`
			INSERT INTO notifications (alert_id, channel_id, event, severity, value, subject, body)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			n.AlertID, channelID, n.Event, n.Severity, n.Value, cleanText(n.Subject), cleanText(n.Body))
	}
	return s.sendBatch(ctx, batch)
}

// NotifiedChannels are the channels told about an alert, which are the
// ones told when it resolves
func (s *Store) NotifiedChannels(ctx context.Context, alertID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx, "SELECT DISTINCT channel_id FROM notifications WHERE alert_id = $1 ORDER BY channel_id", alertID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

const notificationColumns = `n.id, n.alert_id, n.channel_id, n.event, n.severity, n.value, n.subject, n.body, n.status,
	n.attempts, n.last_error, n.created_at, n.sent_at, c.name, c.kind, c.enabled,
	coalesce(h.name, ''), a.rule, a.metric, a.target, a.started_at`

func scanNotification(row pgx.Row, n *Notification, more ...any) error {
	var sentAt *time.Time
	fields := []any{&n.ID, &n.AlertID, &n.ChannelID, &n.Event, &n.Severity, &n.Value, &n.Subject, &n.Body, &n.Status,
		&n.Attempts, &n.LastError, &n.CreatedAt, &sentAt, &n.ChannelName, &n.ChannelKind, &n.ChannelOn,
		&n.Host, &n.Rule, &n.Metric, &n.Target, &n.StartedAt}
	if err := row.Scan(append(fields, more...)...); err != nil {
		return err
	}
	if sentAt != nil {
		n.SentAt = *sentAt
	}
	return nil
}

// DueNotifications are the pending notifications whose time has come,
// oldest first. One waits while an earlier one about the same alert for
// the same channel is still pending, so they arrive in order.
func (s *Store) DueNotifications(ctx context.Context, limit int) ([]Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+notificationColumns+`, c.settings,
			coalesce((SELECT p.ref FROM notifications p
				WHERE p.alert_id = n.alert_id AND p.channel_id = n.channel_id AND p.id < n.id AND p.ref <> ''
				ORDER BY p.id DESC LIMIT 1), '')
		FROM notifications n
		JOIN notification_channels c ON c.id = n.channel_id
		JOIN alerts a ON a.id = n.alert_id
		LEFT JOIN hosts h ON h.id = a.host_id
		WHERE n.status = $1 AND n.next_attempt_at <= now()
			AND NOT EXISTS (SELECT 1 FROM notifications p
				WHERE p.alert_id = n.alert_id AND p.channel_id = n.channel_id AND p.id < n.id AND p.status = $1)
		ORDER BY n.id LIMIT $2`, NotificationPending, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	due := []Notification{}
	for rows.Next() {
		var n Notification
		if err := scanNotification(rows, &n, &n.Settings, &n.PreviousRef); err != nil {
			return nil, err
		}
		due = append(due, n)
	}
	return due, rows.Err()
}

// MarkSent records a delivered notification and what the channel returned
func (s *Store) MarkSent(ctx context.Context, id int64, ref string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET status = $2, attempts = attempts + 1, sent_at = now(), ref = $3, last_error = ''
		WHERE id = $1`, id, NotificationSent, ref)
	return err
}

// MarkRetry records a failed try and when to try again
func (s *Store) MarkRetry(ctx context.Context, id int64, reason string, next time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET attempts = attempts + 1, last_error = $2, next_attempt_at = $3 WHERE id = $1`,
		id, shortError(reason), next)
	return err
}

// MarkFailed gives up on a notification
func (s *Store) MarkFailed(ctx context.Context, id int64, reason string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET status = $2, attempts = attempts + 1, last_error = $3 WHERE id = $1`,
		id, NotificationFailed, shortError(reason))
	return err
}

// RecentNotifications are the newest notifications, for the dashboard
func (s *Store) RecentNotifications(ctx context.Context, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+notificationColumns+`
		FROM notifications n
		JOIN notification_channels c ON c.id = n.channel_id
		JOIN alerts a ON a.id = n.alert_id
		LEFT JOIN hosts h ON h.id = a.host_id
		ORDER BY n.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recent := []Notification{}
	for rows.Next() {
		var n Notification
		if err := scanNotification(rows, &n); err != nil {
			return nil, err
		}
		recent = append(recent, n)
	}
	return recent, rows.Err()
}

// shortError keeps an error readable in a table, and storable
func shortError(text string) string {
	runes := []rune(cleanText(text))
	if len(runes) > maxTaskError {
		runes = runes[:maxTaskError]
	}
	return string(runes)
}
