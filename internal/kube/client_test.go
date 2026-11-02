package kube

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

// fixtures, one page each unless a test splits them
var fixtures = map[string]string{
	"/api/v1/nodes": `{"metadata":{},"items":[
		{"metadata":{"name":"cp1","labels":{"node-role.kubernetes.io/control-plane":""}},
		 "spec":{"unschedulable":false},
		 "status":{"conditions":[
			{"type":"MemoryPressure","status":"False"},
			{"type":"DiskPressure","status":"True"},
			{"type":"Ready","status":"True","lastTransitionTime":"2026-11-01T08:00:00Z"}],
		  "allocatable":{"cpu":"3800m","memory":"8023488Ki"},
		  "nodeInfo":{"kubeletVersion":"v1.37.1"}}},
		{"metadata":{"name":"w1","labels":{}},
		 "spec":{"unschedulable":true},
		 "status":{"conditions":[{"type":"Ready","status":"Unknown","lastTransitionTime":"2026-11-02T11:58:00Z"}],
		  "allocatable":{"cpu":"4","memory":"16Gi"},"nodeInfo":{"kubeletVersion":"v1.37.1"}}}]}`,
	"/api/v1/pods": `{"metadata":{},"items":[
		{"metadata":{"name":"web-7d9f8b6c5-x2k4p","namespace":"media","uid":"u1","creationTimestamp":"2026-11-02T10:00:00Z",
			"labels":{"pod-template-hash":"7d9f8b6c5"},
			"ownerReferences":[{"kind":"ReplicaSet","name":"web-7d9f8b6c5","controller":true}]},
		 "spec":{"nodeName":"w1"},
		 "status":{"phase":"Running","conditions":[{"type":"Ready","status":"False"}],
		  "containerStatuses":[{"name":"web","image":"nginx:1.27","containerID":"containerd://abc","ready":false,"restartCount":4,
			"state":{"waiting":{"reason":"CrashLoopBackOff"}},
			"lastState":{"terminated":{"reason":"OOMKilled","finishedAt":"2026-11-02T11:59:00Z"}}}]}},
		{"metadata":{"name":"db-0","namespace":"media","uid":"u2","creationTimestamp":"2026-11-01T10:00:00Z",
			"ownerReferences":[{"kind":"StatefulSet","name":"db","controller":true}]},
		 "spec":{"nodeName":"cp1"},
		 "status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],
		  "containerStatuses":[{"name":"postgres","image":"postgres:18","containerID":"containerd://def","ready":true,"restartCount":0,"state":{"running":{}}}]}},
		{"metadata":{"name":"big","namespace":"default","uid":"u3","creationTimestamp":"2026-11-02T11:00:00Z"},
		 "spec":{},
		 "status":{"phase":"Pending","conditions":[{"type":"PodScheduled","status":"False","reason":"Unschedulable"}]}},
		{"metadata":{"name":"kube-apiserver-cp1","namespace":"kube-system","uid":"u4",
			"ownerReferences":[{"kind":"Node","name":"cp1","controller":true}]},
		 "spec":{"nodeName":"cp1"},
		 "status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}]}`,
	"/apis/apps/v1/deployments": `{"metadata":{},"items":[
		{"metadata":{"name":"web","namespace":"media"},"spec":{"replicas":2},"status":{"readyReplicas":1,"availableReplicas":1}},
		{"metadata":{"name":"idle","namespace":"media"},"spec":{"replicas":0},"status":{}}]}`,
	"/apis/apps/v1/statefulsets": `{"metadata":{},"items":[
		{"metadata":{"name":"db","namespace":"media"},"spec":{},"status":{"readyReplicas":1,"availableReplicas":1}}]}`,
	"/apis/apps/v1/daemonsets": `{"metadata":{},"items":[
		{"metadata":{"name":"kube-proxy","namespace":"kube-system"},"status":{"desiredNumberScheduled":2,"numberReady":1,"numberAvailable":1}}]}`,
	"/api/v1/events": `{"metadata":{},"items":[
		{"metadata":{"namespace":"media"},"involvedObject":{"kind":"Pod","name":"web-7d9f8b6c5-x2k4p","namespace":"media"},
		 "reason":"BackOff","message":"Back-off restarting failed container","count":12,"lastTimestamp":"2026-11-02T11:59:30Z"},
		{"metadata":{"namespace":"default"},"involvedObject":{"kind":"Pod","name":"big","namespace":"default"},
		 "reason":"FailedScheduling","message":"0/2 nodes are available","eventTime":"2026-11-02T11:50:00.123456Z",
		 "series":{"count":7,"lastObservedTime":"2026-11-02T11:55:00.000000Z"}},
		{"metadata":{"namespace":"media"},"involvedObject":{"kind":"Pod","name":"old","namespace":"media"},
		 "reason":"Failed","message":"long ago","count":1,"lastTimestamp":"2026-11-02T09:00:00Z"}]}`,
}

// fakeAPI serves the fixtures over TLS and returns a client for it
func fakeAPI(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte("secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(server.URL+"/", ca, token)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func serveFixtures(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/events" && r.URL.Query().Get("fieldSelector") != "type=Warning" {
			t.Errorf("events asked without the warning selector: %s", r.URL.RawQuery)
		}
		body, ok := fixtures[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}
}

func TestRead(t *testing.T) {
	state := fakeAPI(t, serveFixtures(t)).Read(context.Background(), "homelab", now)
	if state.Error != "" {
		t.Fatal(state.Error)
	}
	if state.Cluster != "homelab" || state.Time != now.Unix() {
		t.Errorf("cluster and time: %q %d", state.Cluster, state.Time)
	}

	wantNodes := []Node{
		{Name: "cp1", Ready: true, ReadyChanged: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC).Unix(), Roles: []string{"control-plane"},
			KubeletVersion: "v1.37.1", Pressure: []string{"DiskPressure"}, CPUCores: 3.8, MemoryBytes: 8023488 * 1024},
		{Name: "w1", ReadyChanged: time.Date(2026, 11, 2, 11, 58, 0, 0, time.UTC).Unix(), KubeletVersion: "v1.37.1",
			Unschedulable: true, CPUCores: 4, MemoryBytes: 16 << 30},
	}
	if !reflect.DeepEqual(state.Nodes, wantNodes) {
		t.Errorf("nodes:\n got %+v\nwant %+v", state.Nodes, wantNodes)
	}

	wantWorkloads := []Workload{
		{Namespace: "media", Kind: "deployment", Name: "web", Desired: 2, Ready: 1, Available: 1},
		{Namespace: "media", Kind: "deployment", Name: "idle", Desired: 0},
		{Namespace: "media", Kind: "statefulset", Name: "db", Desired: 1, Ready: 1, Available: 1},
		{Namespace: "kube-system", Kind: "daemonset", Name: "kube-proxy", Desired: 2, Ready: 1, Available: 1},
	}
	if !reflect.DeepEqual(state.Workloads, wantWorkloads) {
		t.Errorf("workloads:\n got %+v\nwant %+v", state.Workloads, wantWorkloads)
	}
	if missing := state.Workloads[0].Missing(); missing != 1 {
		t.Errorf("web misses %d replicas, want 1", missing)
	}

	web := state.Pods[0]
	wantWeb := Pod{
		Namespace: "media", Name: "web-7d9f8b6c5-x2k4p", UID: "u1", Node: "w1", Workload: "deployment/web",
		Phase: "Running", Reason: "CrashLoopBackOff", Created: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC).Unix(),
		Containers: []PodContainer{{Name: "web", ID: "abc", Image: "nginx:1.27", Restarts: 4, State: "waiting", Reason: "CrashLoopBackOff",
			LastReason: "OOMKilled", LastFinished: time.Date(2026, 11, 2, 11, 59, 0, 0, time.UTC).Unix()}},
	}
	if !reflect.DeepEqual(web, wantWeb) {
		t.Errorf("web pod:\n got %+v\nwant %+v", web, wantWeb)
	}
	if web.WorkloadKey() != "media/deployment/web" {
		t.Errorf("workload key %q", web.WorkloadKey())
	}
	for i, want := range []struct{ workload, reason string }{
		{"deployment/web", "CrashLoopBackOff"},
		{"statefulset/db", ""},
		{"pod/big", "Unschedulable"},
		{"pod/kube-apiserver-cp1", ""},
	} {
		if got := state.Pods[i]; got.Workload != want.workload || got.Reason != want.reason {
			t.Errorf("pod %s: workload %q reason %q, want %q %q", got.Name, got.Workload, got.Reason, want.workload, want.reason)
		}
	}

	wantEvents := []Event{
		{Namespace: "media", Kind: "Pod", Name: "web-7d9f8b6c5-x2k4p", Reason: "BackOff", Message: "Back-off restarting failed container",
			Count: 12, LastSeen: time.Date(2026, 11, 2, 11, 59, 30, 0, time.UTC).Unix()},
		{Namespace: "default", Kind: "Pod", Name: "big", Reason: "FailedScheduling", Message: "0/2 nodes are available",
			Count: 7, LastSeen: time.Date(2026, 11, 2, 11, 55, 0, 0, time.UTC).Unix()},
	}
	if !reflect.DeepEqual(state.Events, wantEvents) {
		t.Errorf("events:\n got %+v\nwant %+v", state.Events, wantEvents)
	}
}

func TestReadPages(t *testing.T) {
	asked := 0
	client := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/nodes" {
			w.Write([]byte(`{"metadata":{},"items":[]}`))
			return
		}
		asked++
		if r.URL.Query().Get("limit") != "500" {
			t.Errorf("no limit: %s", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("continue") {
		case "":
			w.Write([]byte(`{"metadata":{"continue":"page2"},"items":[{"metadata":{"name":"a"}}]}`))
		case "page2":
			w.Write([]byte(`{"metadata":{},"items":[{"metadata":{"name":"b"}}]}`))
		default:
			t.Errorf("unexpected continue %q", r.URL.Query().Get("continue"))
		}
	})
	state := client.Read(context.Background(), "homelab", now)
	if state.Error != "" || asked != 2 || len(state.Nodes) != 2 || state.Nodes[1].Name != "b" {
		t.Errorf("pages not followed: asked %d, %+v %s", asked, state.Nodes, state.Error)
	}
}

func TestReadErrors(t *testing.T) {
	client := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"kind":"Status","message":"nodes is forbidden: User \"system:serviceaccount:symon:symon-reader\" cannot list resource \"nodes\""}`))
	})
	state := client.Read(context.Background(), "homelab", now)
	want := `the API answered 403 Forbidden for /api/v1/nodes: nodes is forbidden: User "system:serviceaccount:symon:symon-reader" cannot list resource "nodes"`
	if state.Error != want {
		t.Errorf("got %q\nwant %q", state.Error, want)
	}
	if state.Cluster != "homelab" || state.Nodes != nil {
		t.Errorf("a failed read should only carry the error: %+v", state)
	}

	// a missing token file says so without reaching the API
	client.tokenPath = filepath.Join(t.TempDir(), "missing")
	if state := client.Read(context.Background(), "homelab", now); !strings.HasPrefix(state.Error, "cannot read the token") {
		t.Errorf("got %q", state.Error)
	}
	if strings.Contains(state.Error, "secret-token") {
		t.Error("the token is in the error")
	}
}

func TestNewClientChecksSettings(t *testing.T) {
	if _, err := NewClient("http://10.0.0.1:6443", "", "/tmp/token"); err == nil {
		t.Error("accepted a plain http API URL")
	}
	if _, err := NewClient("https://10.0.0.1:6443", filepath.Join(t.TempDir(), "missing.crt"), "/tmp/token"); err == nil {
		t.Error("accepted a missing CA")
	}
}

func TestQuantity(t *testing.T) {
	for q, want := range map[string]float64{"4": 4, "3800m": 3.8, "8023488Ki": 8023488 * 1024, "16Gi": 16 << 30, "1G": 1e9, "1e3": 1000, "": 0, "abc": 0} {
		if got := quantity(q); got != want {
			t.Errorf("quantity(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestRestarts(t *testing.T) {
	prev := ClusterState{Time: 1000, Pods: []Pod{
		{UID: "u1", Namespace: "media", Name: "web-1", Workload: "deployment/web", Created: 500,
			Containers: []PodContainer{{Name: "web", Restarts: 2}, {Name: "sidecar", Restarts: 0}}},
		{UID: "u2", Namespace: "media", Name: "db-0", Workload: "statefulset/db", Created: 500,
			Containers: []PodContainer{{Name: "postgres", Restarts: 5}}},
	}}
	next := ClusterState{Time: 1030, Pods: []Pod{
		// two more for web, sidecar the same
		{UID: "u1", Namespace: "media", Name: "web-1", Workload: "deployment/web", Created: 500,
			Containers: []PodContainer{{Name: "web", Restarts: 4, LastReason: "OOMKilled"}, {Name: "sidecar", Restarts: 0}}},
		// the kubelet lost count, 1 is new
		{UID: "u2", Namespace: "media", Name: "db-0", Workload: "statefulset/db", Created: 500,
			Containers: []PodContainer{{Name: "postgres", Restarts: 1, LastReason: "Error"}}},
		// new since the last state, so its restarts are new too
		{UID: "u3", Namespace: "media", Name: "web-2", Workload: "deployment/web", Created: 1010,
			Containers: []PodContainer{{Name: "web", Restarts: 1, LastReason: "Error"}}},
		// first seen but older than the last state, so history
		{UID: "u4", Namespace: "default", Name: "old", Workload: "pod/old", Created: 100,
			Containers: []PodContainer{{Name: "app", Restarts: 30}}},
	}}
	want := []Restart{
		{Namespace: "media", Workload: "deployment/web", Pod: "web-1", Container: "web", Count: 2, Reason: "OOMKilled"},
		{Namespace: "media", Workload: "statefulset/db", Pod: "db-0", Container: "postgres", Count: 1, Reason: "Error"},
		{Namespace: "media", Workload: "deployment/web", Pod: "web-2", Container: "web", Count: 1, Reason: "Error"},
	}
	if got := Restarts(prev, next); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got := Restarts(ClusterState{}, next); got != nil {
		t.Errorf("the first state counted restarts: %+v", got)
	}
}
