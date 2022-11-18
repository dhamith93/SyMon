package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/stringops"
	"golang.org/x/net/context"
)

type Server struct {
}

type Agents struct {
	AgentIDs []string
}

type CustomMetrics struct {
	CustomMetrics []string
}

func (s *Server) InitAgent(ctx context.Context, in *ServerInfo) (*Message, error) {
	config := config.GetCollector()
	err := initAgent(in.ServerName, in.Timezone, &config)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	return &Message{Body: "agent added"}, nil
}

func (s *Server) HandlePing(ctx context.Context, in *ServerInfo) (*Message, error) {
	config := config.GetCollector()
	err := handlePing(in.ServerName, &config)
	if err != nil {
		return &Message{Body: err.Error()}, err
	}
	return &Message{Body: "pong"}, nil
}

func (s *Server) IsUp(ctx context.Context, in *ServerInfo) (*IsActive, error) {
	config := config.GetCollector()
	upAndRunning, err := isUp(in.ServerName, &config)
	if err != nil {
		return &IsActive{IsUp: false}, err
	}
	return &IsActive{IsUp: upAndRunning}, nil
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

func (s *Server) HandleMonitorDataRequest(ctx context.Context, in *MonitorDataRequest) (*MonitorData, error) {
	config := config.GetCollector()
	convertToJsonArr := false
	switch in.LogType {
	case "networks", "procUsage":
		convertToJsonArr = true
	case "memory-historical":
		convertToJsonArr = true
		in.LogType = "memory"
	}
	monitorData := getMonitorLogs(in.ServerName, in.LogType, in.From, in.To, in.Time, &config, convertToJsonArr, in.IsCustomMetric)
	if len(monitorData) == 0 {
		return &MonitorData{MonitorData: "no data"}, fmt.Errorf("no data found")
	}
	return &MonitorData{MonitorData: monitorData}, nil
}

func (s *Server) HandleAgentIdsRequest(context.Context, *Void) (*Message, error) {
	config := config.GetCollector()
	mysql := getMySQLConnection(&config)
	defer mysql.Close()
	agents := Agents{}
	agents.AgentIDs = mysql.GetAgents()
	if len(agents.AgentIDs) == 0 {
		return &Message{Body: "no data"}, fmt.Errorf("no data found")
	}
	out, err := json.Marshal(agents)
	if err != nil {
		return &Message{Body: "cannot parse data"}, fmt.Errorf("cannot parse data")
	}

	return &Message{Body: string(out)}, nil
}

func (s *Server) HandleCustomMetricNameRequest(ctx context.Context, in *ServerInfo) (*Message, error) {
	config := config.GetCollector()
	mysql := getMySQLConnection(&config)
	defer mysql.Close()
	customMetrics := CustomMetrics{}
	customMetrics.CustomMetrics = mysql.GetCustomMetricNames(in.ServerName)
	if len(customMetrics.CustomMetrics) == 0 {
		return &Message{Body: "no data"}, fmt.Errorf("no data found")
	}
	out, err := json.Marshal(customMetrics)
	if err != nil {
		return &Message{Body: "cannot parse data"}, fmt.Errorf("cannot parse data")
	}

	return &Message{Body: string(out)}, nil
}

func initAgent(agentId string, timezone string, config *config.Collector) error {
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

func handlePing(serverName string, config *config.Collector) error {
	mysql := getMySQLConnection(config)
	defer mysql.Close()
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	err := mysql.Ping(serverName, unixTime)
	if err != nil {
		logger.Log("error", err.Error())
		return fmt.Errorf("error saving ping from %s", serverName)
	}
	return nil
}

func isUp(serverName string, config *config.Collector) (bool, error) {
	mysql := getMySQLConnection(config)
	defer mysql.Close()

	serverPingTimeStr, err := mysql.ServerPingTime(serverName)
	if err != nil {
		// logger.Log("error", err.Error())
		return false, fmt.Errorf("error loading ping time of %s", serverName)
	}

	serverPingTime, err := strconv.ParseInt(serverPingTimeStr, 10, 64)
	if err != nil {
		logger.Log("error", err.Error())
		return false, fmt.Errorf("error loading ping time of %s", serverName)
	}

	return time.Now().Unix()-serverPingTime <= 61, nil // extra sec for delays ¯\_(ツ)_/¯
}

func handleMonitorData(monitorData *monitor.MonitorData) error {
	serverName := monitorData.ServerId
	time := monitorData.UnixTime
	config := config.GetCollector()
	mysql := getMySQLConnection(&config)
	defer mysql.Close()

	data := make(map[string]interface{})
	data["system"] = &monitorData.System
	data["memory"] = &monitorData.Memory
	data["swap"] = &monitorData.Swap
	data["procUsage"] = &monitorData.ProcUsage
	data["processes"] = &monitorData.Processes

	for key, item := range data {
		err := saveToDB(item, mysql, serverName, time, key, "")
		if err != nil {
			return err
		}
	}

	for _, disk := range monitorData.Disk {
		err := saveToDB(disk, mysql, serverName, time, monitor.DISKS, disk.FileSystem)
		if err != nil {
			return err
		}
	}

	for _, service := range monitorData.Services {
		err := saveToDB(service, mysql, serverName, time, monitor.SERVICES, service.Name)
		if err != nil {
			return err
		}
	}

	for _, network := range monitorData.Networks {
		err := saveToDB(network, mysql, serverName, time, monitor.NETWORKS, network.Interface)
		if err != nil {
			return err
		}
	}

	return nil
}

func saveToDB(item interface{}, mysql database.MySql, serverName string, time string, key string, logName string) error {
	res, err := json.Marshal(item)
	if err != nil {
		return err
	}

	err = mysql.SaveLogToDB(serverName, time, string(res), key, logName, false)
	if err != nil {
		return err
	}
	return nil
}

func handleCustomMetric(customMetric *monitor.CustomMetric) error {
	serverName := customMetric.ServerId
	time := customMetric.Time
	config := config.GetCollector()
	mysql := getMySQLConnection(&config)
	defer mysql.Close()

	res, err := json.Marshal(&customMetric)
	if err != nil {
		return err
	}
	return mysql.SaveLogToDB(serverName, time, string(res), customMetric.Name, "", true)
}

func getMonitorLogs(serverName string, logType string, from int64, to int64, time int64, config *config.Collector, convertToJsonArr bool, isCustomMetric bool) string {
	mysql := getMySQLConnection(config)
	defer mysql.Close()
	data := mysql.GetLogFromDB(serverName, logType, from, to, time, isCustomMetric)
	if (convertToJsonArr || (to != 0 && from != 0)) && logType != "system" {
		if logType == monitor.DISKS || logType == monitor.NETWORKS || logType == monitor.SERVICES {
			var arr []string
			for _, row := range data {
				var newData []string
				_ = json.Unmarshal([]byte(row), &newData)
				arr = append(arr, stringops.StringArrToJSONArr(newData))
			}
			return stringops.StringArrToJSONArr(arr)
		}
		return stringops.StringArrToJSONArr(data)
	} else {
		if len(data) == 0 {
			return ""
		}

		if logType == monitor.DISKS || logType == monitor.NETWORKS || logType == monitor.SERVICES {
			var arr []string
			_ = json.Unmarshal([]byte(data[0]), &arr)
			str := stringops.StringArrToJSONArr(arr)
			return str
		}
		return data[0]
	}
}

func getMySQLConnection(c *config.Collector) database.MySql {
	mysql := database.MySql{}
	password := os.Getenv("SYMON_DB_PASSWORD")
	mysql.Connect(c.MySQLUserName, password, c.MySQLHost, c.MySQLDatabaseName, false)
	return mysql
}
