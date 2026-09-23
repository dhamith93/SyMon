package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeCollector answers memory queries, has no services and fails swap
type fakeCollector struct {
	api.UnimplementedMonitorDataServiceServer
}

func (fakeCollector) HandleMonitorDataRequest(ctx context.Context, in *api.MonitorDataRequest) (*api.MonitorData, error) {
	switch in.LogType {
	case "memory":
		return &api.MonitorData{MonitorData: `{"PercentageUsed": 42}`}, nil
	case "services":
		return nil, status.Error(codes.NotFound, "no data found")
	default:
		return nil, status.Error(codes.Internal, "db is down")
	}
}

func newTestServer(t *testing.T) *server {
	t.Helper()
	t.Setenv("SYMON_KEY", "test-key")

	grpcServer, err := transport.NewServer(false, "", "")
	if err != nil {
		t.Fatal(err)
	}
	api.RegisterMonitorDataServiceServer(grpcServer, fakeCollector{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	conn, err := transport.Dial(lis.Addr().String(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &server{collector: api.NewMonitorDataServiceClient(conn)}
}

// get calls a handler and returns the decoded response and what was logged
func get(t *testing.T, handler func(w *httptest.ResponseRecorder), url string) (output, string) {
	t.Helper()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	rec := httptest.NewRecorder()
	handler(rec)

	var out output
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: bad response %q: %v", url, rec.Body.String(), err)
	}
	return out, logs.String()
}

func TestNoDataIsNotLogged(t *testing.T) {
	s := newTestServer(t)
	url := "/services?serverId=test"
	out, logs := get(t, func(w *httptest.ResponseRecorder) {
		s.returnServices(w, httptest.NewRequest("GET", url, nil))
	}, url)

	if out.Status != "ERR" {
		t.Errorf("expected ERR status for no data, got %s", out.Status)
	}
	if logs != "" {
		t.Errorf("expected nothing logged for no data, got %q", logs)
	}
}

func TestFailuresAreLogged(t *testing.T) {
	s := newTestServer(t)
	url := "/swap?serverId=test"
	out, logs := get(t, func(w *httptest.ResponseRecorder) {
		s.returnSwap(w, httptest.NewRequest("GET", url, nil))
	}, url)

	if out.Status != "ERR" {
		t.Errorf("expected ERR status, got %s", out.Status)
	}
	if !strings.Contains(logs, "cannot get swap for test") || !strings.Contains(logs, "db is down") {
		t.Errorf("expected the failed request to be logged, got %q", logs)
	}
}

func TestDataIsReturned(t *testing.T) {
	s := newTestServer(t)
	url := "/memory?serverId=test"
	out, _ := get(t, func(w *httptest.ResponseRecorder) {
		s.returnMemory(w, httptest.NewRequest("GET", url, nil))
	}, url)

	data, ok := out.Data.(map[string]interface{})
	if out.Status != "OK" || !ok || data["PercentageUsed"] != float64(42) {
		t.Errorf("unexpected response: %+v", out)
	}
}
