package config

import (
	"encoding/json"
	"io/ioutil"
)

// Config struct with config
type Config struct {
	MonitorEndpoint        string
	Port                   string
	SSLEnabled             bool
	SSLCertFilePath        string
	SSLKeyFilePath         string
	LogFileEnabled         bool
	LogFilePath            string
	CLIMonitoringInterval  int
	SQLiteDBLoggingEnabled bool
	SQLiteDBPath           string
	CPUThreshold           int
	MemoryThreshold        int
	DiskUsageThreshold     int
	DisksToIgnore          string
	WarnAfterSecs          int
	ServerId               string
	Services               []ServiceToMonitor
}

// ServiceToMonitor holds service info from config.json
type ServiceToMonitor struct {
	Name        string
	ServiceName string
}

// GetConfig return config struct
func GetConfig(path string) Config {
	file, err := ioutil.ReadFile(path)
	config := Config{}

	if err != nil {
		return config
	}

	_ = json.Unmarshal([]byte(file), &config)

	return config
}
