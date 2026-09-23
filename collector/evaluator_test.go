package main

import (
	"context"
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
	alerts   []store.Alert
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
