package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
)

const DEFAULT_INTERVAL_SECS int = 30
const DEFAULT_ENDPOINT_CHECK_INTERVAL int = 60

type Collector struct {
	TLSEnabled                bool
	LogFileEnabled            bool
	EndpointCheckInterval     int
	EndpointMonitoringEnabled bool
	// DatabaseURL is a postgres:// connection string for TimescaleDB
	DatabaseURL string
	// days to keep raw data, 1 minute rollups and 1 hour rollups.
	// 0 means the store's default.
	RetentionRawDays        int
	RetentionMinuteDays     int
	RetentionHourDays       int
	Port                    string
	AlertEndpoint           string
	AlertEndpointCACertPath string
	CertPath                string
	KeyPath                 string
	LogFilePath             string
	AlertsFilePath          string
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
	DisabledCollectors          []string
	ContainerAware              bool
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
		DisabledCollectors:          splitList(os.Getenv("SYMON_DISABLED_COLLECTORS")),
		ContainerAware:              strings.ToUpper(os.Getenv("SYMON_CONTAINER_AWARE")) == "TRUE",
	}
}

// positiveInt parses value, returning 0 when it is empty, invalid or not positive
func positiveInt(value string) int {
	converted, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || converted < 0 {
		return 0
	}
	return converted
}

// splitList turns "a, B,,c" into [a b c]
func splitList(value string) []string {
	items := []string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	return items
}

func GetServicesToMonitor(path string) []ServiceToMonitor {
	services := []ServiceToMonitor{}
	if len(path) == 0 {
		return services
	}

	file, err := os.ReadFile(path)
	if err != nil {
		log.Println("error cannot read service list: " + err.Error())
		return services
	}

	if err := json.Unmarshal(file, &services); err != nil {
		log.Println("error cannot parse service list: " + err.Error())
		return []ServiceToMonitor{}
	}

	return services
}

func GetCollector() Collector {
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
		DatabaseURL:               os.Getenv("SYMON_DATABASE_URL"),
		RetentionRawDays:          positiveInt(os.Getenv("SYMON_RETENTION_RAW_DAYS")),
		RetentionMinuteDays:       positiveInt(os.Getenv("SYMON_RETENTION_MINUTE_DAYS")),
		RetentionHourDays:         positiveInt(os.Getenv("SYMON_RETENTION_HOUR_DAYS")),
		AlertsFilePath:            os.Getenv("SYMON_ALERTS_CONFIG_PATH"),
		EndpointMonitoringEnabled: strings.ToUpper(os.Getenv("SYMON_ENABLE_ENDPOINT_MONITORING")) == "TRUE",
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
