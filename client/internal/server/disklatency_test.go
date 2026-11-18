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

func (f *fakeCollector) DiskLatency(ctx context.Context, in *api.DiskLatencyRequest) (*api.DiskLatencyReport, error) {
	if in.Host != "web1" {
		return nil, status.Error(codes.NotFound, "host "+in.Host+": not found")
	}
	if in.Op != "read" && in.Op != "write" {
		return nil, status.Error(codes.InvalidArgument, "op must be read or write")
	}
	device := in.Device
	if device == "" {
		device = "/dev/sda"
	}
	return &api.DiskLatencyReport{
		Devices:      []string{"/dev/sda", "/dev/sdb"},
		Device:       device,
		Op:           in.Op,
		StepSeconds:  60,
		Steps:        []*api.LatencyStep{{Time: 1700000000, Buckets: []int64{0, 9, 1}, P50: 0.02, P95: 0.04, P99: 0.06}},
		BucketStarts: []float64{0, 0.016, 0.032},
	}, nil
}

func TestDiskLatency(t *testing.T) {
	s, _ := newTestServer(t, nil)
	steps := `"steps":[{"time":1700000000,"buckets":[0,9,1],"p50":0.02,"p95":0.04,"p99":0.06}]`

	// viewers see it like the charts, reads by default
	for _, session := range []string{testSession, viewerSession} {
		rec := call(s, "GET", "/api/v1/hosts/web1/disk-latency?from=1700000000&to=1700003600", "", session, nil)
		want := `{"bucketStarts":[0,0.016,0.032],"device":"/dev/sda","devices":["/dev/sda","/dev/sdb"],"op":"read","stepSeconds":60,` + steps + `}`
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
			t.Errorf("unexpected response for %s: %d %s", session, rec.Code, rec.Body)
		}
	}
	code, body, _ := get(t, s, "/api/v1/hosts/web1/disk-latency?from=1700000000&to=1700003600&device=/dev/sdb&op=write")
	if code != 200 || !strings.Contains(body, `"device":"/dev/sdb"`) || !strings.Contains(body, `"op":"write"`) {
		t.Errorf("expected the asked disk and op, got %d %s", code, body)
	}

	tests := []struct {
		url  string
		code int
	}{
		{"/api/v1/hosts/nope/disk-latency?from=1700000000&to=1700003600", http.StatusNotFound},
		{"/api/v1/hosts/web1/disk-latency?from=soon", http.StatusBadRequest},
		{"/api/v1/hosts/web1/disk-latency?from=1700000000&to=1700003600&op=trim", http.StatusBadRequest},
	}
	for _, test := range tests {
		if code, body, _ := get(t, s, test.url); code != test.code {
			t.Errorf("%s: expected %d, got %d %s", test.url, test.code, code, body)
		}
	}
	if rec := call(s, "GET", "/api/v1/hosts/web1/disk-latency?from=1700000000&to=1700003600", "", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected a login to be needed, got %d", rec.Code)
	}
}
