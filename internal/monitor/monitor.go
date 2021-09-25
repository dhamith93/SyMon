package monitor

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
)

type MonitorData struct {
	UnixTime    string
	System      System
	Memory      []string
	Swap        []string
	Disks       []Disk
	Processor   Processor
	ProcUsage   []string
	Networks    []Network
	MemoryUsage []Process
	CpuUsage    []Process
	Services    []Service
	ServerId    string
}

func MonitorAsJSON(config config.Config) string {
	monitorData := Monitor(config)
	monitorData.ServerId = config.ServerId
	jsonData, err := json.Marshal(&monitorData)
	if err != nil {
		logger.Log("Error", err.Error())
		return ""
	}
	return string(jsonData)
}

func Monitor(config config.Config) MonitorData {
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	system := GetSystem()
	system.Time = unixTime
	memory := []string{unixTime}
	memory = append(memory, GetMemory()...)
	swap := []string{unixTime}
	swap = append(swap, GetSwap()...)
	disks := GetDisks(unixTime, config)
	proc := GetProcessor()
	procUsage := []string{unixTime, GetLoadAvg()}
	proc.Time = unixTime
	network := GetNetwork(unixTime)
	services := GetServices(unixTime, config)
	memUsage := GetProcessesSortedByMem(unixTime)
	cpuUsage := GetProcessesSortedByCPU(unixTime)

	return MonitorData{
		UnixTime:    unixTime,
		System:      system,
		Memory:      memory,
		Swap:        swap,
		Disks:       disks,
		Processor:   proc,
		ProcUsage:   procUsage,
		Networks:    network,
		Services:    services,
		MemoryUsage: memUsage,
		CpuUsage:    cpuUsage,
	}
}
