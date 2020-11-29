package util

import (
	"encoding/json"
	"io/ioutil"
)

// Config struct with config
type Config struct {
	Port                     string
	SSLEnabled               bool
	SSLCertFilePath          string
	SSLKeyFilePath           string
	LogFileEnabled           bool
	LogFilePath              string
	CLIMonitoringInterval    int
	SQLiteDBLoggingEnabled   bool
	SQLiteDBPath             string
	EmailNotificationEnabled bool
	EmailHost                string
	EmailPort                string
	EmailFrom                string
	EmailTo                  string
	CPUThreshold             int
	MemoryThreshold          int
	WarnAfterSecs            int
}

// GetConfig return config struct
func GetConfig() Config {
	file, err := ioutil.ReadFile("config.json")
	config := Config{}

	if err != nil {
		return config
	}

	_ = json.Unmarshal([]byte(file), &config)

	return config
}
