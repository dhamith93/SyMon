package alerts

import (
	"encoding/json"
	"io/ioutil"
)

type AlertConfig struct {
	Name              string
	Description       string
	MetricName        string
	Op                string
	Template          string
	WarnThreshold     int
	CriticalThreshold int
	TriggerIntveral   int
	Servers           []string
	Disk              string
	Service           string
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
	file, err := ioutil.ReadFile(path)
	alertConfig := []AlertConfig{}

	if err != nil {
		return alertConfig
	}

	_ = json.Unmarshal([]byte(file), &alertConfig)

	return alertConfig
}
