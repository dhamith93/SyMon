package alerts

import (
	"strings"
	"testing"
)

func TestParseRules(t *testing.T) {
	rules, err := ParseRules([]byte(`[
		{"Name": "CPU", "MetricName": "procUsage", "Servers": ["*"], "Op": ">", "WarnThreshold": 80, "CriticalThreshold": 95},
		{"Name": "Shop", "MetricName": "endpoint", "Endpoint": "https://shop.example.com", "PagerDuty": true}
	]`))
	if err != nil || len(rules) != 2 || rules[0].Servers[0] != AllHosts || !rules[1].Pagerduty {
		t.Fatalf("expected two rules, got %+v %v", rules, err)
	}

	tests := []struct {
		name string
		json string
		want string
	}{
		{"syntax", "[\n  {\"Name\": \"CPU\",}\n]", "line 2, column 18"},
		{"misspelled field", `[{"Name": "CPU", "MetricName": "procUsage", "Servers": ["web1"], "Op": ">", "WarnTreshold": 80}]`, `unknown field "WarnTreshold"`},
		{"wrong type", `[{"Name": "CPU", "MetricName": "procUsage", "WarnThreshold": "80"}]`, "line 1"},
		{"no name", `[{"MetricName": "procUsage", "Servers": ["web1"], "Op": ">"}]`, "needs a Name"},
		{"unknown metric", `[{"Name": "x", "MetricName": "cpu", "Servers": ["web1"], "Op": ">"}]`, `unknown MetricName "cpu"`},
		{"no servers", `[{"Name": "x", "MetricName": "memory", "Op": ">"}]`, "needs Servers"},
		{"bad op", `[{"Name": "x", "MetricName": "memory", "Servers": ["web1"], "Op": "=>"}]`, `unknown Op "=>"`},
		{"disk without device", `[{"Name": "x", "MetricName": "disks", "Servers": ["web1"], "Op": ">"}]`, "needs Disk"},
		{"service op", `[{"Name": "x", "MetricName": "services", "Service": "nginx", "Servers": ["web1"], "Op": ">"}]`, `Op "inactive"`},
		{"endpoint url", `[{"Name": "x", "MetricName": "endpoint", "Endpoint": "shop.example.com"}]`, "http:// or https:// URL"},
		{"custom without name", `[{"Name": "x", "IsCustom": true, "Servers": ["web1"], "Op": ">"}]`, "custom metric"},
		{"negative cert days", `[{"Name": "x", "MetricName": "endpoint", "Endpoint": "https://example.com", "CertWarnDays": -1}]`, "cannot be negative"},
		{"critical above warning", `[{"Name": "x", "MetricName": "endpoint", "Endpoint": "https://example.com", "CertWarnDays": 5, "CertCriticalDays": 10}]`, "at most CertWarnDays"},
		{"duplicate names", `[{"Name": "x", "MetricName": "ping", "Servers": ["*"]}, {"Name": "x", "MetricName": "ping", "Servers": ["*"]}]`, "already a rule named"},
	}
	for _, tt := range tests {
		_, err := ParseRules([]byte(tt.json))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: expected an error with %q, got %v", tt.name, tt.want, err)
		}
	}
}

func TestSampleRulesAreValid(t *testing.T) {
	if _, err := LoadRules("../../collector/alerts.json"); err != nil {
		t.Errorf("the sample alerts.json does not load: %v", err)
	}
}

func TestCertDays(t *testing.T) {
	seven, zero := 7, 0
	tests := []struct {
		rule          AlertConfig
		warn, critial int
	}{
		{AlertConfig{MetricName: "endpoint", Endpoint: "https://example.com"}, 14, 3},
		{AlertConfig{MetricName: "endpoint", Endpoint: "HTTPS://example.com", CertWarnDays: &seven, CertCriticalDays: &zero}, 7, 0},
		{AlertConfig{MetricName: "endpoint", Endpoint: "http://example.com"}, 0, 0},
		{AlertConfig{MetricName: "memory"}, 0, 0},
	}
	for _, tt := range tests {
		if warn, critical := tt.rule.CertDays(); warn != tt.warn || critical != tt.critial {
			t.Errorf("%s: got %d %d, want %d %d", tt.rule.Endpoint, warn, critical, tt.warn, tt.critial)
		}
	}
}
