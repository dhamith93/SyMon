package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/notify"
	"github.com/dhamith93/SyMon/internal/store"
)

// testChannels: ops email is the default, phone only wants critical alerts
var testChannels = []store.Channel{
	{ID: 1, Name: "ops email", Kind: notify.Email, IsDefault: true, Enabled: true},
	{ID: 2, Name: "team slack", Kind: notify.Slack, Enabled: true},
	{ID: 3, Name: "phone", Kind: notify.Webhook, SkipWarnings: true, Enabled: true},
	{ID: 4, Name: "old default", Kind: notify.Email, IsDefault: true, Enabled: false},
	{ID: 5, Name: "on call", Kind: notify.PagerDuty, Enabled: true},
}

func TestPickChannels(t *testing.T) {
	warning, critical := notify.SeverityWarning, notify.SeverityCritical
	cases := []struct {
		name     string
		rule     alerts.AlertConfig
		event    string
		severity int
		told     map[int64]bool
		want     []int64
	}{
		{"picked channel", alerts.AlertConfig{Channels: []string{"team slack"}}, notify.Opened, warning, nil, []int64{2}},
		{"picks none, so the defaults that are on", alerts.AlertConfig{}, notify.Opened, warning, nil, []int64{1}},
		{"skips warnings", alerts.AlertConfig{Channels: []string{"phone", "team slack"}}, notify.Opened, warning, nil, []int64{2}},
		{"critical reaches it", alerts.AlertConfig{Channels: []string{"phone", "team slack"}}, notify.Changed, critical, nil, []int64{2, 3}},
		{"old email tick", alerts.AlertConfig{Email: true}, notify.Opened, critical, nil, []int64{1}},
		{"old pagerduty tick", alerts.AlertConfig{Pagerduty: true, Channels: []string{"phone"}}, notify.Opened, critical, nil, []int64{3, 5}},
		{"an old tick is a pick, so no defaults", alerts.AlertConfig{Slack: true}, notify.Opened, warning, nil, []int64{2}},
		{"a channel that is not there", alerts.AlertConfig{Channels: []string{"gone"}}, notify.Opened, critical, nil, []int64{}},
		{"resolved goes to who was told", alerts.AlertConfig{Channels: []string{"team slack"}}, notify.Resolved, 0, map[int64]bool{3: true, 4: true, 5: true}, []int64{3, 5}},
	}
	for _, c := range cases {
		if got := pickChannels(&c.rule, testChannels, c.event, c.severity, c.told); !slices.Equal(got, c.want) {
			t.Errorf("%s: expected %v, got %v", c.name, c.want, got)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	want := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour}
	for i, wait := range want {
		if got, again := retryAfter(i + 1); !again || got != wait {
			t.Errorf("after %d failures: expected %s, got %s %v", i+1, wait, got, again)
		}
	}
	if _, again := retryAfter(len(want) + 1); again {
		t.Error("expected to give up after the last wait")
	}
}

// fakeNotices keeps what was queued
type fakeNotices struct {
	told   []int64
	queued []store.Notification
	to     [][]int64
}

func (f *fakeNotices) Channels(ctx context.Context) ([]store.Channel, error) {
	return testChannels, nil
}

func (f *fakeNotices) NotifiedChannels(ctx context.Context, alertID int64) ([]int64, error) {
	return f.told, nil
}

func (f *fakeNotices) QueueNotifications(ctx context.Context, n store.Notification, channelIDs []int64) error {
	f.queued = append(f.queued, n)
	f.to = append(f.to, channelIDs)
	return nil
}

func TestQueueNotice(t *testing.T) {
	fake := &fakeNotices{told: []int64{1, 3}}
	rule := &alerts.AlertConfig{Name: "CPU", Channels: []string{"team slack"}}
	n := notice{alertID: 7, rule: rule, host: "web1", event: notify.Opened, severity: notify.SeverityWarning, value: 85, subject: "[Warning] CPU", body: "at 85%"}
	if err := queueNotice(context.Background(), fake, n); err != nil {
		t.Fatal(err)
	}
	n.event, n.severity = notify.Resolved, 0
	if err := queueNotice(context.Background(), fake, n); err != nil {
		t.Fatal(err)
	}
	first := fake.queued[0]
	if first.AlertID != 7 || first.Event != notify.Opened || first.Severity != notify.SeverityWarning || first.Value != 85 || first.Subject != "[Warning] CPU" || first.Body != "at 85%" {
		t.Errorf("unexpected queued notification %+v", first)
	}
	if !slices.Equal(fake.to[0], []int64{2}) || !slices.Equal(fake.to[1], []int64{1, 3}) {
		t.Errorf("expected the slack channel, then the channels told before, got %v", fake.to)
	}
}

// fakeDeliveries hands out due notifications and records what became of them
type fakeDeliveries struct {
	due     []store.Notification
	sent    map[int64]string
	retries map[int64]time.Time
	failed  map[int64]string
}

func (f *fakeDeliveries) DueNotifications(ctx context.Context, limit int) ([]store.Notification, error) {
	return f.due, nil
}

func (f *fakeDeliveries) MarkSent(ctx context.Context, id int64, ref string) error {
	f.sent[id] = ref
	return nil
}

func (f *fakeDeliveries) MarkRetry(ctx context.Context, id int64, reason string, next time.Time) error {
	f.retries[id] = next
	return nil
}

func (f *fakeDeliveries) MarkFailed(ctx context.Context, id int64, reason string) error {
	f.failed[id] = reason
	return nil
}

func TestDeliverDue(t *testing.T) {
	var mu sync.Mutex
	var bodies []notify.WebhookBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/broken" {
			http.Error(w, "receiver is down", http.StatusServiceUnavailable)
			return
		}
		var body notify.WebhookBody
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
	}))
	defer server.Close()

	webhook := func(id int64, path string, attempts int) store.Notification {
		return store.Notification{
			ID: id, AlertID: 9, Event: notify.Opened, Severity: notify.SeverityCritical, Value: 97, Subject: "[Critical] CPU on web1",
			Body: "at 97%", Attempts: attempts, ChannelName: "hook", ChannelKind: notify.Webhook, ChannelOn: true,
			Settings: []byte(`{"url": "` + server.URL + path + `"}`), Host: "web 1", Rule: "CPU", Metric: monitor.PROC_USAGE,
		}
	}
	off := webhook(4, "/ok", 0)
	off.ChannelOn = false
	broken := webhook(5, "/ok", 0)
	broken.Settings = []byte(`{"url": "ftp://nowhere"}`)
	fake := &fakeDeliveries{
		due:     []store.Notification{webhook(1, "/ok", 0), webhook(2, "/broken", 0), webhook(3, "/broken", 5), off, broken},
		sent:    map[int64]string{},
		retries: map[int64]time.Time{},
		failed:  map[int64]string{},
	}
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	deliverDue(context.Background(), fake, "https://symon.example.com", func() time.Time { return now })

	if _, ok := fake.sent[1]; !ok || len(fake.sent) != 1 {
		t.Errorf("expected only the first to be sent, got %v", fake.sent)
	}
	if len(bodies) != 1 || bodies[0].Subject != "[Critical] CPU on web1" || bodies[0].Host != "web 1" || bodies[0].Link != "https://symon.example.com/hosts/web%201" || bodies[0].At != "2026-10-14T09:00:00Z" {
		t.Errorf("unexpected webhook body %+v", bodies)
	}
	if next, ok := fake.retries[2]; !ok || !next.Equal(now.Add(30*time.Second)) {
		t.Errorf("expected the first failure to be tried again in 30s, got %v", fake.retries)
	}
	if !strings.Contains(fake.failed[3], "receiver is down") {
		t.Errorf("expected the sixth failure to be given up on, got %q", fake.failed[3])
	}
	if fake.failed[4] != "the channel is switched off" || !strings.Contains(fake.failed[5], "settings are broken") {
		t.Errorf("expected the off and broken channels to be given up on right away, got %v", fake.failed)
	}
}

func TestAlertLink(t *testing.T) {
	cases := []struct {
		dashboard, host, metric, want string
	}{
		{"", "web1", monitor.PROC_USAGE, ""},
		{"https://symon.example.com", "web1", monitor.PROC_USAGE, "https://symon.example.com/hosts/web1"},
		{"https://symon.example.com", "", monitor.ENDPOINT, "https://symon.example.com/endpoints"},
		{"https://symon.example.com", "", monitor.CERTIFICATE, "https://symon.example.com/endpoints"},
		{"https://symon.example.com", "", "custom", "https://symon.example.com/alerts"},
	}
	for _, c := range cases {
		if got := alertLink(c.dashboard, c.host, c.metric); got != c.want {
			t.Errorf("alertLink(%q, %q, %q) = %q, want %q", c.dashboard, c.host, c.metric, got, c.want)
		}
	}
}
