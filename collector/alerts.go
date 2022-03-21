package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/collector/internal/config"
	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/pkg/memdb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func handleAlerts(alertConfigs []alerts.AlertConfig, config *config.Config, mysql *database.MySql) {
	ticker := time.NewTicker(15 * time.Second)
	quit := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		incidentTracker := memdb.CreateDatabase("incident_tracker")
		err := incidentTracker.Create(
			"alert",
			memdb.Col{Name: "server_name", Type: memdb.String},
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

func processAlert(alert *alerts.AlertConfig, server string, config *config.Config, mysql *database.MySql, incidentTracker *memdb.Database) {
	alertStatus := buildAlertStatus(alert, &server, config, mysql)
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
	}, alertStatus)

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
			res := incidentTracker.Tables["alert"].Where("server_name", "==", server).And("metric_name", "==", alertStatus.Alert.MetricName).And("status", "==", int(alertstatus.Normal))
			if res.RowCount == 0 {
				err := incidentTracker.Tables["alert"].Insert("server_name, metric_name, time, value, status", server, alertStatus.Alert.MetricName, alertStatus.UnixTime, alertStatus.Value, int(alertstatus.Normal))
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
				sendAlert(alertToSend, config)
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
			sendAlert(alertToSend, config)
		}
		return
	}

	if alertStatus.Type == alertstatus.Warning || alertStatus.Type == alertstatus.Critical {
		res := incidentTracker.Tables["alert"].Where("server_name", "==", server).And("metric_name", "==", alertStatus.Alert.MetricName).And("status", "!=", int(alertstatus.Normal))

		if res.RowCount == 0 {
			err := incidentTracker.Tables["alert"].Insert("server_name, metric_name, time, value, status", server, alertStatus.Alert.MetricName, alertStatus.UnixTime, alertStatus.Value, int(alertStatus.Type))
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
			sendAlert(alertToSend, config)
			if err != nil {
				logger.Log("error", "Error adding alert: "+err.Error())
			}
			res.Delete()
		}
	}
}

func buildAlertStatus(alert *alerts.AlertConfig, server *string, config *config.Config, mysql *database.MySql) alertstatus.AlertStatus {
	var alertStatus alertstatus.AlertStatus

	metricLogs := mysql.GetLogFromDBWithId(*server, alert.MetricName, 0, 0)
	logId := metricLogs[0]
	alertStatus.Alert = *alert
	alertStatus.Server = *server
	alertStatus.Type = alertstatus.Normal

	switch alert.MetricName {
	case "procUsage", "memory", "swap":
		var v []string
		err := json.Unmarshal([]byte(metricLogs[1]), &v)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}
		alertStatus.UnixTime = v[0]
		valStr := strings.Replace(v[1], "%", "", -1)
		val, err := strconv.ParseFloat(valStr, 32)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}

		alertStatus.Value = float32(val)
		alertStatus.Type = getAlertType(alert, val)
	case "disks":
		var diskLog monitor.Disk
		err := json.Unmarshal([]byte(metricLogs[1]), &diskLog)
		if err != nil {
			logger.Log("error", err.Error())
			return alertStatus
		}

		alertStatus.UnixTime = diskLog.Time
		for _, disk := range diskLog.Disks {
			if disk[0] == alert.Disk {
				valStr := strings.Replace(disk[6], "%", "", -1)
				val, err := strconv.ParseFloat(valStr, 32)
				if err != nil {
					logger.Log("error", err.Error())
					return alertStatus
				}
				alertStatus.Value = float32(val)
				alertStatus.Type = getAlertType(alert, val)
			}
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
	}
	return alertstatus.Normal
}

func buildAlert(alert alerts.Alert, status alertstatus.AlertStatus) *alertapi.Alert {
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
		"desc",
		"{value}",
		value,
	)
	content := replacer.Replace(alert.Template)

	if status.Type == alertstatus.Normal {
		content = "\n------------\n" + "Alert is resolved at : " + timestamp.UTC().String() + "\n------------\n"
	}

	return &alertapi.Alert{
		ServerName: alert.ServerName,
		MetricName: alert.MetricName,
		LogId:      status.StartEvent,
		Status:     int32(status.Type),
		Subject:    subject,
		Content:    content,
		Timestamp:  timestamp.UTC().String(),
		Resolved:   (status.Type == alertstatus.Normal),
	}
}

func sendAlert(alert *alertapi.Alert, config *config.Config) {
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

func createClient(config *config.Config) (*grpc.ClientConn, alertapi.AlertServiceClient, context.Context, context.CancelFunc) {
	conn, err := grpc.Dial("localhost:5999", grpc.WithInsecure())
	if err != nil {
		logger.Log("error", "connection error: "+err.Error())
		return nil, nil, nil, nil
	}
	c := alertapi.NewAlertServiceClient(conn)
	token := generateToken()
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{"jwt": token})), time.Second*10)
	return conn, c, ctx, cancel
}