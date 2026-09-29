package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
)

// getMetrics serves every host's latest values in the Prometheus text
// format, so Prometheus can scrape the dashboard
func (s *server) getMetrics(w http.ResponseWriter, r *http.Request) {
	response, err := s.collector.Snapshots(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "snapshots", err)
		return
	}

	metrics := newMetricsWriter()
	for _, host := range response.Hosts {
		metrics.gauge("symon_up", "1 when the host reported within the last minute.", boolValue(host.Up), "host", host.Host)
		if host.LastSeen > 0 {
			metrics.gauge("symon_last_seen_timestamp_seconds", "When the host was last heard from.", float64(host.LastSeen), "host", host.Host)
		}
		// a host that stopped reporting only has old values, which would
		// look current to Prometheus
		if !host.Up || host.SnapshotJson == "" {
			continue
		}
		var data monitor.MonitorData
		if err := json.Unmarshal([]byte(host.SnapshotJson), &data); err != nil {
			logger.Log("error", "cannot read the snapshot of "+host.Host+": "+err.Error())
			continue
		}
		addHostMetrics(metrics, host.Host, &data)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if err := metrics.writeTo(w); err != nil {
		logger.Log("error", "cannot write metrics: "+err.Error())
	}
}

const mib = 1024 * 1024

func addHostMetrics(m *metricsWriter, host string, data *monitor.MonitorData) {
	m.gauge("symon_uptime_seconds", "How long the host has been running.", data.System.UpTimeSeconds, "host", host)
	// LoadAvg keeps its old name, it is the CPU usage
	m.gauge("symon_cpu_usage_percent", "CPU usage.", float64(data.ProcUsage.LoadAvg), "host", host)
	m.gauge("symon_load1", "Load average over 1 minute.", data.ProcUsage.Load1, "host", host)
	m.gauge("symon_load5", "Load average over 5 minutes.", data.ProcUsage.Load5, "host", host)
	m.gauge("symon_load15", "Load average over 15 minutes.", data.ProcUsage.Load15, "host", host)

	// the agent sends memory and swap in MiB
	m.gauge("symon_memory_used_percent", "Memory in use.", data.Memory.PercentageUsed, "host", host)
	m.gauge("symon_memory_total_bytes", "Total memory.", float64(data.Memory.Total)*mib, "host", host)
	m.gauge("symon_memory_available_bytes", "Memory available to new programs.", float64(data.Memory.Available)*mib, "host", host)
	m.gauge("symon_swap_used_percent", "Swap in use.", data.Swap.PercentageUsed, "host", host)
	m.gauge("symon_swap_total_bytes", "Total swap.", float64(data.Swap.Total)*mib, "host", host)

	for _, disk := range data.Disk {
		labels := []string{"host", host, "device", disk.FileSystem, "mount", disk.MountedOn}
		if pct, ok := parsePercent(disk.Usage.Usage); ok {
			m.gauge("symon_disk_used_percent", "Disk space in use, as df shows it.", pct, labels...)
		}
		m.gauge("symon_disk_size_bytes", "Disk size.", float64(disk.Usage.Size), labels...)
		m.gauge("symon_disk_used_bytes", "Disk space in use.", float64(disk.Usage.Used), labels...)
		if pct, ok := parsePercent(disk.Inodes.Usage); ok {
			m.gauge("symon_disk_inodes_used_percent", "Inodes in use.", pct, labels...)
		}
	}
	for _, diskIO := range data.DiskIO {
		// named like the disks above
		labels := []string{"host", host, "device", "/dev/" + diskIO.Device}
		m.gauge("symon_disk_read_bytes_per_second", "Disk reads.", diskIO.ReadBytesPerSec, labels...)
		m.gauge("symon_disk_write_bytes_per_second", "Disk writes.", diskIO.WriteBytesPerSec, labels...)
		m.gauge("symon_disk_util_percent", "Time the disk was busy.", diskIO.UtilPercent, labels...)
	}
	for _, network := range data.Networks {
		labels := []string{"host", host, "iface", network.Interface}
		m.counter("symon_network_receive_bytes_total", "Bytes received since boot.", float64(network.Usage.RxBytes), labels...)
		m.counter("symon_network_transmit_bytes_total", "Bytes sent since boot.", float64(network.Usage.TxBytes), labels...)
	}

	if tcp := data.TCPStates; tcp != nil {
		states := []struct {
			name  string
			count int
		}{
			{"established", tcp.Established}, {"syn_sent", tcp.SynSent}, {"syn_recv", tcp.SynRecv},
			{"fin_wait1", tcp.FinWait1}, {"fin_wait2", tcp.FinWait2}, {"time_wait", tcp.TimeWait},
			{"close", tcp.Close}, {"close_wait", tcp.CloseWait}, {"last_ack", tcp.LastAck},
			{"listen", tcp.Listen}, {"closing", tcp.Closing},
		}
		for _, state := range states {
			m.gauge("symon_tcp_connections", "TCP connections by state.", float64(state.count), "host", host, "state", state.name)
		}
	}

	if p := data.Pressure; p != nil {
		resources := []struct {
			name     string
			pressure monitor.ResourcePressure
		}{{"cpu", p.CPU}, {"memory", p.Memory}, {"io", p.IO}}
		for _, r := range resources {
			m.gauge("symon_pressure_some_percent", "Share of the last 10 seconds some tasks waited on the resource.", r.pressure.Some.Avg10, "host", host, "resource", r.name)
			if r.pressure.FullAvailable {
				m.gauge("symon_pressure_full_percent", "Share of the last 10 seconds all tasks waited on the resource.", r.pressure.Full.Avg10, "host", host, "resource", r.name)
			}
		}
	}

	for _, temp := range data.Temperatures {
		sensor := temp.Name
		if temp.Label != "" {
			sensor += "/" + temp.Label
		}
		m.gauge("symon_temperature_celsius", "Sensor temperature.", temp.Celsius, "host", host, "sensor", sensor)
	}
	for _, service := range data.Services {
		m.gauge("symon_service_up", "1 when the service is running.", boolValue(service.Running), "host", host, "service", service.Name)
	}

	for _, container := range data.Containers {
		name := container.Name
		if name == "" {
			name = container.ShortID
		}
		labels := []string{"host", host, "container", name, "project", container.ComposeProject}
		m.gauge("symon_container_cpu_percent", "Container CPU usage, as a share of the host.", container.CPU.PercentOfHost, labels...)
		m.gauge("symon_container_memory_bytes", "Container memory in use.", container.Memory.Used, labels...)
		if rates := container.Rates; rates != nil {
			// no traffic rates for containers on the host network
			if rates.RxBytesPerSec != nil {
				m.gauge("symon_container_receive_bytes_per_second", "Container network traffic received.", *rates.RxBytesPerSec, labels...)
			}
			if rates.TxBytesPerSec != nil {
				m.gauge("symon_container_transmit_bytes_per_second", "Container network traffic sent.", *rates.TxBytesPerSec, labels...)
			}
			m.gauge("symon_container_read_bytes_per_second", "Container disk reads.", rates.ReadBytesPerSec, labels...)
			m.gauge("symon_container_write_bytes_per_second", "Container disk writes.", rates.WriteBytesPerSec, labels...)
		}
	}
}

// parsePercent turns "40%" into 40
func parsePercent(value string) (float64, bool) {
	pct, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	return pct, err == nil
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// metricsWriter builds the Prometheus text format. Each metric's HELP,
// TYPE and samples have to be written together, so samples are grouped by
// metric and written at the end.
type metricsWriter struct {
	families []*metricFamily
	byName   map[string]*metricFamily
}

type metricFamily struct {
	name    string
	kind    string
	help    string
	samples []string
	// labels already written, a repeated set would make the output invalid
	seen map[string]bool
}

func newMetricsWriter() *metricsWriter {
	return &metricsWriter{byName: map[string]*metricFamily{}}
}

func (w *metricsWriter) gauge(name string, help string, value float64, labels ...string) {
	w.add(name, "gauge", help, value, labels)
}

func (w *metricsWriter) counter(name string, help string, value float64, labels ...string) {
	w.add(name, "counter", help, value, labels)
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

// add records one sample. labels are name and value pairs.
func (w *metricsWriter) add(name string, kind string, help string, value float64, labels []string) {
	family, ok := w.byName[name]
	if !ok {
		family = &metricFamily{name: name, kind: kind, help: help, seen: map[string]bool{}}
		w.byName[name] = family
		w.families = append(w.families, family)
	}

	var set strings.Builder
	for i := 0; i+1 < len(labels); i += 2 {
		if i > 0 {
			set.WriteByte(',')
		}
		fmt.Fprintf(&set, `%s="%s"`, labels[i], labelEscaper.Replace(labels[i+1]))
	}
	if family.seen[set.String()] {
		return
	}
	family.seen[set.String()] = true

	sample := name
	if set.Len() > 0 {
		sample += "{" + set.String() + "}"
	}
	family.samples = append(family.samples, sample+" "+strconv.FormatFloat(value, 'g', -1, 64)+"\n")
}

func (w *metricsWriter) writeTo(out io.Writer) error {
	var b strings.Builder
	for _, family := range w.families {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", family.name, family.help, family.name, family.kind)
		for _, sample := range family.samples {
			b.WriteString(sample)
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}
