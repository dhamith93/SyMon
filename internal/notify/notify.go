// Package notify sends alerts out of SyMon: by email, to Slack, to
// PagerDuty or to any URL as JSON. A channel is a kind plus its settings,
// which admins set up on the dashboard.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/strictjson"
)

// Kinds of channel
const (
	Email     = "email"
	Slack     = "slack"
	PagerDuty = "pagerduty"
	Webhook   = "webhook"
)

// Kinds lists every kind of channel
var Kinds = []string{Email, Slack, PagerDuty, Webhook}

// What happened to an alert
const (
	Opened   = "opened"
	Changed  = "changed"
	Resolved = "resolved"
	// Test is a message sent with Send test on the dashboard
	Test = "test"
)

// Severities, as the alerts table keeps them
const (
	SeverityResolved = 0
	SeverityWarning  = 1
	SeverityCritical = 2
)

// sendTimeout is how long one message may take to go out
const sendTimeout = 15 * time.Second

// Message is one alert event, written for people. Host is empty for
// endpoint alerts, Link too when the dashboard's address is not known.
type Message struct {
	Event     string
	Severity  int
	Subject   string
	Body      string
	Host      string
	Rule      string
	Metric    string
	Target    string
	Value     float64
	AlertID   int64
	StartedAt time.Time
	At        time.Time
	Link      string
}

// Sender delivers messages to one channel
type Sender interface {
	// Send delivers msg. ref is what an earlier Send for the same alert
	// returned, for channels that update their first message instead of
	// posting another. Send returns the ref to keep.
	Send(ctx context.Context, msg Message, ref string) (string, error)
}

// Parse checks a channel's settings and returns its sender. Unknown fields
// are errors, like in the other files people write.
func Parse(kind string, settings []byte) (Sender, error) {
	var sender interface {
		Sender
		validate() error
	}
	switch kind {
	case Email:
		sender = &emailSender{}
	case Slack:
		sender = &slackSender{}
	case PagerDuty:
		sender = &pagerDutySender{}
	case Webhook:
		sender = &webhookSender{}
	default:
		return nil, fmt.Errorf("unknown kind of channel %q, it is one of %s", kind, strings.Join(Kinds, ", "))
	}
	if err := strictjson.Unmarshal(settings, sender); err != nil {
		return nil, err
	}
	if err := sender.validate(); err != nil {
		return nil, err
	}
	return sender, nil
}

// secrets are the settings never sent back to the dashboard
var secrets = map[string][]string{
	Email:     {"password"},
	Slack:     {"token"},
	PagerDuty: {"routingKey"},
	// a webhook URL often carries a token, like Discord's do
	Webhook: {"url", "headerValue"},
}

// Redact blanks a channel's secrets, for showing its settings, and says
// which of them are set
func Redact(kind string, settings []byte) ([]byte, []string, error) {
	fields := map[string]any{}
	if err := json.Unmarshal(settings, &fields); err != nil {
		return nil, nil, err
	}
	set := []string{}
	for _, name := range secrets[kind] {
		if value, ok := fields[name].(string); ok && value != "" {
			set = append(set, name)
			fields[name] = ""
		}
	}
	redacted, err := json.Marshal(fields)
	return redacted, set, err
}

// KeepSecrets fills the secrets left empty in next with the ones in
// current, so a channel can be edited without typing its password again
func KeepSecrets(kind string, current []byte, next []byte) ([]byte, error) {
	old := map[string]any{}
	if err := json.Unmarshal(current, &old); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if err := json.Unmarshal(next, &fields); err != nil {
		return nil, err
	}
	for _, name := range secrets[kind] {
		if value, _ := fields[name].(string); value == "" && old[name] != nil {
			fields[name] = old[name]
		}
	}
	return json.Marshal(fields)
}

// severityName is how a severity reads in a message
func severityName(severity int, event string) string {
	switch {
	case event == Test:
		return "test"
	case severity >= SeverityCritical:
		return "critical"
	case severity == SeverityWarning:
		return "warning"
	}
	return "resolved"
}

var httpClient = &http.Client{Timeout: sendTimeout}

// postJSON sends body to target and fails on anything but a 2xx answer.
// It returns the answer's body. Errors only name the target's host, since
// a URL can carry a token.
func postJSON(ctx context.Context, target string, body any, header http.Header) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("cannot post to %s", hostOf(target))
	}
	for name, values := range header {
		request.Header[name] = values
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		var failed *url.Error
		if errors.As(err, &failed) {
			return nil, fmt.Errorf("%s to %s: %w", failed.Op, hostOf(target), failed.Err)
		}
		return nil, err
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return answer, fmt.Errorf("%s answered %s: %s", hostOf(target), response.Status, shorten(string(answer), 200))
	}
	return answer, nil
}

// hostOf keeps a URL's secrets, like a token in its path, out of errors
func hostOf(url string) string {
	rest := url
	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// shorten cuts text to at most n characters on one line
func shorten(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[:n-1]) + "…"
}
