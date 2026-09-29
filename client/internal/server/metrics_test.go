package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/monitor"
)

// Snapshots has web1 reporting, db1 gone quiet and new1 not sent anything yet
func (f *fakeCollector) Snapshots(ctx context.Context, in *api.Void) (*api.SnapshotList, error) {
	snapshot, err := json.Marshal(monitor.MonitorData{
		System:    monitor.System{UpTimeSeconds: 3600},
		ProcUsage: monitor.CPU{LoadAvg: 37, Load1: 0.5},
		Memory:    monitor.Memory{PercentageUsed: 42.5, Total: 16000, Available: 9000},
		Disk: []monitor.Disk{
			{FileSystem: "/dev/sda1", MountedOn: "/", Usage: monitor.DiskUsage{Size: 1000, Used: 400, Usage: "40%"}, Inodes: monitor.InodeUsage{Usage: "10%"}},
		},
		Networks: []monitor.Network{{Interface: "eth0", Usage: monitor.NetworkUsage{RxBytes: 1000, TxBytes: 2000}}},
		Services: []monitor.Service{{Name: "nginx", Running: true}},
		Containers: []monitor.Container{
			{ShortID: "aaa111", Name: "web", ComposeProject: "shop", CPU: monitor.ContainerCPU{PercentOfHost: 12},
				Rates: &monitor.ContainerRates{RxBytesPerSec: floatPtr(2048), ReadBytesPerSec: 10}},
		},
	})
	if err != nil {
		return nil, err
	}
	return &api.SnapshotList{Hosts: []*api.HostSnapshot{
		{Host: "web1", Up: true, LastSeen: 1700000000, Time: 1700000000, SnapshotJson: string(snapshot)},
		{Host: "db1", LastSeen: 1690000000, Time: 1690000000, SnapshotJson: string(snapshot)},
		{Host: "new1", Up: true, LastSeen: 1700000000},
	}}, nil
}

func TestMetrics(t *testing.T) {
	s, _ := newTestServer(t, nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()

	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("unexpected response %d %q: %s", rec.Code, rec.Header().Get("Content-Type"), body)
	}
	for _, line := range []string{
		"# TYPE symon_up gauge\n" + `symon_up{host="web1"} 1` + "\n" + `symon_up{host="db1"} 0` + "\n" + `symon_up{host="new1"} 1`,
		`symon_last_seen_timestamp_seconds{host="db1"} 1.69e+09`,
		`symon_cpu_usage_percent{host="web1"} 37`,
		`symon_memory_total_bytes{host="web1"} 1.6777216e+10`,
		`symon_disk_used_percent{host="web1",device="/dev/sda1",mount="/"} 40`,
		"# TYPE symon_network_receive_bytes_total counter\n" + `symon_network_receive_bytes_total{host="web1",iface="eth0"} 1000`,
		`symon_service_up{host="web1",service="nginx"} 1`,
		`symon_container_receive_bytes_per_second{host="web1",container="web",project="shop"} 2048`,
	} {
		if !strings.Contains(body, line+"\n") {
			t.Errorf("expected %q in:\n%s", line, body)
		}
	}
	// db1 stopped reporting, so its old values are left out
	if strings.Contains(body, `{host="db1",`) || strings.Contains(body, `symon_cpu_usage_percent{host="db1"}`) {
		t.Errorf("expected only up and last seen for db1:\n%s", body)
	}
	// web1 has no transmit rate, as if it were on the host network
	if strings.Contains(body, "symon_container_transmit_bytes_per_second") {
		t.Errorf("expected no transmit rate:\n%s", body)
	}
}

func TestMetricsWriter(t *testing.T) {
	w := newMetricsWriter()
	w.gauge("a", "First.", 1, "name", `quote " backslash \ newline`+"\n")
	w.counter("b_total", "Second.", 2)
	w.gauge("a", "First.", 3, "name", "other")
	// a repeated label set is dropped, Prometheus rejects the whole scrape otherwise
	w.gauge("a", "First.", 4, "name", "other")

	var out strings.Builder
	if err := w.writeTo(&out); err != nil {
		t.Fatal(err)
	}
	want := "# HELP a First.\n# TYPE a gauge\n" +
		`a{name="quote \" backslash \\ newline\n"} 1` + "\n" +
		`a{name="other"} 3` + "\n" +
		"# HELP b_total Second.\n# TYPE b_total counter\nb_total 2\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}
