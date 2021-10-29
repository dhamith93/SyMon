package api

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"golang.org/x/net/context"
)

type Server struct {
}

func (s *Server) InitAgent(ctx context.Context, in *ServerInfo) (*Message, error) {
	config := config.GetConfig("config.json")
	err := initAgent(in.ServerId, in.Timezone, &config)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	return &Message{Body: "agent added"}, nil
}

func (s *Server) HandleMonitorData(ctx context.Context, in *MonitorData) (*Message, error) {
	var monitorData = monitor.MonitorData{}
	err := json.Unmarshal([]byte(in.MonitorData), &monitorData)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	err = handleMonitorData(&monitorData)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	return &Message{Body: "ok"}, nil
}

func (s *Server) HandleCustomMonitorData(ctx context.Context, in *MonitorData) (*Message, error) {
	var customMetric = monitor.CustomMetric{}
	err := json.Unmarshal([]byte(in.MonitorData), &customMetric)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	err = handleCustomMetric(&customMetric)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	return &Message{Body: "ok"}, nil
}

func initAgent(agentId string, timezone string, config *config.Config) error {
	logger.Log("info", "Initializing agent for "+agentId)

	mysql := getMySQLConnection(config)
	defer mysql.Close()

	if mysql.AgentIDExists(agentId) {
		logger.Log("error", "agent id "+agentId+" exists")
		return fmt.Errorf("agent id " + agentId + " exists")
	}

	err := mysql.AddAgent(agentId, timezone)
	if err != nil {
		logger.Log("error", err.Error())
		return fmt.Errorf("error adding agent")
	}

	return nil
}

func handleMonitorData(monitorData *monitor.MonitorData) error {
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

func handleCustomMetric(customMetric *monitor.CustomMetric) error {
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

func getMySQLConnection(c *config.Config) database.MySql {
	mysql := database.MySql{}
	password := os.Getenv("SYMON_MYSQL_PSWD")
	mysql.Connect(c.MySQLUserName, password, c.MySQLHost, c.MySQLDatabaseName, false)
	return mysql
}
