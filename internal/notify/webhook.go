package notify

import (
	"context"
	"errors"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

// webhookSender POSTs every message as JSON, for anything that takes a
// webhook: Home Assistant, n8n, a chat bridge or a script. Any 2xx answer
// counts as delivered. An optional header carries a token.
type webhookSender struct {
	URL         string `json:"url"`
	HeaderName  string `json:"headerName"`
	HeaderValue string `json:"headerValue"`
}

func (w *webhookSender) validate() error {
	w.URL = strings.TrimSpace(w.URL)
	w.HeaderName = strings.TrimSpace(w.HeaderName)
	parsed, err := url.Parse(w.URL)
	switch {
	case w.URL == "":
		return errors.New("a webhook needs a URL")
	case err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "":
		return errors.New("a webhook URL starts with http:// or https://")
	case w.HeaderName == "" && w.HeaderValue != "":
		return errors.New("a header value needs a header name")
	case w.HeaderName != "" && !validHeaderName(w.HeaderName):
		return errors.New("the header name can only use letters, digits and dashes")
	case strings.ContainsAny(w.HeaderValue, "\r\n"):
		return errors.New("the header value is one line")
	}
	return nil
}

func validHeaderName(name string) bool {
	for _, r := range name {
		if !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// WebhookBody is the JSON a webhook gets. Times are RFC 3339 in UTC. Host
// is empty for endpoint alerts.
type WebhookBody struct {
	Event     string  `json:"event"`
	Severity  string  `json:"severity"`
	Subject   string  `json:"subject"`
	Message   string  `json:"message"`
	Host      string  `json:"host"`
	Rule      string  `json:"rule"`
	Metric    string  `json:"metric"`
	Target    string  `json:"target"`
	Value     float64 `json:"value"`
	AlertID   int64   `json:"alertId"`
	StartedAt string  `json:"startedAt"`
	At        string  `json:"at"`
	Link      string  `json:"link"`
}

func timeText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (w *webhookSender) Send(ctx context.Context, msg Message, _ string) (string, error) {
	body := WebhookBody{
		Event:     msg.Event,
		Severity:  severityName(msg.Severity, msg.Event),
		Subject:   msg.Subject,
		Message:   msg.Body,
		Host:      msg.Host,
		Rule:      msg.Rule,
		Metric:    msg.Metric,
		Target:    msg.Target,
		Value:     msg.Value,
		AlertID:   msg.AlertID,
		StartedAt: timeText(msg.StartedAt),
		At:        timeText(msg.At),
		Link:      msg.Link,
	}
	header := http.Header{}
	if w.HeaderName != "" {
		header[textproto.CanonicalMIMEHeaderKey(w.HeaderName)] = []string{w.HeaderValue}
	}
	_, err := postJSON(ctx, w.URL, body, header)
	return "", err
}
