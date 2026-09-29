package main

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
)

// alertClient is created once in handleAlerts and shared by all alert checks
var alertClient alertapi.AlertServiceClient

// handleAlerts checks the enabled rules every 15 seconds. The rules are
// read from the database each time, so changes on the dashboard apply
// right away.
func handleAlerts(config *config.Collector, st *store.Store) {
	send := sendAlert
	if config.AlertEndpoint == "" {
		logger.Log("info", "no SYMON_ALERT_ENDPOINT, so alerts only show on the dashboard")
		send = func(*alertapi.Alert) {}
	} else {
		conn, err := transport.Dial(config.AlertEndpoint, config.AlertEndpointCACertPath, transport.SharedKey())
		if err != nil {
			log.Fatal("cannot create alert processor client: ", err)
		}
		defer conn.Close()
		alertClient = alertapi.NewAlertServiceClient(conn)
	}

	if config.EndpointMonitoringEnabled {
		logger.Log("info", "starting endpoint monitor")
		go runEndpointChecks(time.Duration(config.EndpointCheckInterval)*time.Second, st)
	}

	logger.Log("info", "starting alert checker")
	evaluator := newEvaluator(st, send)
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

// buildEndpointAlert fills an endpoint rule's template. id is the alert's
// row id, and actualHTTPCode is 0 when there was no response.
func buildEndpointAlert(alert *alerts.AlertConfig, id int64, actualHTTPCode int, errMsg string, resolved bool, unixtime int64) *alertapi.Alert {
	subject := "[Resolved] "
	status := 0
	if !resolved {
		subject = "[Critical] "
		status = 2
	}
	subject += "endpoint check failed on " + alert.Endpoint
	errMsg = strings.ReplaceAll(errMsg, "\"", "'")
	timestamp := time.Unix(unixtime, 0)
	replacer := strings.NewReplacer(
		"{subject}",
		subject,
		"{endpoint}",
		alert.Endpoint,
		"{metricName}",
		alert.MetricName,
		"{expected}",
		strconv.Itoa(expectedCode(alert)),
		"{actual}",
		strconv.Itoa(actualHTTPCode),
		"{timestamp}",
		timestamp.UTC().String(),
		"{error}",
		errMsg,
		"{triggerInterval}",
		strconv.Itoa(alert.TriggerIntveral),
	)
	content := replacer.Replace(alert.Template)
	return &alertapi.Alert{
		ServerName:   alert.Endpoint,
		MetricName:   alert.MetricName,
		LogId:        id,
		Status:       int32(status),
		Subject:      subject,
		Content:      content,
		Timestamp:    timestamp.UTC().String(),
		Resolved:     resolved,
		Pagerduty:    alert.Pagerduty,
		Email:        alert.Email,
		Slack:        alert.Slack,
		SlackChannel: alert.SlackChannel,
	}
}

// buildCertificateAlert says how many days an endpoint's certificate has
// left. id is the alert's row id.
func buildCertificateAlert(rule *alerts.AlertConfig, id int64, days float64, status alertstatus.StatusType, at time.Time) *alertapi.Alert {
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
	return &alertapi.Alert{
		ServerName:   rule.Endpoint,
		MetricName:   monitor.CERTIFICATE,
		LogId:        id,
		Status:       int32(status),
		Subject:      subject,
		Content:      subject + "\n" + content,
		Timestamp:    at.UTC().String(),
		Resolved:     status == alertstatus.Normal,
		Pagerduty:    rule.Pagerduty,
		Email:        rule.Email,
		Slack:        rule.Slack,
		SlackChannel: rule.SlackChannel,
	}
}

func buildAlertToSend(server string, alert *alerts.AlertConfig, alertStatus alertstatus.AlertStatus) *alertapi.Alert {
	alertToSend := buildAlert(alerts.Alert{
		ServerName:        server,
		Name:              alert.Name,
		MetricName:        alert.MetricName,
		Template:          alert.Template,
		Op:                alert.Op,
		WarnThreshold:     alert.WarnThreshold,
		CriticalThreshold: alert.CriticalThreshold,
		TriggerIntveral:   alert.TriggerIntveral,
		Value:             alertStatus.Value,
		Timestamp:         alertStatus.UnixTime,
	}, alertStatus, alert.Pagerduty, alert.Email, alert.Slack, alert.SlackChannel)
	return alertToSend
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

func buildAlert(alert alerts.Alert, status alertstatus.AlertStatus, sendPagerduty bool, sendEmail bool, sendSlack bool, slackChannel string) *alertapi.Alert {
	subject := "[Resolved] "
	expected := ""
	value := fmt.Sprintf("%.2f", alert.Value)
	if status.Type == alertstatus.Critical {
		subject = "[Critical] "
		expected = strconv.Itoa(alert.CriticalThreshold)
	} else if status.Type == alertstatus.Warning {
		subject = "[Warning] "
		expected = strconv.Itoa(alert.WarnThreshold)
	}
	subject += alert.Name + " triggered on " + alert.ServerName
	unixtime, err := strconv.ParseInt(alert.Timestamp, 10, 64)
	if err != nil {
		panic(err)
	}
	timestamp := time.Unix(unixtime, 0)
	replacer := strings.NewReplacer(
		"{subject}",
		subject,
		"{serverName}",
		alert.ServerName,
		"{metricName}",
		alert.MetricName,
		"{op}",
		alert.Op,
		"{expected}",
		expected,
		"{timestamp}",
		timestamp.UTC().String(),
		"{desc}",
		status.Alert.Description,
		"{value}",
		value,
		"{triggerInterval}",
		strconv.Itoa(alert.TriggerIntveral),
	)
	content := replacer.Replace(alert.Template)

	if status.Type == alertstatus.Normal {
		content = "\n------------\n" + "Alert is resolved at : " + timestamp.UTC().String() + "\n------------\n"
	}

	alertToSend := alertapi.Alert{
		ServerName:   alert.ServerName,
		MetricName:   alert.MetricName,
		LogId:        status.StartEvent,
		Status:       int32(status.Type),
		Subject:      subject,
		Content:      content,
		Timestamp:    timestamp.UTC().String(),
		Resolved:     (status.Type == alertstatus.Normal),
		Pagerduty:    sendPagerduty,
		Email:        sendEmail,
		Slack:        sendSlack,
		SlackChannel: slackChannel,
	}

	if alert.MetricName == monitor.DISKS || alert.MetricName == monitor.DISK_FORECAST {
		alertToSend.Disk = status.Alert.Disk
	}
	if alert.MetricName == monitor.SERVICES {
		alertToSend.Service = status.Alert.Service
	}

	return &alertToSend
}

func sendAlert(alert *alertapi.Alert) {
	ctx, cancel := transport.Context()
	defer cancel()
	_, err := alertClient.HandleAlerts(ctx, alert)
	if err != nil {
		logger.Log("error", "cannot send alert to the alert processor: "+err.Error())
	}
}
