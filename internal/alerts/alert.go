package alerts

import (
	"encoding/json"
	"io/ioutil"
)

type AlertConfig struct {
	Name              string
	Description       string
	MetricName        string
	Operand           string
	WarnThreshold     int
	CriticalThreshold int
	TriggerIntveral   int
	Servers           []string
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
