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

const (
	SYSTEM     string = "system"
	MEMORY     string = "memory"
	SWAP       string = "swap"
	PROC_USAGE string = "procUsage"
	PROCESSES  string = "processes"
	DISKS      string = "disks"
	SERVICES   string = "services"
	NETWORKS   string = "networks"
	PING       string = "ping"
)

type Processes struct {
	CPU    []systats.Process
	Memory []systats.Process
}

type Service struct {
	Name    string
	Running bool
	Time    string
}

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

func MonitorAsJSON(config *config.Agent) string {
	monitorData := Monitor(config)
	monitorData.ServerId = config.ServerId
	jsonData, err := json.Marshal(&monitorData)
	if err != nil {
		logger.Log("Error", err.Error())
		return ""
	}
	return string(jsonData)
}

func Monitor(config *config.Agent) MonitorData {
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

func GetProcessor(syStats *systats.SyStats) systats.CPU {
	cpu, err := syStats.GetCPU()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return cpu
}

func GetSystem(syStats *systats.SyStats) systats.System {
	system, err := syStats.GetSystem()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return system
}

func GetMemory(syStats *systats.SyStats) systats.Memory {
	memory, err := syStats.GetMemory(systats.Megabyte)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return memory
}

func GetSwap(syStats *systats.SyStats) systats.Swap {
	swap, err := syStats.GetSwap(systats.Megabyte)
	if err != nil {
		logger.Log("error", err.Error())
	}
	return swap
}

func GetDisks(syStats *systats.SyStats, config *config.Agent) []systats.Disk {
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

func GetNetwork(syStats *systats.SyStats) []systats.Network {
	network, err := syStats.GetNetworks()
	if err != nil {
		logger.Log("error", err.Error())
	}
	return network
}

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

func GetServices(syStats *systats.SyStats, unixTime string, config *config.Agent) []Service {
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
