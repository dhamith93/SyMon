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

var homelab = &api.ClusterSummary{Name: "homelab", Host: "cp1", StateTime: 1700000000, ReportedAt: 1700000030, Error: "the API answered 401 Unauthorized for /api/v1/nodes",
	Nodes: 2, NodesReady: 1, Pods: 3, PodsRunning: 2, PodsPending: 1, Workloads: 2, WorkloadsReady: 1, RestartsLastHour: 4}

func (f *fakeCollector) Clusters(ctx context.Context, in *api.Void) (*api.ClusterList, error) {
	return &api.ClusterList{Clusters: []*api.ClusterSummary{homelab}}, nil
}

func (f *fakeCollector) Cluster(ctx context.Context, in *api.ClusterRequest) (*api.ClusterInfo, error) {
	if in.Name != "homelab" {
		return nil, status.Error(codes.NotFound, "no data")
	}
	return &api.ClusterInfo{
		Summary:   homelab,
		StateJson: `{"Cluster":"homelab","Nodes":[{"Name":"cp1","Ready":true}]}`,
		Usage:     []*api.ContainerUsage{{Id: "abc", Host: "w1", CpuPercent: 12.5, CpuCores: 0.5, MemoryBytes: 1 << 20}},
		Restarts:  []*api.KubeRestart{{Time: 1700000000, Namespace: "media", Workload: "deployment/web", Pod: "web-1", Container: "web", Count: 2, Reason: "OOMKilled"}},
		Hosts:     []string{"cp1", "w1"},
	}, nil
}

func (f *fakeCollector) ClusterSeries(ctx context.Context, in *api.ClusterSeriesRequest) (*api.SeriesResponse, error) {
	if in.Metric != "pods" {
		return nil, status.Error(codes.InvalidArgument, "invalid request: unknown cluster metric")
	}
	return &api.SeriesResponse{Metric: in.Metric, Source: "raw", StepSeconds: 30, Series: []*api.Series{
		{Label: "running", Points: []*api.Point{{Time: 1700000000, Value: 2}}},
	}}, nil
}

func (f *fakeCollector) RemoveCluster(ctx context.Context, in *api.ClusterRequest) (*api.Message, error) {
	if in.Name != "homelab" {
		return nil, status.Error(codes.NotFound, "no data")
	}
	return &api.Message{Body: "ok"}, nil
}

func TestClusters(t *testing.T) {
	s, _ := newTestServer(t, nil)

	// viewers see clusters like admins
	for _, session := range []string{testSession, viewerSession} {
		rec := call(s, "GET", "/api/v1/clusters", "", session, nil)
		if body := rec.Body.String(); rec.Code != 200 || !strings.Contains(body, `"name":"homelab"`) || !strings.Contains(body, `"podsPending":1`) || !strings.Contains(body, `"restartsLastHour":4`) || !strings.Contains(body, "401 Unauthorized") {
			t.Errorf("unexpected clusters for %s: %d %s", session, rec.Code, body)
		}
		rec = call(s, "GET", "/api/v1/clusters/homelab", "", session, nil)
		body := rec.Body.String()
		if rec.Code != 200 || !strings.Contains(body, `"state":{"Cluster":"homelab","Nodes":[{"Name":"cp1","Ready":true}]}`) ||
			!strings.Contains(body, `"usage":[{"id":"abc","host":"w1","cpuPercent":12.5`) || !strings.Contains(body, `"reason":"OOMKilled"`) || !strings.Contains(body, `"hosts":["cp1","w1"]`) {
			t.Errorf("unexpected cluster for %s: %d %s", session, rec.Code, body)
		}
	}

	rec := call(s, "GET", "/api/v1/clusters/homelab/series?metric=pods&from=1699990000&to=1700000100", "", viewerSession, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"label":"running"`) {
		t.Errorf("unexpected series %d %s", rec.Code, rec.Body)
	}

	tests := []struct {
		method, url, cookie string
		code                int
		want                string
	}{
		{"GET", "/api/v1/clusters/lab2", testSession, http.StatusNotFound, ""},
		{"GET", "/api/v1/clusters/homelab/series?metric=secrets", testSession, http.StatusBadRequest, "unknown cluster metric"},
		{"GET", "/api/v1/clusters", "", http.StatusUnauthorized, ""},
		{"DELETE", "/api/v1/clusters/homelab", viewerSession, http.StatusForbidden, "only admins"},
		{"DELETE", "/api/v1/clusters/lab2", testSession, http.StatusNotFound, ""},
		{"DELETE", "/api/v1/clusters/homelab", testSession, 200, `{"name":"homelab"}`},
	}
	for _, tt := range tests {
		rec := call(s, tt.method, tt.url, "", tt.cookie, nil)
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.want) {
			t.Errorf("%s %s as %q: got %d %s, want %d with %q", tt.method, tt.url, tt.cookie, rec.Code, rec.Body, tt.code, tt.want)
		}
	}
}
