package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/notify"
	"github.com/dhamith93/SyMon/internal/snmp"
	"github.com/dhamith93/SyMon/internal/store"
)

// deviceRun feeds an evaluator device reports, one set per step
type deviceRun struct {
	t    *testing.T
	fake *fakeStore
	e    *evaluator
	sent []notice
	now  time.Time
}

func newDeviceRun(t *testing.T) *deviceRun {
	r := &deviceRun{t: t, fake: &fakeStore{}, now: time.Unix(1700000000, 0)}
	r.e = newEvaluator(r.fake, func(_ context.Context, n notice) { r.sent = append(r.sent, n) })
	r.e.now = func() time.Time { return r.now }
	return r
}

// step reports the devices 30 seconds after the last step and evaluates
// rule. A device with an error keeps the state it was given.
func (r *deviceRun) step(rule alerts.AlertConfig, devices ...store.Device) []notice {
	r.t.Helper()
	r.now = r.now.Add(30 * time.Second)
	for i := range devices {
		devices[i].ReportedAt = r.now
		devices[i].Interval = 30 * time.Second
		devices[i].Host = "pve"
		devices[i].State.Device = devices[i].Name
		if devices[i].Error == "" {
			devices[i].State.Time = r.now.Unix()
		}
	}
	r.fake.devices = devices
	before := len(r.sent)
	if err := r.e.evaluate(context.Background(), &rule, ""); err != nil {
		r.t.Fatal(err)
	}
	return r.sent[before:]
}

func (r *deviceRun) open() []store.Alert {
	var open []store.Alert
	for _, a := range r.fake.alerts {
		if a.ResolvedAt == nil {
			open = append(open, a)
		}
	}
	return open
}

func switchDevice(uplinkUp bool) store.Device {
	up := 0.0
	if uplinkUp {
		up = 1
	}
	return store.Device{Name: "sw1", Kind: snmp.KindSwitch, State: snmp.DeviceReport{
		Interfaces: []snmp.Interface{{Name: "Gi1/0/24", Alias: "uplink", AdminUp: true, Up: uplinkUp}, {Name: "Gi1/0/1", AdminUp: true, Up: true}},
		Values:     []snmp.Value{{Metric: snmp.IfUp, Label: "Gi1/0/24", Value: up}, {Metric: snmp.IfUp, Label: "Gi1/0/1", Value: 1}},
	}}
}

func upsDevice(charge float64, onBattery bool) store.Device {
	battery := 0.0
	if onBattery {
		battery = 1
	}
	return store.Device{Name: "ups1", Kind: snmp.KindUPS, State: snmp.DeviceReport{
		Values: []snmp.Value{{Metric: snmp.UPSChargePct, Unit: "%", Value: charge}, {Metric: snmp.UPSOnBattery, Value: battery}},
	}}
}

func nasDevice() store.Device {
	return store.Device{Name: "nas", Kind: snmp.KindServer, State: snmp.DeviceReport{
		Values: []snmp.Value{
			{Metric: snmp.StorageUsedPct, Label: "/", Unit: "%", Value: 85},
			{Metric: snmp.StorageUsedPct, Label: "/volume1", Unit: "%", Value: 95},
			{Metric: snmp.CPUPct, Unit: "%", Value: 12},
		},
	}}
}

func TestDeviceDownAlert(t *testing.T) {
	rule := alerts.AlertConfig{Name: "Devices down", MetricName: monitor.DEVICE_DOWN, Device: alerts.AllHosts, TriggerIntveral: 60}
	r := newDeviceRun(t)
	dead := upsDevice(100, false)
	dead.Error = "cannot reach 10.0.0.3: request timeout (after 1 retries)"
	if sent := r.step(rule, switchDevice(true), dead); len(sent) != 0 {
		t.Fatalf("opened before the trigger interval: %+v", sent)
	}
	r.step(rule, switchDevice(true), dead)
	sent := r.step(rule, switchDevice(true), dead)
	if len(sent) != 1 || sent[0].event != notify.Opened || sent[0].severity != int(alertstatus.Critical) || sent[0].target != "ups1" || sent[0].host != "" {
		t.Fatalf("expected ups1 to open after 60 seconds, got %+v", sent)
	}
	if sent[0].subject != "[Critical] Devices down: ups1 is unreachable" || !strings.Contains(sent[0].body, "request timeout") {
		t.Errorf("unexpected text %q / %q", sent[0].subject, sent[0].body)
	}
	if open := r.open(); len(open) != 1 || open[0].Target != "ups1" {
		t.Errorf("expected only ups1 open, got %+v", open)
	}

	// answering again, resolved once that has lasted the interval
	r.step(rule, switchDevice(true), upsDevice(100, false))
	r.step(rule, switchDevice(true), upsDevice(100, false))
	sent = r.step(rule, switchDevice(true), upsDevice(100, false))
	if len(sent) != 1 || sent[0].event != notify.Resolved || sent[0].subject != "[Resolved] Devices down: ups1 answers again" {
		t.Fatalf("expected ups1 to resolve, got %+v", sent)
	}
}

func TestDevicePortAlert(t *testing.T) {
	rule := alerts.AlertConfig{Name: "Uplink", MetricName: monitor.DEVICE_PORT, Device: "sw1", Interface: "Gi1/0/24"}
	r := newDeviceRun(t)
	r.step(rule, switchDevice(false))
	sent := r.step(rule, switchDevice(false))
	if len(sent) != 1 || sent[0].target != "sw1/Gi1/0/24" || sent[0].subject != "[Critical] Uplink: port Gi1/0/24 (uplink) on sw1 is down" {
		t.Fatalf("expected the uplink to open, got %+v", sent)
	}
	r.step(rule, switchDevice(true))
	if sent := r.step(rule, switchDevice(true)); len(sent) != 1 || sent[0].event != notify.Resolved || !strings.Contains(sent[0].subject, "port Gi1/0/24 (uplink) on sw1 is up again") {
		t.Fatalf("expected the uplink to resolve, got %+v", sent)
	}

	// a port that disappears from the switch resolves as gone
	r.step(rule, switchDevice(false))
	r.step(rule, switchDevice(false))
	if len(r.open()) != 1 {
		t.Fatalf("expected the uplink to be open again, got %+v", r.fake.alerts)
	}
	gone := switchDevice(false)
	gone.State.Interfaces = gone.State.Interfaces[1:]
	sent = r.step(rule, gone)
	if len(sent) != 1 || sent[0].event != notify.Resolved || sent[0].subject != "[Resolved] Uplink: port Gi1/0/24 on sw1 is gone" {
		t.Fatalf("expected the missing port to resolve, got %+v", sent)
	}

	// an unreachable switch says nothing new about its ports
	r.step(rule, switchDevice(false))
	r.step(rule, switchDevice(false))
	dead := switchDevice(false)
	dead.Error = "cannot reach 10.0.0.2: request timeout"
	dead.State.Time = r.now.Unix()
	for range 3 {
		if sent := r.step(rule, dead); len(sent) != 0 {
			t.Errorf("an unreachable switch changed a port alert: %+v", sent)
		}
	}
}

func TestDeviceMetricAlert(t *testing.T) {
	rule := alerts.AlertConfig{Name: "Volumes", MetricName: monitor.DEVICE_METRIC, Device: "nas", DeviceMetric: snmp.StorageUsedPct, Op: ">", WarnThreshold: 80, CriticalThreshold: 90}
	r := newDeviceRun(t)
	r.step(rule, nasDevice())
	sent := r.step(rule, nasDevice())
	if len(sent) != 2 || sent[0].target != "nas//" || sent[0].severity != int(alertstatus.Warning) || sent[1].target != "nas//volume1" || sent[1].severity != int(alertstatus.Critical) {
		t.Fatalf("expected a warning for / and a critical for /volume1, got %+v", sent)
	}
	if sent[0].subject != "[Warning] Volumes: storage_used_pct of / on nas is 85% > 80%" || sent[1].body != "storage_used_pct of /volume1 on nas is 95% > 90%." {
		t.Errorf("unexpected text %q / %q", sent[0].subject, sent[1].body)
	}

	// a label keeps the rule to one volume
	one := rule
	one.Name, one.Label = "Volume 1", "/volume1"
	r = newDeviceRun(t)
	r.step(one, nasDevice())
	if sent := r.step(one, nasDevice()); len(sent) != 1 || sent[0].target != "nas//volume1" {
		t.Errorf("expected only /volume1, got %+v", sent)
	}

	// every device, and a scalar value
	battery := alerts.AlertConfig{Name: "On battery", MetricName: monitor.DEVICE_METRIC, Device: alerts.AllHosts, DeviceMetric: snmp.UPSOnBattery, Op: "==", WarnThreshold: 1, CriticalThreshold: 1}
	r = newDeviceRun(t)
	r.step(battery, switchDevice(true), upsDevice(80, true))
	sent = r.step(battery, switchDevice(true), upsDevice(80, true))
	if len(sent) != 1 || sent[0].target != "ups1" || sent[0].severity != int(alertstatus.Critical) || sent[0].subject != "[Critical] On battery: ups_on_battery on ups1 is 1 == 1" {
		t.Fatalf("expected ups1 on battery, got %+v", sent)
	}
	r.step(battery, switchDevice(true), upsDevice(80, false))
	if sent := r.step(battery, switchDevice(true), upsDevice(80, false)); len(sent) != 1 || sent[0].event != notify.Resolved || !strings.Contains(sent[0].subject, "ups_on_battery on ups1 is back at 0") {
		t.Errorf("expected ups1 to resolve, got %+v", sent)
	}
}

func TestDeviceStaleAndUnknown(t *testing.T) {
	rule := alerts.AlertConfig{Name: "Uplink", MetricName: monitor.DEVICE_PORT, Device: "sw1", Interface: "Gi1/0/24"}
	r := newDeviceRun(t)
	r.step(rule, switchDevice(false))
	r.step(rule, switchDevice(false))
	if len(r.open()) != 1 {
		t.Fatalf("expected the uplink to be open, got %+v", r.fake.alerts)
	}
	// the agent stops: nothing opens or resolves, and the alert is not gone
	r.now = r.now.Add(10 * time.Minute)
	before := len(r.sent)
	if err := r.e.evaluate(context.Background(), &rule, ""); err != nil {
		t.Fatal(err)
	}
	if len(r.sent) != before || len(r.open()) != 1 {
		t.Errorf("a stale device changed alerts: %+v", r.sent[before:])
	}
	other := rule
	other.Device = "sw9"
	if err := r.e.evaluate(context.Background(), &other, ""); err != nil {
		t.Errorf("an unknown device should be no data, got %v", err)
	}
}
