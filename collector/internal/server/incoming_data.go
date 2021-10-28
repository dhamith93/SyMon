package server

import (
	"encoding/json"

	"github.com/dhamith93/SyMon/collector/internal/config"
	"github.com/dhamith93/SyMon/internal/monitor"
)

func HandleMonitorData(monitorData monitor.MonitorData) error {
	serverName := monitorData.ServerId
	time := monitorData.UnixTime
	config := config.GetConfig("config.json")
	mysql := getMySQLConnection(&config)
	defer mysql.Close()

	data := make(map[string]interface{})
	data["system"] = &monitorData.System
	data["memory"] = &monitorData.Memory
	data["swap"] = &monitorData.Swap
	data["disks"] = &monitorData.Disk
	data["procUsage"] = &monitorData.ProcUsage
	data["networks"] = &monitorData.Networks
	data["services"] = &monitorData.Services
	data["processes"] = &monitorData.Processes

	for key, item := range data {
		res, err := json.Marshal(item)
		if err != nil {
			return err
		}

		err = mysql.SaveLogToDB(serverName, time, string(res), key)
		if err != nil {
			return err
		}
	}
	return nil
}

func HandleCustomMetric(customMetric monitor.CustomMetric) error {
	serverName := customMetric.ServerId
	time := customMetric.Time
	config := config.GetConfig("config.json")
	mysql := getMySQLConnection(&config)
	defer mysql.Close()

	res, err := json.Marshal(&customMetric)
	if err != nil {
		return err
	}
	return mysql.SaveLogToDB(serverName, time, string(res), customMetric.Name)
}
