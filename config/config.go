package config

// Config holds the configurations
type Config struct {
	MonitorInterval uint8
	LogFileEnabled  bool
	LogFilePath     string
	DBPath          string
}
