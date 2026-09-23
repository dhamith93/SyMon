package monitor

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/systats"
)

// a payload as sent by an agent built with systats v0.2.0
const oldPayload = `{
	"UnixTime": "1700000000",
	"System": {"HostName": "pi", "OS": "Debian", "Kernel": "6.1", "UpTime": "1h0m0s",
		"LastBootDate": "2023-11-14T21:13:20Z", "LoggedInUsers": [], "Time": 1700000000, "TimeZone": "UTC"},
	"Memory": {"PercentageUsed": 42.5, "Available": 9000, "Free": 3000, "Used": 7315, "Time": 1700000000, "Total": 16315, "Unit": "MB"},
	"Swap": {"PercentageUsed": 0, "Free": 2047, "Used": 0, "Time": 1700000000, "Total": 2047, "Unit": "MB"},
	"Disk": [{"FileSystem": "/dev/sda1", "Type": "ext4", "MountedOn": "/",
		"Usage": {"Size": 100000, "Used": 40000, "Available": 60000, "Usage": "40%", "Unit": "B"},
		"Inodes": {"Inodes": 100, "Available": 90, "Used": 10, "Usage": "10%"}, "Time": 1700000000}],
	"ProcUsage": {"LoadAvg": 12, "CoreAvg": [10, 14], "Model": "arm", "NoOfCores": 4, "Freq": "1.5", "Cache": "", "Time": 1700000000},
	"Networks": [{"Interface": "eth0", "Ip": "10.0.0.2",
		"Usage": {"RxBytes": 100, "TxBytes": 200, "RxPackets": 1, "TxPackets": 2}, "Time": 1700000000}],
	"Processes": {"CPU": [{"Pid": 1, "ExecPath": "/sbin/init", "User": "root", "CPUUsage": 0.5, "MemUsage": 0.1}], "Memory": []},
	"Services": null,
	"ServerId": "pi"
}`

func TestOldPayloadDecodes(t *testing.T) {
	var data MonitorData
	if err := json.Unmarshal([]byte(oldPayload), &data); err != nil {
		t.Fatalf("old payload did not decode: %v", err)
	}
	if data.Memory.Total != 16315 || data.ProcUsage.LoadAvg != 12 || data.Disk[0].Usage.Usage != "40%" {
		t.Errorf("old payload decoded wrong: %+v", data)
	}
	if data.DiskIO != nil || data.TCPStates != nil || data.Pressure != nil || data.Temperatures != nil {
		t.Error("expected optional sections to be empty for an old payload")
	}
}

// the collector and the current frontend read these keys, so their names
// and JSON types must not change
func TestWireFormatKeepsOldFields(t *testing.T) {
	var data MonitorData
	if err := json.Unmarshal([]byte(oldPayload), &data); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(&data)
	if err != nil {
		t.Fatal(err)
	}

	var want, got map[string]interface{}
	if err := json.Unmarshal([]byte(oldPayload), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	assertContains(t, "payload", want, got)

	for _, key := range []string{"DiskIO", "TCPStates", "Pressure", "Temperatures"} {
		if _, ok := got[key]; ok {
			t.Errorf("expected empty %s to be left out", key)
		}
	}
	if _, ok := got["Networks"].([]interface{})[0].(map[string]interface{})["Rates"]; ok {
		t.Error("expected nil network rates to be left out")
	}
}

// assertContains checks every key in want is in got with the same value,
// so integer fields must still encode as integers
func assertContains(t *testing.T, path string, want interface{}, got interface{}) {
	t.Helper()
	switch w := want.(type) {
	case map[string]interface{}:
		g, ok := got.(map[string]interface{})
		if !ok {
			t.Errorf("%s: want object, got %T", path, got)
			return
		}
		for key, value := range w {
			assertContains(t, path+"."+key, value, g[key])
		}
	case []interface{}:
		g, ok := got.([]interface{})
		if !ok || len(g) != len(w) {
			t.Errorf("%s: want %v, got %v", path, want, got)
			return
		}
		for i := range w {
			assertContains(t, path, w[i], g[i])
		}
	case nil:
		// null lists like Services may come back as null or empty
	default:
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("%s: want %s, got %s", path, wantJSON, gotJSON)
		}
	}
}

// an older collector decodes these fields into uint64, so a fraction
// from systats v0.4 would make it reject the whole payload
func TestMappedSizesEncodeAsIntegers(t *testing.T) {
	data := MonitorData{
		Memory: fromSystatsMemory(systats.Memory{Total: 15932.6, Used: 1000.4, Free: 3000.5, Available: 9000.2}),
		Swap:   fromSystatsSwap(systats.Swap{Total: 2047.7, Used: 0.3, Free: 2047.4}),
		Disk:   []Disk{fromSystatsDisk(systats.Disk{Usage: systats.DiskUsage{Size: 1e11 + 0.5, Used: 4e10 + 0.5, Available: 6e10 + 0.5}})},
	}
	encoded, err := json.Marshal(&data)
	if err != nil {
		t.Fatal(err)
	}

	var old struct {
		Memory struct{ Total, Used, Free, Available uint64 }
		Swap   struct{ Total, Used, Free uint64 }
		Disk   []struct{ Usage struct{ Size, Used, Available uint64 } }
	}
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("payload does not decode into the old integer fields: %v", err)
	}
	if old.Memory.Total != 15933 || old.Swap.Total != 2048 || old.Disk[0].Usage.Size != 100000000001 {
		t.Errorf("sizes were not rounded as expected: %+v", old)
	}
}

func TestFromSystatsMemoryRounds(t *testing.T) {
	mem := fromSystatsMemory(systats.Memory{Total: 15932.6, Used: 1000.4, Free: -1, Unit: systats.Megabyte})
	if mem.Total != 15933 || mem.Used != 1000 || mem.Free != 0 || mem.Unit != "MB" {
		t.Errorf("memory conversion was incorrect: %+v", mem)
	}
}

func TestNetworkRates(t *testing.T) {
	prev := NetworkUsage{RxBytes: 1000, TxBytes: 2000, RxPackets: 10, TxPackets: 20}
	cur := NetworkUsage{RxBytes: 3000, TxBytes: 2500, RxPackets: 30, TxPackets: 25}

	rates := networkRates(prev, cur, 10)
	if rates == nil {
		t.Fatal("expected rates")
	}
	if rates.RxBytesPerSec != 200 || rates.TxBytesPerSec != 50 || rates.RxPacketsPerSec != 2 || rates.TxPacketsPerSec != 0.5 {
		t.Errorf("rates were incorrect: %+v", rates)
	}

	reset := NetworkUsage{RxBytes: 10, TxBytes: 2500, RxPackets: 30, TxPackets: 25}
	if networkRates(prev, reset, 10) != nil {
		t.Error("expected nil rates after a counter reset")
	}
	if networkRates(prev, cur, 0) != nil {
		t.Error("expected nil rates for zero elapsed time")
	}
}

func testCollector(t *testing.T, cfg config.Agent) *Collector {
	t.Helper()
	c := NewCollector(&cfg)
	dir := t.TempDir()
	c.stats.DiskStatsPath = filepath.Join(dir, "diskstats")
	c.stats.PressurePath = filepath.Join(dir, "pressure")
	c.stats.NetTCPPath = filepath.Join(dir, "tcp")
	c.stats.NetTCP6Path = filepath.Join(dir, "tcp6")
	c.stats.HwmonPath = filepath.Join(dir, "hwmon")
	return c
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDiskIO(t *testing.T) {
	c := testCollector(t, config.Agent{DisksToIgnore: "/dev/sdb"})
	writeFile(t, c.stats.DiskStatsPath, `
   8       0 sda 1000 0 20000 500 2000 0 40000 800 0 1200 1300
   8      16 sdb 1000 0 20000 500 2000 0 40000 800 0 1200 1300
   7       0 loop0 10 0 20 1 0 0 0 0 0 1 1
`)
	if rates := c.diskIO(); rates != nil {
		t.Fatalf("expected no rates on the first sample, got %+v", rates)
	}

	// pretend the first sample was taken 10 seconds ago
	c.prevDiskIOTime = c.prevDiskIOTime.Add(-10 * time.Second)
	writeFile(t, c.stats.DiskStatsPath, `
   8       0 sda 1100 0 40000 600 2500 0 60000 900 0 6200 6300
   8      16 sdb 1100 0 40000 600 2500 0 60000 900 0 6200 6300
`)
	rates := c.diskIO()
	if len(rates) != 1 || rates[0].Device != "sda" {
		t.Fatalf("expected rates for sda only, got %+v", rates)
	}
	// 20000 sectors of 512 bytes over about 10 seconds
	if !near(rates[0].ReadBytesPerSec, 1024000) || !near(rates[0].ReadsPerSec, 10) || !near(rates[0].UtilPercent, 50) {
		t.Errorf("disk io rates were incorrect: %+v", rates[0])
	}
}

func near(got float64, want float64) bool {
	return math.Abs(got-want) <= want*0.01
}

func TestPressure(t *testing.T) {
	c := testCollector(t, config.Agent{})
	if p := c.pressure(); p != nil {
		t.Fatalf("expected nil pressure without PSI files, got %+v", p)
	}

	writeFile(t, filepath.Join(c.stats.PressurePath, "cpu"), "some avg10=1.50 avg60=0.80 avg300=0.20 total=12345\n")
	writeFile(t, filepath.Join(c.stats.PressurePath, "memory"), "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n")
	writeFile(t, filepath.Join(c.stats.PressurePath, "io"), "some avg10=4.00 avg60=2.00 avg300=1.00 total=999\nfull avg10=3.00 avg60=1.00 avg300=0.50 total=500\n")

	p := c.pressure()
	if p == nil {
		t.Fatal("expected pressure")
	}
	if p.CPU.Some.Avg10 != 1.5 || p.CPU.FullAvailable || p.IO.Full.Avg10 != 3 || !p.IO.FullAvailable {
		t.Errorf("pressure was incorrect: %+v", p)
	}
}

func TestTCPStates(t *testing.T) {
	c := testCollector(t, config.Agent{})
	header := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	writeFile(t, c.stats.NetTCPPath, header+
		"   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1 1 0 100 0 0 10 0\n"+
		"   1: 0100007F:1F90 0100007F:C350 01 00000000:00000000 00:00000000 00000000  1000        0 2 1 0 20 4 30 10 -1\n"+
		"   2: 0100007F:1F90 0100007F:C351 06 00000000:00000000 00:00000000 00000000     0        0 0 3 0\n")
	writeFile(t, c.stats.NetTCP6Path, header)

	states := c.tcpStates()
	if states == nil {
		t.Fatal("expected tcp states")
	}
	if states.Listen != 1 || states.Established != 1 || states.TimeWait != 1 || states.Total != 3 {
		t.Errorf("tcp states were incorrect: %+v", states)
	}
}

func TestTemperatures(t *testing.T) {
	c := testCollector(t, config.Agent{})
	if temps := c.temperatures(); temps != nil {
		t.Fatalf("expected no temperatures without hwmon, got %+v", temps)
	}

	chip := filepath.Join(c.stats.HwmonPath, "hwmon0")
	writeFile(t, filepath.Join(chip, "name"), "cpu_thermal\n")
	writeFile(t, filepath.Join(chip, "temp1_input"), "48312\n")
	writeFile(t, filepath.Join(chip, "temp1_crit"), "90000\n")

	temps := c.temperatures()
	if len(temps) != 1 {
		t.Fatalf("expected one temperature, got %+v", temps)
	}
	if temps[0].Name != "cpu_thermal" || !near(temps[0].Celsius, 48.312) || !temps[0].CriticalAvailable || temps[0].HighAvailable {
		t.Errorf("temperature was incorrect: %+v", temps[0])
	}
}

func TestDisabledCollectors(t *testing.T) {
	c := NewCollector(&config.Agent{DisabledCollectors: []string{"temps", "tcp", "bogus"}})
	if c.enabled[CollectorTemps] || c.enabled[CollectorTCP] {
		t.Error("expected temps and tcp to be disabled")
	}
	if !c.enabled[CollectorDiskIO] || !c.enabled[CollectorPressure] {
		t.Error("expected diskio and pressure to stay enabled")
	}
	if _, ok := c.enabled["bogus"]; ok {
		t.Error("unknown collector names should be ignored")
	}
}

func TestContainerAware(t *testing.T) {
	if NewCollector(&config.Agent{}).stats.ContainerAware {
		t.Error("expected container aware to be off by default")
	}
	if !NewCollector(&config.Agent{ContainerAware: true}).stats.ContainerAware {
		t.Error("expected container aware to follow the config")
	}
}
