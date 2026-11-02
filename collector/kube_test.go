package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/kube"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/notify"
	"github.com/dhamith93/SyMon/internal/store"
)

// kubeRun feeds an evaluator cluster states, one per step
type kubeRun struct {
	t    *testing.T
	fake *fakeStore
	e    *evaluator
	sent []notice
	now  time.Time
}

func newKubeRun(t *testing.T) *kubeRun {
	r := &kubeRun{t: t, fake: &fakeStore{}, now: time.Unix(1700000000, 0)}
	r.e = newEvaluator(r.fake, func(_ context.Context, n notice) { r.sent = append(r.sent, n) })
	r.e.now = func() time.Time { return r.now }
	return r
}

// step reports state 30 seconds after the last one and evaluates rule
func (r *kubeRun) step(rule alerts.AlertConfig, state kube.ClusterState) []notice {
	r.t.Helper()
	r.now = r.now.Add(30 * time.Second)
	state.Cluster, state.Time = "homelab", r.now.Unix()
	r.fake.cluster = store.Cluster{Name: "homelab", Host: "cp1", State: state}
	before := len(r.sent)
	if err := r.e.evaluate(context.Background(), &rule, ""); err != nil {
		r.t.Fatal(err)
	}
	return r.sent[before:]
}

func (r *kubeRun) open() []store.Alert {
	var open []store.Alert
	for _, a := range r.fake.alerts {
		if a.ResolvedAt == nil {
			open = append(open, a)
		}
	}
	return open
}

var workloadRule = alerts.AlertConfig{Name: "Workloads", MetricName: monitor.KUBE_WORKLOAD, Cluster: "homelab", TriggerIntveral: 60}

func webState(available int) kube.ClusterState {
	return kube.ClusterState{
		Workloads: []kube.Workload{
			{Namespace: "media", Kind: "deployment", Name: "web", Desired: 2, Ready: available, Available: available},
			{Namespace: "kube-system", Kind: "daemonset", Name: "kube-proxy", Desired: 2, Ready: 2, Available: 2},
		},
		Pods: []kube.Pod{
			{Namespace: "media", Name: "web-1", UID: "u1", Workload: "deployment/web", Phase: "Running", Ready: true},
			{Namespace: "media", Name: "web-2", UID: "u2", Workload: "deployment/web", Phase: "Running", Reason: "CrashLoopBackOff"},
		},
	}
}

func TestKubeWorkloadAlert(t *testing.T) {
	r := newKubeRun(t)
	if sent := r.step(workloadRule, webState(1)); len(sent) != 0 {
		t.Fatalf("opened before the trigger interval: %+v", sent)
	}
	r.step(workloadRule, webState(1))
	sent := r.step(workloadRule, webState(1))
	if len(sent) != 1 || sent[0].event != notify.Opened || sent[0].severity != int(alertstatus.Critical) || sent[0].target != "media/deployment/web" || sent[0].host != "" {
		t.Fatalf("expected web to open after 60 seconds, got %+v", sent)
	}
	if !strings.Contains(sent[0].subject, "media/deployment/web is missing 1 replica in homelab") || !strings.Contains(sent[0].body, "CrashLoopBackOff") {
		t.Errorf("unexpected text %q / %q", sent[0].subject, sent[0].body)
	}
	if open := r.open(); len(open) != 1 || open[0].Target != "media/deployment/web" {
		t.Errorf("expected only web open, got %+v", open)
	}

	// all replicas again, resolved once that has lasted the interval
	r.step(workloadRule, webState(2))
	r.step(workloadRule, webState(2))
	sent = r.step(workloadRule, webState(2))
	if len(sent) != 1 || sent[0].event != notify.Resolved || !strings.Contains(sent[0].subject, "has all its replicas again") {
		t.Fatalf("expected web to resolve, got %+v", sent)
	}
}

func TestKubeAlertResolvesWhenGone(t *testing.T) {
	r := newKubeRun(t)
	for range 3 {
		r.step(workloadRule, webState(0))
	}
	if len(r.open()) != 1 {
		t.Fatalf("expected web to be open, got %+v", r.fake.alerts)
	}
	gone := webState(0)
	gone.Workloads = gone.Workloads[1:]
	gone.Pods = nil
	sent := r.step(workloadRule, gone)
	if len(sent) != 1 || sent[0].event != notify.Resolved || !strings.Contains(sent[0].subject, "media/deployment/web is gone from homelab") {
		t.Fatalf("expected the deleted deployment to resolve, got %+v", sent)
	}
	if len(r.open()) != 0 {
		t.Errorf("still open: %+v", r.open())
	}
	// back and broken right away waits the interval again
	if sent := r.step(workloadRule, webState(0)); len(sent) != 0 {
		t.Errorf("a target that came back opened without waiting: %+v", sent)
	}
}

func TestKubeStaleState(t *testing.T) {
	r := newKubeRun(t)
	for range 3 {
		r.step(workloadRule, webState(1))
	}
	// the reader stops: the old state is not checked again once stale
	r.now = r.now.Add(kube.StaleAfter + time.Minute)
	before := len(r.sent)
	if err := r.e.evaluate(context.Background(), &workloadRule, ""); err != nil {
		t.Fatal(err)
	}
	if len(r.sent) != before || len(r.open()) != 1 {
		t.Errorf("a stale state changed alerts: %+v", r.sent[before:])
	}
	// no cluster at all is no data
	other := workloadRule
	other.Cluster = "lab2"
	if err := r.e.evaluate(context.Background(), &other, ""); err != nil {
		t.Errorf("an unknown cluster should be no data, got %v", err)
	}
}

func TestKubeNodeAlert(t *testing.T) {
	rule := alerts.AlertConfig{Name: "Nodes", MetricName: monitor.KUBE_NODE, Cluster: "homelab", Namespace: "media"}
	state := kube.ClusterState{Nodes: []kube.Node{{Name: "cp1", Ready: true}, {Name: "w1", Pressure: []string{"DiskPressure"}}}}
	r := newKubeRun(t)
	r.step(rule, state)
	sent := r.step(rule, state)
	if len(sent) != 1 || sent[0].target != "w1" || !strings.Contains(sent[0].subject, "node w1 is not ready in homelab") || !strings.Contains(sent[0].body, "It reports DiskPressure") {
		t.Fatalf("expected w1 to open, the namespace does not matter for nodes, got %+v", sent)
	}
}

func TestKubePendingAndRestarts(t *testing.T) {
	state := kube.ClusterState{
		Workloads: []kube.Workload{{Namespace: "media", Kind: "deployment", Name: "web", Desired: 1, Available: 1}},
		Pods: []kube.Pod{
			{Namespace: "media", Name: "web-1", UID: "u1", Workload: "deployment/web", Phase: "Running", Ready: true,
				Containers: []kube.PodContainer{{Name: "web", LastReason: "OOMKilled"}}},
			{Namespace: "default", Name: "big", UID: "u2", Workload: "pod/big", Phase: "Pending", Reason: "Unschedulable"},
			{Namespace: "batch", Name: "backup-1", UID: "u3", Workload: "job/backup-1", Phase: "Pending"},
		},
	}

	pending := alerts.AlertConfig{Name: "Pending", MetricName: monitor.KUBE_PENDING, Cluster: "homelab", Namespace: "default"}
	r := newKubeRun(t)
	r.step(pending, state)
	sent := r.step(pending, state)
	if len(sent) != 1 || sent[0].target != "default/pod/big" || !strings.Contains(sent[0].subject, "1 pod of default/pod/big stuck pending") || !strings.Contains(sent[0].body, "Unschedulable") {
		t.Fatalf("expected only the pending pod in default, got %+v", sent)
	}

	restarts := alerts.AlertConfig{Name: "Restarts", MetricName: monitor.KUBE_RESTARTS, Cluster: "homelab", Op: ">", WarnThreshold: 2, CriticalThreshold: 5}
	r = newKubeRun(t)
	r.fake.restarts = map[string]int{"media/deployment/web": 3}
	r.step(restarts, state)
	sent = r.step(restarts, state)
	if len(sent) != 1 || sent[0].severity != int(alertstatus.Warning) || !strings.Contains(sent[0].subject, "[Warning] Restarts: media/deployment/web restarted 3 times in the last hour") || !strings.Contains(sent[0].body, "OOMKilled") {
		t.Fatalf("expected a restart warning for web, got %+v", sent)
	}
	r.fake.restarts = map[string]int{"media/deployment/web": 6}
	if sent := r.step(restarts, state); len(sent) != 1 || sent[0].event != notify.Changed || sent[0].severity != int(alertstatus.Critical) {
		t.Errorf("expected web to turn critical, got %+v", sent)
	}
}
