package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/notify"
	"github.com/dhamith93/SyMon/internal/store"
)

// Notifications. When an alert opens, changes or resolves, one
// notification per channel is queued in the database. A loop sends what is
// due, retries what failed, and keeps the messages about one alert in order.

const (
	deliverEvery = 5 * time.Second
	deliverBatch = 20
)

// retryWaits are the waits after each failed try. After the last one the
// notification is given up on.
var retryWaits = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour}

// notice is an alert event, written for people
type notice struct {
	alertID  int64
	rule     *alerts.AlertConfig
	host     string
	target   string
	event    string
	severity int
	value    float64
	subject  string
	body     string
	at       time.Time
}

// noticeStore is the part of the store queueing uses
type noticeStore interface {
	Channels(ctx context.Context) ([]store.Channel, error)
	NotifiedChannels(ctx context.Context, alertID int64) ([]int64, error)
	QueueNotifications(ctx context.Context, n store.Notification, channelIDs []int64) error
}

func queueNotice(ctx context.Context, st noticeStore, n notice) error {
	channels, err := st.Channels(ctx)
	if err != nil {
		return err
	}
	told := map[int64]bool{}
	if n.event == notify.Resolved {
		ids, err := st.NotifiedChannels(ctx, n.alertID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			told[id] = true
		}
	}
	picked := pickChannels(n.rule, channels, n.event, n.severity, told)
	return st.QueueNotifications(ctx, store.Notification{
		AlertID:  n.alertID,
		Event:    n.event,
		Severity: n.severity,
		Value:    n.value,
		Subject:  n.subject,
		Body:     n.body,
	}, picked)
}

// pickChannels decides who hears about an alert event. A resolution goes
// to the channels told about the alert. Anything else goes to the channels
// the rule picks, or the default channels when it picks none, apart from
// warnings for channels that skip them. Channels that are off get nothing.
func pickChannels(rule *alerts.AlertConfig, channels []store.Channel, event string, severity int, told map[int64]bool) []int64 {
	picked := []int64{}
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		if event == notify.Resolved {
			if told[channel.ID] {
				picked = append(picked, channel.ID)
			}
			continue
		}
		if !ruleWants(rule, channel) {
			continue
		}
		if channel.SkipWarnings && severity < notify.SeverityCritical {
			continue
		}
		picked = append(picked, channel.ID)
	}
	return picked
}

// ruleWants says whether a rule sends to a channel. The Email, Slack and
// Pagerduty ticks from before channels mean every channel of that kind.
func ruleWants(rule *alerts.AlertConfig, channel store.Channel) bool {
	if slices.Contains(rule.Channels, channel.Name) {
		return true
	}
	switch {
	case channel.Kind == notify.Email && rule.Email,
		channel.Kind == notify.Slack && rule.Slack,
		channel.Kind == notify.PagerDuty && rule.Pagerduty:
		return true
	}
	picksNone := len(rule.Channels) == 0 && !rule.Email && !rule.Slack && !rule.Pagerduty
	return picksNone && channel.IsDefault
}

// retryAfter is how long to wait after a notification's nth failed try,
// and false once it is time to give up
func retryAfter(failures int) (time.Duration, bool) {
	if failures < 1 || failures > len(retryWaits) {
		return 0, false
	}
	return retryWaits[failures-1], true
}

// deliveryStore is the part of the store delivery uses
type deliveryStore interface {
	DueNotifications(ctx context.Context, limit int) ([]store.Notification, error)
	MarkSent(ctx context.Context, id int64, ref string) error
	MarkRetry(ctx context.Context, id int64, reason string, next time.Time) error
	MarkFailed(ctx context.Context, id int64, reason string) error
}

// deliverNotifications sends what is due every few seconds. dashboard is
// SYMON_DASHBOARD_URL, for links in the messages, or empty.
func deliverNotifications(st deliveryStore, dashboard string) {
	ticker := time.NewTicker(deliverEvery)
	defer ticker.Stop()
	for range ticker.C {
		deliverDue(context.Background(), st, dashboard, time.Now)
	}
}

// deliverDue sends one batch of due notifications, one after the other, so
// the messages about an alert keep their order
func deliverDue(ctx context.Context, st deliveryStore, dashboard string, now func() time.Time) {
	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	due, err := st.DueNotifications(listCtx, deliverBatch)
	cancel()
	if err != nil {
		logger.Log("error", "cannot read the notifications to send: "+err.Error())
		return
	}
	for _, n := range due {
		ref, retry, err := deliverOne(ctx, n, dashboard, now())
		about := "notification " + strconv.FormatInt(n.ID, 10) + " to " + n.ChannelName

		markCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err == nil {
			err = st.MarkSent(markCtx, n.ID, ref)
		} else if wait, again := retryAfter(n.Attempts + 1); retry && again {
			logger.Log("error", about+" failed, trying again in "+wait.String()+": "+err.Error())
			err = st.MarkRetry(markCtx, n.ID, err.Error(), now().Add(wait))
		} else {
			logger.Log("error", "giving up on "+about+": "+err.Error())
			err = st.MarkFailed(markCtx, n.ID, err.Error())
		}
		cancel()
		if err != nil {
			logger.Log("error", "cannot record "+about+": "+err.Error())
		}
	}
}

// deliverOne sends a notification. retry is false when trying again would
// not help, like for a channel switched off since it was queued.
func deliverOne(ctx context.Context, n store.Notification, dashboard string, at time.Time) (ref string, retry bool, err error) {
	if !n.ChannelOn {
		return "", false, errors.New("the channel is switched off")
	}
	sender, err := notify.Parse(n.ChannelKind, n.Settings)
	if err != nil {
		return "", false, fmt.Errorf("the channel's settings are broken: %w", err)
	}
	ref, err = sender.Send(ctx, message(n, dashboard, at), n.PreviousRef)
	return ref, true, err
}

// message is what a channel gets for a queued notification
func message(n store.Notification, dashboard string, at time.Time) notify.Message {
	return notify.Message{
		Event:     n.Event,
		Severity:  n.Severity,
		Subject:   n.Subject,
		Body:      n.Body,
		Host:      n.Host,
		Rule:      n.Rule,
		Metric:    n.Metric,
		Target:    n.Target,
		Value:     n.Value,
		AlertID:   n.AlertID,
		StartedAt: n.StartedAt,
		At:        at,
		Link:      alertLink(dashboard, n.Host, n.Metric),
	}
}

// alertLink points at the page that shows an alert best, when the
// dashboard's address is set
func alertLink(dashboard string, host string, metric string) string {
	switch {
	case dashboard == "":
		return ""
	case metric == monitor.ENDPOINT || metric == monitor.CERTIFICATE:
		return dashboard + "/endpoints"
	case host != "":
		return dashboard + "/hosts/" + url.PathEscape(host)
	}
	return dashboard + "/alerts"
}
