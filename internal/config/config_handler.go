package config

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

const DEFAULT_RETENTION_DAYS int32 = 30
const DEFAULT_INTERVAL_SECS int = 30
const DEFAULT_ENDPOINT_CHECK_INTERVAL int = 60

type Collector struct {
	TLSEnabled                bool
	LogFileEnabled            bool
	EndpointCheckInterval     int
	DataRetentionDays         int32
	EndpointMonitoringEnabled bool
	Port                      string
	AlertEndpoint             string
	AlertEndpointCACertPath   string
	CertPath                  string
	KeyPath                   string
	LogFilePath               string
	MySQLUserName             string
	MySQLHost                 string
	MySQLDatabaseName         string
	MySQLPassword             string
	AlertsFilePath            string
}

type Client struct {
	LogFileEnabled              bool
	Port                        string
	CollectorEndpoint           string
	AlertEndpoint               string
	CollectorEndpointCACertPath string
	AlertEndpointCACertPath     string
	LogFilePath                 string
}

type AlertProcessor struct {
	LogFileEnabled bool
	TLSEnabled     bool
	Port           string
	CertPath       string
	KeyPath        string
	LogFilePath    string
}

type Agent struct {
	MonitorIntervalSeconds      int
	LogFileEnabled              bool
	CollectorEndpoint           string
	CollectorEndpointCACertPath string
	LogFilePath                 string
	DisksToIgnore               string
	ServerId                    string
	Services                    []ServiceToMonitor
}

type ServiceToMonitor struct {
	Name        string
	ServiceName string
}

func GetAgent() Agent {
	intervalSecs := os.Getenv("SYMON_MONITOR_INTERVAL_SECONDS")
	intervalSecsInt := DEFAULT_INTERVAL_SECS
	if len(intervalSecs) > 0 {
		converted, _ := strconv.Atoi(intervalSecs)
		if converted > 0 {
			intervalSecsInt = converted
		}
	}
	return Agent{
		ServerId:                    os.Getenv("SYMON_SERVER_ID"),
		CollectorEndpoint:           os.Getenv("SYMON_COLLECTOR_ENDPOINT"),
		CollectorEndpointCACertPath: os.Getenv("SYMON_COLLECTOR_ENDPOINT_CERT_PATH"),
		LogFileEnabled:              strings.ToUpper(os.Getenv("SYMON_AGENT_LOG_FILE_ENABLED")) == "TRUE",
		LogFilePath:                 os.Getenv("SYMON_AGENT_LOG_FILE_PATH"),
		DisksToIgnore:               os.Getenv("SYMON_DISKS_TO_IGNORE"),
		Services:                    GetServicesToMonitor(os.Getenv("SYMON_SERVICE_LIST_PATH")),
		MonitorIntervalSeconds:      intervalSecsInt,
	}
}

func GetServicesToMonitor(path string) []ServiceToMonitor {
	services := []ServiceToMonitor{}
	file, err := ioutil.ReadFile(path)
	config := Agent{}

	if err != nil {
		return services
	}

	_ = json.Unmarshal([]byte(file), &config)

	return services
}

func GetCollector() Collector {
	retentionDays := os.Getenv("SYMON_DATA_RETENTION_DAYES")
	retentionDaysInt := DEFAULT_RETENTION_DAYS
	if len(retentionDays) > 0 {
		converted, _ := strconv.Atoi(retentionDays)
		if converted > 0 {
			retentionDaysInt = int32(converted)
		}
	}
	checkInterval := os.Getenv("SYMON_ENDPOINT_CHECK_INTERVAL")
	checkIntervalInt := DEFAULT_ENDPOINT_CHECK_INTERVAL
	if len(checkInterval) > 0 {
		converted, _ := strconv.Atoi(checkInterval)
		if converted > 0 {
			checkIntervalInt = converted
		}
	}
	return Collector{
		Port:                      os.Getenv("SYMON_PORT"),
		AlertEndpoint:             os.Getenv("SYMON_ALERT_ENDPOINT"),
		AlertEndpointCACertPath:   os.Getenv("SYMON_ALERT_ENDPOINT_CERT_PATH"),
		TLSEnabled:                strings.ToUpper(os.Getenv("SYMON_TLS_ENABLED")) == "TRUE",
		CertPath:                  os.Getenv("SYMON_TLS_CERT_PATH"),
		KeyPath:                   os.Getenv("SYMON_TLS_KEY_PATH"),
		LogFileEnabled:            strings.ToUpper(os.Getenv("SYMON_LOG_FILE_ENABLED")) == "TRUE",
		LogFilePath:               os.Getenv("SYMON_LOG_FILE_PATH"),
		MySQLUserName:             os.Getenv("SYMON_DB_USER"),
		MySQLHost:                 os.Getenv("SYMON_DB_HOST"),
		MySQLDatabaseName:         os.Getenv("SYMON_DB_NAME"),
		MySQLPassword:             os.Getenv("SYMON_DB_PASSWORD"),
		AlertsFilePath:            os.Getenv("SYMON_ALERTS_CONFIG_PATH"),
		EndpointMonitoringEnabled: strings.ToUpper(os.Getenv("SYMON_ENABLE_ENDPOINT_MONITORING")) == "TRUE",
		DataRetentionDays:         retentionDaysInt,
		EndpointCheckInterval:     checkIntervalInt,
	}
}

func GetClient() Client {
	return Client{
		Port:                        os.Getenv("SYMON_CLIENT_PORT"),
		CollectorEndpoint:           os.Getenv("SYMON_CLIENT_COLLECTOR_ENDPOINT"),
		CollectorEndpointCACertPath: os.Getenv("SYMON_CLIENT_COLLECTOR_ENDPOINT_CA_CERT_PATH"),
		AlertEndpoint:               os.Getenv("SYMON_CLIENT_ALERT_ENDPOINT"),
		AlertEndpointCACertPath:     os.Getenv("SYMON_CLIENT_ALERT_ENDPOINT_CERT_PATH"),
		LogFileEnabled:              strings.ToUpper(os.Getenv("SYMON_CLIENT_LOG_FILE_ENABLED")) == "TRUE",
		LogFilePath:                 os.Getenv("SYMON_CLIENT_LOG_FILE_PATH"),
	}
}

func GetAlertProcessor() AlertProcessor {
	return AlertProcessor{
		Port:           os.Getenv("SYMON_ALERT_PORT"),
		TLSEnabled:     strings.ToUpper(os.Getenv("SYMON_ALERT_TLS_ENABLED")) == "TRUE",
		CertPath:       os.Getenv("SYMON_ALERT_CERT_PATH"),
		KeyPath:        os.Getenv("SYMON_ALERT_KEY_PATH"),
		LogFileEnabled: strings.ToUpper(os.Getenv("SYMON_ALERT_LOG_FILE_ENABLED")) == "TRUE",
		LogFilePath:    os.Getenv("SYMON_ALERT_LOG_FILE_PATH"),
	}
}

func LogFileEnabled() bool {
	return GetAgent().LogFileEnabled || GetAlertProcessor().LogFileEnabled || GetCollector().LogFileEnabled || GetClient().LogFileEnabled
}
