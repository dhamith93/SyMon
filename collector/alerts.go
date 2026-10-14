package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
)

// handleAlerts checks the enabled rules every 15 seconds. The rules are
// read from the database each time, so changes on the dashboard apply
// right away. What the rules find is queued for the notification channels.
func handleAlerts(config *config.Collector, st *store.Store) {
	if os.Getenv("SYMON_ALERT_ENDPOINT") != "" {
		logger.Log("info", "SYMON_ALERT_ENDPOINT is not used any more: the collector sends alerts itself, to the channels set up on the dashboard's Notifications page")
	}
	if config.EndpointMonitoringEnabled {
		logger.Log("info", "starting endpoint monitor")
		go runEndpointChecks(time.Duration(config.EndpointCheckInterval)*time.Second, st)
	}
	go deliverNotifications(st, strings.TrimRight(config.DashboardURL, "/"))

	logger.Log("info", "starting alert checker")
	evaluator := newEvaluator(st, func(ctx context.Context, n notice) {
		if err := queueNotice(ctx, st, n); err != nil {
			logger.Log("error", "cannot queue notifications for alert "+strconv.FormatInt(n.alertID, 10)+": "+err.Error())
		}
	})
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := transport.Context()
		rules, err := st.EnabledRules(ctx)
		if err != nil {
			cancel()
			logger.Log("error", "cannot read alert rules: "+err.Error())
			continue
		}
		hosts, err := st.Hosts(ctx)
		cancel()
		if err != nil {
			logger.Log("error", "cannot read hosts: "+err.Error())
			continue
		}
		names := make([]string, 0, len(hosts))
		for _, host := range hosts {
			names = append(names, host.Name)
		}

		for i := range rules {
			rule := &rules[i]
			for _, host := range ruleHosts(rule, names) {
				ctx, cancel := transport.Context()
				if err := evaluator.evaluate(ctx, rule, host); err != nil {
					logger.Log("error", "alert "+rule.Name+" on "+cmp.Or(host, rule.Endpoint)+": "+err.Error())
				}
				cancel()
			}
			if certificate := certificateRule(rule); certificate != nil {
				ctx, cancel := transport.Context()
				if err := evaluator.evaluate(ctx, certificate, ""); err != nil {
					logger.Log("error", "certificate alert "+rule.Name+": "+err.Error())
				}
				cancel()
			}
		}
	}
}

// ruleHosts are the hosts a rule checks. "*" is every registered host, and
// endpoint rules are checked from the collector, so they have none.
func ruleHosts(rule *alerts.AlertConfig, hosts []string) []string {
	if rule.MetricName == monitor.ENDPOINT {
		return []string{""}
	}
	if slices.Contains(rule.Servers, alerts.AllHosts) {
		return hosts
	}
	return rule.Servers
}

// endpointText fills an endpoint rule's template. actualHTTPCode is 0 when
// there was no response.
func endpointText(rule *alerts.AlertConfig, actualHTTPCode int, errMsg string, resolved bool, at time.Time) (string, string) {
	if resolved {
		subject := "[Resolved] " + rule.Endpoint + " is answering again"
		return subject, rule.Endpoint + " answered with " + strconv.Itoa(actualHTTPCode) + " at " + at.UTC().Format(time.RFC1123) + "."
	}
	subject := "[Critical] endpoint check failed on " + rule.Endpoint
	replacer := strings.NewReplacer(
		"{subject}", subject,
		"{endpoint}", rule.Endpoint,
		"{metricName}", rule.MetricName,
		"{expected}", strconv.Itoa(expectedCode(rule)),
		"{actual}", strconv.Itoa(actualHTTPCode),
		"{timestamp}", at.UTC().String(),
		"{error}", errMsg,
		"{triggerInterval}", strconv.Itoa(rule.TriggerIntveral),
	)
	body := replacer.Replace(rule.Template)
	if strings.TrimSpace(body) == "" {
		body = rule.Endpoint + " should answer with " + strconv.Itoa(expectedCode(rule)) + " and answered with " + strconv.Itoa(actualHTTPCode)
		if errMsg != "" {
			body += ": " + errMsg
		}
	}
	return subject, body
}

// certificateText says how many days an endpoint's certificate has left
func certificateText(rule *alerts.AlertConfig, days float64, status alertstatus.StatusType, at time.Time) (string, string) {
	subject := "[Resolved] certificate renewed for " + rule.Endpoint
	content := rule.Endpoint + " has a certificate valid for " + strconv.Itoa(int(days)) + " more days"
	switch status {
	case alertstatus.Critical:
		subject = "[Critical] certificate expiring for " + rule.Endpoint
	case alertstatus.Warning:
		subject = "[Warning] certificate expiring for " + rule.Endpoint
	}
	if status != alertstatus.Normal {
		expires := at.Add(time.Duration(days * 24 * float64(time.Hour)))
		content = rule.Endpoint + " has a certificate that expires on " + expires.UTC().Format("Jan 2 2006 15:04 MST")
		if days < 0 {
			content = rule.Endpoint + " has a certificate that expired on " + expires.UTC().Format("Jan 2 2006 15:04 MST")
		}
	}
	return subject, content
}

// alertText fills a host rule's template
func alertText(rule *alerts.AlertConfig, host string, status alertstatus.StatusType, value float64, at time.Time) (string, string) {
	subject := rule.Name + " triggered on " + host
	expected := ""
	switch status {
	case alertstatus.Critical:
		subject = "[Critical] " + subject
		expected = strconv.Itoa(rule.CriticalThreshold)
	case alertstatus.Warning:
		subject = "[Warning] " + subject
		expected = strconv.Itoa(rule.WarnThreshold)
	default:
		subject = "[Resolved] " + rule.Name + " on " + host
		if rule.MetricName == monitor.PING {
			return subject, host + " is reporting again at " + at.UTC().Format(time.RFC1123) + "."
		}
		return subject, rule.Name + " on " + host + " is back to normal at " + at.UTC().Format(time.RFC1123) + ", with " + fmt.Sprintf("%.2f", value) + "."
	}
	replacer := strings.NewReplacer(
		"{subject}", subject,
		"{serverName}", host,
		"{metricName}", rule.MetricName,
		"{op}", rule.Op,
		"{expected}", expected,
		"{timestamp}", at.UTC().String(),
		"{desc}", rule.Description,
		"{value}", fmt.Sprintf("%.2f", value),
		"{triggerInterval}", strconv.Itoa(rule.TriggerIntveral),
	)
	body := replacer.Replace(rule.Template)
	if strings.TrimSpace(body) == "" {
		body = subject + ": " + fmt.Sprintf("%.2f", value)
		if expected != "" {
			body += " " + rule.Op + " " + expected
		}
	}
	return subject, body
}

func getAlertType(alert *alerts.AlertConfig, val float64) alertstatus.StatusType {
	switch alert.Op {
	case "==":
		if val == float64(alert.CriticalThreshold) {
			return alertstatus.Critical
		} else if val == float64(alert.WarnThreshold) {
			return alertstatus.Warning
		}
		return alertstatus.Normal
	case "!=":
		if val != float64(alert.CriticalThreshold) {
			return alertstatus.Critical
		} else if val != float64(alert.WarnThreshold) {
			return alertstatus.Warning
		}
		return alertstatus.Normal
	case ">":
		if val > float64(alert.CriticalThreshold) {
			return alertstatus.Critical
		} else if val > float64(alert.WarnThreshold) {
			return alertstatus.Warning
		}
		return alertstatus.Normal
	case "<":
		if val < float64(alert.CriticalThreshold) {
			return alertstatus.Critical
		} else if val < float64(alert.WarnThreshold) {
			return alertstatus.Warning
		}
		return alertstatus.Normal
	case ">=":
		if val >= float64(alert.CriticalThreshold) {
			return alertstatus.Critical
		} else if val >= float64(alert.WarnThreshold) {
			return alertstatus.Warning
		}
		return alertstatus.Normal
	case "<=":
		if val <= float64(alert.CriticalThreshold) {
			return alertstatus.Critical
		} else if val <= float64(alert.WarnThreshold) {
			return alertstatus.Warning
		}
		return alertstatus.Normal
	case "inactive":
		if val == 0.0 {
			return alertstatus.Critical
		}
	case "active":
		if val == 1.0 {
			return alertstatus.Critical
		}
	}
	return alertstatus.Normal
}
