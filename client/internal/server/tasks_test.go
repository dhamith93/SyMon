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

// web1 lists two tasks and has two runs, the newest still going. db1 is
// busy with a run.

func (f *fakeCollector) HostTasks(ctx context.Context, in *api.HostRequest) (*api.TaskList, error) {
	if in.Host != "web1" {
		return nil, status.Error(codes.NotFound, "no data found")
	}
	return &api.TaskList{Tasks: []*api.TaskInfo{
		{Name: "Restart nginx", Description: "after a config change", Command: "systemctl restart nginx", TimeoutSeconds: 300},
		{Name: "Clean up", Command: "apt-get clean", TimeoutSeconds: 600},
	}}, nil
}

func fakeRun(id int64) *api.TaskRunInfo {
	return &api.TaskRunInfo{
		Id: id, Host: "web1", Task: "Restart nginx", Command: "systemctl restart nginx", Status: "succeeded",
		RequestedBy: "tester", RequestedAt: 1700000000, StartedAt: 1700000001, FinishedAt: 1700000003,
	}
}

func (f *fakeCollector) TaskRuns(ctx context.Context, in *api.TaskRunsRequest) (*api.TaskRunList, error) {
	if in.Host != "web1" {
		return nil, status.Error(codes.NotFound, "no data found")
	}
	going := fakeRun(2)
	going.Status, going.FinishedAt = "running", 0
	runs := []*api.TaskRunInfo{going, fakeRun(1)}
	if in.Limit > 0 && int(in.Limit) < len(runs) {
		runs = runs[:in.Limit]
	}
	return &api.TaskRunList{Runs: runs}, nil
}

func (f *fakeCollector) TaskRun(ctx context.Context, in *api.TaskRunQuery) (*api.TaskRunInfo, error) {
	if in.Id != 1 {
		return nil, status.Error(codes.NotFound, "no data found")
	}
	run := fakeRun(1)
	run.Output = "nginx restarted\n"
	return run, nil
}

func (f *fakeCollector) RequestTaskRun(ctx context.Context, in *api.TaskRunRequest) (*api.TaskRunInfo, error) {
	f.lastTaskRun.Store(in)
	switch {
	case in.Host == "db1":
		return nil, status.Error(codes.AlreadyExists, "this host is already running a task")
	case in.Task != "Restart nginx":
		return nil, status.Error(codes.NotFound, in.Host+` does not list a task named "`+in.Task+`"`)
	}
	run := fakeRun(3)
	run.Status, run.StartedAt, run.FinishedAt = "queued", 0, 0
	return run, nil
}

func TestHostTasks(t *testing.T) {
	s, _ := newTestServer(t, nil)

	rec := call(s, "GET", "/api/v1/hosts/web1/tasks", "", testSession, nil)
	want := `{"tasks":[{"name":"Restart nginx","description":"after a config change","command":"systemctl restart nginx","timeoutSeconds":300},` +
		`{"name":"Clean up","description":"","command":"apt-get clean","timeoutSeconds":600}]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("unexpected tasks for an admin: %d %s", rec.Code, rec.Body)
	}
	// viewers see which tasks there are, not what they run
	rec = call(s, "GET", "/api/v1/hosts/web1/tasks", "", viewerSession, nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "command") || !strings.Contains(rec.Body.String(), `"name":"Clean up"`) {
		t.Errorf("unexpected tasks for a viewer: %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, "GET", "/api/v1/hosts/ghost/tasks", "", testSession, nil); rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for an unknown host, got %d", rec.Code)
	}
	if rec := call(s, "GET", "/api/v1/hosts/web1/tasks", "", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a login, got %d", rec.Code)
	}
}

func TestTaskRuns(t *testing.T) {
	s, _ := newTestServer(t, nil)

	rec := call(s, "GET", "/api/v1/hosts/web1/task-runs", "", testSession, nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `"id":2`) || !strings.Contains(body, `"status":"running"`) ||
		!strings.Contains(body, `"command":"systemctl restart nginx"`) || strings.Contains(body, "output") {
		t.Errorf("unexpected runs for an admin: %d %s", rec.Code, body)
	}
	if rec := call(s, "GET", "/api/v1/hosts/web1/task-runs?limit=1", "", testSession, nil); strings.Contains(rec.Body.String(), `"id":1`) {
		t.Errorf("expected the limit to be passed on, got %s", rec.Body)
	}
	// viewers see who ran what and how it went
	rec = call(s, "GET", "/api/v1/hosts/web1/task-runs", "", viewerSession, nil)
	body = rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "command") || strings.Contains(body, "output") ||
		!strings.Contains(body, `"requestedBy":"tester"`) || !strings.Contains(body, `"status":"succeeded"`) {
		t.Errorf("unexpected runs for a viewer: %d %s", rec.Code, body)
	}

	// the output is for admins
	rec = call(s, "GET", "/api/v1/task-runs/1", "", testSession, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"output":"nginx restarted\n"`) {
		t.Errorf("unexpected run for an admin: %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, "GET", "/api/v1/task-runs/1", "", viewerSession, nil); rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "nginx") {
		t.Errorf("expected 403 for a viewer, got %d %s", rec.Code, rec.Body)
	}
	for _, id := range []string{"99", "abc", "0"} {
		if rec := call(s, "GET", "/api/v1/task-runs/"+id, "", testSession, nil); rec.Code != http.StatusNotFound {
			t.Errorf("run %s: expected 404, got %d", id, rec.Code)
		}
	}
}

func TestRunTask(t *testing.T) {
	s, fake := newTestServer(t, nil)
	body := `{"task":"Restart nginx"}`

	rec := call(s, "POST", "/api/v1/hosts/web1/tasks/run", body, testSession, asJSON)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"id":3`) || !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Fatalf("unexpected response %d %s", rec.Code, rec.Body)
	}
	request := fake.lastTaskRun.Load().(*api.TaskRunRequest)
	if request.Host != "web1" || request.Task != "Restart nginx" || request.By != "tester" {
		t.Errorf("unexpected request %+v", request)
	}

	fake.lastTaskRun.Store((*api.TaskRunRequest)(nil))
	refused := []struct {
		host    string
		body    string
		cookie  string
		prepare func(*http.Request)
		code    int
	}{
		{"web1", body, viewerSession, asJSON, http.StatusForbidden},
		{"web1", body, "", asJSON, http.StatusUnauthorized},
		// a form post from another site cannot start a task
		{"web1", body, testSession, nil, http.StatusUnsupportedMediaType},
		{"web1", `{"task":""}`, testSession, asJSON, http.StatusBadRequest},
		{"web1", `not json`, testSession, asJSON, http.StatusBadRequest},
	}
	for _, tt := range refused {
		if rec := call(s, "POST", "/api/v1/hosts/"+tt.host+"/tasks/run", tt.body, tt.cookie, tt.prepare); rec.Code != tt.code {
			t.Errorf("%s as %q: got %d %s, want %d", tt.body, tt.cookie, rec.Code, rec.Body, tt.code)
		}
	}
	if request := fake.lastTaskRun.Load().(*api.TaskRunRequest); request != nil {
		t.Errorf("expected none of the refused requests to reach the collector, got %+v", request)
	}

	// what the collector refuses
	if rec := call(s, "POST", "/api/v1/hosts/db1/tasks/run", body, testSession, asJSON); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already running") {
		t.Errorf("expected 409 for a busy host, got %d %s", rec.Code, rec.Body)
	}
	if rec := call(s, "POST", "/api/v1/hosts/web1/tasks/run", `{"task":"rm -rf /"}`, testSession, asJSON); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "does not list") {
		t.Errorf("expected 404 for a task the host does not list, got %d %s", rec.Code, rec.Body)
	}
}
