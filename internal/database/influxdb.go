package database

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/monitor"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

type InfluxDB struct {
	Connected bool
	URL       string
	Error     error
	Bucket    string
	Org       string
	Token     string
	Client    influxdb2.Client
}

func (influxdb *InfluxDB) Init(config *config.Collector) {
	influxdb.URL = config.InfluxDBURL
	influxdb.Bucket = config.InfluxDBBucket
	influxdb.Org = config.InfluxDBOrg
	influxdb.Token = config.InfluxDBToken
}

func (influxdb *InfluxDB) Connect() {
	influxdb.Client = influxdb2.NewClient(influxdb.URL, influxdb.Token)
	influxdb.Connected = true
}

func (influxdb *InfluxDB) Close() {
	influxdb.Client.Close()
	influxdb.Connected = false
}

func (influxdb *InfluxDB) WriteMetrics(tags map[string]string, fields map[string]interface{}, collected_time time.Time) error {
	writeAPI := influxdb.Client.WriteAPIBlocking(influxdb.Org, influxdb.Bucket)

	point := write.NewPoint("metrics", tags, fields, time.Now())

	if err := writeAPI.WritePoint(context.Background(), point); err != nil {
		return err
	}

	return nil
}

func (influxdb *InfluxDB) Save(monitorData *monitor.MonitorData) error {
	if !influxdb.Connected {
		influxdb.Connect()
	}
	unixTime, err := strconv.ParseInt(monitorData.UnixTime, 10, 64)
	if err != nil {
		return err
	}
	t := time.Unix(unixTime, 0)
	if err := influxdb.writeSystemMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeMemoryMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeSwapMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeDiskMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeProcUsageMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeNetworkMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeProcessMetrics(monitorData, t); err != nil {
		return err
	}
	if err := influxdb.writeServiceMetrics(monitorData, t); err != nil {
		return err
	}
	return nil
}

func (influxdb *InfluxDB) GetLastPing(serverName string) (bool, error) {
	if !influxdb.Connected {
		influxdb.Connect()
	}
	query := `from(bucket: "<bucket>")
	|> range(start: -61s)
	|> filter(fn: (r) => r["_measurement"] == "metrics")
	|> filter(fn: (r) => r["server_name"] == "<server_name>")
	|> filter(fn: (r) => r["metric_name"] == "ping")
	|> filter(fn: (r) => r["_field"] == "up")
	|> last()`
	query = strings.ReplaceAll(query, "<bucket>", influxdb.Bucket)
	query = strings.ReplaceAll(query, "<server_name>", serverName)
	results, err := influxdb.Client.QueryAPI(influxdb.Org).Query(context.Background(), query)
	if err != nil {
		return false, err
	}
	return results.Next(), results.Err()
}

func (influxdb *InfluxDB) Ping(serverName string, t time.Time) error {
	if !influxdb.Connected {
		influxdb.Connect()
	}
	tags := map[string]string{
		"server_name": serverName,
		"metric_name": "ping",
	}
	fields := map[string]interface{}{
		"up": 1,
	}
	return influxdb.WriteMetrics(tags, fields, t)
}

func (influxdb *InfluxDB) writeSystemMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	tags := map[string]string{
		"server_name": monitorData.ServerId,
		"metric_name": "system",
	}
	fields := map[string]interface{}{
		"hostname":        monitorData.System.HostName,
		"os":              monitorData.System.OS,
		"kernel":          monitorData.System.Kernel,
		"uptime":          monitorData.System.UpTime,
		"last_boot_date":  monitorData.System.LastBootDate,
		"logged_in_users": monitorData.System.LoggedInUsers,
		"timezone":        monitorData.System.TimeZone,
	}
	return influxdb.WriteMetrics(tags, fields, t)
}

func (influxdb *InfluxDB) writeMemoryMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	tags := map[string]string{
		"server_name": monitorData.ServerId,
		"metric_name": "memory",
	}
	fields := map[string]interface{}{
		"used_percentage": monitorData.Memory.PercentageUsed,
		"available":       monitorData.Memory.Available,
		"free":            monitorData.Memory.Free,
		"used":            monitorData.Memory.Used,
		"total":           monitorData.Memory.Total,
		"unit":            monitorData.Memory.Unit,
	}
	return influxdb.WriteMetrics(tags, fields, t)
}

func (influxdb *InfluxDB) writeSwapMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	tags := map[string]string{
		"server_name": monitorData.ServerId,
		"metric_name": "swap",
	}
	fields := map[string]interface{}{
		"used_percentage": monitorData.Swap.PercentageUsed,
		"free":            monitorData.Swap.Free,
		"used":            monitorData.Swap.Used,
		"total":           monitorData.Swap.Total,
		"unit":            monitorData.Swap.Unit,
	}
	return influxdb.WriteMetrics(tags, fields, t)
}

func (influxdb *InfluxDB) writeDiskMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	for _, disk := range monitorData.Disk {
		tags := map[string]string{
			"server_name": monitorData.ServerId,
			"metric_name": "disk",
			"disk_name":   disk.FileSystem,
		}
		fields := map[string]interface{}{
			"type":        disk.Type,
			"mounted_on":  disk.MountedOn,
			"usage":       disk.Usage.Usage,
			"available":   disk.Usage.Available,
			"used":        disk.Usage.Used,
			"size":        disk.Usage.Size,
			"unit":        disk.Usage.Unit,
			"i_usage":     disk.Inodes.Usage,
			"i_available": disk.Inodes.Available,
			"i_used":      disk.Inodes.Used,
			"inodes":      disk.Inodes.Inodes,
		}
		err := influxdb.WriteMetrics(tags, fields, t)
		if err != nil {
			return err
		}
	}
	return nil
}

func (influxdb *InfluxDB) writeProcUsageMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	tags := map[string]string{
		"server_name": monitorData.ServerId,
		"metric_name": "cpu",
	}
	fields := map[string]interface{}{
		"load_avg":    monitorData.ProcUsage.LoadAvg,
		"core_avg":    monitorData.ProcUsage.CoreAvg,
		"model":       monitorData.ProcUsage.Model,
		"no_of_cores": monitorData.ProcUsage.NoOfCores,
		"freq":        monitorData.ProcUsage.Freq,
		"cache":       monitorData.ProcUsage.Cache,
	}
	return influxdb.WriteMetrics(tags, fields, t)
}

func (influxdb *InfluxDB) writeNetworkMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	for _, network := range monitorData.Networks {
		tags := map[string]string{
			"server_name": monitorData.ServerId,
			"metric_name": "network",
			"interface":   network.Interface,
		}
		fields := map[string]interface{}{
			"rx_bytes":   network.Usage.RxBytes,
			"rx_packets": network.Usage.RxPackets,
			"tx_bytes":   network.Usage.TxBytes,
			"tx_packets": network.Usage.TxPackets,
		}
		err := influxdb.WriteMetrics(tags, fields, t)
		if err != nil {
			return err
		}
	}
	return nil
}

func (influxdb *InfluxDB) writeProcessMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	for _, process := range monitorData.Processes.CPU {
		tags := map[string]string{
			"server_name":  monitorData.ServerId,
			"metric_name":  "process",
			"process_type": "cpu",
		}
		fields := map[string]interface{}{
			"pid":       process.Pid,
			"cpu":       process.CPUUsage,
			"mem":       process.MemUsage,
			"exec_path": process.ExecPath,
			"user":      process.User,
		}
		err := influxdb.WriteMetrics(tags, fields, t)
		if err != nil {
			return err
		}
	}

	for _, process := range monitorData.Processes.Memory {
		tags := map[string]string{
			"server_name":  monitorData.ServerId,
			"metric_name":  "process",
			"process_type": "memory",
		}
		fields := map[string]interface{}{
			"pid":       process.Pid,
			"cpu":       process.CPUUsage,
			"mem":       process.MemUsage,
			"exec_path": process.ExecPath,
			"user":      process.User,
		}
		err := influxdb.WriteMetrics(tags, fields, t)
		if err != nil {
			return err
		}
	}
	return nil
}

func (influxdb *InfluxDB) writeServiceMetrics(monitorData *monitor.MonitorData, t time.Time) error {
	for _, service := range monitorData.Services {
		tags := map[string]string{
			"server_name":  monitorData.ServerId,
			"metric_name":  "service",
			"service_name": service.Name,
		}
		fields := map[string]interface{}{
			"is_running": service.Running,
		}
		err := influxdb.WriteMetrics(tags, fields, t)
		if err != nil {
			return err
		}
	}
	return nil
}
