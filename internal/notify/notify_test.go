package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// awkward has everything that breaks hand-built JSON or headers
var awkward = Message{
	Event:     Opened,
	Severity:  SeverityCritical,
	Subject:   `[Critical] CPU "usage" triggered on wéb1`,
	Body:      "CPU is at 93%\nline \"two\"\tand a \\ backslash",
	Host:      "wéb1",
	Rule:      "CPU usage",
	Metric:    "procUsage",
	Value:     93.5,
	AlertID:   42,
	StartedAt: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC),
	At:        time.Date(2026, 10, 14, 8, 1, 0, 0, time.UTC),
	Link:      "https://symon.example.com/hosts/w%C3%A9b1",
}

func TestParse(t *testing.T) {
	valid := map[string]string{
		Email:     `{"host": "smtp.example.com", "user": "u", "password": "p", "from": "SyMon <symon@example.com>", "to": ["ops@example.com"]}`,
		Slack:     `{"token": "xoxb-1", "channel": "#ops"}`,
		PagerDuty: `{"routingKey": "R0UT1NG"}`,
		Webhook:   `{"url": "https://hooks.example.com/abc", "headerName": "Authorization", "headerValue": "Bearer t"}`,
	}
	for kind, settings := range valid {
		if _, err := Parse(kind, []byte(settings)); err != nil {
			t.Errorf("%s: expected valid settings, got %v", kind, err)
		}
	}
	sender, _ := Parse(Email, []byte(valid[Email]))
	if sender.(*emailSender).Port != 587 {
		t.Errorf("expected port 587 by default, got %d", sender.(*emailSender).Port)
	}

	invalid := []struct {
		kind     string
		settings string
		want     string
	}{
		{"sms", `{}`, "unknown kind"},
		{Slack, `{"token": "xoxb-1", "chanel": "#ops"}`, `unknown field "chanel"`},
		{Slack, `{"token": "", "channel": "#ops"}`, "bot token"},
		{Slack, `{"token": "xoxb-1"}`, "channel to post to"},
		{PagerDuty, `{"routingKey": " "}`, "integration key"},
		{Email, `{"host": "", "from": "a@example.com", "to": ["b@example.com"]}`, "host"},
		{Email, `{"host": "smtp", "from": "a@example.com", "to": []}`, "at least one address"},
		{Email, `{"host": "smtp", "from": "not an address", "to": ["b@example.com"]}`, "from address"},
		{Email, `{"host": "smtp", "from": "a@example.com", "to": ["b@example.com\r\nBcc: c@example.com"]}`, "the address"},
		{Email, `{"host": "smtp", "user": "u", "from": "a@example.com", "to": ["b@example.com"]}`, "needs a password"},
		{Email, `{"host": "smtp", "port": 70000, "from": "a@example.com", "to": ["b@example.com"]}`, "port"},
		{Webhook, `{"url": "ftp://example.com"}`, "http:// or https://"},
		{Webhook, `{"url": "https://"}`, "http:// or https://"},
		{Webhook, `{"url": "https://example.com", "headerValue": "x"}`, "needs a header name"},
		{Webhook, `{"url": "https://example.com", "headerName": "X Token", "headerValue": "x"}`, "header name"},
		{Webhook, `{"url": "https://example.com", "headerName": "X-Token", "headerValue": "a\r\nb"}`, "one line"},
	}
	for _, c := range invalid {
		if _, err := Parse(c.kind, []byte(c.settings)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %s: expected an error with %q, got %v", c.kind, c.settings, c.want, err)
		}
	}
}

func TestSecrets(t *testing.T) {
	settings := []byte(`{"url": "https://hooks.example.com/t0ken", "headerName": "Authorization", "headerValue": "Bearer s3cret"}`)
	redacted, set, err := Redact(Webhook, settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(redacted), "t0ken") || strings.Contains(string(redacted), "s3cret") || !strings.Contains(string(redacted), "Authorization") {
		t.Errorf("expected the secrets blanked and the rest kept, got %s", redacted)
	}
	if strings.Join(set, ",") != "url,headerValue" {
		t.Errorf("expected both secrets to be reported as set, got %v", set)
	}

	// an edit that leaves the secrets empty keeps the stored ones
	kept, err := KeepSecrets(Webhook, settings, []byte(`{"url": "", "headerName": "X-Token", "headerValue": ""}`))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	json.Unmarshal(kept, &fields)
	if fields["url"] != "https://hooks.example.com/t0ken" || fields["headerValue"] != "Bearer s3cret" || fields["headerName"] != "X-Token" {
		t.Errorf("expected the secrets kept and the name changed, got %v", fields)
	}
	// and a new value replaces one
	kept, _ = KeepSecrets(Webhook, settings, []byte(`{"url": "https://new.example.com", "headerValue": "Bearer new"}`))
	json.Unmarshal(kept, &fields)
	if fields["url"] != "https://new.example.com" || fields["headerValue"] != "Bearer new" {
		t.Errorf("expected the new secrets, got %v", fields)
	}
}

// recorder answers like a JSON API and keeps what it was sent
type recorder struct {
	mu       sync.Mutex
	requests []recorded
	answer   func(path string) (int, string)
}

type recorded struct {
	path   string
	header http.Header
	body   map[string]any
	raw    string
}

func (rec *recorder) start(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		json.Unmarshal(raw, &body)
		rec.mu.Lock()
		rec.requests = append(rec.requests, recorded{r.URL.Path, r.Header.Clone(), body, string(raw)})
		rec.mu.Unlock()
		code, answer := 200, `{"ok": true}`
		if rec.answer != nil {
			code, answer = rec.answer(r.URL.Path)
		}
		w.WriteHeader(code)
		io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSlack(t *testing.T) {
	rec := &recorder{answer: func(path string) (int, string) {
		return 200, `{"ok": true, "channel": "C0123", "ts": "1700000000.000100"}`
	}}
	server := rec.start(t)
	slackAPI = server.URL + "/api/"
	t.Cleanup(func() { slackAPI = "https://slack.com/api/" })

	sender, _ := Parse(Slack, []byte(`{"token": "xoxb-1", "channel": "#ops"}`))
	ref, err := sender.Send(context.Background(), awkward, "")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "C0123 1700000000.000100" {
		t.Errorf("expected the channel id and time stamp as the ref, got %q", ref)
	}
	first := rec.requests[0]
	if first.path != "/api/chat.postMessage" || first.header.Get("Authorization") != "Bearer xoxb-1" || first.body["channel"] != "#ops" {
		t.Errorf("unexpected first request %s %v %v", first.path, first.header, first.body)
	}
	if !strings.Contains(first.raw, `CPU \"usage\" triggered on wéb1`) || !strings.Contains(first.raw, `line \"two\"\tand a \\ backslash`) {
		t.Errorf("expected the text encoded as JSON, got %s", first.raw)
	}

	// later messages about the alert update the first one
	resolved := awkward
	resolved.Event, resolved.Severity = Resolved, SeverityResolved
	if _, err := sender.Send(context.Background(), resolved, ref); err != nil {
		t.Fatal(err)
	}
	second := rec.requests[1]
	if second.path != "/api/chat.update" || second.body["channel"] != "C0123" || second.body["ts"] != "1700000000.000100" || !strings.Contains(second.raw, slackColors["resolved"]) {
		t.Errorf("expected an update of the first message in green, got %s %s", second.path, second.raw)
	}

	rec.answer = func(string) (int, string) { return 200, `{"ok": false, "error": "channel_not_found"}` }
	if _, err := sender.Send(context.Background(), awkward, ""); err == nil || err.Error() != "the Slack API said channel_not_found" {
		t.Errorf("expected Slack's error, got %v", err)
	}
}

func TestPagerDuty(t *testing.T) {
	rec := &recorder{answer: func(string) (int, string) { return 202, `{"status": "success"}` }}
	server := rec.start(t)
	pagerDutyEvents = server.URL + "/v2/enqueue"
	t.Cleanup(func() { pagerDutyEvents = "https://events.pagerduty.com/v2/enqueue" })

	sender, _ := Parse(PagerDuty, []byte(`{"routingKey": "R0UT1NG"}`))
	if _, err := sender.Send(context.Background(), awkward, ""); err != nil {
		t.Fatal(err)
	}
	resolved := awkward
	resolved.Event, resolved.Severity = Resolved, SeverityResolved
	if _, err := sender.Send(context.Background(), resolved, ""); err != nil {
		t.Fatal(err)
	}
	trigger, resolve := rec.requests[0].body, rec.requests[1].body
	payload, _ := trigger["payload"].(map[string]any)
	if trigger["event_action"] != "trigger" || trigger["routing_key"] != "R0UT1NG" || trigger["dedup_key"] != "symon-alert-42" ||
		payload["severity"] != "critical" || payload["source"] != "wéb1" || payload["summary"] != awkward.Subject {
		t.Errorf("unexpected trigger %v", trigger)
	}
	if resolve["event_action"] != "resolve" || resolve["dedup_key"] != "symon-alert-42" || resolve["payload"] != nil {
		t.Errorf("expected a resolve of the same incident, got %v", resolve)
	}

	// a test opens nothing that stays open
	rec.requests = nil
	test := awkward
	test.Event = Test
	if _, err := sender.Send(context.Background(), test, ""); err != nil {
		t.Fatal(err)
	}
	if len(rec.requests) != 2 || rec.requests[0].body["event_action"] != "trigger" || rec.requests[1].body["event_action"] != "resolve" ||
		rec.requests[0].body["dedup_key"] != rec.requests[1].body["dedup_key"] {
		t.Errorf("expected a test to trigger and resolve one incident, got %v", rec.requests)
	}

	rec.answer = func(string) (int, string) {
		return 400, `{"status": "invalid event", "errors": ["routing key is invalid"]}`
	}
	if _, err := sender.Send(context.Background(), awkward, ""); err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "routing key is invalid") {
		t.Errorf("expected PagerDuty's answer in the error, got %v", err)
	}
}

func TestWebhook(t *testing.T) {
	rec := &recorder{}
	server := rec.start(t)
	sender, err := Parse(Webhook, []byte(`{"url": "`+server.URL+`/hook/t0ken", "headerName": "x-symon-token", "headerValue": "s3cret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Send(context.Background(), awkward, ""); err != nil {
		t.Fatal(err)
	}
	got := rec.requests[0]
	if got.path != "/hook/t0ken" || got.header.Get("X-Symon-Token") != "s3cret" || got.header.Get("Content-Type") != "application/json" {
		t.Errorf("unexpected request %s %v", got.path, got.header)
	}
	var body WebhookBody
	if err := json.Unmarshal([]byte(got.raw), &body); err != nil {
		t.Fatal(err)
	}
	want := WebhookBody{
		Event: "opened", Severity: "critical", Subject: awkward.Subject, Message: awkward.Body, Host: "wéb1", Rule: "CPU usage",
		Metric: "procUsage", Value: 93.5, AlertID: 42, StartedAt: "2026-10-14T08:00:00Z", At: "2026-10-14T08:01:00Z", Link: awkward.Link,
	}
	if body != want {
		t.Errorf("unexpected body\n got %+v\nwant %+v", body, want)
	}

	// a URL that cannot be reached is not repeated in the error either
	closed, _ := Parse(Webhook, []byte(`{"url": "http://127.0.0.1:1/hook/t0ken"}`))
	if _, err := closed.Send(context.Background(), awkward, ""); err == nil || strings.Contains(err.Error(), "t0ken") || !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("expected an error naming the host only, got %v", err)
	}

	rec.answer = func(string) (int, string) { return 500, "the receiver broke\nbadly" }
	_, err = sender.Send(context.Background(), awkward, "")
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "the receiver broke badly") || strings.Contains(err.Error(), "t0ken") {
		t.Errorf("expected the status and answer without the URL's path, got %v", err)
	}
}

// fakeSMTP takes one email the way a mail server would, without TLS
func fakeSMTP(t *testing.T) (port int, received chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	received = make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		reply := func(line string) { io.WriteString(conn, line+"\r\n") }
		reply("220 fake ready")
		var transcript strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			transcript.WriteString(line)
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"):
				reply("250-fake")
				reply("250 AUTH PLAIN")
			case strings.HasPrefix(command, "AUTH"):
				reply("235 ok")
			case strings.HasPrefix(command, "DATA"):
				reply("354 go on")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if line == ".\r\n" {
						break
					}
					transcript.WriteString(line)
				}
				reply("250 queued")
			case strings.HasPrefix(command, "QUIT"):
				reply("221 bye")
				received <- transcript.String()
				return
			default:
				reply("250 ok")
			}
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, received
}

func TestEmail(t *testing.T) {
	port, received := fakeSMTP(t)
	settings := `{"host": "127.0.0.1", "port": ` + strconv.Itoa(port) + `, "user": "symon", "password": "p",
		"from": "SyMon <symon@example.com>", "to": ["ops@example.com", "Dev Team <dev@example.com>"]}`
	sender, err := Parse(Email, []byte(settings))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sender.Send(context.Background(), awkward, ""); err != nil {
		t.Fatal(err)
	}
	var transcript string
	select {
	case transcript = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("the fake server got no email")
	}
	for _, want := range []string{
		"AUTH PLAIN",
		"MAIL FROM:<symon@example.com>",
		"RCPT TO:<ops@example.com>",
		"RCPT TO:<dev@example.com>",
		`To: <ops@example.com>, "Dev Team" <dev@example.com>`,
		"Content-Type: text/plain; charset=utf-8",
		"https://symon.example.com/hosts/w%C3%A9b1",
	} {
		if !strings.Contains(transcript, want) {
			t.Errorf("expected %q in what the server got:\n%s", want, transcript)
		}
	}
}

func TestEmailHeaders(t *testing.T) {
	sender := &emailSender{From: "symon@example.com", To: []string{"ops@example.com"}}
	msg := awkward
	msg.Subject = "[Critical] disk full\r\nBcc: someone@example.com"
	data, err := sender.compose(msg, time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	head, _, _ := strings.Cut(string(data), "\r\n\r\n")
	if strings.Contains(head, "\r\nBcc:") {
		t.Errorf("a line break in the subject started a new header:\n%s", head)
	}
	var subject string
	for _, line := range strings.Split(head, "\r\n") {
		if value, ok := strings.CutPrefix(line, "Subject: "); ok {
			subject, _ = new(mime.WordDecoder).DecodeHeader(value)
		}
	}
	if subject != "[Critical] disk full  Bcc: someone@example.com" {
		t.Errorf("unexpected subject %q", subject)
	}

	data, _ = sender.compose(awkward, time.Now())
	head, body, _ := strings.Cut(string(data), "\r\n\r\n")
	if !strings.Contains(head, "Subject: =?utf-8?q?") {
		t.Errorf("expected the subject with é to be encoded, got\n%s", head)
	}
	if !strings.Contains(body, "CPU is at 93%\r\nline \"two\"") || !strings.Contains(body, "-- SyMon") {
		t.Errorf("unexpected body %q", body)
	}
}
