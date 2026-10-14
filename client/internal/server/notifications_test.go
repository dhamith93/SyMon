package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// channel 1 is an email channel with a saved password that last failed,
// channel 2 a webhook that rules send to

func fakeChannel() *api.ChannelInfo {
	return &api.ChannelInfo{
		Id: 1, Name: "ops email", Kind: "email", SettingsJson: `{"host":"smtp.example.com","password":""}`, SecretsSet: []string{"password"},
		IsDefault: true, Enabled: true, UpdatedAt: 1700000000, UpdatedBy: "tester", LastStatus: "failed", LastAt: 1700000100,
		LastError: "smtp.example.com refused the login",
	}
}

func (f *fakeCollector) NotificationChannels(ctx context.Context, in *api.Void) (*api.ChannelList, error) {
	return &api.ChannelList{Channels: []*api.ChannelInfo{fakeChannel()}}, nil
}

func (f *fakeCollector) SaveChannel(ctx context.Context, in *api.SaveChannelRequest) (*api.ChannelInfo, error) {
	f.lastChannel.Store(in)
	switch {
	case in.Name == "taken":
		return nil, status.Error(codes.AlreadyExists, "there is already a channel with that name")
	case strings.Contains(in.SettingsJson, "ftp://"):
		return nil, status.Error(codes.InvalidArgument, "a webhook URL starts with http:// or https://")
	}
	saved := fakeChannel()
	saved.Id, saved.Name, saved.Kind = 3, in.Name, in.Kind
	return saved, nil
}

func (f *fakeCollector) DeleteChannel(ctx context.Context, in *api.ChannelRequest) (*api.Message, error) {
	if in.Id == 2 {
		return nil, status.Error(codes.FailedPrecondition, `rules send to this channel: "CPU". Take it out of them first`)
	}
	return &api.Message{Body: "ok"}, nil
}

func (f *fakeCollector) TestChannel(ctx context.Context, in *api.ChannelRequest) (*api.TestResult, error) {
	if in.Id == 1 {
		return &api.TestResult{Error: "smtp.example.com refused the login"}, nil
	}
	return &api.TestResult{Ok: true}, nil
}

func (f *fakeCollector) RecentNotifications(ctx context.Context, in *api.NotificationsRequest) (*api.NotificationList, error) {
	return &api.NotificationList{Notifications: []*api.NotificationInfo{{
		Id: 9, AlertId: 4, Channel: "ops email", Kind: "email", Event: "opened", Severity: 2, Subject: "[Critical] CPU on web1",
		Status: "pending", Attempts: 1, LastError: "smtp.example.com refused the login", CreatedAt: 1700000000, Host: "web1", Rule: "CPU",
	}}}, nil
}

func TestChannels(t *testing.T) {
	s, _ := newTestServer(t, nil)

	rec := call(s, "GET", "/api/v1/notifications/channels", "", testSession, nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `"settings":{"host":"smtp.example.com","password":""}`) || !strings.Contains(body, `"secretsSet":["password"]`) ||
		!strings.Contains(body, `"lastError":"smtp.example.com refused the login"`) || !strings.Contains(body, `"isDefault":true`) {
		t.Errorf("unexpected channels for an admin: %d %s", rec.Code, body)
	}
	// viewers see that a channel exists and how it is doing, not how it is set up
	rec = call(s, "GET", "/api/v1/notifications/channels", "", viewerSession, nil)
	body = rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "settings") || strings.Contains(body, "secretsSet") || strings.Contains(body, "refused") ||
		!strings.Contains(body, `"name":"ops email"`) || !strings.Contains(body, `"lastStatus":"failed"`) {
		t.Errorf("unexpected channels for a viewer: %d %s", rec.Code, body)
	}
	if rec := call(s, "GET", "/api/v1/notifications/channels", "", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a login, got %d", rec.Code)
	}
}

func TestSaveChannel(t *testing.T) {
	s, fake := newTestServer(t, nil)
	body := `{"name": "hook", "kind": "webhook", "settings": {"url": "https://hooks.example.com/x"}, "isDefault": true, "enabled": true}`

	rec := call(s, "POST", "/api/v1/notifications/channels", body, testSession, asJSON)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"id":3`) || !strings.Contains(rec.Body.String(), `"settings"`) {
		t.Fatalf("unexpected response %d %s", rec.Code, rec.Body)
	}
	saved := fake.lastChannel.Load().(*api.SaveChannelRequest)
	if saved.Id != 0 || saved.Name != "hook" || saved.Kind != "webhook" || saved.SettingsJson != `{"url": "https://hooks.example.com/x"}` ||
		!saved.IsDefault || saved.SkipWarnings || !saved.Enabled || saved.By != "tester" {
		t.Errorf("unexpected request to the collector %+v", saved)
	}

	rec = call(s, "PUT", "/api/v1/notifications/channels/1", body, testSession, asJSON)
	if rec.Code != 200 || fake.lastChannel.Load().(*api.SaveChannelRequest).Id != 1 {
		t.Errorf("expected an update of channel 1, got %d %s", rec.Code, rec.Body)
	}

	tests := []struct {
		method, url, body, cookie string
		prepare                   func(*http.Request)
		code                      int
		want                      string
	}{
		{"POST", "/api/v1/notifications/channels", body, viewerSession, asJSON, http.StatusForbidden, "only admins"},
		{"POST", "/api/v1/notifications/channels", body, testSession, nil, http.StatusUnsupportedMediaType, "JSON"},
		{"POST", "/api/v1/notifications/channels", `{"name": "hook", "kind": "webhook"}`, testSession, asJSON, http.StatusBadRequest, "settings"},
		{"POST", "/api/v1/notifications/channels", strings.Replace(body, `"hook"`, `"taken"`, 1), testSession, asJSON, http.StatusConflict, "already a channel"},
		{"POST", "/api/v1/notifications/channels", strings.Replace(body, "https://", "ftp://", 1), testSession, asJSON, http.StatusBadRequest, "http:// or https://"},
		{"PUT", "/api/v1/notifications/channels/abc", body, testSession, asJSON, http.StatusNotFound, "no such channel"},
		{"DELETE", "/api/v1/notifications/channels/2", "", testSession, nil, http.StatusConflict, `rules send to this channel: \"CPU\"`},
		{"DELETE", "/api/v1/notifications/channels/1", "", viewerSession, nil, http.StatusForbidden, "only admins"},
	}
	for _, tt := range tests {
		rec := call(s, tt.method, tt.url, tt.body, tt.cookie, tt.prepare)
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s %s as %q: got %d %s, want %d with %q", tt.method, tt.url, tt.cookie, rec.Code, rec.Body, tt.code, tt.want)
		}
	}
	if rec := call(s, "DELETE", "/api/v1/notifications/channels/1", "", testSession, nil); rec.Code != 200 {
		t.Errorf("expected the delete to go through, got %d %s", rec.Code, rec.Body)
	}
}

func TestTestChannel(t *testing.T) {
	s, _ := newTestServer(t, nil)
	rec := call(s, "POST", "/api/v1/notifications/channels/1/test", "{}", testSession, asJSON)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"error":"smtp.example.com refused the login","ok":false}` {
		t.Errorf("expected the failed test as an answer, got %d %s", rec.Code, rec.Body)
	}
	rec = call(s, "POST", "/api/v1/notifications/channels/3/test", "{}", testSession, asJSON)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"error":"","ok":true}` {
		t.Errorf("expected a passing test, got %d %s", rec.Code, rec.Body)
	}
	// a test sends a message, so a page on another site must not be able to start one
	if rec := call(s, "POST", "/api/v1/notifications/channels/3/test", "", testSession, nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 without JSON, got %d", rec.Code)
	}
	if rec := call(s, "POST", "/api/v1/notifications/channels/3/test", "{}", viewerSession, asJSON); rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for a viewer, got %d", rec.Code)
	}
}

func TestNotifications(t *testing.T) {
	s, _ := newTestServer(t, nil)
	rec := call(s, "GET", "/api/v1/notifications", "", testSession, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"lastError":"smtp.example.com refused the login"`) || !strings.Contains(rec.Body.String(), `"subject":"[Critical] CPU on web1"`) {
		t.Errorf("unexpected notifications for an admin: %d %s", rec.Code, rec.Body)
	}
	rec = call(s, "GET", "/api/v1/notifications", "", viewerSession, nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "refused") || !strings.Contains(rec.Body.String(), `"status":"pending"`) {
		t.Errorf("unexpected notifications for a viewer: %d %s", rec.Code, rec.Body)
	}
}
