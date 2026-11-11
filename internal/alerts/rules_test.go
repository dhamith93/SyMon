package alerts

import (
	"strings"
	"testing"
)

func TestParseRules(t *testing.T) {
	rules, err := ParseRules([]byte(`[
		{"Name": "CPU", "MetricName": "procUsage", "Servers": ["*"], "Op": ">", "WarnThreshold": 80, "CriticalThreshold": 95},
		{"Name": "Shop", "MetricName": "endpoint", "Endpoint": "https://shop.example.com", "PagerDuty": true, "Channels": [" ops email ", "phone"]},
		{"Name": "Pods", "MetricName": "kube_workload", "Cluster": " homelab ", "Namespace": " media "},
		{"Name": "Uplink", "MetricName": "device_port", "Device": " sw1 ", "Interface": " Gi1/0/24 "},
		{"Name": "Volumes", "MetricName": "device_metric", "Device": "*", "DeviceMetric": " storage_used_pct ", "Label": " /volume1 ", "Op": ">", "WarnThreshold": 80, "CriticalThreshold": 90}
	]`))
	if err != nil || len(rules) != 5 || rules[2].Cluster != "homelab" || rules[2].Namespace != "media" || rules[0].Servers[0] != AllHosts || !rules[1].Pagerduty || strings.Join(rules[1].Channels, ",") != "ops email,phone" {
		t.Fatalf("expected five rules, got %+v %v", rules, err)
	}
	if rules[3].Device != "sw1" || rules[3].Interface != "Gi1/0/24" || rules[4].DeviceMetric != "storage_used_pct" || rules[4].Label != "/volume1" {
		t.Errorf("expected the device fields trimmed, got %+v and %+v", rules[3], rules[4])
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
		{"empty channel", `[{"Name": "x", "MetricName": "ping", "Servers": ["*"], "Channels": [" "]}]`, "channel name in Channels is empty"},
		{"channel twice", `[{"Name": "x", "MetricName": "ping", "Servers": ["*"], "Channels": ["ops", "ops "]}]`, `the channel "ops" is in Channels twice`},
		{"kube without cluster", `[{"Name": "x", "MetricName": "kube_workload", "Cluster": " "}]`, "needs Cluster"},
		{"kube restarts op", `[{"Name": "x", "MetricName": "kube_restarts", "Cluster": "homelab", "Op": "=>"}]`, `unknown Op "=>"`},
		{"device without device", `[{"Name": "x", "MetricName": "device_down", "Device": " "}]`, "needs Device"},
		{"port without interface", `[{"Name": "x", "MetricName": "device_port", "Device": "sw1"}]`, "needs Interface"},
		{"device metric without metric", `[{"Name": "x", "MetricName": "device_metric", "Device": "sw1", "Op": ">"}]`, "needs DeviceMetric"},
		{"device metric op", `[{"Name": "x", "MetricName": "device_metric", "Device": "ups1", "DeviceMetric": "ups_charge_pct", "Op": "=>"}]`, `unknown Op "=>"`},
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
