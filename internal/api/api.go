package api

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UpWindow is how recently a host must have been heard from to count as up.
// Agents ping every minute and send data every monitor interval.
const UpWindow = 61 * time.Second

// errNoData is returned when a query matches nothing. It is a normal
// result, for example a host with no services configured.
var errNoData = status.Error(codes.NotFound, "no data found")

type Server struct {
	UnimplementedMonitorDataServiceServer
	Store *store.Store
}

// toStatus turns a store error into a grpc status. Unexpected errors are
// logged here and not passed on, so callers do not see database details.
func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return errNoData
	case errors.Is(err, store.ErrHostExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, store.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.Canceled):
		// the caller went away, nothing to report
		return status.Error(codes.Canceled, "canceled")
	case errors.Is(err, context.DeadlineExceeded):
		logger.Log("error", "database query timed out")
		return status.Error(codes.DeadlineExceeded, "timed out")
	default:
		logger.Log("error", err.Error())
		return status.Error(codes.Internal, "internal error")
	}
}

// agentStatus is toStatus for agent calls, where an unknown host means the
// agent was never registered
func agentStatus(host string, err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return status.Error(codes.NotFound, "agent "+host+" is not registered, run the agent with -init")
	}
	return toStatus(err)
}

// host names show up in URLs and commands, so keep them simple
var validHostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func (s *Server) Enroll(ctx context.Context, in *EnrollRequest) (*EnrollResponse, error) {
	if !validHostName.MatchString(in.HostName) {
		return nil, status.Error(codes.InvalidArgument, "host names may use letters, digits, dots, dashes and underscores, up to 63 characters")
	}
	key, err := s.Store.Enroll(ctx, in.Token, in.HostName, in.Timezone)
	if errors.Is(err, store.ErrBadToken) {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	if err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "enrolled "+in.HostName)
	return &EnrollResponse{HostName: in.HostName, AgentKey: key}, nil
}

func (s *Server) InitAgent(ctx context.Context, in *ServerInfo) (*Message, error) {
	logger.Log("info", "initializing agent for "+in.ServerName)
	if err := s.Store.AddHost(ctx, in.ServerName, in.Timezone); err != nil {
		return nil, toStatus(err)
	}
	return &Message{Body: "agent added"}, nil
}

func (s *Server) HandlePing(ctx context.Context, in *ServerInfo) (*Message, error) {
	host := in.ServerName
	if authenticated, ok := agentHost(ctx); ok {
		host = authenticated
	}
	if err := s.Store.Heartbeat(ctx, host, time.Now()); err != nil {
		return nil, agentStatus(host, err)
	}
	return &Message{Body: "pong"}, nil
}

func (s *Server) HandleMonitorData(ctx context.Context, in *MonitorData) (*Message, error) {
	var monitorData monitor.MonitorData
	if err := json.Unmarshal([]byte(in.MonitorData), &monitorData); err != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot parse monitor data: "+err.Error())
	}
	if host, ok := agentHost(ctx); ok {
		monitorData.ServerId = host
	}
	if err := s.Store.SaveSnapshot(ctx, &monitorData); err != nil {
		return nil, agentStatus(monitorData.ServerId, err)
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) HandleCustomMonitorData(ctx context.Context, in *MonitorData) (*Message, error) {
	var customMetric monitor.CustomMetric
	if err := json.Unmarshal([]byte(in.MonitorData), &customMetric); err != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot parse custom metric: "+err.Error())
	}
	if host, ok := agentHost(ctx); ok {
		customMetric.ServerId = host
	}
	if err := s.Store.SaveCustomMetric(ctx, &customMetric); err != nil {
		return nil, agentStatus(customMetric.ServerId, err)
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) Fleet(ctx context.Context, in *Void) (*FleetSummary, error) {
	summaries, err := s.Store.FleetSummary(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	now := time.Now()
	fleet := &FleetSummary{}
	for _, summary := range summaries {
		fleet.Hosts = append(fleet.Hosts, &HostSummary{
			Name:          summary.Name,
			Up:            isUp(summary.LastSeen, now),
			LastSeen:      unix(summary.LastSeen),
			Time:          unix(summary.Time),
			Os:            summary.OS,
			UptimeSeconds: summary.UptimeSeconds,
			CpuPct:        summary.CPUPct,
			MemUsedPct:    summary.MemUsedPct,
			SwapUsedPct:   summary.SwapUsedPct,
			DiskUsedPct:   summary.DiskUsedPct,
			RxBps:         summary.RxBps,
			TxBps:         summary.TxBps,
			ActiveAlerts:  int32(summary.ActiveAlerts),
			WorstSeverity: int32(summary.WorstSeverity),
		})
	}
	return fleet, nil
}

func (s *Server) Snapshot(ctx context.Context, in *HostRequest) (*HostSnapshot, error) {
	latest, err := s.Store.LatestSnapshot(ctx, in.Host)
	if err != nil {
		return nil, toStatus(err)
	}
	return &HostSnapshot{
		Host:         in.Host,
		Time:         latest.Time.Unix(),
		SnapshotJson: string(latest.Snapshot),
		LastSeen:     unix(latest.LastSeen),
		Up:           isUp(latest.LastSeen, time.Now()),
	}, nil
}

func (s *Server) QuerySeries(ctx context.Context, in *SeriesRequest) (*SeriesResponse, error) {
	result, err := s.Store.QuerySeries(ctx, store.SeriesQuery{
		Host:      in.Host,
		Metric:    in.Metric,
		Label:     in.Label,
		From:      time.Unix(in.From, 0),
		To:        time.Unix(in.To, 0),
		MaxPoints: int(in.MaxPoints),
		Max:       in.Max,
	})
	if err != nil {
		return nil, toStatus(err)
	}

	response := &SeriesResponse{Metric: in.Metric, Source: result.Source, StepSeconds: int64(result.Step.Seconds())}
	for _, series := range result.Series {
		points := make([]*Point, 0, len(series.Points))
		for _, point := range series.Points {
			points = append(points, &Point{Time: point.Time.Unix(), Value: point.Value})
		}
		response.Series = append(response.Series, &Series{Label: series.Label, Points: points})
	}
	return response, nil
}

func (s *Server) Processes(ctx context.Context, in *ProcessesRequest) (*ProcessesResponse, error) {
	at := time.Now()
	if in.At > 0 {
		at = time.Unix(in.At, 0)
	}
	snapshotTime, processes, err := s.Store.Processes(ctx, in.Host, at)
	if err != nil {
		return nil, toStatus(err)
	}
	return &ProcessesResponse{Time: snapshotTime.Unix(), ProcessesJson: string(processes)}, nil
}

func (s *Server) CustomMetricNames(ctx context.Context, in *HostRequest) (*NameList, error) {
	names, err := s.Store.CustomMetricNames(ctx, in.Host)
	if err != nil {
		return nil, toStatus(err)
	}
	return &NameList{Names: names}, nil
}

func (s *Server) Alerts(ctx context.Context, in *AlertsRequest) (*AlertList, error) {
	filter := store.AlertFilter{Host: in.Host, OpenOnly: in.OpenOnly}
	if in.From > 0 {
		filter.From = time.Unix(in.From, 0)
	}
	if in.To > 0 {
		filter.To = time.Unix(in.To, 0)
	}
	alerts, err := s.Store.Alerts(ctx, filter)
	if err != nil {
		return nil, toStatus(err)
	}

	list := &AlertList{}
	for _, alert := range alerts {
		record := &AlertRecord{
			Id:        alert.ID,
			Host:      alert.Host,
			Rule:      alert.Rule,
			Metric:    alert.Metric,
			Target:    alert.Target,
			Severity:  int32(alert.Severity),
			Value:     alert.Value,
			StartedAt: alert.StartedAt.Unix(),
			UpdatedAt: alert.UpdatedAt.Unix(),
		}
		if alert.ResolvedAt != nil {
			record.ResolvedAt = alert.ResolvedAt.Unix()
		}
		list.Alerts = append(list.Alerts, record)
	}
	return list, nil
}

func isUp(lastSeen time.Time, now time.Time) bool {
	return !lastSeen.IsZero() && now.Sub(lastSeen) <= UpWindow
}

// unix returns 0 for a zero time instead of a large negative number
func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
