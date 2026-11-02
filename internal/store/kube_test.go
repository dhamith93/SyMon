package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/kube"
)

// clusterState is a small cluster with web restarting restarts times
func clusterState(at time.Time, restarts int) kube.ClusterState {
	return kube.ClusterState{
		Cluster: "homelab",
		Time:    at.Unix(),
		Nodes:   []kube.Node{{Name: "cp1", Ready: true}, {Name: "w1"}},
		Workloads: []kube.Workload{
			{Namespace: "media", Kind: "deployment", Name: "web", Desired: 2, Ready: 1, Available: 1},
			{Namespace: "media", Kind: "statefulset", Name: "db", Desired: 1, Ready: 1, Available: 1},
		},
		Pods: []kube.Pod{
			{Namespace: "media", Name: "web-1", UID: "u1", Workload: "deployment/web", Phase: "Running", Created: at.Add(-time.Hour).Unix(),
				Containers: []kube.PodContainer{{Name: "web", ID: "c1", Restarts: restarts, LastReason: "OOMKilled"}}},
			{Namespace: "media", Name: "db-0", UID: "u2", Workload: "statefulset/db", Phase: "Running", Ready: true, Created: at.Add(-time.Hour).Unix()},
			{Namespace: "default", Name: "big", UID: "u3", Workload: "pod/big", Phase: "Pending", Reason: "Unschedulable", Created: at.Add(-time.Hour).Unix()},
		},
	}
}

func TestClusterReports(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, host := range []string{"cp1", "w1"} {
		if err := st.AddHost(ctx, host, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveClusterReport(ctx, "ghost", clusterState(time.Now(), 0)); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected an unregistered host to be refused, got %v", err)
	}
	if err := st.SaveClusterReport(ctx, "cp1", kube.ClusterState{Time: time.Now().Unix()}); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected a nameless cluster to be refused, got %v", err)
	}

	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	if err := st.SaveClusterReport(ctx, "cp1", clusterState(start, 3)); err != nil {
		t.Fatal(err)
	}
	// another host cannot report the same cluster
	if err := st.SaveClusterReport(ctx, "w1", clusterState(start.Add(30*time.Second), 3)); !errors.Is(err, ErrClusterTaken) {
		t.Errorf("expected another host to be refused, got %v", err)
	}
	// two restarts, then the API goes away, then one more restart
	if err := st.SaveClusterReport(ctx, "cp1", clusterState(start.Add(30*time.Second), 5)); err != nil {
		t.Fatal(err)
	}
	failed := kube.ClusterState{Cluster: "homelab", Time: start.Add(time.Minute).Unix(), Error: "the API answered 401 Unauthorized for /api/v1/nodes"}
	if err := st.SaveClusterReport(ctx, "cp1", failed); err != nil {
		t.Fatal(err)
	}
	cluster, err := st.Cluster(ctx, "homelab")
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Host != "cp1" || cluster.Error != failed.Error || cluster.State.Time != start.Add(30*time.Second).Unix() || len(cluster.State.Pods) != 3 {
		t.Errorf("a failed read should keep the last state and add the error: %+v", cluster)
	}
	if err := st.SaveClusterReport(ctx, "cp1", clusterState(start.Add(90*time.Second), 6)); err != nil {
		t.Fatal(err)
	}
	// a late report is ignored
	if err := st.SaveClusterReport(ctx, "cp1", clusterState(start.Add(60*time.Second), 9)); err != nil {
		t.Fatal(err)
	}

	clusters, err := st.Clusters(ctx)
	if err != nil || len(clusters) != 1 || clusters[0].Error != "" || clusters[0].State.Pods[0].Containers[0].Restarts != 6 {
		t.Fatalf("unexpected clusters %+v %v", clusters, err)
	}
	if clusters[0].Stale(start.Add(2*time.Minute)) || !clusters[0].Stale(start.Add(10*time.Minute)) {
		t.Error("stale should follow the state's time")
	}

	// the first report's 3 are history, 2 and 1 are new
	restarts, err := st.RecentRestarts(ctx, "homelab", start.Add(-time.Hour), 10)
	if err != nil || len(restarts) != 2 || restarts[0].Count != 1 || restarts[1].Count != 2 || restarts[0].Reason != "OOMKilled" || restarts[0].Workload != "deployment/web" {
		t.Errorf("unexpected restarts %+v %v", restarts, err)
	}
	totals, err := st.RestartsSince(ctx, "homelab", start.Add(-time.Hour))
	if err != nil || len(totals) != 1 || totals["media/deployment/web"] != 3 {
		t.Errorf("unexpected restart totals %v %v", totals, err)
	}

	result, err := st.ClusterSeries(ctx, "homelab", "pods", start.Add(-time.Minute), start.Add(5*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 4 || result.Series[0].Label != "running" || len(result.Series[0].Points) != 3 || result.Series[0].Points[0].Value != 2 || result.Series[1].Points[0].Value != 1 {
		t.Errorf("unexpected pod series %+v", result)
	}
	restartSeries, err := st.ClusterSeries(ctx, "homelab", "restarts", start.Add(-time.Minute), start.Add(5*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, p := range restartSeries.Series[0].Points {
		total += p.Value
	}
	if total != 3 {
		t.Errorf("restart series adds up to %v, want 3", total)
	}
	if _, err := st.ClusterSeries(ctx, "homelab", "secrets", start, start.Add(time.Minute), 10); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected an unknown metric to be refused, got %v", err)
	}

	// once its host is gone another may take the cluster over
	if err := st.RemoveHost(ctx, "cp1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveClusterReport(ctx, "w1", clusterState(start.Add(2*time.Minute), 6)); err != nil {
		t.Errorf("expected w1 to take the cluster over, got %v", err)
	}
	if err := st.RemoveCluster(ctx, "homelab"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Cluster(ctx, "homelab"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected the cluster to be gone, got %v", err)
	}
	if err := st.RemoveCluster(ctx, "homelab"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for a removed cluster, got %v", err)
	}
}
