package server

import (
	"database/sql"
	"encoding/json"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/monitor"
)

func HandleMonitorData(monitorData monitor.MonitorData) error {
	var db *sql.DB
	var err error

	serverId := monitorData.ServerId
	time := monitorData.UnixTime
	path := config.GetConfig("config.json").SQLiteDBPath + "/" + serverId + ".db"
	db, err = database.OpenDB(db, path)

	if err != nil {
		return err
	}

	defer db.Close()

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
		errDB := database.SaveLogToDB(db, time, string(res), key)
		if errDB != nil {
			return err
		}
	}
	return nil
}

func HandleCustomMetric(customMetric monitor.CustomMetric) error {
	var db *sql.DB
	var err error
	serverId := customMetric.ServerId
	time := customMetric.Time
	path := config.GetConfig("config.json").SQLiteDBPath + "/" + serverId + ".db"
	db, err = database.OpenDB(db, path)
	if err != nil {
		return err
	}
	defer db.Close()
	res, err := json.Marshal(&customMetric)
	if err != nil {
		return err
	}
	return database.SaveLogToDB(db, time, string(res), customMetric.Name)
}
