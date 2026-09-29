package alerts

import (
	"encoding/json"
	"os"
)

type AlertConfig struct {
	Name              string
	Description       string
	MetricName        string
	IsCustom          bool
	Op                string
	Template          string
	WarnThreshold     int
	CriticalThreshold int
	TriggerIntveral   int
	Servers           []string
	Endpoint          string
	ExpectedHTTPCode  int
	Method            string
	CustomCACert      string
	POSTContentType   string
	POSTBody          string
	Disk              string
	Service           string
	Email             bool
	Pagerduty         bool
	Slack             bool
	SlackChannel      string
}

type Alert struct {
	ServerName        string
	Name              string
	Template          string
	MetricName        string
	Op                string
	Timestamp         string
	Value             float32
	WarnThreshold     int
	CriticalThreshold int
	TriggerIntveral   int
}

func GetAlertConfig(path string) []AlertConfig {
	file, err := os.ReadFile(path)
	alertConfig := []AlertConfig{}

	if err != nil {
		return alertConfig
	}

	_ = json.Unmarshal([]byte(file), &alertConfig)

	return alertConfig
}
