package config

import (
	"encoding/json"
	"io/ioutil"
)

// Config struct with config
type Config struct {
	Port              string
	AlertEndpoint     string
	SSLEnabled        bool
	SSLCertFilePath   string
	SSLKeyFilePath    string
	LogFileEnabled    bool
	LogFilePath       string
	SQLiteDBPath      string
	MySQLUserName     string
	MySQLHost         string
	MySQLDatabaseName string
	DataRetentionDays int32
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
