package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/snmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeDevicePoller struct {
	mu sync.Mutex
	// errors is what polling each device fails with, none by default
	errors map[string]string
	polled []string
	forgot []string
}

func (f *fakeDevicePoller) Poll(ctx context.Context, device snmp.Device, now time.Time) snmp.DeviceReport {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polled = append(f.polled, device.Name)
	return snmp.DeviceReport{
		Device: device.Name, Time: now.Unix(), IntervalSeconds: device.IntervalSeconds, Address: device.Address,
		Error:  f.errors[device.Name],
		Values: []snmp.Value{{Metric: snmp.CPUPct, Value: 5}},
	}
}

func (f *fakeDevicePoller) Forget(name string) {
	f.forgot = append(f.forgot, name)
}

func (f *fakeDevicePoller) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	polled := slices.Sorted(slices.Values(f.polled))
	f.polled = nil
	return polled
}

type fakeDeviceClient struct {
	reports []*api.DeviceReport
	err     error
}

func (f *fakeDeviceClient) ReportDevice(ctx context.Context, in *api.DeviceReport, opts ...grpc.CallOption) (*api.Message, error) {
	f.reports = append(f.reports, in)
	return &api.Message{Body: "ok"}, f.err
}

func (f *fakeDeviceClient) last(t *testing.T) snmp.DeviceReport {
	t.Helper()
	var report snmp.DeviceReport
	if err := json.Unmarshal([]byte(f.reports[len(f.reports)-1].ReportJson), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func testDevicePoller(fake *fakeDevicePoller, client *fakeDeviceClient, load func(string) ([]snmp.Device, error), now *time.Time) *devicePoller {
	return &devicePoller{
		path: "/etc/symon/snmp.json", host: "pve", interval: time.Minute,
		poller: fake, client: client, load: load, now: func() time.Time { return *now },
		pollErrors: map[string]string{}, reportErrors: map[string]string{}, due: map[string]time.Time{},
	}
}

func TestDevicePollerSchedule(t *testing.T) {
	devices := []snmp.Device{
		{Name: "sw1", Address: "10.0.0.2", Community: "c", Profiles: []string{snmp.ProfileInterfaces}},
		{Name: "ups1", Address: "10.0.0.3", Community: "c", Profiles: []string{snmp.ProfileUPS}, IntervalSeconds: 120},
	}
	var loadErr error
	fake := &fakeDevicePoller{errors: map[string]string{}}
	client := &fakeDeviceClient{}
	now := time.Unix(1700000000, 0)
	p := testDevicePoller(fake, client, func(string) ([]snmp.Device, error) { return devices, loadErr }, &now)

	if wait := p.step(); wait != time.Minute {
		t.Errorf("waits %v, want a minute", wait)
	}
	if polled := fake.take(); !slices.Equal(polled, []string{"sw1", "ups1"}) || len(client.reports) != 2 || client.reports[0].Host != "pve" {
		t.Fatalf("unexpected first round: polled %v, reports %v", polled, client.reports)
	}
	var first, second snmp.DeviceReport
	json.Unmarshal([]byte(client.reports[0].ReportJson), &first)
	json.Unmarshal([]byte(client.reports[1].ReportJson), &second)
	if first.Device != "sw1" || first.IntervalSeconds != 60 || second.Device != "ups1" || second.IntervalSeconds != 120 || first.Time != now.Unix() {
		t.Errorf("unexpected reports %+v %+v", first, second)
	}

	// a minute later only the switch is due, after two both are
	now = now.Add(time.Minute)
	p.step()
	if polled := fake.take(); !slices.Equal(polled, []string{"sw1"}) {
		t.Errorf("after a minute polled %v, want only sw1", polled)
	}
	now = now.Add(time.Minute)
	p.step()
	if polled := fake.take(); !slices.Equal(polled, []string{"sw1", "ups1"}) {
		t.Errorf("after two minutes polled %v, want both", polled)
	}

	// a broken file stops polling until it is fixed
	loadErr = errors.New("/etc/symon/snmp.json: group or others can read it, run chmod 600 on it")
	now = now.Add(time.Minute)
	if wait := p.step(); wait != time.Minute || len(fake.take()) != 0 || p.fileError == "" {
		t.Errorf("a broken file did not stop polling: %v %q", wait, p.fileError)
	}
	loadErr = nil
	now = now.Add(time.Minute)
	p.step()
	if polled := fake.take(); p.fileError != "" || !slices.Equal(polled, []string{"sw1", "ups1"}) {
		t.Errorf("polling did not resume: %q %v", p.fileError, polled)
	}

	// a device that leaves the file is forgotten
	devices = devices[:1]
	now = now.Add(time.Minute)
	p.step()
	if polled := fake.take(); !slices.Equal(fake.forgot, []string{"ups1"}) || !slices.Equal(polled, []string{"sw1"}) {
		t.Errorf("ups1 was not forgotten: forgot %v, polled %v", fake.forgot, polled)
	}

	// a device that cannot be read is still reported, with its error
	fake.errors["sw1"] = "cannot reach 10.0.0.2: request timeout (after 1 retries)"
	now = now.Add(time.Minute)
	p.step()
	if report := client.last(t); report.Error != fake.errors["sw1"] || len(report.Values) != 1 || p.pollErrors["sw1"] == "" {
		t.Errorf("a failed poll was not reported: %+v %q", report, p.pollErrors["sw1"])
	}
}

func TestDevicePollerCollectorErrors(t *testing.T) {
	devices := []snmp.Device{{Name: "sw1", Address: "10.0.0.2", Community: "c", Profiles: []string{snmp.ProfileInterfaces}}}
	fake := &fakeDevicePoller{}
	client := &fakeDeviceClient{err: status.Error(codes.Unimplemented, "unknown method")}
	now := time.Unix(1700000000, 0)
	p := testDevicePoller(fake, client, func(string) ([]snmp.Device, error) { return devices, nil }, &now)

	// a collector from before devices is left alone for a while
	if wait := p.step(); wait != snmpOldCollector {
		t.Errorf("waits %v for an old collector, want %v", wait, snmpOldCollector)
	}
	// a collector that is away is tried again next round
	client.err = status.Error(codes.Unavailable, "collector away")
	now = now.Add(time.Minute)
	if wait := p.step(); wait != time.Minute || p.reportErrors["sw1"] == "" {
		t.Errorf("an unavailable collector: waits %v, error %q", wait, p.reportErrors["sw1"])
	}
	client.err = nil
	now = now.Add(time.Minute)
	p.step()
	if p.reportErrors["sw1"] != "" || len(client.reports) != 3 {
		t.Errorf("reporting did not recover: %q %d", p.reportErrors["sw1"], len(client.reports))
	}
}
