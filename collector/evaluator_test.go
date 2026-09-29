package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
)

// fakeStore returns one value at a time and keeps alerts in memory
type fakeStore struct {
	value    float64
	at       time.Time
	lastSeen time.Time
	// daysUntilFull is keyed by device, a missing device has no forecast yet
	daysUntilFull map[string]float64
	// check is the newest endpoint check, none while its time is zero
	check store.EndpointCheck
	// certExpires is the newest check's certificate, none while zero
	certExpires time.Time
	alerts      []store.Alert
}

func (f *fakeStore) LatestCertificate(ctx context.Context, name string) (time.Time, time.Time, error) {
	if f.certExpires.IsZero() {
		return time.Time{}, time.Time{}, store.ErrNotFound
	}
	return f.certExpires, f.check.Time, nil
}

func (f *fakeStore) LatestValue(ctx context.Context, host string, metric string, target string, isCustom bool) (float64, time.Time, error) {
	if f.at.IsZero() {
		return 0, time.Time{}, store.ErrNotFound
	}
	return f.value, f.at, nil
}

func (f *fakeStore) LastSeen(ctx context.Context, host string) (time.Time, error) {
	return f.lastSeen, nil
}

func (f *fakeStore) DaysUntilFull(ctx context.Context, host string, device string) (float64, error) {
	days, ok := f.daysUntilFull[device]
	if !ok {
		return 0, store.ErrNotFound
	}
	return days, nil
}

func (f *fakeStore) LatestEndpointCheck(ctx context.Context, name string) (store.EndpointCheck, error) {
	if f.check.Time.IsZero() {
		return store.EndpointCheck{}, store.ErrNotFound
	}
	return f.check, nil
}

func (f *fakeStore) OpenAlert(ctx context.Context, host string, rule string, metric string, target string) (*store.Alert, error) {
	for i := range f.alerts {
		a := &f.alerts[i]
		if a.Host == host && a.Rule == rule && a.Metric == metric && a.Target == target && a.ResolvedAt == nil {
			return a, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) CreateAlert(ctx context.Context, alert store.Alert) (int64, error) {
	alert.ID = int64(len(f.alerts) + 1)
	f.alerts = append(f.alerts, alert)
	return alert.ID, nil
}

func (f *fakeStore) UpdateAlert(ctx context.Context, id int64, severity int, value float64, at time.Time) error {
	f.alerts[id-1].Severity = severity
	return nil
}

func (f *fakeStore) ResolveAlert(ctx context.Context, id int64, value float64, at time.Time) error {
	f.alerts[id-1].ResolvedAt = &at
	return nil
}

var cpuRule = alerts.AlertConfig{
	Name:              "CPU Usage",
	MetricName:        monitor.PROC_USAGE,
	Op:                ">",
	WarnThreshold:     50,
	CriticalThreshold: 80,
	TriggerIntveral:   60,
	Template:          "{subject}",
}

type step struct {
	offset time.Duration
	value  float64
	// sent is the status of the alert sent at this step, -1 for none
	sent int
}

func runSteps(t *testing.T, rule alerts.AlertConfig, steps []step) *fakeStore {
	t.Helper()
	fake := &fakeStore{}
	var sent []*alertapi.Alert
	e := newEvaluator(fake, func(a *alertapi.Alert) { sent = append(sent, a) })
	start := time.Unix(1700000000, 0)

	for i, s := range steps {
		fake.value, fake.at = s.value, start.Add(s.offset)
		before := len(sent)
		if err := e.evaluate(context.Background(), &rule, "web1"); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		switch {
		case s.sent < 0 && len(sent) != before:
			t.Fatalf("step %d: expected nothing sent, got %q", i, sent[len(sent)-1].Subject)
		case s.sent >= 0 && len(sent) != before+1:
			t.Fatalf("step %d: expected an alert with status %d, got none", i, s.sent)
		case s.sent >= 0 && sent[len(sent)-1].Status != int32(s.sent):
			t.Fatalf("step %d: expected status %d, got %d", i, s.sent, sent[len(sent)-1].Status)
		}
	}
	return fake
}

func TestAlertOpensAfterTriggerInterval(t *testing.T) {
	fake := runSteps(t, cpuRule, []step{
		{0, 10, -1},
		{15 * time.Second, 60, -1}, // warning starts
		{45 * time.Second, 90, -1}, // still inside the trigger interval
		{75 * time.Second, 90, int(alertstatus.Critical)}, // 60s since the breach started
	})
	if len(fake.alerts) != 1 || fake.alerts[0].Severity != int(alertstatus.Critical) {
		t.Errorf("expected one critical alert, got %+v", fake.alerts)
	}
}

func TestShortSpikeDoesNotAlert(t *testing.T) {
	fake := runSteps(t, cpuRule, []step{
		{0, 90, -1},
		{30 * time.Second, 10, -1},
		{60 * time.Second, 90, -1},
		{90 * time.Second, 10, -1},
	})
	if len(fake.alerts) != 0 {
		t.Errorf("expected no alerts, got %+v", fake.alerts)
	}
}

func TestAlertEscalatesAndResolves(t *testing.T) {
	fake := runSteps(t, cpuRule, []step{
		{0, 60, -1},
		{60 * time.Second, 60, int(alertstatus.Warning)},
		{75 * time.Second, 90, int(alertstatus.Critical)}, // severity change is sent right away
		{90 * time.Second, 10, -1},                        // normal starts
		{120 * time.Second, 90, -1},                       // breached again, normal timer resets
		{135 * time.Second, 10, -1},
		{180 * time.Second, 10, -1},                      // only 45s of normal
		{195 * time.Second, 10, int(alertstatus.Normal)}, // 60s of normal
	})
	if len(fake.alerts) != 1 || fake.alerts[0].ResolvedAt == nil {
		t.Errorf("expected one resolved alert, got %+v", fake.alerts)
	}
}

func TestSameSampleIsOnlyUsedOnce(t *testing.T) {
	// the agent sends every 30s but rules run every 15s, so the same sample
	// is seen twice and must not count as time passing
	fake := runSteps(t, cpuRule, []step{
		{0, 90, -1},
		{0, 90, -1},
		{30 * time.Second, 90, -1},
		{30 * time.Second, 90, -1},
		{60 * time.Second, 90, int(alertstatus.Critical)},
	})
	if len(fake.alerts) != 1 {
		t.Errorf("expected one alert, got %+v", fake.alerts)
	}
}

func TestAlertIDIsSentAsLogID(t *testing.T) {
	fake := &fakeStore{}
	var sent []*alertapi.Alert
	e := newEvaluator(fake, func(a *alertapi.Alert) { sent = append(sent, a) })
	rule := cpuRule
	rule.TriggerIntveral = 0

	start := time.Unix(1700000000, 0)
	for i, value := range []float64{90, 90, 10, 10} {
		fake.value, fake.at = value, start.Add(time.Duration(i)*time.Second)
		if err := e.evaluate(context.Background(), &rule, "web1"); err != nil {
			t.Fatal(err)
		}
	}
	if len(sent) != 2 || sent[0].LogId != 1 || sent[1].LogId != 1 {
		t.Errorf("expected open and resolve to carry alert id 1, got %+v", sent)
	}
}

func TestPingAlert(t *testing.T) {
	fake := &fakeStore{}
	var sent []*alertapi.Alert
	e := newEvaluator(fake, func(a *alertapi.Alert) { sent = append(sent, a) })
	now := time.Unix(1700000000, 0)
	e.now = func() time.Time { return now }
	rule := alerts.AlertConfig{Name: "Ping status", MetricName: monitor.PING, TriggerIntveral: 60, Template: "{subject}"}

	check := func() {
		t.Helper()
		if err := e.evaluate(context.Background(), &rule, "web1"); err != nil {
			t.Fatal(err)
		}
	}

	// never heard from, nothing to say yet
	check()
	fake.lastSeen = now
	for i := 0; i < 3; i++ {
		now = now.Add(45 * time.Second)
		check()
	}
	// silent for 135s: critical since 90s, open once that lasted 60s
	if len(sent) != 0 {
		t.Fatalf("expected no alert yet, got %d", len(sent))
	}
	now = now.Add(15 * time.Second)
	check()
	if len(sent) != 1 || sent[0].Status != int32(alertstatus.Critical) {
		t.Fatalf("expected a critical ping alert, got %+v", sent)
	}
}

func TestDiskForecastAlert(t *testing.T) {
	fake := &fakeStore{daysUntilFull: map[string]float64{}}
	var sent []*alertapi.Alert
	e := newEvaluator(fake, func(a *alertapi.Alert) { sent = append(sent, a) })
	now := time.Unix(1700000000, 0)
	e.now = func() time.Time { return now }
	rule := alerts.AlertConfig{
		Name:              "Data disk filling up",
		MetricName:        monitor.DISK_FORECAST,
		Disk:              "/dev/sdb1",
		Op:                "<",
		WarnThreshold:     14,
		CriticalThreshold: 3,
		TriggerIntveral:   60,
		Template:          "{subject}",
	}

	check := func(days float64, forecast bool) {
		t.Helper()
		delete(fake.daysUntilFull, "/dev/sdb1")
		if forecast {
			fake.daysUntilFull["/dev/sdb1"] = days
		}
		if err := e.evaluate(context.Background(), &rule, "web1"); err != nil {
			t.Fatal(err)
		}
		now = now.Add(30 * time.Second)
	}

	// too little history, then about 10 days left for a minute
	check(0, false)
	check(10, true)
	check(10, true)
	check(9.9, true)
	if len(sent) != 1 || sent[0].Status != int32(alertstatus.Warning) || sent[0].Disk != "/dev/sdb1" {
		t.Fatalf("expected one warning for /dev/sdb1, got %+v", sent)
	}
	if len(fake.alerts) != 1 || fake.alerts[0].Target != "/dev/sdb1" {
		t.Fatalf("expected an alert on /dev/sdb1, got %+v", fake.alerts)
	}

	// growth stops, so the disk reads as the forecast horizon and resolves
	check(365, true)
	check(365, true)
	check(365, true)
	if len(sent) != 2 || sent[1].Status != int32(alertstatus.Normal) || fake.alerts[0].ResolvedAt == nil {
		t.Errorf("expected the alert to resolve, got %+v", sent)
	}
}

func TestEndpointAlert(t *testing.T) {
	fake := &fakeStore{}
	var sent []*alertapi.Alert
	e := newEvaluator(fake, func(a *alertapi.Alert) { sent = append(sent, a) })
	rule := alerts.AlertConfig{
		Name:            "API up",
		MetricName:      monitor.ENDPOINT,
		Endpoint:        "https://api.example.com/health",
		TriggerIntveral: 60,
		Template:        "{subject} expected {expected} got {actual}: {error}",
	}
	start := time.Unix(1700000000, 0)

	// a check every 2 minutes, evaluated every 15s, so each is seen more than once
	check := func(minutes int, statusCode int, errMsg string) {
		t.Helper()
		fake.check = store.EndpointCheck{Time: start.Add(time.Duration(minutes) * time.Minute), Name: rule.Name, StatusCode: statusCode, Error: errMsg}
		for i := 0; i < 2; i++ {
			if err := e.evaluate(context.Background(), &rule, ""); err != nil {
				t.Fatal(err)
			}
		}
	}

	// nothing checked yet
	if err := e.evaluate(context.Background(), &rule, ""); err != nil {
		t.Fatal(err)
	}
	check(0, 200, "")
	check(2, 0, "connection refused")
	if len(sent) != 0 {
		t.Fatalf("expected no alert after one failed check, got %+v", sent)
	}
	check(4, 0, "connection refused")
	if len(sent) != 1 || sent[0].Status != int32(alertstatus.Critical) || sent[0].LogId != 1 || sent[0].ServerName != rule.Endpoint {
		t.Fatalf("expected a critical alert for the endpoint, got %+v", sent)
	}
	if sent[0].Content != "[Critical] endpoint check failed on https://api.example.com/health expected 200 got 0: connection refused" {
		t.Errorf("unexpected message %q", sent[0].Content)
	}
	if len(fake.alerts) != 1 || fake.alerts[0].Host != "" || fake.alerts[0].Target != rule.Endpoint {
		t.Errorf("expected an alert without a host on the URL, got %+v", fake.alerts)
	}

	check(6, 200, "")
	check(8, 200, "")
	if len(sent) != 2 || !sent[1].Resolved || sent[1].LogId != 1 || fake.alerts[0].ResolvedAt == nil {
		t.Errorf("expected the alert to resolve, got %+v", sent)
	}
}

func TestCertificateAlert(t *testing.T) {
	fake := &fakeStore{}
	var sent []*alertapi.Alert
	e := newEvaluator(fake, func(a *alertapi.Alert) { sent = append(sent, a) })
	rule := certificateRule(&alerts.AlertConfig{Name: "Shop", MetricName: monitor.ENDPOINT, Endpoint: "https://shop.example.com", Slack: true})
	if rule == nil || rule.WarnThreshold != 14 || rule.CriticalThreshold != 3 {
		t.Fatalf("expected a certificate rule at 14 and 3 days, got %+v", rule)
	}

	start := time.Unix(1700000000, 0)
	check := func(minutes int, daysLeft float64) {
		t.Helper()
		fake.check.Time = start.Add(time.Duration(minutes) * time.Minute)
		fake.certExpires = fake.check.Time.Add(time.Duration(daysLeft * 24 * float64(time.Hour)))
		if err := e.evaluate(context.Background(), rule, ""); err != nil {
			t.Fatal(err)
		}
	}

	check(0, 40)
	check(1, 13)
	check(2, 13)
	if len(sent) != 1 || sent[0].Status != int32(alertstatus.Warning) || sent[0].MetricName != monitor.CERTIFICATE || !sent[0].Slack {
		t.Fatalf("expected one warning, got %+v", sent)
	}
	if !strings.Contains(sent[0].Subject, "certificate expiring for https://shop.example.com") || !strings.Contains(sent[0].Content, "expires on") {
		t.Errorf("unexpected message %q %q", sent[0].Subject, sent[0].Content)
	}
	check(3, 2)
	if len(sent) != 2 || sent[1].Status != int32(alertstatus.Critical) {
		t.Fatalf("expected it to turn critical, got %+v", sent)
	}
	// renewed
	check(4, 90)
	check(5, 90)
	if len(sent) != 3 || !sent[2].Resolved || !strings.Contains(sent[2].Subject, "renewed") {
		t.Errorf("expected the alert to resolve, got %+v", sent)
	}
}

func TestCertificateRule(t *testing.T) {
	zero, thirty := 0, 30
	tests := []struct {
		rule alerts.AlertConfig
		want bool
	}{
		{alerts.AlertConfig{MetricName: monitor.ENDPOINT, Endpoint: "http://example.com"}, false},
		{alerts.AlertConfig{MetricName: monitor.ENDPOINT, Endpoint: "https://example.com", CertWarnDays: &zero, CertCriticalDays: &zero}, false},
		{alerts.AlertConfig{MetricName: monitor.ENDPOINT, Endpoint: "https://example.com", CertWarnDays: &thirty}, true},
		{alerts.AlertConfig{MetricName: monitor.PING, Servers: []string{"*"}}, false},
	}
	for _, tt := range tests {
		if got := certificateRule(&tt.rule); (got != nil) != tt.want {
			t.Errorf("%s %s: got %+v, want a rule: %v", tt.rule.MetricName, tt.rule.Endpoint, got, tt.want)
		}
	}
}
