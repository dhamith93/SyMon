package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
)

// alertStore is the part of the store the evaluator uses
type alertStore interface {
	LatestValue(ctx context.Context, host string, metric string, target string, isCustom bool) (float64, time.Time, error)
	LastSeen(ctx context.Context, host string) (time.Time, error)
	DaysUntilFull(ctx context.Context, host string, device string) (float64, error)
	LatestEndpointCheck(ctx context.Context, name string) (store.EndpointCheck, error)
	LatestCertificate(ctx context.Context, name string) (time.Time, time.Time, error)
	OpenAlert(ctx context.Context, host string, rule string, metric string, target string) (*store.Alert, error)
	CreateAlert(ctx context.Context, alert store.Alert) (int64, error)
	UpdateAlert(ctx context.Context, id int64, severity int, value float64, at time.Time) error
	ResolveAlert(ctx context.Context, id int64, value float64, at time.Time) error
}

// evaluator checks alert rules against the latest values. An alert opens
// once a rule has been breached for its trigger interval, and resolves once
// it has been back to normal for the same interval. Open alerts are kept in
// the store, so they survive a restart.
type evaluator struct {
	store alertStore
	send  func(*alertapi.Alert)
	now   func() time.Time
	// pending is a status change that has not lasted the trigger interval yet
	pending map[string]pendingStatus
	// checked is the newest sample time seen, so each sample counts once
	checked map[string]time.Time
	// checkErrors is the error of each endpoint rule's newest check, for
	// its alert message
	checkErrors map[string]string
}

type pendingStatus struct {
	status alertstatus.StatusType
	since  time.Time
}

func newEvaluator(st alertStore, send func(*alertapi.Alert)) *evaluator {
	return &evaluator{
		store:       st,
		send:        send,
		now:         time.Now,
		pending:     map[string]pendingStatus{},
		checked:     map[string]time.Time{},
		checkErrors: map[string]string{},
	}
}

// ruleTarget is the disk, service or URL a rule watches, empty for other
// metrics
func ruleTarget(rule *alerts.AlertConfig) string {
	switch rule.MetricName {
	case monitor.DISKS, monitor.DISK_FORECAST:
		return rule.Disk
	case monitor.SERVICES:
		return rule.Service
	case monitor.ENDPOINT, monitor.CERTIFICATE:
		return rule.Endpoint
	}
	return ""
}

func (e *evaluator) evaluate(ctx context.Context, rule *alerts.AlertConfig, host string) error {
	target := ruleTarget(rule)
	key := host + "|" + rule.Name + "|" + rule.MetricName + "|" + target

	value, at, err := e.latest(ctx, rule, host, target)
	if errors.Is(err, store.ErrNotFound) {
		// no data yet
		return nil
	}
	if err != nil {
		return err
	}
	if last, ok := e.checked[key]; ok && !at.After(last) {
		return nil
	}
	e.checked[key] = at

	status := ruleStatus(rule, value)
	open, err := e.store.OpenAlert(ctx, host, rule.Name, rule.MetricName, target)
	if err != nil {
		return err
	}
	if open != nil {
		return e.updateOpen(ctx, rule, host, key, open, status, value, at)
	}
	return e.openIfBreached(ctx, rule, host, target, key, status, value, at)
}

// latest returns the value a rule checks. For ping it is the number of
// seconds since the host was last heard from, for a disk forecast the days
// until the disk is full, and for an endpoint the status code of its newest
// check.
func (e *evaluator) latest(ctx context.Context, rule *alerts.AlertConfig, host string, target string) (float64, time.Time, error) {
	if rule.MetricName == monitor.ENDPOINT {
		check, err := e.store.LatestEndpointCheck(ctx, rule.Name)
		if err != nil {
			return 0, time.Time{}, err
		}
		e.checkErrors[rule.Name] = check.Error
		return float64(check.StatusCode), check.Time, nil
	}
	if rule.MetricName == monitor.CERTIFICATE {
		expires, checked, err := e.store.LatestCertificate(ctx, rule.Name)
		return expires.Sub(checked).Hours() / 24, checked, err
	}
	if rule.MetricName == monitor.DISK_FORECAST {
		days, err := e.store.DaysUntilFull(ctx, host, target)
		return days, e.now(), err
	}
	if rule.MetricName != monitor.PING {
		return e.store.LatestValue(ctx, host, rule.MetricName, target, rule.IsCustom)
	}
	lastSeen, err := e.store.LastSeen(ctx, host)
	if err != nil {
		return 0, time.Time{}, err
	}
	if lastSeen.IsZero() {
		return 0, time.Time{}, store.ErrNotFound
	}
	now := e.now()
	return now.Sub(lastSeen).Seconds(), now, nil
}

// ruleStatus compares a value with a rule. Ping rules have no thresholds,
// a host is critical once it has been silent longer than the trigger interval.
// An endpoint is critical while it answers with another status code.
func ruleStatus(rule *alerts.AlertConfig, value float64) alertstatus.StatusType {
	if rule.MetricName == monitor.ENDPOINT {
		if int(value) != expectedCode(rule) {
			return alertstatus.Critical
		}
		return alertstatus.Normal
	}
	if rule.MetricName == monitor.PING {
		if value > float64(rule.TriggerIntveral) {
			return alertstatus.Critical
		}
		return alertstatus.Normal
	}
	return getAlertType(rule, value)
}

func (e *evaluator) updateOpen(ctx context.Context, rule *alerts.AlertConfig, host string, key string, open *store.Alert, status alertstatus.StatusType, value float64, at time.Time) error {
	if status == alertstatus.Normal {
		pending, ok := e.pending[key]
		if !ok || pending.status != alertstatus.Normal {
			e.pending[key] = pendingStatus{status: alertstatus.Normal, since: at}
			return nil
		}
		if at.Sub(pending.since) < triggerInterval(rule) {
			return nil
		}
		if err := e.store.ResolveAlert(ctx, open.ID, value, at); err != nil {
			return err
		}
		delete(e.pending, key)
		e.send(e.message(rule, host, open.ID, status, value, at))
		return nil
	}

	// breached again, so the wait for normal starts over
	delete(e.pending, key)
	if int(status) == open.Severity {
		return nil
	}
	if err := e.store.UpdateAlert(ctx, open.ID, int(status), value, at); err != nil {
		return err
	}
	e.send(e.message(rule, host, open.ID, status, value, at))
	return nil
}

func (e *evaluator) openIfBreached(ctx context.Context, rule *alerts.AlertConfig, host string, target string, key string, status alertstatus.StatusType, value float64, at time.Time) error {
	if status == alertstatus.Normal {
		delete(e.pending, key)
		return nil
	}
	pending, ok := e.pending[key]
	if !ok || pending.status == alertstatus.Normal {
		e.pending[key] = pendingStatus{status: status, since: at}
		return nil
	}
	if at.Sub(pending.since) < triggerInterval(rule) {
		return nil
	}

	id, err := e.store.CreateAlert(ctx, store.Alert{
		Host:      host,
		Rule:      rule.Name,
		Metric:    rule.MetricName,
		Target:    target,
		Severity:  int(status),
		Value:     value,
		StartedAt: at,
	})
	if err != nil {
		return err
	}
	delete(e.pending, key)
	e.send(e.message(rule, host, id, status, value, at))
	return nil
}

// message builds what is sent to the alert processor. Endpoint rules have
// their own template fields, like the status code and the error.
func (e *evaluator) message(rule *alerts.AlertConfig, host string, id int64, status alertstatus.StatusType, value float64, at time.Time) *alertapi.Alert {
	switch rule.MetricName {
	case monitor.ENDPOINT:
		return buildEndpointAlert(rule, id, int(value), e.checkErrors[rule.Name], status == alertstatus.Normal, at.Unix())
	case monitor.CERTIFICATE:
		return buildCertificateAlert(rule, id, value, status, at)
	}
	return buildAlertToSend(host, rule, alertStatus(rule, host, id, status, value, at))
}

// certificateRule is the alert on an HTTPS endpoint rule's certificate, or
// nil when the rule has none. Days left only fall, so it alerts on the
// first check below a level.
func certificateRule(rule *alerts.AlertConfig) *alerts.AlertConfig {
	warn, critical := rule.CertDays()
	if warn == 0 && critical == 0 {
		return nil
	}
	certificate := *rule
	certificate.MetricName = monitor.CERTIFICATE
	certificate.Op = "<"
	certificate.WarnThreshold = warn
	certificate.CriticalThreshold = critical
	certificate.TriggerIntveral = 0
	return &certificate
}

func triggerInterval(rule *alerts.AlertConfig) time.Duration {
	return time.Duration(rule.TriggerIntveral) * time.Second
}

// alertStatus fills the struct the alert message builders take. StartEvent
// carries the alert id, which stays the same from open to resolved.
func alertStatus(rule *alerts.AlertConfig, host string, id int64, status alertstatus.StatusType, value float64, at time.Time) alertstatus.AlertStatus {
	return alertstatus.AlertStatus{
		Alert:      *rule,
		Server:     host,
		UnixTime:   strconv.FormatInt(at.Unix(), 10),
		Type:       status,
		StartEvent: id,
		Value:      float32(value),
	}
}
