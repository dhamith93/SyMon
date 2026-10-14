package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestNotificationChannels(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	email := Channel{Name: "ops email", Kind: "email", Settings: []byte(`{"host": "smtp.example.com"}`), IsDefault: true, Enabled: true}
	id, err := st.CreateChannel(ctx, email, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateChannel(ctx, email, "alice"); !errors.Is(err, ErrChannelExists) {
		t.Errorf("expected ErrChannelExists for a taken name, got %v", err)
	}
	slack := Channel{Name: "ops slack", Kind: "slack", Settings: []byte(`{"channel": "#ops"}`), SkipWarnings: true, Enabled: true}
	slackID, err := st.CreateChannel(ctx, slack, "alice")
	if err != nil {
		t.Fatal(err)
	}

	channels, err := st.Channels(ctx)
	if err != nil || len(channels) != 2 || channels[0].Name != "ops email" || channels[1].Name != "ops slack" {
		t.Fatalf("expected both channels by name, got %+v %v", channels, err)
	}
	if !channels[0].IsDefault || channels[0].SkipWarnings || !channels[1].SkipWarnings || channels[0].UpdatedBy != "alice" || channels[0].LastStatus != "" {
		t.Errorf("unexpected channels %+v", channels)
	}

	// the name and kind stay whatever an update says
	changed := Channel{ID: id, Name: "renamed", Kind: "webhook", Settings: []byte(`{"host": "mail.example.com"}`), IsDefault: false, Enabled: false}
	if err := st.UpdateChannel(ctx, changed, "bob"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Channel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ops email" || got.Kind != "email" || string(got.Settings) != `{"host": "mail.example.com"}` || got.IsDefault || got.Enabled || got.UpdatedBy != "bob" {
		t.Errorf("unexpected channel after the update %+v", got)
	}
	if err := st.UpdateChannel(ctx, Channel{ID: 999999, Settings: []byte(`{}`)}, "bob"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound updating an unknown channel, got %v", err)
	}
	if _, err := st.Channel(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown channel, got %v", err)
	}

	if err := st.DeleteChannel(ctx, slackID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteChannel(ctx, slackID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound deleting twice, got %v", err)
	}
}

func TestNotificationQueue(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute).Truncate(time.Second)
	alertID, err := st.CreateAlert(ctx, Alert{Host: "web1", Rule: "CPU usage", Metric: "procUsage", Severity: 1, Value: 85, StartedAt: started})
	if err != nil {
		t.Fatal(err)
	}
	endpointAlert, err := st.CreateAlert(ctx, Alert{Rule: "API", Metric: "endpoint", Target: "https://api.example.com", Severity: 2, StartedAt: started})
	if err != nil {
		t.Fatal(err)
	}
	email, _ := st.CreateChannel(ctx, Channel{Name: "email", Kind: "email", Settings: []byte(`{"host": "smtp"}`), Enabled: true}, "")
	slack, _ := st.CreateChannel(ctx, Channel{Name: "slack", Kind: "slack", Settings: []byte(`{"channel": "#ops"}`), Enabled: true}, "")
	both := []int64{email, slack}

	if err := st.QueueNotifications(ctx, Notification{AlertID: alertID, Event: "opened", Severity: 1, Value: 85, Subject: "[Warning] CPU", Body: "at 85\x00%"}, both); err != nil {
		t.Fatal(err)
	}
	if err := st.QueueNotifications(ctx, Notification{AlertID: alertID, Event: "changed", Severity: 2, Value: 95, Subject: "[Critical] CPU", Body: "at 95%"}, both); err != nil {
		t.Fatal(err)
	}
	if err := st.QueueNotifications(ctx, Notification{AlertID: endpointAlert, Event: "opened", Severity: 2, Subject: "[Critical] API", Body: "down"}, []int64{email}); err != nil {
		t.Fatal(err)
	}
	if err := st.QueueNotifications(ctx, Notification{AlertID: endpointAlert}, nil); err != nil {
		t.Errorf("expected queueing for no channel to do nothing, got %v", err)
	}

	// what changed waits for what opened, per alert and channel
	due, err := st.DueNotifications(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 3 || due[0].Event != "opened" || due[1].Event != "opened" || due[2].AlertID != endpointAlert {
		t.Fatalf("expected the three opened notifications, got %+v", due)
	}
	first := due[0]
	if first.Host != "web1" || first.Rule != "CPU usage" || first.Metric != "procUsage" || !first.StartedAt.Equal(started) || first.ChannelName != "email" ||
		first.ChannelKind != "email" || !first.ChannelOn || string(first.Settings) != `{"host": "smtp"}` || first.Body != "at 85%" || first.Value != 85 || first.PreviousRef != "" {
		t.Errorf("unexpected due notification %+v", first)
	}
	if due[2].Host != "" || due[2].Target != "https://api.example.com" {
		t.Errorf("expected the endpoint alert without a host, got %+v", due[2])
	}
	if limited, err := st.DueNotifications(ctx, 1); err != nil || len(limited) != 1 {
		t.Errorf("expected the limit to apply, got %d %v", len(limited), err)
	}

	if err := st.MarkSent(ctx, due[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkSent(ctx, due[1].ID, "C0123 1700000000.000100"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkSent(ctx, due[2].ID, ""); err != nil {
		t.Fatal(err)
	}
	due, err = st.DueNotifications(ctx, 10)
	if err != nil || len(due) != 2 || due[0].Event != "changed" || due[1].Event != "changed" {
		t.Fatalf("expected the two changed notifications next, got %+v %v", due, err)
	}
	if due[0].PreviousRef != "" || due[1].PreviousRef != "C0123 1700000000.000100" {
		t.Errorf("expected Slack's ref to be handed to the next message, got %q %q", due[0].PreviousRef, due[1].PreviousRef)
	}

	// a failed try waits for its time, and gets given up on
	if err := st.MarkRetry(ctx, due[0].ID, "smtp is down", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkRetry(ctx, due[1].ID, "slack is down", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	again, err := st.DueNotifications(ctx, 10)
	if err != nil || len(again) != 1 || again[0].ID != due[0].ID || again[0].Attempts != 1 || again[0].LastError != "smtp is down" {
		t.Fatalf("expected only the email retry to be due, got %+v %v", again, err)
	}
	if err := st.MarkFailed(ctx, due[0].ID, "gave up"); err != nil {
		t.Fatal(err)
	}

	told, err := st.NotifiedChannels(ctx, alertID)
	if err != nil || !slices.Equal(told, both) {
		t.Errorf("expected both channels to have been told, got %v %v", told, err)
	}
	if told, err := st.NotifiedChannels(ctx, 999999); err != nil || len(told) != 0 {
		t.Errorf("expected no channels for an unknown alert, got %v %v", told, err)
	}

	recent, err := st.RecentNotifications(ctx, 10)
	if err != nil || len(recent) != 5 || recent[0].AlertID != endpointAlert {
		t.Fatalf("expected five notifications newest first, got %+v %v", recent, err)
	}
	statuses := map[int64]string{}
	for _, n := range recent {
		statuses[n.ID] = n.Status
		if n.Settings != nil {
			t.Error("expected no settings in the recent list")
		}
	}
	if statuses[due[0].ID] != NotificationFailed || statuses[due[1].ID] != NotificationPending || statuses[first.ID] != NotificationSent {
		t.Errorf("unexpected statuses %v", statuses)
	}

	channels, err := st.Channels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range channels {
		switch channel.Name {
		case "email":
			// the endpoint alert's went out after the CPU alert's failed
			if channel.LastStatus != NotificationSent || channel.LastError != "" || channel.LastAt.IsZero() {
				t.Errorf("expected the email channel's newest notification to have gone out, got %+v", channel)
			}
		case "slack":
			if channel.LastStatus != NotificationPending || channel.LastError != "slack is down" {
				t.Errorf("expected the slack channel to be retrying, got %+v", channel)
			}
		}
	}

	// a channel switched off still shows up, so its notifications can be given up on
	if err := st.UpdateChannel(ctx, Channel{ID: slack, Settings: []byte(`{}`), Enabled: false}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, "UPDATE notifications SET next_attempt_at = now() WHERE channel_id = $1", slack); err != nil {
		t.Fatal(err)
	}
	if due, err := st.DueNotifications(ctx, 10); err != nil || len(due) != 1 || due[0].ChannelOn {
		t.Errorf("expected the off channel's notification with ChannelOn false, got %+v %v", due, err)
	}

	// deleting a channel or an alert takes their notifications along
	if err := st.DeleteChannel(ctx, slack); err != nil {
		t.Fatal(err)
	}
	if _, err := st.pool.Exec(ctx, "DELETE FROM alerts WHERE id = $1", endpointAlert); err != nil {
		t.Fatal(err)
	}
	if recent, err := st.RecentNotifications(ctx, 10); err != nil || len(recent) != 2 {
		t.Errorf("expected only the email notifications of the CPU alert, got %d %v", len(recent), err)
	}
}
