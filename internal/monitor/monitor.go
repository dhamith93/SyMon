package monitor

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/systats"
)

// log types, used as keys when the collector stores a snapshot
const (
	SYSTEM       string = "system"
	MEMORY       string = "memory"
	SWAP         string = "swap"
	PROC_USAGE   string = "procUsage"
	PROCESSES    string = "processes"
	DISKS        string = "disks"
	SERVICES     string = "services"
	NETWORKS     string = "networks"
	PING         string = "ping"
	DISK_IO      string = "diskIO"
	TCP_STATES   string = "tcpStates"
	PRESSURE     string = "pressure"
	TEMPERATURES string = "temperatures"
)

// optional collectors, SYMON_DISABLED_COLLECTORS can switch these off
const (
	CollectorDiskIO   string = "diskio"
	CollectorTCP      string = "tcp"
	CollectorPressure string = "pressure"
	CollectorTemps    string = "temps"
)

type Processes struct {
	CPU    []Process
	Memory []Process
}

type Service struct {
	Name    string
	Running bool
	Time    string
}

type MonitorData struct {
	UnixTime  string
	System    System
	Memory    Memory
	Swap      Swap
	Disk      []Disk
	ProcUsage CPU
	Networks  []Network
	Processes Processes
	Services  []Service
	ServerId  string

	// optional sections, left out when disabled, unsupported on the host,
	// or (for DiskIO) on the first tick before there is a previous sample
	DiskIO       []DiskIO      `json:",omitempty"`
	TCPStates    *TCPStates    `json:",omitempty"`
	Pressure     *Pressure     `json:",omitempty"`
	Temperatures []Temperature `json:",omitempty"`
}

// Collector gathers a MonitorData snapshot on every tick. It keeps the
// previous disk and network counters so it can turn them into rates.
type Collector struct {
	config  *config.Agent
	stats   systats.SyStats
	enabled map[string]bool

	prevDiskIO     map[string]systats.DiskIO
	prevDiskIOTime time.Time
	prevNetworks   map[string]NetworkUsage
	prevNetTime    time.Time
}

func NewCollector(config *config.Agent) *Collector {
	stats := systats.New()
	stats.ContainerAware = config.ContainerAware

	c := &Collector{
		config: config,
		stats:  stats,
		enabled: map[string]bool{
			CollectorDiskIO:   true,
			CollectorTCP:      true,
			CollectorPressure: true,
			CollectorTemps:    true,
		},
	}
	for _, name := range config.DisabledCollectors {
		if _, ok := c.enabled[name]; !ok {
			logger.Log("error", "unknown collector in SYMON_DISABLED_COLLECTORS: "+name)
			continue
		}
		c.enabled[name] = false
	}
	return c
}

// CollectJSON returns a snapshot as the JSON string the collector expects
func (c *Collector) CollectJSON(ctx context.Context) string {
	data := c.Collect(ctx)
	jsonData, err := json.Marshal(&data)
	if err != nil {
		logger.Log("error", err.Error())
		return ""
	}
	return string(jsonData)
}

func (c *Collector) Collect(ctx context.Context) MonitorData {
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	data := MonitorData{
		UnixTime:  unixTime,
		ServerId:  c.config.ServerId,
		System:    c.system(ctx),
		Memory:    c.memory(),
		Swap:      c.swap(),
		Disk:      c.disks(),
		ProcUsage: c.cpu(ctx),
		Networks:  c.networks(),
		Services:  c.services(ctx, unixTime),
		Processes: c.processes(ctx),
	}

	if c.enabled[CollectorDiskIO] {
		data.DiskIO = c.diskIO()
	}
	if c.enabled[CollectorTCP] {
		data.TCPStates = c.tcpStates()
	}
	if c.enabled[CollectorPressure] {
		data.Pressure = c.pressure()
	}
	if c.enabled[CollectorTemps] {
		data.Temperatures = c.temperatures()
	}
	return data
}

// TimeZone returns the host's time zone, sent when the agent registers
func TimeZone() string {
	stats := systats.New()
	system, err := stats.GetSystem()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return system.TimeZone
}

func (c *Collector) cpu(ctx context.Context) CPU {
	cpu, err := c.stats.GetCPUWithContext(ctx)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return fromSystatsCPU(cpu)
}

func (c *Collector) system(ctx context.Context) System {
	system, err := c.stats.GetSystemWithContext(ctx)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return fromSystatsSystem(system)
}

func (c *Collector) memory() Memory {
	memory, err := c.stats.GetMemory(systats.Megabyte)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return fromSystatsMemory(memory)
}

func (c *Collector) swap() Swap {
	swap, err := c.stats.GetSwap(systats.Megabyte)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return fromSystatsSwap(swap)
}

func (c *Collector) disks() []Disk {
	disks, err := c.stats.GetDisks()
	if err != nil {
		logger.Log("error", err.Error())
	}
	output := []Disk{}
	for _, disk := range disks {
		if !c.ignoredDisk(disk.FileSystem) {
			output = append(output, fromSystatsDisk(disk))
		}
	}
	return output
}

// ignoredDisk checks a device path like /dev/loop0 against SYMON_DISKS_TO_IGNORE
func (c *Collector) ignoredDisk(fileSystem string) bool {
	for _, diskToIgnore := range strings.Split(c.config.DisksToIgnore, ",") {
		if fileSystem == strings.TrimSpace(diskToIgnore) {
			return true
		}
	}
	return false
}

func (c *Collector) networks() []Network {
	stats, err := c.stats.GetNetworks()
	if err != nil {
		logger.Log("error", err.Error())
	}
	sampled := time.Now()
	elapsed := sampled.Sub(c.prevNetTime).Seconds()

	networks := make([]Network, 0, len(stats))
	current := make(map[string]NetworkUsage, len(stats))
	for _, n := range stats {
		network := fromSystatsNetwork(n)
		if prev, ok := c.prevNetworks[network.Interface]; ok {
			network.Rates = networkRates(prev, network.Usage, elapsed)
		}
		current[network.Interface] = network.Usage
		networks = append(networks, network)
	}

	c.prevNetworks = current
	c.prevNetTime = sampled
	return networks
}

// networkRates returns nil when a counter went backwards, which happens
// when an interface is reset
func networkRates(prev NetworkUsage, cur NetworkUsage, elapsedSeconds float64) *NetworkRates {
	if elapsedSeconds <= 0 ||
		cur.RxBytes < prev.RxBytes || cur.TxBytes < prev.TxBytes ||
		cur.RxPackets < prev.RxPackets || cur.TxPackets < prev.TxPackets {
		return nil
	}
	return &NetworkRates{
		RxBytesPerSec:   float64(cur.RxBytes-prev.RxBytes) / elapsedSeconds,
		TxBytesPerSec:   float64(cur.TxBytes-prev.TxBytes) / elapsedSeconds,
		RxPacketsPerSec: float64(cur.RxPackets-prev.RxPackets) / elapsedSeconds,
		TxPacketsPerSec: float64(cur.TxPackets-prev.TxPackets) / elapsedSeconds,
	}
}

func (c *Collector) processes(ctx context.Context) Processes {
	cpu, err := c.stats.GetTopProcessesWithContext(ctx, 10, systats.SortByCPU)
	if err != nil {
		logger.Log("error", err.Error())
	}
	mem, err := c.stats.GetTopProcessesWithContext(ctx, 10, systats.SortByMemory)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return Processes{
		CPU:    fromSystatsProcesses(cpu),
		Memory: fromSystatsProcesses(mem),
	}
}

func fromSystatsProcesses(processes []systats.Process) []Process {
	output := make([]Process, 0, len(processes))
	for _, p := range processes {
		output = append(output, fromSystatsProcess(p))
	}
	return output
}

func (c *Collector) services(ctx context.Context, unixTime string) []Service {
	var services []Service
	for _, serviceToCheck := range c.config.Services {
		running, err := c.stats.IsServiceRunningWithContext(ctx, serviceToCheck.ServiceName)
		if err != nil {
			logger.Log("error", "cannot check service "+serviceToCheck.ServiceName+": "+err.Error())
		}
		services = append(services, Service{
			Name:    serviceToCheck.Name,
			Running: running,
			Time:    unixTime,
		})
	}
	return services
}

// diskIO returns nil on the first call, rates need two samples
func (c *Collector) diskIO() []DiskIO {
	samples, err := c.stats.GetDiskIO()
	if err != nil {
		logger.Log("error", err.Error())
		return nil
	}
	sampled := time.Now()
	elapsed := sampled.Sub(c.prevDiskIOTime).Seconds()

	var rates []DiskIO
	current := make(map[string]systats.DiskIO, len(samples))
	for _, sample := range samples {
		if c.ignoredDisk("/dev/" + sample.Device) {
			continue
		}
		if prev, ok := c.prevDiskIO[sample.Device]; ok {
			rates = append(rates, fromSystatsDiskIORates(sample.RatesSince(prev, elapsed)))
		}
		current[sample.Device] = sample
	}

	c.prevDiskIO = current
	c.prevDiskIOTime = sampled
	return rates
}

func (c *Collector) tcpStates() *TCPStates {
	states, err := c.stats.GetTCPConnectionStates()
	if err != nil {
		logger.Log("error", err.Error())
		return nil
	}
	tcpStates := fromSystatsTCPStates(states)
	return &tcpStates
}

// pressure returns nil when the kernel has no PSI support
func (c *Collector) pressure() *Pressure {
	p, err := c.stats.GetPressure()
	if err != nil {
		logger.Log("error", err.Error())
		return nil
	}
	if !p.Available {
		return nil
	}
	pressure := fromSystatsPressure(p)
	return &pressure
}

// temperatures returns nil on hosts without sensors, like most VMs
func (c *Collector) temperatures() []Temperature {
	temps, err := c.stats.GetTemperatures()
	if err != nil {
		logger.Log("error", err.Error())
		return nil
	}
	var output []Temperature
	for _, t := range temps {
		output = append(output, fromSystatsTemperature(t))
	}
	return output
}
