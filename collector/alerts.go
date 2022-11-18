package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
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
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		incidentTracker := memdb.CreateDatabase("incident_tracker")
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
		}
		for {
			select {
			case <-ticker.C:
				for _, alert := range alertConfigs {
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
	alertFromDbForStartEvent := mysql.GetAlertByStartEvent(strconv.FormatInt(alertStatus.StartEvent, 10))
	if alertFromDbForStartEvent != nil {
		return
	}

	// check if an active alert is present in DB
	previousAlert := mysql.GetPreviousOpenAlert(&alertStatus)
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

	metricLogs := mysql.GetLogFromDBWithId(*server, alert.MetricName, logName, 0, 0)
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
