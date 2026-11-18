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

func (f *fakeCollector) ProcessNetwork(ctx context.Context, in *api.ProcessNetworkRequest) (*api.ProcessNetworkReport, error) {
	if in.Host != "web1" {
		return nil, status.Error(codes.NotFound, "host "+in.Host+": not found")
	}
	return &api.ProcessNetworkReport{
		RxBytes:  5000000,
		TxBytes:  90000,
		Seconds:  3600,
		Programs: []*api.ProgramTraffic{{Name: "nginx", RxBytes: 4000000, TxBytes: 80000, RxPeak: 250000, TxPeak: 4000}},
	}, nil
}

func TestProcessNetwork(t *testing.T) {
	s, _ := newTestServer(t, nil)
	want := `{"programs":[{"name":"nginx","rxBytes":4000000,"txBytes":80000,"rxPeak":250000,"txPeak":4000}],"rxBytes":5000000,"seconds":3600,"txBytes":90000}`

	// viewers see it like the process lists
	for _, session := range []string{testSession, viewerSession} {
		rec := call(s, "GET", "/api/v1/hosts/web1/process-network?from=1700000000&to=1700003600", "", session, nil)
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
			t.Errorf("unexpected response for %s: %d %s", session, rec.Code, rec.Body)
		}
	}

	tests := []struct {
		url  string
		code int
	}{
		{"/api/v1/hosts/nope/process-network?from=1700000000&to=1700003600", http.StatusNotFound},
		{"/api/v1/hosts/web1/process-network?from=soon", http.StatusBadRequest},
	}
	for _, test := range tests {
		if code, body, _ := get(t, s, test.url); code != test.code {
			t.Errorf("%s: expected %d, got %d %s", test.url, test.code, code, body)
		}
	}
	if rec := call(s, "GET", "/api/v1/hosts/web1/process-network?from=1700000000&to=1700003600", "", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected a login to be needed, got %d", rec.Code)
	}
}
