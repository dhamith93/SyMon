package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeCollector knows one host, web1, and fails for the host "broken"
type fakeCollector struct {
	api.UnimplementedMonitorDataServiceServer
	lastSeries *api.SeriesRequest
}

func (f *fakeCollector) Fleet(ctx context.Context, in *api.Void) (*api.FleetSummary, error) {
	return &api.FleetSummary{Hosts: []*api.HostSummary{{Name: "web1", Up: true, CpuPct: 37, ActiveAlerts: 2}}}, nil
}

func (f *fakeCollector) Snapshot(ctx context.Context, in *api.HostRequest) (*api.HostSnapshot, error) {
	switch in.Host {
	case "web1":
		return &api.HostSnapshot{Host: "web1", Time: 1700000000, SnapshotJson: `{"System":{"OS":"Debian"}}`}, nil
	case "broken":
		return nil, status.Error(codes.Internal, "internal error")
	case "gone":
		return nil, status.Error(codes.Canceled, "canceled")
	}
	return nil, status.Error(codes.NotFound, "no data found")
}

func (f *fakeCollector) QuerySeries(ctx context.Context, in *api.SeriesRequest) (*api.SeriesResponse, error) {
	f.lastSeries = in
	if in.Metric == "nope" {
		return nil, status.Error(codes.InvalidArgument, `invalid request: unknown metric "nope"`)
	}
	return &api.SeriesResponse{Metric: in.Metric, Source: "raw", StepSeconds: 4, Series: []*api.Series{
		{Label: "/", Points: []*api.Point{{Time: 1700000000, Value: 40}, {Time: 1700000004, Value: 41}}},
	}}, nil
}

func (f *fakeCollector) CustomMetricNames(ctx context.Context, in *api.HostRequest) (*api.NameList, error) {
	return &api.NameList{}, nil
}

func (f *fakeCollector) Alerts(ctx context.Context, in *api.AlertsRequest) (*api.AlertList, error) {
	return &api.AlertList{Alerts: []*api.AlertRecord{{Id: 7, Host: "web1", Rule: "CPU", Severity: 2, StartedAt: 1700000000}}}, nil
}

func newTestServer(t *testing.T, files fstest.MapFS) (*server, *fakeCollector) {
	t.Helper()
	t.Setenv("SYMON_KEY", "test-key")

	fake := &fakeCollector{}
	grpcServer, err := transport.NewServer(false, "", "", transport.AuthInterceptor)
	if err != nil {
		t.Fatal(err)
	}
	api.RegisterMonitorDataServiceServer(grpcServer, fake)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	conn, err := transport.Dial(lis.Addr().String(), "", transport.SharedKey())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &server{collector: api.NewMonitorDataServiceClient(conn), refreshSeconds: 15, files: files}, fake
}

// get calls the server and returns the status, the body and what was logged
func get(t *testing.T, s *server, url string) (int, string, string) {
	t.Helper()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	return rec.Code, rec.Body.String(), logs.String()
}

func TestFleet(t *testing.T) {
	s, _ := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/fleet")

	var out struct {
		Hosts []hostSummary `json:"hosts"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if code != 200 || len(out.Hosts) != 1 || out.Hosts[0].Name != "web1" || out.Hosts[0].CPUPct != 37 || out.Hosts[0].ActiveAlerts != 2 {
		t.Errorf("unexpected response %d: %s", code, body)
	}
}

func TestHostSnapshotIsPassedThrough(t *testing.T) {
	s, _ := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/hosts/web1")
	if code != 200 || !strings.Contains(body, `"snapshot":{"System":{"OS":"Debian"}}`) {
		t.Errorf("unexpected response %d: %s", code, body)
	}
}

func TestErrorsMapToStatusCodes(t *testing.T) {
	s, _ := newTestServer(t, nil)
	tests := []struct {
		url    string
		code   int
		logged bool
	}{
		{"/api/v1/hosts/ghost", http.StatusNotFound, false},
		{"/api/v1/hosts/broken", http.StatusInternalServerError, true},
		{"/api/v1/hosts/gone", 499, false},
		{"/api/v1/hosts/web1/series?metric=nope", http.StatusBadRequest, false},
		{"/api/v1/hosts/web1/series?metric=cpu&from=soon", http.StatusBadRequest, false},
		{"/api/v1/nothing", http.StatusNotFound, false},
	}
	for _, tt := range tests {
		code, body, logs := get(t, s, tt.url)
		if code != tt.code || !strings.Contains(body, `"error"`) {
			t.Errorf("%s: got %d %s, want %d", tt.url, code, body, tt.code)
		}
		if (logs != "") != tt.logged {
			t.Errorf("%s: logged %q, want logged=%v", tt.url, logs, tt.logged)
		}
	}
}

func TestSeries(t *testing.T) {
	s, fake := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/hosts/web1/series?metric=disk_used&label=/&from=1700000000&to=1700003600&maxPoints=500&max=1")
	if code != 200 || !strings.Contains(body, `"points":[[1700000000,40],[1700000004,41]]`) {
		t.Errorf("unexpected response %d: %s", code, body)
	}
	q := fake.lastSeries
	if q.Host != "web1" || q.Metric != "disk_used" || q.Label != "/" || q.From != 1700000000 || q.To != 1700003600 || q.MaxPoints != 500 || !q.Max {
		t.Errorf("request was not passed on: %+v", q)
	}
}

func TestSeriesDefaultsToTheLastHour(t *testing.T) {
	s, fake := newTestServer(t, nil)
	get(t, s, "/api/v1/hosts/web1/series?metric=cpu")
	if span := fake.lastSeries.To - fake.lastSeries.From; span != 3600 {
		t.Errorf("expected an hour, got %ds", span)
	}
}

func TestEmptyListsAreArrays(t *testing.T) {
	s, _ := newTestServer(t, nil)
	_, body, _ := get(t, s, "/api/v1/hosts/web1/custom-metrics")
	if strings.TrimSpace(body) != `{"names":[]}` {
		t.Errorf("expected an empty array, got %s", body)
	}
}

func TestAlerts(t *testing.T) {
	s, _ := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/alerts?open=1")
	if code != 200 || !strings.Contains(body, `"rule":"CPU"`) || !strings.Contains(body, `"resolvedAt":0`) {
		t.Errorf("unexpected response %d: %s", code, body)
	}
}

func TestApp(t *testing.T) {
	files := fstest.MapFS{
		"index.html":      {Data: []byte("<html>symon</html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
		"favicon.svg":     {Data: []byte("<svg/>")},
	}
	s, _ := newTestServer(t, files)

	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/assets/app-1.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("assets should be cached forever, got %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}

	// app routes are not files, they get index.html
	for _, url := range []string{"/", "/hosts/web1", "/alerts"} {
		code, body, _ := get(t, s, url)
		if code != 200 || body != "<html>symon</html>" {
			t.Errorf("%s: got %d %q", url, code, body)
		}
	}
}

func TestAppNotBuilt(t *testing.T) {
	s, _ := newTestServer(t, fstest.MapFS{".gitkeep": {}})
	code, body, _ := get(t, s, "/")
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "make web") {
		t.Errorf("expected a hint to build the dashboard, got %d %q", code, body)
	}
}
