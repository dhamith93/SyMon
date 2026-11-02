package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/kube"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAPIReader struct{ state kube.ClusterState }

func (f fakeAPIReader) Read(ctx context.Context, cluster string, now time.Time) kube.ClusterState {
	state := f.state
	state.Cluster, state.Time = cluster, now.Unix()
	return state
}

type fakeClusterClient struct {
	reports []*api.ClusterReport
	err     error
}

func (f *fakeClusterClient) ReportCluster(ctx context.Context, in *api.ClusterReport, opts ...grpc.CallOption) (*api.Message, error) {
	f.reports = append(f.reports, in)
	return &api.Message{Body: "ok"}, f.err
}

func TestClusterReaderReports(t *testing.T) {
	client := &fakeClusterClient{}
	r := &clusterReader{cluster: "homelab", host: "cp1", client: client,
		reader: fakeAPIReader{kube.ClusterState{Nodes: []kube.Node{{Name: "cp1", Ready: true}}}}}
	if wait := r.step(); wait != kube.ReportInterval {
		t.Errorf("waits %v, want %v", wait, kube.ReportInterval)
	}
	if len(client.reports) != 1 || client.reports[0].Host != "cp1" {
		t.Fatalf("unexpected reports %v", client.reports)
	}
	var sent kube.ClusterState
	if err := json.Unmarshal([]byte(client.reports[0].StateJson), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Cluster != "homelab" || len(sent.Nodes) != 1 {
		t.Errorf("unexpected state %+v", sent)
	}

	// a failed read is still reported, so the dashboard can say why
	r.reader = fakeAPIReader{kube.ClusterState{Error: "the API answered 401 Unauthorized for /api/v1/nodes"}}
	r.step()
	if r.lastError == "" || len(client.reports) != 2 {
		t.Errorf("a failed read was not reported: %q %d", r.lastError, len(client.reports))
	}

	client.err = status.Error(codes.Unimplemented, "unknown method")
	if wait := r.step(); wait != kubeOldCollector {
		t.Errorf("an old collector should be left alone for %v, got %v", kubeOldCollector, wait)
	}
}
