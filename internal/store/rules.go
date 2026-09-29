package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrRuleExists is a rule name that is taken. Alerts are kept per rule
// name, so names are unique.
var ErrRuleExists = errors.New("there is already a rule with that name")

// DefaultHeartbeat is added when the rules are set up, unless a heartbeat
// rule for all hosts exists already
var DefaultHeartbeat = alerts.AlertConfig{
	Name:            "Host not reporting",
	Description:     "Added by SyMon for every host",
	MetricName:      monitor.PING,
	Servers:         []string{alerts.AllHosts},
	TriggerIntveral: 300,
	Template:        "{subject}\n{serverName} has sent nothing for over {triggerInterval} seconds",
}

// AlertRule is a rule as the dashboard edits it
type AlertRule struct {
	ID        int64
	Enabled   bool
	Rule      alerts.AlertConfig
	UpdatedAt time.Time
	// UpdatedBy is the user who last changed it, empty for SyMon itself
	UpdatedBy string
}

func (s *Store) AlertRules(ctx context.Context) ([]AlertRule, error) {
	return s.queryRules(ctx, "SELECT id, enabled, rule, updated_at, updated_by FROM alert_rules ORDER BY name")
}

// EnabledRules are the rules the collector checks
func (s *Store) EnabledRules(ctx context.Context) ([]alerts.AlertConfig, error) {
	rules, err := s.queryRules(ctx, "SELECT id, enabled, rule, updated_at, updated_by FROM alert_rules WHERE enabled ORDER BY name")
	if err != nil {
		return nil, err
	}
	configs := make([]alerts.AlertConfig, 0, len(rules))
	for _, rule := range rules {
		configs = append(configs, rule.Rule)
	}
	return configs, nil
}

func (s *Store) queryRules(ctx context.Context, sql string, args ...any) ([]AlertRule, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []AlertRule{}
	for rows.Next() {
		var rule AlertRule
		var data []byte
		if err := rows.Scan(&rule.ID, &rule.Enabled, &data, &rule.UpdatedAt, &rule.UpdatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &rule.Rule); err != nil {
			return nil, fmt.Errorf("rule %d: %w", rule.ID, err)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func checkRule(rule *alerts.AlertConfig) error {
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return nil
}

// CreateRule adds a rule and returns its id
func (s *Store) CreateRule(ctx context.Context, rule alerts.AlertConfig, enabled bool, by string) (int64, error) {
	if err := checkRule(&rule); err != nil {
		return 0, err
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (name, rule, enabled, updated_by) VALUES ($1, $2, $3, $4)
		RETURNING id`, rule.Name, data, enabled, by).Scan(&id)
	if isUniqueViolation(err) {
		return 0, ErrRuleExists
	}
	return id, err
}

// UpdateRule replaces a rule. A new name carries its open alerts and its
// endpoint check history along, and a disabled rule's open alerts resolve.
func (s *Store) UpdateRule(ctx context.Context, id int64, rule alerts.AlertConfig, enabled bool, by string) error {
	if err := checkRule(&rule); err != nil {
		return err
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var oldName string
	err = tx.QueryRow(ctx, "SELECT name FROM alert_rules WHERE id = $1 FOR UPDATE", id).Scan(&oldName)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE alert_rules SET name = $2, rule = $3, enabled = $4, updated_at = now(), updated_by = $5 WHERE id = $1`,
		id, rule.Name, data, enabled, by)
	if isUniqueViolation(err) {
		return ErrRuleExists
	}
	if err != nil {
		return err
	}
	if rule.Name != oldName {
		if _, err := tx.Exec(ctx, "UPDATE alerts SET rule = $2 WHERE rule = $1 AND resolved_at IS NULL", oldName, rule.Name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE endpoint_checks SET name = $2 WHERE name = $1", oldName, rule.Name); err != nil {
			return err
		}
	}
	if !enabled {
		if err := resolveRuleAlerts(ctx, tx, rule.Name); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DeleteRule removes a rule and resolves its open alerts, which nothing
// would check any more
func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var name string
	err = tx.QueryRow(ctx, "DELETE FROM alert_rules WHERE id = $1 RETURNING name", id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := resolveRuleAlerts(ctx, tx, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func resolveRuleAlerts(ctx context.Context, tx pgx.Tx, rule string) error {
	_, err := tx.Exec(ctx, "UPDATE alerts SET resolved_at = now(), updated_at = now() WHERE rule = $1 AND resolved_at IS NULL", rule)
	return err
}

// RulesSetUp is true once SetupRules ran
func (s *Store) RulesSetUp(ctx context.Context) (bool, error) {
	var done bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM rule_setup)").Scan(&done)
	return done, err
}

// RuleSetup is what SetupRules did
type RuleSetup struct {
	// Done is true when the rules were set up before, and nothing changed
	Done     bool
	Imported int
	// Added are the default rules added
	Added []string
}

// SetupRules imports the rules from alerts.json and adds the default
// heartbeat rule, once. source is the file they came from.
func (s *Store) SetupRules(ctx context.Context, rules []alerts.AlertConfig, source string) (RuleSetup, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RuleSetup{}, err
	}
	defer tx.Rollback(ctx)

	// two collectors starting at once set up the rules only once
	if _, err := tx.Exec(ctx, "LOCK TABLE rule_setup IN EXCLUSIVE MODE"); err != nil {
		return RuleSetup{}, err
	}
	var done bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM rule_setup)").Scan(&done); err != nil || done {
		return RuleSetup{Done: done}, err
	}

	setup := RuleSetup{}
	names := map[string]bool{}
	allHostsHeartbeat := false
	for _, rule := range rules {
		if err := checkRule(&rule); err != nil {
			return RuleSetup{}, fmt.Errorf("rule %q: %w", rule.Name, err)
		}
		if err := insertRule(ctx, tx, rule, ""); err != nil {
			return RuleSetup{}, fmt.Errorf("rule %q: %w", rule.Name, err)
		}
		names[rule.Name] = true
		if rule.MetricName == monitor.PING && slices.Contains(rule.Servers, alerts.AllHosts) {
			allHostsHeartbeat = true
		}
		setup.Imported++
	}
	if !allHostsHeartbeat && !names[DefaultHeartbeat.Name] {
		if err := insertRule(ctx, tx, DefaultHeartbeat, ""); err != nil {
			return RuleSetup{}, err
		}
		setup.Added = append(setup.Added, DefaultHeartbeat.Name)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO rule_setup (source, imported) VALUES ($1, $2)", source, setup.Imported); err != nil {
		return RuleSetup{}, err
	}
	return setup, tx.Commit(ctx)
}

func insertRule(ctx context.Context, tx pgx.Tx, rule alerts.AlertConfig, by string) error {
	data, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO alert_rules (name, rule, updated_by) VALUES ($1, $2, $3)", rule.Name, data, by)
	if isUniqueViolation(err) {
		return ErrRuleExists
	}
	return err
}

// ImportRules adds rules from a file, and replaces the ones with the same
// name. Rules not in the file stay as they are.
func (s *Store) ImportRules(ctx context.Context, rules []alerts.AlertConfig, by string) (added int, updated int, err error) {
	batch := &pgx.Batch{}
	for _, rule := range rules {
		if err := checkRule(&rule); err != nil {
			return 0, 0, fmt.Errorf("rule %q: %w", rule.Name, err)
		}
		data, err := json.Marshal(rule)
		if err != nil {
			return 0, 0, err
		}
		// xmax is 0 for a row the statement inserted
		batch.Queue(`
			INSERT INTO alert_rules (name, rule, updated_by) VALUES ($1, $2, $3)
			ON CONFLICT (name) DO UPDATE SET rule = excluded.rule, updated_at = now(), updated_by = excluded.updated_by
			RETURNING xmax = 0`, rule.Name, data, by).QueryRow(func(row pgx.Row) error {
			var inserted bool
			if err := row.Scan(&inserted); err != nil {
				return err
			}
			if inserted {
				added++
			} else {
				updated++
			}
			return nil
		})
	}
	err = s.sendBatch(ctx, batch)
	return added, updated, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
