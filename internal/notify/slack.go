package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// slackAPI is Slack's Web API, replaced in tests
var slackAPI = "https://slack.com/api/"

// slackSender posts with a bot token. The first message about an alert is
// posted, and later ones update it, so a channel shows one message per
// alert that turns green when it resolves.
type slackSender struct {
	Token   string `json:"token"`
	Channel string `json:"channel"`
}

func (s *slackSender) validate() error {
	s.Channel = strings.TrimSpace(s.Channel)
	switch {
	case s.Token == "":
		return errors.New("a Slack channel needs a bot token, which starts with xoxb-")
	case s.Channel == "":
		return errors.New("a Slack channel needs the channel to post to, like #ops")
	}
	return nil
}

var slackColors = map[string]string{
	"critical": "#d1242f",
	"warning":  "#d4a72c",
	"resolved": "#1a7f37",
	"test":     "#8c959f",
}

// Slack cuts a header at 150 characters and a section at 3000
func slackPayload(msg Message) map[string]any {
	text := msg.Body
	if msg.Link != "" {
		text += "\n" + msg.Link
	}
	return map[string]any{
		"text": shorten(msg.Subject, 150),
		"attachments": []any{map[string]any{
			"color": slackColors[severityName(msg.Severity, msg.Event)],
			"blocks": []any{
				map[string]any{"type": "header", "text": map[string]any{"type": "plain_text", "text": shorten(msg.Subject, 150)}},
				map[string]any{"type": "section", "text": map[string]any{"type": "plain_text", "text": cut(text, 3000)}},
			},
		}},
	}
}

// cut keeps line breaks, unlike shorten
func cut(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[:n-1]) + "…"
}

// Send returns the posted message's channel id and time stamp as its ref,
// which chat.update needs
func (s *slackSender) Send(ctx context.Context, msg Message, ref string) (string, error) {
	payload := slackPayload(msg)
	method := "chat.postMessage"
	payload["channel"] = s.Channel
	if channel, ts, ok := strings.Cut(ref, " "); ok && msg.Event != Test {
		method = "chat.update"
		payload["channel"] = channel
		payload["ts"] = ts
	}
	header := http.Header{"Authorization": {"Bearer " + s.Token}}
	answer, err := postJSON(ctx, slackAPI+method, payload, header)
	if err != nil {
		return "", err
	}
	var reply struct {
		Ok      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel string `json:"channel"`
		Ts      string `json:"ts"`
	}
	if err := json.Unmarshal(answer, &reply); err != nil {
		return "", errors.New("the Slack API answered something that is not JSON")
	}
	if !reply.Ok {
		return "", errors.New("the Slack API said " + reply.Error)
	}
	return reply.Channel + " " + reply.Ts, nil
}
