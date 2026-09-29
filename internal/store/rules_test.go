package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/monitor"
)

func cpuRule(name string) alerts.AlertConfig {
	return alerts.AlertConfig{Name: name, MetricName: monitor.PROC_USAGE, Servers: []string{alerts.AllHosts}, Op: ">", WarnThreshold: 80, CriticalThreshold: 95, TriggerIntveral: 60}
}

func TestAlertRules(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.AddHost(ctx, "web1", "UTC"); err != nil {
		t.Fatal(err)
	}

	id, err := st.CreateRule(ctx, cpuRule("CPU"), true, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRule(ctx, cpuRule("CPU"), true, "alice"); !errors.Is(err, ErrRuleExists) {
		t.Errorf("expected ErrRuleExists for a taken name, got %v", err)
	}
	broken := cpuRule("Broken")
	broken.Op = "=>"
	if _, err := st.CreateRule(ctx, broken, true, "alice"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected ErrInvalid for a bad rule, got %v", err)
	}
	if _, err := st.CreateRule(ctx, cpuRule("Off"), false, "alice"); err != nil {
		t.Fatal(err)
	}

	rules, err := st.AlertRules(ctx)
	if err != nil || len(rules) != 2 || rules[0].Rule.Name != "CPU" || rules[0].UpdatedBy != "alice" || rules[0].Rule.Servers[0] != "*" {
		t.Fatalf("expected CPU and Off, got %+v %v", rules, err)
	}
	enabled, err := st.EnabledRules(ctx)
	if err != nil || len(enabled) != 1 || enabled[0].Name != "CPU" {
		t.Errorf("expected only CPU enabled, got %+v %v", enabled, err)
	}

	// an open alert and endpoint history follow a rename
	at := time.Now().Truncate(time.Second)
	if _, err := st.CreateAlert(ctx, Alert{Host: "web1", Rule: "CPU", Metric: monitor.PROC_USAGE, Severity: 2, StartedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEndpointCheck(ctx, EndpointCheck{Time: at, Name: "CPU", URL: "https://example.com", Method: "GET", StatusCode: 200, OK: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRule(ctx, id, cpuRule("CPU usage"), true, "bob"); err != nil {
		t.Fatal(err)
	}
	if open, err := st.OpenAlert(ctx, "web1", "CPU usage", monitor.PROC_USAGE, ""); err != nil || open == nil {
		t.Errorf("expected the open alert under the new name, got %+v %v", open, err)
	}
	if check, err := st.LatestEndpointCheck(ctx, "CPU usage"); err != nil || check.StatusCode != 200 {
		t.Errorf("expected the check history under the new name, got %+v %v", check, err)
	}
	if err := st.UpdateRule(ctx, id, cpuRule("Off"), true, "bob"); !errors.Is(err, ErrRuleExists) {
		t.Errorf("expected ErrRuleExists renaming onto a taken name, got %v", err)
	}
	if err := st.UpdateRule(ctx, 99999, cpuRule("Nope"), true, "bob"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for an unknown rule, got %v", err)
	}

	// disabling a rule resolves its open alerts
	if err := st.UpdateRule(ctx, id, cpuRule("CPU usage"), false, "bob"); err != nil {
		t.Fatal(err)
	}
	if open, err := st.OpenAlert(ctx, "web1", "CPU usage", monitor.PROC_USAGE, ""); err != nil || open != nil {
		t.Errorf("expected the alert resolved with the rule disabled, got %+v %v", open, err)
	}

	// so does deleting one
	if err := st.UpdateRule(ctx, id, cpuRule("CPU usage"), true, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAlert(ctx, Alert{Host: "web1", Rule: "CPU usage", Metric: monitor.PROC_USAGE, Severity: 1, StartedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteRule(ctx, id); err != nil {
		t.Fatal(err)
	}
	if open, err := st.OpenAlert(ctx, "web1", "CPU usage", monitor.PROC_USAGE, ""); err != nil || open != nil {
		t.Errorf("expected the alert resolved with the rule deleted, got %+v %v", open, err)
	}
	if err := st.DeleteRule(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound deleting twice, got %v", err)
	}
}

func TestSetupRules(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	fromFile := []alerts.AlertConfig{cpuRule("CPU"), {Name: "Web heartbeat", MetricName: monitor.PING, Servers: []string{"web1"}, TriggerIntveral: 120}}
	setup, err := st.SetupRules(ctx, fromFile, "/etc/symon/alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	// a heartbeat for one host does not cover the others
	if setup.Done || setup.Imported != 2 || len(setup.Added) != 1 || setup.Added[0] != DefaultHeartbeat.Name {
		t.Errorf("expected 2 imported and the default heartbeat added, got %+v", setup)
	}
	if done, err := st.RulesSetUp(ctx); err != nil || !done {
		t.Errorf("expected the rules to count as set up, got %v %v", done, err)
	}

	// it only happens once, so a deleted default stays deleted
	rules, _ := st.AlertRules(ctx)
	for _, rule := range rules {
		if rule.Rule.Name == DefaultHeartbeat.Name {
			if err := st.DeleteRule(ctx, rule.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if again, err := st.SetupRules(ctx, fromFile, "/etc/symon/alerts.json"); err != nil || !again.Done {
		t.Errorf("expected nothing to happen the second time, got %+v %v", again, err)
	}
	if rules, _ := st.AlertRules(ctx); len(rules) != 2 {
		t.Errorf("expected the 2 imported rules, got %+v", rules)
	}

	// importing a file later replaces rules by name and adds new ones
	changed := cpuRule("CPU")
	changed.WarnThreshold = 70
	added, updated, err := st.ImportRules(ctx, []alerts.AlertConfig{changed, cpuRule("CPU spikes")}, "alice")
	if err != nil || added != 1 || updated != 1 {
		t.Fatalf("expected 1 added and 1 updated, got %d %d %v", added, updated, err)
	}
	rules, _ = st.AlertRules(ctx)
	if len(rules) != 3 || rules[0].Rule.Name != "CPU" || rules[0].Rule.WarnThreshold != 70 || rules[0].UpdatedBy != "alice" {
		t.Errorf("expected CPU at 70 and a new rule, got %+v", rules)
	}
}

func TestSetupRulesKeepsAnAllHostsHeartbeat(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	heartbeat := alerts.AlertConfig{Name: "Silent", MetricName: monitor.PING, Servers: []string{alerts.AllHosts}, TriggerIntveral: 60}
	setup, err := st.SetupRules(ctx, []alerts.AlertConfig{heartbeat}, "alerts.json")
	if err != nil || setup.Imported != 1 || len(setup.Added) != 0 {
		t.Errorf("expected no default next to an all hosts heartbeat, got %+v %v", setup, err)
	}
}
