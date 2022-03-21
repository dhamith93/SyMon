package monitor

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/systats"
)

// Processes hold CPU and Memory usage data
type Processes struct {
	CPU    []systats.Process
	Memory []systats.Process
}

// Service holds service activity info
type Service struct {
	Name    string
	Running bool
	Time    string
}

// MonitorData holds individual system stats
type MonitorData struct {
	UnixTime  string
	System    systats.System
	Memory    systats.Memory
	Swap      systats.Swap
	Disk      []systats.Disk
	ProcUsage systats.CPU
	Networks  []systats.Network
	Processes Processes
	Services  []Service
	ServerId  string
}

// MonitorAsJSON returns MonitorData struct as an JSON object
func MonitorAsJSON(config *config.Config) string {
	monitorData := Monitor(config)
	monitorData.ServerId = config.ServerId
	jsonData, err := json.Marshal(&monitorData)
	if err != nil {
		logger.Log("Error", err.Error())
		return ""
	}
	return string(jsonData)
}

// Monitor returns MonitorData struct with system stats
func Monitor(config *config.Config) MonitorData {
	syStats := systats.New()
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	system := GetSystem(&syStats)
	memory := GetMemory(&syStats)
	swap := GetSwap(&syStats)
	disk := GetDisks(&syStats, config)
	procUsage := GetProcessor(&syStats)
	network := GetNetwork(&syStats)
	services := GetServices(&syStats, unixTime, config)
	processes := GetProcesses(&syStats)

	return MonitorData{
		UnixTime:  unixTime,
		System:    system,
		Memory:    memory,
		Swap:      swap,
		Disk:      disk,
		ProcUsage: procUsage,
		Networks:  network,
		Services:  services,
		Processes: processes,
	}
}

// GetProcessor returns a systats.CPU struct with CPU info and usage data
func GetProcessor(syStats *systats.SyStats) systats.CPU {
	cpu, err := syStats.GetCPU()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return cpu
}

// GetSystem returns a systats.System struct with system info
func GetSystem(syStats *systats.SyStats) systats.System {
	system, err := syStats.GetSystem()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return system
}

// GetMemory returns systats.Memory struct with memory usage
func GetMemory(syStats *systats.SyStats) systats.Memory {
	memory, err := syStats.GetMemory(systats.Megabyte)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return memory
}

// GetSwap returns systats.Swap struct with swap usage
func GetSwap(syStats *systats.SyStats) systats.Swap {
	swap, err := syStats.GetSwap(systats.Megabyte)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return swap
}

// GetDisks returns array of systats.Disk structs with disk info and usage data
func GetDisks(syStats *systats.SyStats, config *config.Config) []systats.Disk {
	disks, err := syStats.GetDisks()
	output := []systats.Disk{}
	disksTOIgnore := strings.Split(config.DisksToIgnore, ",")
	if err != nil {
		logger.Log("error", err.Error())
	}
	for _, disk := range disks {
		ignore := false
		for _, diskToIgnore := range disksTOIgnore {
			if disk.FileSystem == strings.TrimSpace(diskToIgnore) {
				ignore = true
			}
		}
		if !ignore {
			output = append(output, disk)
		}
	}
	return output
}

// GetNetwork returns an array of systats.Network struct with network usage
func GetNetwork(syStats *systats.SyStats) []systats.Network {
	network, err := syStats.GetNetworks()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return network
}

// GetProcesses returns struct with process info
func GetProcesses(syStats *systats.SyStats) Processes {
	cpu, err := syStats.GetTopProcesses(10, "cpu")
	if err != nil {
		logger.Log("error", err.Error())
	}
	mem, err := syStats.GetTopProcesses(10, "memory")
	if err != nil {
		logger.Log("error", err.Error())
	}
	return Processes{
		CPU:    cpu,
		Memory: mem,
	}
}

// GetServices returns array of Service structs with service status
func GetServices(syStats *systats.SyStats, unixTime string, config *config.Config) []Service {
	servicesToCheck := config.Services
	var services []Service
	for _, serviceToCheck := range servicesToCheck {
		services = append(services, Service{
			Name:    serviceToCheck.Name,
			Running: syStats.IsServiceRunning(serviceToCheck.ServiceName),
			Time:    unixTime,
		})
	}
	return services
}
