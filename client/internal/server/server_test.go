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
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/transport"
	"github.com/dhamith93/SyMon/internal/version"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeCollector knows one host, web1, and fails for the host "broken"
type fakeCollector struct {
	api.UnimplementedMonitorDataServiceServer
	lastSeries       *api.SeriesRequest
	lastProcessUsage *api.ProcessUsageRequest

	// login: testSession is valid, alice's password is "correct horse
	// battery", and the user "locked" has failed too often
	noUsers        atomic.Bool
	sessionChecks  atomic.Int32
	passwordChecks atomic.Int32
	loggedOut      atomic.Value
}

const (
	testSession   = "test-session"
	viewerSession = "viewer-session"
)

func (f *fakeCollector) ChangePassword(ctx context.Context, in *api.ChangePasswordRequest) (*api.Message, error) {
	switch {
	case in.Current != "correct horse battery":
		return nil, status.Error(codes.Unauthenticated, "wrong user name or password")
	case len(in.NewPassword) < 12:
		return nil, status.Error(codes.InvalidArgument, "invalid request: a password needs at least 12 characters")
	}
	return &api.Message{Body: "ok"}, nil
}

func (f *fakeCollector) CheckSession(ctx context.Context, in *api.SessionRequest) (*api.SessionInfo, error) {
	f.sessionChecks.Add(1)
	switch in.Token {
	case testSession, "new-token":
		return &api.SessionInfo{User: "tester", Role: "admin", Expires: time.Now().Add(time.Hour).Unix()}, nil
	case viewerSession:
		return &api.SessionInfo{User: "vera", Role: "viewer", Expires: time.Now().Add(time.Hour).Unix()}, nil
	}
	return nil, status.Error(codes.Unauthenticated, "not logged in")
}

func (f *fakeCollector) checkCredentials(in *api.Credentials) error {
	switch {
	case in.User == "locked":
		return status.Error(codes.ResourceExhausted, "too many failed logins, try again later")
	case in.User == "alice" && in.Password == "correct horse battery":
		return nil
	}
	return status.Error(codes.Unauthenticated, "wrong user name or password")
}

func (f *fakeCollector) Login(ctx context.Context, in *api.Credentials) (*api.SessionInfo, error) {
	if err := f.checkCredentials(in); err != nil {
		return nil, err
	}
	return &api.SessionInfo{Token: "new-token", User: in.User, Role: "admin", Expires: 1900000000}, nil
}

func (f *fakeCollector) CheckPassword(ctx context.Context, in *api.Credentials) (*api.Message, error) {
	f.passwordChecks.Add(1)
	if err := f.checkCredentials(in); err != nil {
		return nil, err
	}
	return &api.Message{Body: "ok"}, nil
}

func (f *fakeCollector) Logout(ctx context.Context, in *api.SessionRequest) (*api.Message, error) {
	f.loggedOut.Store(in.Token)
	return &api.Message{Body: "ok"}, nil
}

func (f *fakeCollector) HasUsers(ctx context.Context, in *api.Void) (*api.UserStatus, error) {
	return &api.UserStatus{HasUsers: !f.noUsers.Load()}, nil
}

func (f *fakeCollector) Fleet(ctx context.Context, in *api.Void) (*api.FleetSummary, error) {
	return &api.FleetSummary{Hosts: []*api.HostSummary{
		{Name: "web1", Up: true, CpuPct: 37, ActiveAlerts: 2, DiskFullDays: floatPtr(12.5), AgentVersion: "v3.1.0"},
		{Name: "db1", Up: true},
	}}, nil
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

func (f *fakeCollector) DiskForecasts(ctx context.Context, in *api.HostRequest) (*api.DiskForecastList, error) {
	return &api.DiskForecastList{Disks: []*api.DiskForecast{
		{Device: "/dev/sda1", Mount: "/", UsedPct: 40, NoForecast: "collecting", Samples: 12},
		{Device: "/dev/sdb1", Mount: "/data", UsedPct: 60, PctPerDay: 2, BytesPerDay: 2e7, DaysToFull: floatPtr(20)},
	}}, nil
}

func (f *fakeCollector) ProcessUsage(ctx context.Context, in *api.ProcessUsageRequest) (*api.ProcessUsageList, error) {
	f.lastProcessUsage = in
	return &api.ProcessUsageList{Snapshots: 4, FirstTime: 1700000000, Processes: []*api.ProcessUsage{
		{Name: "php-fpm", CpuAvg: 17.5, CpuPeak: 40, MemAvg: 3.75, MemPeak: 10, SeenPct: 50},
	}}, nil
}

func (f *fakeCollector) Endpoints(ctx context.Context, in *api.EndpointsRequest) (*api.EndpointList, error) {
	return &api.EndpointList{Endpoints: []*api.EndpointStatus{
		{Name: "api", Url: "https://api.example.com", Method: "GET", Time: in.To, StatusCode: 0, Error: "connection refused", Checks: 30, UptimePct: 90, AvgLatencyMs: 110.5},
	}}, nil
}

func (f *fakeCollector) EndpointSeries(ctx context.Context, in *api.EndpointSeriesRequest) (*api.SeriesResponse, error) {
	if in.Metric != "latency" {
		return nil, status.Error(codes.InvalidArgument, "invalid request: unknown endpoint metric")
	}
	return &api.SeriesResponse{Metric: in.Metric, Source: "raw", StepSeconds: 4, Series: []*api.Series{
		{Label: in.Name, Points: []*api.Point{{Time: 1700000000, Value: 100}}},
	}}, nil
}

func (f *fakeCollector) Version(ctx context.Context, in *api.Void) (*api.Message, error) {
	return &api.Message{Body: "v3.1.0"}, nil
}

func floatPtr(v float64) *float64 {
	return &v
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
	return &server{collector: api.NewMonitorDataServiceClient(conn), refreshSeconds: 15, files: files, metricsEnabled: true}, fake
}

// get calls the server as a logged in browser and returns the status, the
// body and what was logged
func get(t *testing.T, s *server, url string) (int, string, string) {
	t.Helper()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	rec := httptest.NewRecorder()
	request := httptest.NewRequest("GET", url, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: testSession})
	s.routes().ServeHTTP(rec, request)
	return rec.Code, rec.Body.String(), logs.String()
}

func TestConfig(t *testing.T) {
	version.Version = "v3.1.1"
	t.Cleanup(func() { version.Version = "" })
	s, _ := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/config")
	if code != 200 || strings.TrimSpace(body) != `{"collectorVersion":"v3.1.0","refreshSeconds":15,"version":"v3.1.1"}` {
		t.Errorf("unexpected response %d: %s", code, body)
	}
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
	if code != 200 || len(out.Hosts) != 2 || out.Hosts[0].Name != "web1" || out.Hosts[0].CPUPct != 37 || out.Hosts[0].ActiveAlerts != 2 {
		t.Errorf("unexpected response %d: %s", code, body)
	}
	if !strings.Contains(body, `"diskFullDays":12.5`) || !strings.Contains(body, `"diskFullDays":null`) || !strings.Contains(body, `"agentVersion":"v3.1.0"`) {
		t.Errorf("expected a forecast for web1 and null for db1: %s", body)
	}
}

func TestProcessUsage(t *testing.T) {
	s, fake := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/hosts/web1/process-usage?from=1700000000&to=1700003600")
	want := `{"firstTime":1700000000,"processes":[` +
		`{"name":"php-fpm","cpuAvg":17.5,"cpuPeak":40,"memAvg":3.75,"memPeak":10,"seenPct":50}],"snapshots":4}`
	if code != 200 || strings.TrimSpace(body) != want {
		t.Errorf("unexpected response %d: %s", code, body)
	}
	if fake.lastProcessUsage.Host != "web1" || fake.lastProcessUsage.From != 1700000000 || fake.lastProcessUsage.To != 1700003600 {
		t.Errorf("unexpected request %+v", fake.lastProcessUsage)
	}
}

func TestEndpoints(t *testing.T) {
	s, _ := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/endpoints?from=1700000000&to=1700003600")
	want := `{"endpoints":[{"name":"api","url":"https://api.example.com","method":"GET","time":1700003600,"ok":false,` +
		`"statusCode":0,"latencyMs":0,"error":"connection refused","checks":30,"uptimePct":90,"avgLatencyMs":110.5}]}`
	if code != 200 || strings.TrimSpace(body) != want {
		t.Errorf("unexpected response %d: %s", code, body)
	}

	code, body, _ = get(t, s, "/api/v1/endpoints/series?name=api&metric=latency&from=1700000000&to=1700003600")
	want = `{"metric":"latency","series":[{"label":"api","points":[[1700000000,100]]}],"source":"raw","stepSeconds":4}`
	if code != 200 || strings.TrimSpace(body) != want {
		t.Errorf("unexpected response %d: %s", code, body)
	}
	if code, _, _ := get(t, s, "/api/v1/endpoints/series?name=api&metric=nope"); code != http.StatusBadRequest {
		t.Errorf("expected 400 for an unknown metric, got %d", code)
	}
}

func TestDiskForecasts(t *testing.T) {
	s, _ := newTestServer(t, nil)
	code, body, _ := get(t, s, "/api/v1/hosts/web1/disk-forecasts")
	want := `{"disks":[` +
		`{"device":"/dev/sda1","mount":"/","usedPct":40,"pctPerDay":0,"bytesPerDay":0,"daysToFull":null,"noForecast":"collecting","samples":12},` +
		`{"device":"/dev/sdb1","mount":"/data","usedPct":60,"pctPerDay":2,"bytesPerDay":20000000,"daysToFull":20,"noForecast":"","samples":0}]}`
	if code != 200 || strings.TrimSpace(body) != want {
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
