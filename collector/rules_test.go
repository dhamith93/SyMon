package main

import (
	"slices"
	"testing"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/monitor"
)

func TestRuleHosts(t *testing.T) {
	hosts := []string{"db1", "web1"}
	tests := []struct {
		rule alerts.AlertConfig
		want []string
	}{
		{alerts.AlertConfig{MetricName: monitor.PING, Servers: []string{"*"}}, hosts},
		{alerts.AlertConfig{MetricName: monitor.MEMORY, Servers: []string{"web1"}}, []string{"web1"}},
		{alerts.AlertConfig{MetricName: monitor.ENDPOINT, Endpoint: "https://example.com"}, []string{""}},
	}
	for _, tt := range tests {
		if got := ruleHosts(&tt.rule, hosts); !slices.Equal(got, tt.want) {
			t.Errorf("%s %v: got %v, want %v", tt.rule.MetricName, tt.rule.Servers, got, tt.want)
		}
	}
}
