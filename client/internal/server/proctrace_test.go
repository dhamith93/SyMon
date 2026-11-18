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

func (f *fakeCollector) ProcessActivity(ctx context.Context, in *api.ProcessActivityRequest) (*api.ProcessActivityReport, error) {
	if in.Host != "web1" {
		return nil, status.Error(codes.NotFound, "host "+in.Host+": not found")
	}
	return &api.ProcessActivityReport{
		Totals:    &api.ProcessActivityTotals{Seconds: 3600, Execs: 7200, FailedExecs: 900, Exits: 7100, NonZeroExits: 40, Signaled: 3, Dropped: 2, Listed: 41, Oom: 1, Crashes: 1},
		Groups:    []*api.ExitGroup{{Name: "java", Signal: 9, Oom: true, Count: 1, First: in.From + 60, Last: in.From + 60, Parent: "systemd", LifetimeSeconds: 86400}},
		Latest:    []*api.ProcessExit{{Time: in.From + 60, Name: "java", Parent: "systemd", Pid: 4242, Ppid: 1, Uid: 998, Signal: 9, Oom: true, LifetimeSeconds: 86400}},
		ExitsFrom: in.From,
	}, nil
}

func TestProcessActivity(t *testing.T) {
	s, _ := newTestServer(t, nil)
	want := `{"exitsFrom":1700000000,` +
		`"groups":[{"name":"java","status":0,"signal":9,"coreDumped":false,"oom":true,"count":1,"first":1700000060,"last":1700000060,"parent":"systemd","lifetimeSeconds":86400}],` +
		`"latest":[{"time":1700000060,"name":"java","parent":"systemd","pid":4242,"ppid":1,"uid":998,"status":0,"signal":9,"coreDumped":false,"oom":true,"lifetimeSeconds":86400}],` +
		`"totals":{"seconds":3600,"execs":7200,"failedExecs":900,"exits":7100,"nonZeroExits":40,"signaled":3,"dropped":2,"listed":41,"oom":1,"crashes":1}}`

	// viewers see it like the process lists
	for _, session := range []string{testSession, viewerSession} {
		rec := call(s, "GET", "/api/v1/hosts/web1/process-activity?from=1700000000&to=1700003600", "", session, nil)
		if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
			t.Errorf("unexpected response for %s: %d %s", session, rec.Code, rec.Body)
		}
	}

	tests := []struct {
		url  string
		code int
	}{
		{"/api/v1/hosts/nope/process-activity?from=1700000000&to=1700003600", http.StatusNotFound},
		{"/api/v1/hosts/web1/process-activity?from=soon", http.StatusBadRequest},
	}
	for _, test := range tests {
		if code, body, _ := get(t, s, test.url); code != test.code {
			t.Errorf("%s: expected %d, got %d %s", test.url, test.code, code, body)
		}
	}
	if rec := call(s, "GET", "/api/v1/hosts/web1/process-activity?from=1700000000&to=1700003600", "", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected a login to be needed, got %d", rec.Code)
	}
}
