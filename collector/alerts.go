package main

import (
	"cmp"
	"fmt"
	"log"
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

func handleAlerts(alertConfigs []alerts.AlertConfig, config *config.Collector, st *store.Store) {
	conn, err := transport.Dial(config.AlertEndpoint, config.AlertEndpointCACertPath, transport.SharedKey())
	if err != nil {
		log.Fatal("cannot create alert processor client: ", err)
	}
	defer conn.Close()
	alertClient = alertapi.NewAlertServiceClient(conn)

	if config.EndpointMonitoringEnabled {
		logger.Log("info", "starting endpoint monitor")
		go runEndpointChecks(alertConfigs, time.Duration(config.EndpointCheckInterval)*time.Second, st)
	}

	logger.Log("info", "starting alert checker")
	rules := newEvaluator(st, sendAlert)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		for _, alert := range alertConfigs {
			// endpoint rules are checked from the collector, not on a host
			hosts := alert.Servers
			if alert.MetricName == monitor.ENDPOINT {
				hosts = []string{""}
			}
			for _, server := range hosts {
				ctx, cancel := transport.Context()
				if err := rules.evaluate(ctx, &alert, server); err != nil {
					logger.Log("error", "alert "+alert.Name+" on "+cmp.Or(server, alert.Endpoint)+": "+err.Error())
				}
				cancel()
			}
		}
	}
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
