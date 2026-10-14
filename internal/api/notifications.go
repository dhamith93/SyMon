package api

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/notify"
	"github.com/dhamith93/SyMon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Notification channels, set up by admins on the dashboard. Their secrets
// stay in the collector: the dashboard gets settings with them blanked.

const maxChannelName = 64

// testTimeout is how long a test message may take
const testTimeout = 20 * time.Second

func channelInfo(channel store.Channel) (*ChannelInfo, error) {
	settings, set, err := notify.Redact(channel.Kind, channel.Settings)
	if err != nil {
		return nil, err
	}
	return &ChannelInfo{
		Id:           channel.ID,
		Name:         channel.Name,
		Kind:         channel.Kind,
		SettingsJson: string(settings),
		SecretsSet:   set,
		IsDefault:    channel.IsDefault,
		SkipWarnings: channel.SkipWarnings,
		Enabled:      channel.Enabled,
		UpdatedAt:    unix(channel.UpdatedAt),
		UpdatedBy:    channel.UpdatedBy,
		LastStatus:   channel.LastStatus,
		LastAt:       unix(channel.LastAt),
		LastError:    channel.LastError,
	}, nil
}

func (s *Server) NotificationChannels(ctx context.Context, in *Void) (*ChannelList, error) {
	channels, err := s.Store.Channels(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	list := &ChannelList{}
	for _, channel := range channels {
		info, err := channelInfo(channel)
		if err != nil {
			return nil, toStatus(err)
		}
		list.Channels = append(list.Channels, info)
	}
	return list, nil
}

func checkChannelName(name string) error {
	switch {
	case name == "":
		return errors.New("a channel needs a name")
	case utf8.RuneCountInString(name) > maxChannelName:
		return errors.New("a channel name can be at most 64 characters")
	case strings.ContainsFunc(name, unicode.IsControl):
		return errors.New("a channel name is one line of text")
	}
	return nil
}

// SaveChannel creates a channel for id 0 and changes one otherwise. The
// settings are checked as the sender would read them.
func (s *Server) SaveChannel(ctx context.Context, in *SaveChannelRequest) (*ChannelInfo, error) {
	channel := store.Channel{
		ID:           in.Id,
		Name:         strings.TrimSpace(in.Name),
		Kind:         in.Kind,
		Settings:     []byte(in.SettingsJson),
		IsDefault:    in.IsDefault,
		SkipWarnings: in.SkipWarnings,
		Enabled:      in.Enabled,
	}
	if channel.ID != 0 {
		current, err := s.Store.Channel(ctx, channel.ID)
		if err != nil {
			return nil, toStatus(err)
		}
		// the name and kind stay, rules pick the channel by name
		channel.Name, channel.Kind = current.Name, current.Kind
		if channel.Settings, err = notify.KeepSecrets(channel.Kind, current.Settings, channel.Settings); err != nil {
			return nil, status.Error(codes.InvalidArgument, "cannot read the settings: "+err.Error())
		}
	}
	if err := checkChannelName(channel.Name); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if _, err := notify.Parse(channel.Kind, channel.Settings); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var err error
	if channel.ID == 0 {
		channel.ID, err = s.Store.CreateChannel(ctx, channel, in.By)
	} else {
		err = s.Store.UpdateChannel(ctx, channel, in.By)
	}
	if err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "notification channel "+strconv.Quote(channel.Name)+" saved by "+strconv.Quote(in.By))
	saved, err := s.Store.Channel(ctx, channel.ID)
	if err != nil {
		return nil, toStatus(err)
	}
	info, err := channelInfo(saved)
	if err != nil {
		return nil, toStatus(err)
	}
	return info, nil
}

// DeleteChannel removes a channel no rule picks
func (s *Server) DeleteChannel(ctx context.Context, in *ChannelRequest) (*Message, error) {
	channel, err := s.Store.Channel(ctx, in.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	rules, err := s.Store.AlertRules(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	using := []string{}
	for _, rule := range rules {
		if slices.Contains(rule.Rule.Channels, channel.Name) {
			using = append(using, strconv.Quote(rule.Rule.Name))
		}
	}
	if len(using) > 0 {
		return nil, status.Error(codes.FailedPrecondition, "rules send to this channel: "+strings.Join(using, ", ")+". Take it out of them first")
	}
	if err := s.Store.DeleteChannel(ctx, in.Id); err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "notification channel "+strconv.Quote(channel.Name)+" deleted by "+strconv.Quote(in.By))
	return &Message{Body: "ok"}, nil
}

// TestChannel sends a test message to a saved channel right away
func (s *Server) TestChannel(ctx context.Context, in *ChannelRequest) (*TestResult, error) {
	channel, err := s.Store.Channel(ctx, in.Id)
	if err != nil {
		return nil, toStatus(err)
	}
	sender, err := notify.Parse(channel.Kind, channel.Settings)
	if err != nil {
		return &TestResult{Error: "the settings are broken: " + err.Error()}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	now := time.Now()
	_, err = sender.Send(ctx, notify.Message{
		Event:     notify.Test,
		Subject:   "Test message from SyMon",
		Body:      "This is a test sent by " + in.By + " from the Notifications page. Alerts arrive here like this.",
		Rule:      "Test",
		StartedAt: now,
		At:        now,
	}, "")
	if err != nil {
		logger.Log("info", "test message to "+strconv.Quote(channel.Name)+" failed: "+err.Error())
		return &TestResult{Error: err.Error()}, nil
	}
	return &TestResult{Ok: true}, nil
}

func (s *Server) RecentNotifications(ctx context.Context, in *NotificationsRequest) (*NotificationList, error) {
	recent, err := s.Store.RecentNotifications(ctx, int(in.Limit))
	if err != nil {
		return nil, toStatus(err)
	}
	list := &NotificationList{}
	for _, n := range recent {
		list.Notifications = append(list.Notifications, &NotificationInfo{
			Id:        n.ID,
			AlertId:   n.AlertID,
			Channel:   n.ChannelName,
			Kind:      n.ChannelKind,
			Event:     n.Event,
			Severity:  int32(n.Severity),
			Subject:   n.Subject,
			Status:    n.Status,
			Attempts:  int32(n.Attempts),
			LastError: n.LastError,
			CreatedAt: unix(n.CreatedAt),
			SentAt:    unix(n.SentAt),
			Host:      n.Host,
			Rule:      n.Rule,
		})
	}
	return list, nil
}

// checkRuleChannels refuses a rule that picks channels that do not exist
func (s *Server) checkRuleChannels(ctx context.Context, picked []string) error {
	if len(picked) == 0 {
		return nil
	}
	channels, err := s.Store.Channels(ctx)
	if err != nil {
		return toStatus(err)
	}
	for _, name := range picked {
		if !slices.ContainsFunc(channels, func(c store.Channel) bool { return c.Name == strings.TrimSpace(name) }) {
			return status.Error(codes.InvalidArgument, "there is no notification channel named "+strconv.Quote(name))
		}
	}
	return nil
}
