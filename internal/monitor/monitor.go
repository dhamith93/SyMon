package monitor

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
)

type MonitorData struct {
	UnixTime  string
	System    System
	Memory    []string
	Swap      []string
	Disk      Disk
	ProcUsage []string
	Networks  []Network
	Processes Process
	Services  []Service
	ServerId  string
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
	disk := GetDisks(unixTime, config)
	procUsage := []string{unixTime, GetLoadAvg()}
	network := GetNetwork(unixTime)
	services := GetServices(unixTime, config)
	processes := GetProcesses()

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
