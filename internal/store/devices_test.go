package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/snmp"
)

// switchReport is a two port switch with rx bits per second on the uplink
func switchReport(at time.Time, rx float64) snmp.DeviceReport {
	return snmp.DeviceReport{
		Device: "sw1", Time: at.Unix(), IntervalSeconds: 30, Kind: snmp.KindSwitch, Address: "10.0.0.2",
		System:     snmp.System{Name: "sw1", UptimeSeconds: 86400},
		Interfaces: []snmp.Interface{{Name: "Gi1", Up: true, AdminUp: true, SpeedBps: 1_000_000_000}, {Name: "Gi2", AdminUp: true}},
		Values: []snmp.Value{
			{Metric: snmp.IfUp, Label: "Gi1", Value: 1},
			{Metric: snmp.IfUp, Label: "Gi2", Value: 0},
			{Metric: snmp.IfRxBps, Label: "Gi1", Unit: "bps", Value: rx},
			{Metric: snmp.CPUPct, Unit: "%", Value: 12},
		},
	}
}

func TestDeviceReports(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, host := range []string{"pve", "nas"} {
		if err := st.AddHost(ctx, host, "UTC"); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SaveDeviceReport(ctx, "ghost", switchReport(time.Now(), 0)); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected an unregistered host to be refused, got %v", err)
	}
	if err := st.SaveDeviceReport(ctx, "pve", snmp.DeviceReport{Time: time.Now().Unix()}); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected a nameless device to be refused, got %v", err)
	}

	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	if err := st.SaveDeviceReport(ctx, "pve", switchReport(start, 1000)); err != nil {
		t.Fatal(err)
	}
	// another host cannot report the same device
	if err := st.SaveDeviceReport(ctx, "nas", switchReport(start.Add(30*time.Second), 1000)); !errors.Is(err, ErrDeviceTaken) {
		t.Errorf("expected another host to be refused, got %v", err)
	}
	if err := st.SaveDeviceReport(ctx, "pve", switchReport(start.Add(30*time.Second), 2000)); err != nil {
		t.Fatal(err)
	}
	// the switch stops answering: the last state stays, with the error
	failed := snmp.DeviceReport{Device: "sw1", Time: start.Add(time.Minute).Unix(), IntervalSeconds: 30, Kind: snmp.KindSwitch, Address: "10.0.0.2", Error: "cannot reach 10.0.0.2: request timeout"}
	if err := st.SaveDeviceReport(ctx, "pve", failed); err != nil {
		t.Fatal(err)
	}
	device, err := st.Device(ctx, "sw1")
	if err != nil {
		t.Fatal(err)
	}
	if device.Host != "pve" || device.Error != failed.Error || device.State.Time != start.Add(30*time.Second).Unix() || len(device.State.Values) != 4 || device.Interval != 30*time.Second || !device.ReportedAt.Equal(start.Add(time.Minute)) {
		t.Errorf("a failed poll should keep the last state and add the error: %+v", device)
	}
	if device.Stale(start.Add(2*time.Minute)) || !device.Stale(start.Add(3*time.Minute)) {
		t.Error("stale should be three missed polls after the last report")
	}
	if err := st.SaveDeviceReport(ctx, "pve", switchReport(start.Add(90*time.Second), 3000)); err != nil {
		t.Fatal(err)
	}
	// a late report is ignored
	if err := st.SaveDeviceReport(ctx, "pve", switchReport(start.Add(45*time.Second), 9999)); err != nil {
		t.Fatal(err)
	}
	// a device that never answered still shows up, with the error
	if err := st.SaveDeviceReport(ctx, "pve", snmp.DeviceReport{Device: "ups1", Time: start.Unix(), Kind: snmp.KindUPS, Address: "10.0.0.3", Error: "cannot reach 10.0.0.3: request timeout"}); err != nil {
		t.Fatal(err)
	}

	devices, err := st.Devices(ctx)
	if err != nil || len(devices) != 2 || devices[0].Name != "sw1" || devices[0].Error != "" || devices[0].State.Values[2].Value != 3000 || devices[0].Kind != snmp.KindSwitch {
		t.Fatalf("unexpected devices %+v %v", devices, err)
	}
	if devices[1].Name != "ups1" || devices[1].Error == "" || devices[1].State.Time != 0 || devices[1].State.Kind != snmp.KindUPS || devices[1].Interval != time.Minute {
		t.Errorf("unexpected never read device %+v", devices[1])
	}

	result, err := st.DeviceSeries(ctx, "sw1", snmp.IfRxBps, start.Add(-time.Minute), start.Add(5*time.Minute), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 1 || result.Series[0].Label != "Gi1" || len(result.Series[0].Points) != 3 || result.Series[0].Points[0].Value != 1000 || result.Series[0].Points[2].Value != 3000 {
		t.Errorf("unexpected rx series %+v", result)
	}
	up, err := st.DeviceSeries(ctx, "sw1", snmp.IfUp, start.Add(-time.Minute), start.Add(5*time.Minute), 1000)
	if err != nil || len(up.Series) != 2 || up.Series[0].Label != "Gi1" || up.Series[1].Label != "Gi2" || up.Series[1].Points[0].Value != 0 {
		t.Errorf("unexpected link series %+v %v", up, err)
	}
	if _, err := st.DeviceSeries(ctx, "sw1", "", start, start.Add(time.Minute), 10); !errors.Is(err, ErrInvalid) {
		t.Errorf("expected an empty metric to be refused, got %v", err)
	}
	if _, err := st.DeviceSeries(ctx, "nope", snmp.IfRxBps, start, start.Add(time.Minute), 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected an unknown device to be not found, got %v", err)
	}

	// once its host is gone another may take the device over
	if err := st.RemoveHost(ctx, "pve"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveDeviceReport(ctx, "nas", switchReport(start.Add(2*time.Minute), 4000)); err != nil {
		t.Errorf("expected nas to take the device over, got %v", err)
	}
	if err := st.RemoveDevice(ctx, "sw1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Device(ctx, "sw1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected the device to be gone, got %v", err)
	}
	if err := st.RemoveDevice(ctx, "sw1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for a removed device, got %v", err)
	}
}
