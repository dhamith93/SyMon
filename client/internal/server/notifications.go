package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dhamith93/SyMon/internal/api"
)

// Notification channels and what was sent to them. Everyone logged in sees
// the channels and the deliveries. Settings, which never include secrets,
// and error messages are for admins, and only admins change channels.

type channel struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	SecretsSet   []string        `json:"secretsSet,omitempty"`
	IsDefault    bool            `json:"isDefault"`
	SkipWarnings bool            `json:"skipWarnings"`
	Enabled      bool            `json:"enabled"`
	UpdatedAt    int64           `json:"updatedAt"`
	UpdatedBy    string          `json:"updatedBy"`
	LastStatus   string          `json:"lastStatus"`
	LastAt       int64           `json:"lastAt"`
	LastError    string          `json:"lastError,omitempty"`
}

func toChannel(info *api.ChannelInfo, admin bool) channel {
	out := channel{
		ID:           info.Id,
		Name:         info.Name,
		Kind:         info.Kind,
		IsDefault:    info.IsDefault,
		SkipWarnings: info.SkipWarnings,
		Enabled:      info.Enabled,
		UpdatedAt:    info.UpdatedAt,
		UpdatedBy:    info.UpdatedBy,
		LastStatus:   info.LastStatus,
		LastAt:       info.LastAt,
	}
	if admin {
		out.Settings = json.RawMessage(info.SettingsJson)
		out.SecretsSet = info.SecretsSet
		if out.SecretsSet == nil {
			out.SecretsSet = []string{}
		}
		out.LastError = info.LastError
	}
	return out
}

func (s *server) getChannels(w http.ResponseWriter, r *http.Request) {
	response, err := s.collector.NotificationChannels(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "notification channels", err)
		return
	}
	admin := s.isAdmin(r)
	channels := make([]channel, 0, len(response.Channels))
	for _, info := range response.Channels {
		channels = append(channels, toChannel(info, admin))
	}
	writeJSON(w, map[string]any{"channels": channels})
}

func (s *server) postChannel(w http.ResponseWriter, r *http.Request) {
	s.saveChannel(w, r, 0)
}

func (s *server) putChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := channelID(w, r)
	if ok {
		s.saveChannel(w, r, id)
	}
}

func channelID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "no such channel")
		return 0, false
	}
	return id, true
}

// saveChannel creates a channel for id 0 and changes one otherwise. A
// secret left empty keeps the saved one.
func (s *server) saveChannel(w http.ResponseWriter, r *http.Request, id int64) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the channel as JSON")
		return
	}
	var body struct {
		Name         string          `json:"name"`
		Kind         string          `json:"kind"`
		Settings     json.RawMessage `json:"settings"`
		IsDefault    bool            `json:"isDefault"`
		SkipWarnings bool            `json:"skipWarnings"`
		Enabled      bool            `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || len(body.Settings) == 0 {
		writeError(w, http.StatusBadRequest, "send the channel's name, kind and settings")
		return
	}
	session, _ := s.sessionOf(r)
	saved, err := s.collector.SaveChannel(r.Context(), &api.SaveChannelRequest{
		Id:           id,
		Name:         body.Name,
		Kind:         body.Kind,
		SettingsJson: string(body.Settings),
		IsDefault:    body.IsDefault,
		SkipWarnings: body.SkipWarnings,
		Enabled:      body.Enabled,
		By:           session.user,
	})
	if err != nil {
		writeGRPCError(w, "saving a notification channel", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if id == 0 {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(toChannel(saved, true))
}

func (s *server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := channelID(w, r)
	if !ok {
		return
	}
	session, _ := s.sessionOf(r)
	if _, err := s.collector.DeleteChannel(r.Context(), &api.ChannelRequest{Id: id, By: session.user}); err != nil {
		writeGRPCError(w, "deleting a notification channel", err)
		return
	}
	writeJSON(w, map[string]int64{"id": id})
}

// testChannel sends a test message. A failed test is an answer, not an
// error: it says what the channel's service said.
func (s *server) testChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := channelID(w, r)
	if !ok {
		return
	}
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the test as JSON")
		return
	}
	session, _ := s.sessionOf(r)
	result, err := s.collector.TestChannel(r.Context(), &api.ChannelRequest{Id: id, By: session.user})
	if err != nil {
		writeGRPCError(w, "testing a notification channel", err)
		return
	}
	writeJSON(w, map[string]any{"ok": result.Ok, "error": result.Error})
}

type notification struct {
	ID        int64  `json:"id"`
	AlertID   int64  `json:"alertId"`
	Channel   string `json:"channel"`
	Kind      string `json:"kind"`
	Event     string `json:"event"`
	Severity  int32  `json:"severity"`
	Subject   string `json:"subject"`
	Status    string `json:"status"`
	Attempts  int32  `json:"attempts"`
	LastError string `json:"lastError,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	SentAt    int64  `json:"sentAt"`
	Host      string `json:"host"`
	Rule      string `json:"rule"`
}

func (s *server) getNotifications(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	response, err := s.collector.RecentNotifications(r.Context(), &api.NotificationsRequest{Limit: int32(limit)})
	if err != nil {
		writeGRPCError(w, "notifications", err)
		return
	}
	admin := s.isAdmin(r)
	list := make([]notification, 0, len(response.Notifications))
	for _, n := range response.Notifications {
		item := notification{
			ID: n.Id, AlertID: n.AlertId, Channel: n.Channel, Kind: n.Kind, Event: n.Event, Severity: n.Severity,
			Subject: n.Subject, Status: n.Status, Attempts: n.Attempts, CreatedAt: n.CreatedAt, SentAt: n.SentAt, Host: n.Host, Rule: n.Rule,
		}
		if admin {
			item.LastError = n.LastError
		}
		list = append(list, item)
	}
	writeJSON(w, map[string]any{"notifications": list})
}
