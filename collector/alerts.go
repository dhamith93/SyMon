package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/pkg/memdb"
	"github.com/dhamith93/systats"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func handleAlerts(alertConfigs []alerts.AlertConfig, config *config.Collector, mysql *database.MySql) {
	mysql.ClearAllAlertsWithNullEnd()
	ticker := time.NewTicker(15 * time.Second)
	endpointTicker := time.NewTicker(time.Duration(config.EndpointCheckInterval) * time.Second)
	quit := make(chan struct{})
	var wg sync.WaitGroup
	incidentTracker := memdb.CreateDatabase("incident_tracker")

	delta := 1
	if config.EndpointMonitoringEnabled {
		delta = 2
	}

	wg.Add(delta)
	logger.Log("info", "starting alert checker")
	err := incidentTracker.Create(
		"alert",
		memdb.Col{Name: "server_name", Type: memdb.String},
		memdb.Col{Name: "metric_type", Type: memdb.String},
		memdb.Col{Name: "metric_name", Type: memdb.String},
		memdb.Col{Name: "time", Type: memdb.String},
		memdb.Col{Name: "status", Type: memdb.Int},
		memdb.Col{Name: "value", Type: memdb.Float32},
	)
	if err != nil {
		logger.Log("error", "memdb: "+err.Error())
	} else {
		go func() {
			for {
				select {
				case <-ticker.C:
					for _, alert := range alertConfigs {
						if alert.MetricName == "endpoint" {
							continue
						}
						for _, server := range alert.Servers {
							processAlert(&alert, server, config, mysql, &incidentTracker)
						}
					}
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
	}

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

func processAlert(alert *alerts.AlertConfig, server string, config *config.Collector, mysql *database.MySql, incidentTracker *memdb.Database) {
	metricType := alert.MetricName
	metricName := ""
	if metricType == monitor.DISKS {
		metricName = alert.Disk
	}
	if metricType == monitor.SERVICES {
		metricName = alert.Service
	}
	alertStatus := buildAlertStatus(alert, &server, config, mysql)

	// duplicate check
	if metricType != monitor.PING {
		alertFromDbForStartEvent := mysql.GetAlertByStartEvent(strconv.FormatInt(alertStatus.StartEvent, 10))
		if alertFromDbForStartEvent != nil {
			return
		}
	}

	// check if an active alert is present in DB
	previousAlert := mysql.GetPreviousOpenAlert(&alertStatus, alert.IsCustom)
	if previousAlert != nil {
		// if current alert status is normal, check if normal status continued for threshold period and update alert status in DB
		if alertStatus.Type != alertstatus.Warning && alertStatus.Type != alertstatus.Critical {
			res := incidentTracker.Tables["alert"].Where("server_name", "==", server).And("metric_type", "==", metricType).And("status", "==", int(alertstatus.Normal))
			if metricType == monitor.DISKS || metricType == monitor.SERVICES {
				res = res.And("metric_name", "==", metricName)
			}
			if res.RowCount == 0 {
				err := incidentTracker.Tables["alert"].Insert("server_name, metric_type, metric_name, time, value, status", server, metricType, metricName, alertStatus.UnixTime, alertStatus.Value, int(alertstatus.Normal))
				if err != nil {
					logger.Log("error", "memdb: "+err.Error())
				}
				return
			}

			prevTime, err := strconv.ParseInt(res.Rows[0].Columns["time"].StringVal, 10, 64)
			if err != nil {
				logger.Log("error", err.Error())
			}
			currTime, err := strconv.ParseInt(alertStatus.UnixTime, 10, 64)
			if err != nil {
				logger.Log("error", err.Error())
			}

			if (currTime - prevTime) >= int64(alertStatus.Alert.TriggerIntveral) {
				err = mysql.SetAlertEndLog(&alertStatus, previousAlert[6])
				// queue an alert resolved
				sendAlert(buildAlertToSend(server, alert, alertStatus), config)
				if err != nil {
					logger.Log("error", "Error updating alert: "+err.Error())
				}
				res.Delete()
				return
			}
		}

		// Alert status changed between warn & crit
		prevAlertStatus, _ := strconv.Atoi(previousAlert[2])
		if (alertStatus.Type == alertstatus.Critical && prevAlertStatus == int(alertstatus.Warning)) || (alertStatus.Type == alertstatus.Warning && prevAlertStatus == int(alertstatus.Critical)) {
			err := mysql.UpdateAlert(&alertStatus, previousAlert[6])
			if err != nil {
				logger.Log("error", "Error updating alert: "+err.Error())
			}
			// queue an alert status changed
			sendAlert(buildAlertToSend(server, alert, alertStatus), config)
		}
		return
	}

	if alertStatus.Type == alertstatus.Warning || alertStatus.Type == alertstatus.Critical {
		res := incidentTracker.Tables["alert"].Where("server_name", "==", server).And("metric_type", "==", metricType).And("status", "!=", int(alertstatus.Normal))
		if metricType == monitor.DISKS || metricType == monitor.SERVICES {
			res = res.And("metric_name", "==", metricName)
		}

		if res.RowCount == 0 {
			err := incidentTracker.Tables["alert"].Insert("server_name, metric_type, metric_name, time, value, status", server, metricType, metricName, alertStatus.UnixTime, alertStatus.Value, int(alertStatus.Type))
			if err != nil {
				logger.Log("error", "memdb: "+err.Error())
			}
			return
		}

		prevTime, err := strconv.ParseInt(res.Rows[0].Columns["time"].StringVal, 10, 64)
		if err != nil {
			logger.Log("error", err.Error())
		}
		currTime, err := strconv.ParseInt(alertStatus.UnixTime, 10, 64)
		if err != nil {
			logger.Log("error", err.Error())
		}

		if (currTime - prevTime) >= int64(alertStatus.Alert.TriggerIntveral) {
			err = mysql.AddAlert(&alertStatus)
			// queue a new alert
			sendAlert(buildAlertToSend(server, alert, alertStatus), config)
			if err != nil {
				logger.Log("error", "Error adding alert: "+err.Error())
			}
			res.Delete()
		}
	}
}

func checkEndpoint(alert *alerts.AlertConfig, incidentTracker *memdb.Database, config *config.Collector) {
	var (
		res *http.Response
		err error
	)
	customCACertUsed := len(strings.TrimSpace(alert.CustomCACert)) > 0
	client := &http.Client{}

	if customCACertUsed {
		caCert, err := ioutil.ReadFile(alert.CustomCACert)
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
					sendAlert(alertToSend, config)
					existingRecord.Update("alerted", true)
				}
			}
		} else {
			if alert.ExpectedHTTPCode != existingActual && alerted {
				alertToSend := buildEndpointAlert(alert, statusCode, errMsg, true, timeNow)
				sendAlert(alertToSend, config)
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

func buildAlertStatus(alert *alerts.AlertConfig, server *string, config *config.Collector, mysql *database.MySql) alertstatus.AlertStatus {
	var alertStatus alertstatus.AlertStatus
	logName := ""

	switch alert.MetricName {
	case monitor.DISKS:
		logName = alert.Disk
	case monitor.SERVICES:
		logName = alert.Service
	}

	metricLogs := mysql.GetLogFromDBWithId(*server, alert.MetricName, logName, 0, 0, alert.IsCustom)
	if len(metricLogs) == 0 {
		return alertStatus
	}
	logId := metricLogs[0][0]
	alertStatus.Alert = *alert
	alertStatus.Server = *server
	alertStatus.Type = alertstatus.Normal

	switch alert.MetricName {
	case monitor.PROC_USAGE:
		var cpu systats.CPU
		err := json.Unmarshal([]byte(metricLogs[0][1]), &cpu)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}
		alertStatus.UnixTime = strconv.FormatInt(cpu.Time, 10)
		alertStatus.Value = float32(cpu.LoadAvg)
		alertStatus.Type = getAlertType(alert, float64(cpu.LoadAvg))
	case monitor.MEMORY:
		var mem systats.Memory
		err := json.Unmarshal([]byte(metricLogs[0][1]), &mem)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}
		alertStatus.UnixTime = strconv.FormatInt(mem.Time, 10)
		alertStatus.Value = float32(mem.PercentageUsed)
		alertStatus.Type = getAlertType(alert, mem.PercentageUsed)
	case monitor.SWAP:
		var swap systats.Swap
		err := json.Unmarshal([]byte(metricLogs[0][1]), &swap)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}
		alertStatus.UnixTime = strconv.FormatInt(swap.Time, 10)
		alertStatus.Value = float32(swap.PercentageUsed)
		alertStatus.Type = getAlertType(alert, swap.PercentageUsed)
	case monitor.DISKS:
		var disk systats.Disk
		err := json.Unmarshal([]byte(metricLogs[0][1]), &disk)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}

		if disk.FileSystem == alert.Disk {
			valStr := strings.Replace(disk.Usage.Usage, "%", "", -1)
			val, err := strconv.ParseFloat(valStr, 32)
			if err != nil {
				logger.Log("error", err.Error())
				return alertStatus
			}
			alertStatus.Value = float32(val)
			alertStatus.Type = getAlertType(alert, val)
			alertStatus.UnixTime = strconv.FormatInt(disk.Time, 10)
			break
		}
	case monitor.SERVICES:
		var service monitor.Service
		err := json.Unmarshal([]byte(metricLogs[0][1]), &service)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}

		if service.Name == alert.Service {
			val := 0.0
			if service.Running {
				val = 1.0
			}
			alertStatus.Value = float32(val)
			alertStatus.Type = getAlertType(alert, val)
			alertStatus.UnixTime = service.Time
			break
		}
	case monitor.PING:
		pingLog := mysql.GetLogFromDBWithId(*server, alert.MetricName, "", 0, 0, alert.IsCustom)
		alertStatus.Type = alertstatus.Normal
		if len(pingLog) > 0 {
			lastPingTime, _ := strconv.Atoi(pingLog[0][1])
			timeNow := time.Now().Unix()
			diff := timeNow - int64(lastPingTime)
			if diff > int64(alert.TriggerIntveral) {
				alertStatus.Type = alertstatus.Critical
			}
		}
		alertStatus.UnixTime = strconv.FormatInt(time.Now().Unix(), 10)
	default:
		var customMetric monitor.CustomMetric
		err := json.Unmarshal([]byte(metricLogs[0][1]), &customMetric)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}
		alertStatus.UnixTime = customMetric.Time
		value, err := strconv.ParseFloat(customMetric.Value, 32)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}
		alertStatus.Value = float32(value)
		alertStatus.Type = getAlertType(alert, float64(alertStatus.Value))
	}
	logIdInt, err := strconv.ParseInt(logId, 10, 64)
	if err != nil {
		logger.Log("error", err.Error())
		return alertStatus
	}
	alertStatus.StartEvent = logIdInt
	return alertStatus
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

func sendAlert(alert *alertapi.Alert, config *config.Collector) {
	conn, c, ctx, cancel := createClient(config)
	if conn == nil {
		logger.Log("error", "error creating connection")
		return
	}
	defer conn.Close()
	defer cancel()
	_, err := c.HandleAlerts(ctx, alert)
	if err != nil {
		logger.Log("error", "error sending data: "+err.Error())
	}
}

func generateToken() string {
	token, err := auth.GenerateJWT()
	if err != nil {
		logger.Log("error", "error generating token: "+err.Error())
		os.Exit(1)
	}
	return token
}

func createClient(config *config.Collector) (*grpc.ClientConn, alertapi.AlertServiceClient, context.Context, context.CancelFunc) {
	var (
		conn     *grpc.ClientConn
		tlsCreds credentials.TransportCredentials
		err      error
	)

	if len(config.AlertEndpointCACertPath) > 0 {
		tlsCreds, err = loadTLSCredsAsClient(config)
		if err != nil {
			log.Fatal("cannot load TLS credentials: ", err)
		}
		conn, err = grpc.Dial(config.AlertEndpoint, grpc.WithTransportCredentials(tlsCreds))
	} else {
		conn, err = grpc.Dial(config.AlertEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if err != nil {
		logger.Log("error", "connection error: "+err.Error())
		return nil, nil, nil, nil
	}
	c := alertapi.NewAlertServiceClient(conn)
	token := generateToken()
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{"jwt": token})), time.Second*10)
	return conn, c, ctx, cancel
}

func loadTLSCredsAsClient(config *config.Collector) (credentials.TransportCredentials, error) {
	cert, err := ioutil.ReadFile(config.AlertEndpointCACertPath)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(cert) {
		return nil, fmt.Errorf("failed to add server CA cert")
	}

	tlsConfig := &tls.Config{
		RootCAs: certPool,
	}

	return credentials.NewTLS(tlsConfig), nil
}
