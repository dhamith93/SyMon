package snmp

import (
	"context"
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeSession answers from a map of OID to value, the way a device would
type fakeSession struct {
	values map[string]any
	err    error
	closed bool
}

func (f *fakeSession) Get(oids []string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]any{}
	for _, oid := range oids {
		if v, ok := f.values[oid]; ok {
			out[oid] = v
		}
	}
	return out, nil
}

func (f *fakeSession) Walk(root string, fn func(oid string, value any) error) error {
	if f.err != nil {
		return f.err
	}
	for _, oid := range slices.Sorted(maps.Keys(f.values)) {
		if strings.HasPrefix(oid, root+".") {
			if err := fn(oid, f.values[oid]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

func newPoller(s session, err error) *Poller {
	p := NewPoller()
	p.open = func(context.Context, Device) (session, error) { return s, err }
	return p
}

// byKey indexes a report's values as metric, or metric|label
func byKey(report DeviceReport) map[string]float64 {
	out := map[string]float64{}
	for _, v := range report.Values {
		key := v.Metric
		if v.Label != "" {
			key += "|" + v.Label
		}
		out[key] = v.Value
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// a switch with an uplink, an unplugged port and a VLAN interface, as
// snmpwalk shows them
func switchValues() map[string]any {
	ifRow := func(index string, descr string, typ int64, speed uint64, admin, oper int64, in, inErr, out, outErr uint64) map[string]any {
		row := oidIfTable + "."
		return map[string]any{
			row + ifDescr + "." + index: descr, row + ifType + "." + index: typ, row + ifSpeed + "." + index: speed,
			row + ifAdminStatus + "." + index: admin, row + ifOperStatus + "." + index: oper,
			row + ifInOctets + "." + index: in, row + ifInErrors + "." + index: inErr,
			row + ifOutOctets + "." + index: out, row + ifOutErrors + "." + index: outErr,
		}
	}
	values := map[string]any{
		oidSysDescr: "Cisco IOS Software", oidSysObjectID: "1.3.6.1.4.1.9.1.1208", oidSysUpTime: uint64(8_640_000), oidSysName: "sw1",
		// the uplink has 64 bit counters and an alias
		oidIfXTable + ".1.1": "Gi1/0/1", oidIfXTable + ".6.1": uint64(10_000_000), oidIfXTable + ".10.1": uint64(20_000_000), oidIfXTable + ".15.1": uint64(1000), oidIfXTable + ".18.1": "uplink",
		// the unplugged port only has the 32 bit ones
		oidIfXTable + ".1.2":  "Gi1/0/2",
		oidIfXTable + ".1.10": "Vl1",
	}
	maps.Copy(values, ifRow("1", "GigabitEthernet1/0/1", ifTypeEthernet, 1_000_000_000, 1, 1, 1000, 0, 2000, 1))
	maps.Copy(values, ifRow("2", "GigabitEthernet1/0/2", ifTypeEthernet, 100_000_000, 1, 2, 1000, 0, 2000, 0))
	maps.Copy(values, ifRow("10", "Vlan1", 53, 1_000_000_000, 1, 1, 500, 0, 500, 0))
	return values
}

func TestInterfacesProfile(t *testing.T) {
	fake := &fakeSession{values: switchValues()}
	p := newPoller(fake, nil)
	device := Device{Name: "sw1", Address: "10.0.0.2", Community: "public", Profiles: []string{ProfileInterfaces}}
	now := time.Unix(1700000000, 0)

	first := p.Poll(context.Background(), device, now)
	if first.Error != "" || !fake.closed {
		t.Fatalf("poll failed or left the session open: %+v", first)
	}
	if first.Kind != KindSwitch || first.System.Name != "sw1" || first.System.UptimeSeconds != 86400 || first.Address != "10.0.0.2" || first.Time != now.Unix() {
		t.Errorf("unexpected header %+v", first)
	}
	if len(first.Interfaces) != 2 {
		t.Fatalf("expected the two physical ports without the VLAN, got %+v", first.Interfaces)
	}
	uplink, spare := first.Interfaces[0], first.Interfaces[1]
	if uplink.Name != "Gi1/0/1" || uplink.Alias != "uplink" || !uplink.Up || !uplink.AdminUp || uplink.SpeedBps != 1_000_000_000 {
		t.Errorf("unexpected uplink %+v", uplink)
	}
	if spare.Name != "Gi1/0/2" || spare.Up || !spare.AdminUp || spare.SpeedBps != 100_000_000 {
		t.Errorf("unexpected spare port %+v", spare)
	}
	got := byKey(first)
	if got["if_up|Gi1/0/1"] != 1 || got["if_up|Gi1/0/2"] != 0 {
		t.Errorf("unexpected link values %v", got)
	}
	if _, ok := got["if_rx_bps|Gi1/0/1"]; ok {
		t.Error("a rate on the first poll, with nothing to compare against")
	}

	// 30 seconds later: 3 MB in and 1.5 MB out on the uplink, 3 errors, the
	// spare port's 32 bit out counter wrapped
	fake.values[oidIfXTable+".6.1"] = uint64(13_000_000)
	fake.values[oidIfXTable+".10.1"] = uint64(21_500_000)
	fake.values[oidIfTable+".20.1"] = uint64(4)
	fake.values[oidIfTable+".10.2"] = uint64(4000)
	fake.values[oidIfTable+".16.2"] = uint64(500)
	second := byKey(p.Poll(context.Background(), device, now.Add(30*time.Second)))
	if !near(second["if_rx_bps|Gi1/0/1"], 800_000) || !near(second["if_tx_bps|Gi1/0/1"], 400_000) {
		t.Errorf("unexpected uplink rates %v", second)
	}
	if !near(second["if_tx_errors_ps|Gi1/0/1"], 0.1) || !near(second["if_rx_errors_ps|Gi1/0/1"], 0) {
		t.Errorf("unexpected error rates %v", second)
	}
	if !near(second["if_rx_bps|Gi1/0/2"], 800) {
		t.Errorf("the 32 bit counters were not used: %v", second)
	}
	if _, ok := second["if_tx_bps|Gi1/0/2"]; ok {
		t.Error("a wrapped counter gave a rate")
	}

	// a port list keeps only those, in that order
	device.Interfaces = []string{"Gi1/0/2", "Gi1/0/1", "Gi1/0/48"}
	third := p.Poll(context.Background(), device, now.Add(60*time.Second))
	if len(third.Interfaces) != 2 || third.Interfaces[0].Name != "Gi1/0/2" || third.Interfaces[1].Name != "Gi1/0/1" {
		t.Errorf("unexpected picked ports %+v", third.Interfaces)
	}
}

func TestHostProfile(t *testing.T) {
	storage := oidHrStorageTable + "."
	fake := &fakeSession{values: map[string]any{
		oidSysDescr: "Linux nas 6.1", oidSysObjectID: "1.3.6.1.4.1.8072.3.2.10", oidSysUpTime: uint64(100), oidSysName: "nas",
		oidHrProcessorLoad + ".196608": int64(10), oidHrProcessorLoad + ".196609": int64(30),
		storage + "2.1": hrStorageRam, storage + "3.1": "Physical memory", storage + "4.1": int64(1024), storage + "5.1": int64(8_000_000), storage + "6.1": int64(4_000_000),
		storage + "2.3": "1.3.6.1.2.1.25.2.1.1", storage + "3.3": "Memory buffers", storage + "4.3": int64(1024), storage + "5.3": int64(8_000_000), storage + "6.3": int64(100),
		storage + "2.31": hrStorageFixedDisk, storage + "3.31": "/", storage + "4.31": int64(4096), storage + "5.31": int64(1000), storage + "6.31": int64(250),
		storage + "2.36": hrStorageFixedDisk, storage + "3.36": "/volume1", storage + "4.36": int64(4096), storage + "5.36": int64(0), storage + "6.36": int64(0),
	}}
	report := newPoller(fake, nil).Poll(context.Background(), Device{Name: "nas", Address: "10.0.0.4", Community: "public", Profiles: []string{ProfileHost}}, time.Now())
	if report.Error != "" || report.Kind != KindServer {
		t.Fatalf("unexpected report %+v", report)
	}
	got := byKey(report)
	if !near(got["cpu_pct"], 20) || !near(got["mem_used_pct"], 50) || !near(got["storage_used_pct|/"], 25) {
		t.Errorf("unexpected values %v", got)
	}
	if _, ok := got["storage_used_pct|/volume1"]; ok || len(got) != 3 {
		t.Errorf("an empty or non disk row was kept: %v", got)
	}
}

func TestUPSProfile(t *testing.T) {
	fake := &fakeSession{values: map[string]any{
		oidSysObjectID: "1.3.6.1.4.1.935.1", oidSysUpTime: uint64(100),
		oidUpsOutputSource: int64(3), oidUpsChargeRemaining: int64(100), oidUpsOutputLoad: int64(23), oidUpsMinutesRemaining: int64(45), oidUpsInputVoltage: int64(230), oidUpsBatteryTemperature: int64(28),
	}}
	p := newPoller(fake, nil)
	device := Device{Name: "ups", Address: "10.0.0.3", Community: "public", Profiles: []string{ProfileUPS}}
	report := p.Poll(context.Background(), device, time.Now())
	got := byKey(report)
	if report.Kind != KindUPS || got["ups_on_battery"] != 0 || got["ups_charge_pct"] != 100 || got["ups_load_pct"] != 23 || got["ups_minutes_left"] != 45 || got["ups_input_volts"] != 230 || got["ups_battery_celsius"] != 28 {
		t.Errorf("unexpected values %v", got)
	}
	fake.values[oidUpsOutputSource] = int64(upsOutputSourceBattery)
	if got := byKey(p.Poll(context.Background(), device, time.Now())); got["ups_on_battery"] != 1 {
		t.Errorf("not on battery: %v", got)
	}

	// an APC card answers its own MIB
	apc := &fakeSession{values: map[string]any{
		oidSysObjectID: "1.3.6.1.4.1.318.1.3.27", oidSysUpTime: uint64(100),
		oidApcOutputStatus: int64(2), oidApcBatteryCapacity: uint64(100), oidApcOutputLoad: uint64(12), oidApcRunTimeRemaining: uint64(300_000), oidApcInputLineVoltage: uint64(231), oidApcBatteryTemperature: uint64(29),
	}}
	p = newPoller(apc, nil)
	got = byKey(p.Poll(context.Background(), device, time.Now()))
	if got["ups_on_battery"] != 0 || got["ups_charge_pct"] != 100 || got["ups_load_pct"] != 12 || !near(got["ups_minutes_left"], 50) || got["ups_input_volts"] != 231 || got["ups_battery_celsius"] != 29 {
		t.Errorf("unexpected APC values %v", got)
	}
	apc.values[oidApcOutputStatus] = int64(apcOutputOnBattery)
	if got := byKey(p.Poll(context.Background(), device, time.Now())); got["ups_on_battery"] != 1 {
		t.Errorf("APC not on battery: %v", got)
	}
}

func TestCustomMetrics(t *testing.T) {
	fake := &fakeSession{values: map[string]any{
		oidSysUpTime:              uint64(100),
		"1.3.6.1.4.1.6574.1.2.0":  int64(41),
		"1.3.6.1.4.1.6574.1.3.0":  uint64(1000),
		"1.3.6.1.4.1.6574.1.4.0":  int64(235),
		"1.3.6.1.4.1.6574.1.99.0": "n/a",
	}}
	device := Device{Name: "nas", Address: "10.0.0.4", Community: "public", Kind: "nas", Metrics: []Metric{
		{Name: "temperature", OID: "1.3.6.1.4.1.6574.1.2.0", Unit: "°C"},
		{Name: "packets", OID: "1.3.6.1.4.1.6574.1.3.0", Unit: "/s", Counter: true},
		{Name: "voltage", OID: "1.3.6.1.4.1.6574.1.4.0", Unit: "V", Scale: 0.1},
		{Name: "text", OID: "1.3.6.1.4.1.6574.1.99.0"},
		{Name: "missing", OID: "1.3.6.1.4.1.6574.1.98.0"},
	}}
	p := newPoller(fake, nil)
	now := time.Unix(1700000000, 0)
	first := p.Poll(context.Background(), device, now)
	got := byKey(first)
	if first.Kind != "nas" || got["temperature"] != 41 || !near(got["voltage"], 23.5) || len(got) != 2 {
		t.Errorf("unexpected first values %v", got)
	}
	if first.Values[0].Unit != "°C" {
		t.Errorf("unit lost: %+v", first.Values[0])
	}
	fake.values["1.3.6.1.4.1.6574.1.3.0"] = uint64(1300)
	if got := byKey(p.Poll(context.Background(), device, now.Add(30*time.Second))); !near(got["packets"], 10) {
		t.Errorf("unexpected counter rate %v", got)
	}
}

func TestPollErrors(t *testing.T) {
	device := Device{Name: "sw9", Address: "10.0.0.9", Community: "public", Profiles: []string{ProfileInterfaces}}
	report := newPoller(nil, errors.New("dial udp: no route to host")).Poll(context.Background(), device, time.Now())
	if !strings.Contains(report.Error, "cannot reach 10.0.0.9") || report.Kind != KindSwitch || report.Device != "sw9" || len(report.Values) != 0 {
		t.Errorf("unexpected report %+v", report)
	}

	// a timeout half way through leaves nothing behind
	fake := &fakeSession{values: switchValues()}
	p := newPoller(fake, nil)
	p.Poll(context.Background(), device, time.Now())
	fake.err = errors.New("request timeout (after 1 retries)")
	report = p.Poll(context.Background(), device, time.Now())
	if !strings.Contains(report.Error, "cannot read 10.0.0.9: request timeout") || len(report.Values) != 0 || len(report.Interfaces) != 0 || report.System.Name != "" {
		t.Errorf("unexpected report %+v", report)
	}
	if strings.Contains(report.Error, "public") {
		t.Error("the community leaked into the error")
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snmp.json")
	content := `[{"Name": " sw1 ", "Address": "10.0.0.2", "Community": "public", "Profiles": ["interfaces"],
		"Metrics": [{"Name": "temp", "OID": ".1.3.6.1.4.1.9.9.13.1.3.1.3.1", "Unit": "°C"}]}]`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	devices, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].Name != "sw1" || devices[0].Metrics[0].OID != "1.3.6.1.4.1.9.9.13.1.3.1.3.1" {
		t.Errorf("unexpected devices %+v", devices)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Errorf("a readable file was accepted: %v", err)
	}

	if devices, err := Load(filepath.Join(t.TempDir(), "none.json")); err != nil || devices != nil {
		t.Errorf("a missing file is not no devices: %v %v", devices, err)
	}
}

func TestParseRefusesMistakes(t *testing.T) {
	cases := map[string]string{
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c", "Profiles": ["ups"], "Comunity": "x"}]`:                                                             "Comunity",
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c", "Profiles": ["ups"]}, {"Name": "a", "Address": "10.0.0.2", "Community": "c", "Profiles": ["ups"]}]`: "two devices named",
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c", "Metrics": [{"Name": "t", "OID": "temperature"}]}]`:                                                 "not an OID",
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c"}]`:                                                                                                   "at least one profile",
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c", "Profiles": ["printer"]}]`:                                                                          "unknown profile",
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c", "Metrics": [{"Name": "cpu_pct", "OID": "1.3.6.1.2.1.1.3.0"}]}]`:                                     "taken by a profile",
		`[{"Name": "a", "Address": "10.0.0.1", "Community": "c", "Profiles": ["ups"], "IntervalSeconds": 5}]`:                                                        "IntervalSeconds",
		`[{"Name": "a", "Address": "10.0.0.1:99999", "Community": "c", "Profiles": ["ups"]}]`:                                                                        "not a port",
		`[{"Name": "a/b", "Address": "10.0.0.1", "Community": "c", "Profiles": ["ups"]}]`:                                                                            "cannot contain /",
		`[{"Name": "a", "Address": "10.0.0.1", "Profiles": ["ups"]}]`:                                                                                                "Community",
		`[{"Name": "a", "Community": "c", "Profiles": ["ups"]}]`:                                                                                                     "Address",
	}
	for content, want := range cases {
		_, err := Parse([]byte(content))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want an error about %q", content, err, want)
		}
	}
}

func TestSampleFile(t *testing.T) {
	data, err := os.ReadFile("../../agent/snmp.json")
	if err != nil {
		t.Fatal(err)
	}
	devices, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 3 || kindOf(devices[0]) != KindSwitch || kindOf(devices[1]) != KindUPS || kindOf(devices[2]) != "nas" {
		t.Errorf("unexpected sample %+v", devices)
	}
}

func TestHostPort(t *testing.T) {
	cases := []struct {
		address string
		host    string
		port    uint16
	}{
		{"10.0.0.1", "10.0.0.1", 161},
		{"switch.lan:1161", "switch.lan", 1161},
		{"[fe80::1]:162", "fe80::1", 162},
		{"fe80::1", "fe80::1", 161},
	}
	for _, c := range cases {
		host, port, err := hostPort(c.address)
		if err != nil || host != c.host || port != c.port {
			t.Errorf("%s: got %s %d %v", c.address, host, port, err)
		}
	}
	if _, _, err := hostPort(":161"); err == nil {
		t.Error("an address without a host was accepted")
	}
}
