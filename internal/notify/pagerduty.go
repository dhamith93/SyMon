package notify

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

// pagerDutyEvents is PagerDuty's Events API v2, replaced in tests
var pagerDutyEvents = "https://events.pagerduty.com/v2/enqueue"

// pagerDutySender raises an incident through an integration key, the
// Events API v2 one of a PagerDuty service. One incident per alert: it is
// triggered when the alert opens or changes, and resolved with it.
type pagerDutySender struct {
	RoutingKey string `json:"routingKey"`
}

func (p *pagerDutySender) validate() error {
	p.RoutingKey = strings.TrimSpace(p.RoutingKey)
	if p.RoutingKey == "" {
		return errors.New("a PagerDuty channel needs the integration key of an Events API v2 integration")
	}
	return nil
}

func (p *pagerDutySender) Send(ctx context.Context, msg Message, _ string) (string, error) {
	dedupKey := "symon-alert-" + strconv.FormatInt(msg.AlertID, 10)
	if msg.Event == Test {
		dedupKey = "symon-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if msg.Event == Resolved {
		_, err := postJSON(ctx, pagerDutyEvents, map[string]any{
			"routing_key":  p.RoutingKey,
			"event_action": "resolve",
			"dedup_key":    dedupKey,
		}, nil)
		return "", err
	}

	source := msg.Host
	if source == "" {
		source = msg.Target
	}
	if source == "" {
		source = "SyMon"
	}
	severity := severityName(msg.Severity, msg.Event)
	if severity == "test" {
		severity = "info"
	}
	event := map[string]any{
		"routing_key":  p.RoutingKey,
		"event_action": "trigger",
		"dedup_key":    dedupKey,
		"payload": map[string]any{
			"summary":   shorten(msg.Subject, 1024),
			"source":    source,
			"severity":  severity,
			"component": msg.Metric,
			"class":     msg.Rule,
			"custom_details": map[string]any{
				"message": msg.Body,
				"value":   msg.Value,
			},
		},
		"client": "SyMon",
	}
	if msg.Link != "" {
		event["client_url"] = msg.Link
		event["links"] = []any{map[string]any{"href": msg.Link, "text": "Open in SyMon"}}
	}
	if _, err := postJSON(ctx, pagerDutyEvents, event, nil); err != nil {
		return "", err
	}
	// a test should not leave an incident open
	if msg.Event == Test {
		_, err := postJSON(ctx, pagerDutyEvents, map[string]any{
			"routing_key":  p.RoutingKey,
			"event_action": "resolve",
			"dedup_key":    dedupKey,
		}, nil)
		return "", err
	}
	return "", nil
}
