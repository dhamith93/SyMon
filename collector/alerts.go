package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
	"github.com/dhamith93/SyMon/pkg/memdb"
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

	ticker := time.NewTicker(15 * time.Second)
	endpointTicker := time.NewTicker(time.Duration(config.EndpointCheckInterval) * time.Second)
	quit := make(chan struct{})
	var wg sync.WaitGroup
	incidentTracker := memdb.CreateDatabase("incident_tracker")
	rules := newEvaluator(st, sendAlert)

	delta := 1
	if config.EndpointMonitoringEnabled {
		delta = 2
	}

	wg.Add(delta)
	logger.Log("info", "starting alert checker")
	go func() {
		for {
			select {
			case <-ticker.C:
				for _, alert := range alertConfigs {
					if alert.MetricName == "endpoint" {
						continue
					}
					for _, server := range alert.Servers {
						ctx, cancel := transport.Context()
						if err := rules.evaluate(ctx, &alert, server); err != nil {
							logger.Log("error", "alert "+alert.Name+" on "+server+": "+err.Error())
						}
						cancel()
					}
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	if config.EndpointMonitoringEnabled {
		logger.Log("info", "starting endpoint monitor")
		err := incidentTracker.Create(
			"endpoint_monitor",
			memdb.Col{Name: "url", Type: memdb.String},
			memdb.Col{Name: "method", Type: memdb.String},
			memdb.Col{Name: "expected", Type: memdb.Int},
			memdb.Col{Name: "actual", Type: memdb.Int},
			memdb.Col{Name: "time", Type: memdb.Int64},
			memdb.Col{Name: "failed", Type: memdb.Bool},
			memdb.Col{Name: "error", Type: memdb.String},
			memdb.Col{Name: "alerted", Type: memdb.Bool},
		)
		if err != nil {
			logger.Log("error", "memdb: "+err.Error())
		} else {
			go func() {
				for {
					select {
					case <-endpointTicker.C:
						for _, alert := range alertConfigs {
							if alert.MetricName == "endpoint" {
								checkEndpoint(&alert, &incidentTracker, config)
							}
						}
					case <-quit:
						ticker.Stop()
						return
					}
				}
			}()
		}
	}
	wg.Wait()
	fmt.Println("Exiting")
}

func checkEndpoint(alert *alerts.AlertConfig, incidentTracker *memdb.Database, config *config.Collector) {
	var (
		res *http.Response
		err error
	)
	customCACertUsed := len(strings.TrimSpace(alert.CustomCACert)) > 0
	client := &http.Client{}

	if customCACertUsed {
		caCert, err := os.ReadFile(alert.CustomCACert)
		if err != nil {
			logger.Log("error", err.Error())
			return
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			},
		}
	}

	method := strings.ToUpper(alert.Method)

	if method == alerts.ENDPOINT_METHOD_GET {
		res, err = client.Get(alert.Endpoint)
	}
	if method == alerts.ENDPOINT_METHOD_POST {
		body := []byte(alert.POSTBody)
		bodyReader := bytes.NewReader(body)
		res, err = client.Post(alert.Endpoint, alert.POSTContentType, bodyReader)
	}

	timeNow := time.Now().Unix()
	failed := false
	errMsg := ""

	if err != nil {
		logger.Log("error", err.Error())
		errMsg = err.Error()
		failed = true
	}

	statusCode := -1

	if !failed {
		defer res.Body.Close()
		statusCode = res.StatusCode
	}

	existingRecord := incidentTracker.Tables["endpoint_monitor"].Where("url", "==", alert.Endpoint).And("method", "==", method).And("expected", "==", alert.ExpectedHTTPCode)

	if existingRecord.RowCount > 0 {
		diff := timeNow - existingRecord.Rows[0].Columns["time"].Int64Val
		existingActual := existingRecord.Rows[0].Columns["actual"].IntVal
		alerted := existingRecord.Rows[0].Columns["alerted"].BoolVal

		existingRecord.Update("actual", statusCode)
		existingRecord.Update("failed", failed)
		existingRecord.Update("error", errMsg)

		if statusCode != alert.ExpectedHTTPCode {
			if alert.ExpectedHTTPCode != existingActual && !alerted {
				if diff > int64(alert.TriggerIntveral) {
					alertToSend := buildEndpointAlert(alert, statusCode, errMsg, false, timeNow)
					sendAlert(alertToSend)
					existingRecord.Update("alerted", true)
				}
			}
		} else {
			if alert.ExpectedHTTPCode != existingActual && alerted {
				alertToSend := buildEndpointAlert(alert, statusCode, errMsg, true, timeNow)
				sendAlert(alertToSend)
				existingRecord.Update("alerted", false)
			}
			existingRecord.Update("time", timeNow)
		}

	} else {
		incidentTracker.Tables["endpoint_monitor"].Insert(
			"url, method, expected, actual, time, failed, error, alerted",
			alert.Endpoint, method, alert.ExpectedHTTPCode, statusCode, timeNow, failed, errMsg, false,
		)
	}
}

func buildEndpointAlert(alert *alerts.AlertConfig, actualHTTPCode int, errMsg string, resolved bool, unixtime int64) *alertapi.Alert {
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
		strconv.Itoa(alert.ExpectedHTTPCode),
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

	if alert.MetricName == monitor.DISKS {
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
