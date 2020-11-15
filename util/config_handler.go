package util

import (
	"encoding/json"
	"io/ioutil"
)

// Config struct with config
type Config struct {
	Port                   string
	SSLEnabled             bool
	SSLCertFilePath        string
	SSLKeyFilePath         string
	LogFileEnabled         bool
	LogFilePath            string
	MonitoringInterval     int
	SQLiteDBLoggingEnabled bool
	SQLiteDBPath           string
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
