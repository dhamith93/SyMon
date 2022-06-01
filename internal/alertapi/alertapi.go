package alertapi

import (
	context "context"

	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/email"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/pagerduty"
	"github.com/dhamith93/SyMon/internal/slack"
	"github.com/dhamith93/SyMon/pkg/memdb"
)

type Server struct {
	Database *memdb.Database
}

func (s *Server) HandleAlerts(ctx context.Context, in *Alert) (*Response, error) {
	metricName := ""
	sendPagerDuty := in.Pagerduty
	sendEmail := in.Email
	sendSlack := in.Slack

	if in.MetricName == monitor.DISKS {
		metricName = in.Disk
	}
	if in.MetricName == monitor.SERVICES {
		metricName = in.Service
	}
	res := s.Database.Tables["alert"].Where("server_name", "==", in.ServerName).And("metric_type", "==", in.MetricName).And("metric_name", "==", metricName).And("resolved", "==", false)

	if res.RowCount == 0 {
		err := s.Database.Tables["alert"].Insert(
			"server_name, metric_type, metric_name, log_id, subject, content, status, timestamp, resolved, pg_incident_id, slack_msg_ts",
			in.ServerName,
			in.MetricName,
			metricName,
			in.LogId,
			in.Subject,
			in.Content,
			int(in.Status),
			in.Timestamp,
			in.Resolved,
			"",
			"",
		)

		if err != nil {
			logger.Log("error", "notification_tracker: "+err.Error())
		}
	} else {
		if !res.Rows[0].Columns["resolved"].BoolVal && (in.Status != int32(alertstatus.Warning) && in.Status != int32(alertstatus.Critical)) {
			in.Content += "\n" + res.Rows[0].Columns["content"].StringVal
			res.Update("resolved", true)
			if res.Rows[0].Columns["pg_incident_id"].StringVal != "" {
				err := pagerduty.UpdateIncident(res.Rows[0].Columns["pg_incident_id"].StringVal)
				if err != nil {
					logger.Log("error", err.Error())
				}
				sendPagerDuty = false
			}
			if res.Rows[0].Columns["slack_msg_ts"].StringVal != "" {
				msgTs := res.Rows[0].Columns["slack_msg_ts"].StringVal
				_, err := slack.SendSlackMessage(in.Subject, in.Content, in.SlackChannel, true, msgTs)
				if err != nil {
					logger.Log("error", err.Error())
				}
				sendSlack = false
			}
		}

		if in.Status != int32(res.Rows[0].Columns["status"].IntVal) && (in.Status == int32(alertstatus.Warning) || in.Status == int32(alertstatus.Critical)) {
			res.Update("status", int(in.Status))
			res.Update("subject", in.Subject)
			res.Update("content", in.Content)

			if res.Rows[0].Columns["slack_msg_ts"].StringVal != "" {
				msgTs := res.Rows[0].Columns["slack_msg_ts"].StringVal
				_, err := slack.SendSlackMessage(in.Subject, in.Content, in.SlackChannel, false, msgTs)
				if err != nil {
					logger.Log("error", err.Error())
				}
				sendSlack = false
			}
		}
	}

	if sendEmail {
		err := email.SendEmail(in.Subject, in.Content)
		if err != nil {
			logger.Log("error", err.Error())
		}
	}
	if sendPagerDuty {
		id, err := pagerduty.CreateIncident(createIncident(in.Subject, in.Content))
		if err != nil {
			logger.Log("error", err.Error())
		}
		res = s.Database.Tables["alert"].Where("server_name", "==", in.ServerName).And("metric_type", "==", in.MetricName).And("metric_name", "==", metricName).And("resolved", "==", false)
		res.Update("pg_incident_id", id)
	}
	if sendSlack {
		msgTs, err := slack.SendSlackMessage(in.Subject, in.Content, in.SlackChannel, false, "")
		if err != nil {
			logger.Log("error", err.Error())
		}
		res = s.Database.Tables["alert"].Where("server_name", "==", in.ServerName).And("metric_type", "==", in.MetricName).And("metric_name", "==", metricName).And("resolved", "==", false)
		res.Update("slack_msg_ts", msgTs)
	}

	return &Response{Success: true, Msg: "alert processed"}, nil
}

func (s *Server) AlertRequest(ctx context.Context, in *Request) (*AlertArray, error) {
	alerts := AlertArray{}
	res := s.Database.Tables["alert"].Where("server_name", "==", in.ServerName)

	for _, row := range res.Rows {
		alerts.Alerts = append(alerts.Alerts, &Alert{
			ServerName: row.Columns["server_name"].StringVal,
			MetricName: row.Columns["metric_type"].StringVal,
			Subject:    row.Columns["subject"].StringVal,
			Content:    row.Columns["content"].StringVal,
			Timestamp:  row.Columns["timestamp"].StringVal,
			Resolved:   row.Columns["resolved"].BoolVal,
			Disk:       row.Columns["metric_name"].StringVal,
			Service:    row.Columns["metric_name"].StringVal,
		})
	}

	return &alerts, nil
}

func createIncident(subject string, content string) pagerduty.Incident {
	incident := pagerduty.Incident{}
	incident.Incident.Type = "incident"
	incident.Incident.Urgency = "high"
	incident.Incident.Body.Type = "incident_body"
	incident.Incident.Service.Type = "service_reference"
	incident.Incident.Title = subject
	incident.Incident.Body.Details = content
	return incident
}
