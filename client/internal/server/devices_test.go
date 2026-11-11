package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	coreSwitch = &api.DeviceSummary{Name: "core-switch", Kind: "switch", Address: "10.0.0.2:161", Host: "pve", ReportedAt: 1700000030, IntervalSeconds: 60,
		UptimeSeconds: 864000, Ports: 24, PortsUp: 9, RxBps: 1.5e6, TxBps: 2.5e5}
	rackUPS = &api.DeviceSummary{Name: "rack ups", Kind: "ups", Address: "10.0.0.3:161", Host: "pve", ReportedAt: 1700000030, IntervalSeconds: 60,
		Error: "cannot reach 10.0.0.3:161: request timeout", UpsChargePct: proto.Float64(100), UpsOnBattery: proto.Bool(false)}
)

func (f *fakeCollector) Devices(ctx context.Context, in *api.Void) (*api.DeviceList, error) {
	return &api.DeviceList{Devices: []*api.DeviceSummary{coreSwitch, rackUPS}}, nil
}

func (f *fakeCollector) Device(ctx context.Context, in *api.DeviceRequest) (*api.DeviceInfo, error) {
	switch in.Name {
	case "core-switch":
		return &api.DeviceInfo{Summary: coreSwitch, StateJson: `{"Device":"core-switch","Interfaces":[{"Name":"Gi1/0/24","Up":true}]}`}, nil
	case "rack ups":
		return &api.DeviceInfo{Summary: rackUPS, StateJson: `{"Device":"rack ups"}`}, nil
	}
	return nil, status.Error(codes.NotFound, "no data")
}

func (f *fakeCollector) DeviceSeries(ctx context.Context, in *api.DeviceSeriesRequest) (*api.SeriesResponse, error) {
	if in.Name != "core-switch" {
		return nil, status.Error(codes.NotFound, "no data")
	}
	if in.Metric == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request: a metric is needed, like if_rx_bps")
	}
	return &api.SeriesResponse{Metric: in.Metric, Source: "raw", StepSeconds: 60, Series: []*api.Series{
		{Label: "Gi1/0/24", Points: []*api.Point{{Time: 1700000000, Value: 1.5e6}}},
	}}, nil
}

func (f *fakeCollector) RemoveDevice(ctx context.Context, in *api.DeviceRequest) (*api.Message, error) {
	if in.Name != "core-switch" {
		return nil, status.Error(codes.NotFound, "no data")
	}
	return &api.Message{Body: "ok"}, nil
}

func TestDevices(t *testing.T) {
	s, _ := newTestServer(t, nil)

	// viewers see devices like admins
	for _, session := range []string{testSession, viewerSession} {
		rec := call(s, "GET", "/api/v1/devices", "", session, nil)
		body := rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, `"name":"core-switch"`) || !strings.Contains(body, `"portsUp":9`) || !strings.Contains(body, `"cpuPct":null`) ||
			!strings.Contains(body, `"upsChargePct":100`) || !strings.Contains(body, `"upsOnBattery":false`) || !strings.Contains(body, "request timeout") {
			t.Errorf("unexpected devices for %s: %d %s", session, rec.Code, body)
		}
		rec = call(s, "GET", "/api/v1/devices/core-switch", "", session, nil)
		body = rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, `"state":{"Device":"core-switch","Interfaces":[{"Name":"Gi1/0/24","Up":true}]}`) || !strings.Contains(body, `"rxBps":1500000`) {
			t.Errorf("unexpected device for %s: %d %s", session, rec.Code, body)
		}
	}

	rec := call(s, "GET", "/api/v1/devices/rack%20ups", "", viewerSession, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"rack ups"`) {
		t.Errorf("unexpected device with a space in its name: %d %s", rec.Code, rec.Body)
	}
	rec = call(s, "GET", "/api/v1/devices/core-switch/series?metric=if_rx_bps&from=1699990000&to=1700000100", "", viewerSession, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"label":"Gi1/0/24"`) {
		t.Errorf("unexpected series %d %s", rec.Code, rec.Body)
	}

	tests := []struct {
		method, url, cookie string
		code                int
		want                string
	}{
		{"GET", "/api/v1/devices/sw9", testSession, http.StatusNotFound, ""},
		{"GET", "/api/v1/devices/core-switch/series", testSession, http.StatusBadRequest, "a metric is needed"},
		{"GET", "/api/v1/devices/core-switch/series?metric=if_rx_bps&from=soon", testSession, http.StatusBadRequest, ""},
		{"GET", "/api/v1/devices", "", http.StatusUnauthorized, ""},
		{"DELETE", "/api/v1/devices/core-switch", viewerSession, http.StatusForbidden, "only admins"},
		{"DELETE", "/api/v1/devices/sw9", testSession, http.StatusNotFound, ""},
		{"DELETE", "/api/v1/devices/core-switch", testSession, 200, `{"name":"core-switch"}`},
	}
	for _, tt := range tests {
		rec := call(s, tt.method, tt.url, "", tt.cookie, nil)
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s %s as %q: got %d %s, want %d with %q", tt.method, tt.url, tt.cookie, rec.Code, rec.Body, tt.code, tt.want)
		}
	}
}
