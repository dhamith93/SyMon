package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/snmp"
	"github.com/dhamith93/SyMon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s *Server) ReportDevice(ctx context.Context, in *DeviceReport) (*Message, error) {
	var report snmp.DeviceReport
	if err := json.Unmarshal([]byte(in.ReportJson), &report); err != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot parse the device report: "+err.Error())
	}
	host := in.Host
	if agent, ok := agentHost(ctx); ok {
		host = agent
	}
	if err := s.Store.SaveDeviceReport(ctx, host, report); err != nil {
		return nil, agentStatus(host, err)
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) Devices(ctx context.Context, in *Void) (*DeviceList, error) {
	devices, err := s.Store.Devices(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	list := &DeviceList{}
	now := time.Now()
	for _, device := range devices {
		list.Devices = append(list.Devices, deviceSummary(device, now))
	}
	return list, nil
}

// deviceSummary is what the devices list shows, worked out from the newest
// report so the page needs no series
func deviceSummary(device store.Device, now time.Time) *DeviceSummary {
	summary := &DeviceSummary{
		Name:            device.Name,
		Kind:            device.Kind,
		Address:         device.Address,
		Host:            device.Host,
		ReportedAt:      device.ReportedAt.Unix(),
		IntervalSeconds: int32(device.Interval / time.Second),
		Error:           device.Error,
		Stale:           device.Stale(now),
		UptimeSeconds:   device.State.System.UptimeSeconds,
		Ports:           int32(len(device.State.Interfaces)),
	}
	for _, port := range device.State.Interfaces {
		if port.Up {
			summary.PortsUp++
		}
	}
	for _, v := range device.State.Values {
		switch v.Metric {
		case snmp.IfRxBps:
			summary.RxBps += v.Value
		case snmp.IfTxBps:
			summary.TxBps += v.Value
		case snmp.CPUPct:
			summary.CpuPct = proto.Float64(v.Value)
		case snmp.MemUsedPct:
			summary.MemUsedPct = proto.Float64(v.Value)
		case snmp.UPSChargePct:
			summary.UpsChargePct = proto.Float64(v.Value)
		case snmp.UPSLoadPct:
			summary.UpsLoadPct = proto.Float64(v.Value)
		case snmp.UPSMinutesLeft:
			summary.UpsMinutesLeft = proto.Float64(v.Value)
		case snmp.UPSOnBattery:
			summary.UpsOnBattery = proto.Bool(v.Value > 0)
		}
	}
	return summary
}

// Device returns a device's newest report
func (s *Server) Device(ctx context.Context, in *DeviceRequest) (*DeviceInfo, error) {
	device, err := s.Store.Device(ctx, in.Name)
	if err != nil {
		return nil, toStatus(err)
	}
	state, err := json.Marshal(device.State)
	if err != nil {
		return nil, toStatus(err)
	}
	return &DeviceInfo{Summary: deviceSummary(device, time.Now()), StateJson: string(state)}, nil
}

func (s *Server) DeviceSeries(ctx context.Context, in *DeviceSeriesRequest) (*SeriesResponse, error) {
	result, err := s.Store.DeviceSeries(ctx, in.Name, in.Metric, time.Unix(in.From, 0), time.Unix(in.To, 0), int(in.MaxPoints))
	if err != nil {
		return nil, toStatus(err)
	}
	return seriesResponse(in.Metric, result), nil
}

func (s *Server) RemoveDevice(ctx context.Context, in *DeviceRequest) (*Message, error) {
	if err := s.Store.RemoveDevice(ctx, in.Name); err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "device "+in.Name+" removed")
	return &Message{Body: "ok"}, nil
}
