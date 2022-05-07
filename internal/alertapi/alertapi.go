package alertapi

import (
	context "context"

	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/email"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/pkg/memdb"
)

type Server struct {
	Database *memdb.Database
}

func (s *Server) HandleAlerts(ctx context.Context, in *Alert) (*Response, error) {
	res := s.Database.Tables["alert"].Where("server_name", "==", in.ServerName).And("metric_name", "==", in.MetricName).And("resolved", "==", false)

	if res.RowCount == 0 {
		err := s.Database.Tables["alert"].Insert(
			"server_name, metric_name, log_id, subject, content, status, timestamp, resolved",
			in.ServerName,
			in.MetricName,
			in.LogId,
			in.Subject,
			in.Content,
			int(in.Status),
			in.Timestamp,
			in.Resolved,
		)

		if err != nil {
			logger.Log("error", "notification_tracker: "+err.Error())
		}
	} else {
		if !res.Rows[0].Columns["resolved"].BoolVal && (in.Status != int32(alertstatus.Warning) && in.Status != int32(alertstatus.Critical)) {
			in.Content += "\n" + res.Rows[0].Columns["content"].StringVal
			res.Update("resolved", true)
		}

		if in.Status != int32(res.Rows[0].Columns["status"].IntVal) && (in.Status == int32(alertstatus.Warning) || in.Status == int32(alertstatus.Critical)) {
			res.Update("status", int(in.Status))
			res.Update("subject", in.Subject)
			res.Update("content", in.Content)
		}
	}

	// send notification
	// fmt.Println(in)
	err := email.SendEmail(in.Subject, in.Content)
	if err != nil {
		logger.Log("err", err.Error())
	}
	return &Response{Success: true, Msg: "alert processed"}, nil
}

func (s *Server) AlertRequest(ctx context.Context, in *Request) (*AlertArray, error) {
	alerts := AlertArray{}
	res := s.Database.Tables["alert"].Where("server_name", "==", in.ServerName).And("resolved", "==", false)

	for _, row := range res.Rows {
		alerts.Alerts = append(alerts.Alerts, &Alert{
			ServerName: row.Columns["server_name"].StringVal,
			MetricName: row.Columns["metric_name"].StringVal,
			Subject:    row.Columns["subject"].StringVal,
			Content:    row.Columns["content"].StringVal,
			Timestamp:  row.Columns["timestamp"].StringVal,
			Resolved:   row.Columns["resolved"].BoolVal,
		})
	}

	return &alerts, nil
}
