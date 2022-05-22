package database

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/dhamith93/SyMon/internal/alertstatus"
	"github.com/dhamith93/SyMon/internal/fileops"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	_ "github.com/go-sql-driver/mysql"
)

type MySql struct {
	DB        *sql.DB
	SqlErr    error
	Connected bool
	User      string
	Host      string
	Database  string
}

func (mysql *MySql) Connect(user string, password string, host string, database string, isMultiStatement bool) {
	mysql.User = user
	mysql.Host = host
	mysql.Database = database
	connStr := user + ":" + password + "@" + "tcp(" + host + ")/" + database
	if isMultiStatement {
		connStr += "?multiStatements=true"
	}
	mysql.DB, mysql.SqlErr = sql.Open("mysql", connStr)
	mysql.SqlErr = mysql.DB.Ping()
	if mysql.SqlErr != nil {
		mysql.Connected = false
		logger.Log("error", "cannot connect to mysql database "+mysql.SqlErr.Error())
		return
	}
	mysql.Connected = true
}

func (mysql *MySql) Init() error {
	q := fileops.ReadFile("init.sql")
	_, err := mysql.DB.Exec(q)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

func (mysql *MySql) Close() {
	mysql.DB.Close()
}

func (mysql *MySql) Select(query string, args ...interface{}) (Table, error) {
	table := Table{}
	row, err := mysql.DB.Query(query, args...)
	mysql.SqlErr = err
	if err != nil {
		return table, err
	}
	defer row.Close()

	columns, err := row.Columns()
	mysql.SqlErr = err
	if err != nil {
		return table, err
	}

	output := make([][]string, 0)
	rawResult := make([][]byte, len(columns))
	dest := make([]interface{}, len(columns))
	for i := range rawResult {
		dest[i] = &rawResult[i]
	}

	for row.Next() {
		row.Scan(dest...)
		res := make([]string, 0)
		for _, raw := range rawResult {
			if raw != nil {
				res = append(res, string(raw))
			}
		}
		output = append(output, res)
	}

	table.Headers = columns
	table.Data = output
	return table, mysql.SqlErr
}

func (mysql *MySql) monitorDataSelect(query string, args ...interface{}) []string {
	rows, err := mysql.DB.Query(query, args...)
	mysql.SqlErr = err

	if err != nil {
		return nil
	}

	defer rows.Close()

	out := []string{}

	for rows.Next() {
		var logText string
		rows.Scan(&logText)
		out = append(out, logText)
	}

	return out
}

func (mysql *MySql) SaveLogToDB(serverName string, unixTime string, jsonStr string, logType string, logName string) error {
	serverId := mysql.getServerId(serverName)
	if len(serverId) == 0 {
		err := fmt.Errorf("server %s not registered", serverName)
		logger.Log("ERROR", err.Error())
		return err
	}

	stmt, err := mysql.DB.Prepare("INSERT INTO monitor_log (server_id, log_time, log_type, log_name, log_text) VALUES (?, ?, ?, ?, ?)")

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	_, err = stmt.Exec(serverId, unixTime, logType, logName, jsonStr)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

func (mysql *MySql) AddAgent(agentId string, timezone string) error {
	stmt, err := mysql.DB.Prepare("INSERT INTO server (name, timezone) VALUES (?, ?)")

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	_, err = stmt.Exec(agentId, timezone)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

func (mysql *MySql) RemoveAgent(agentId string) error {
	stmt, err := mysql.DB.Prepare("DELETE FROM server WHERE name = ?")
	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(agentId)
	return err
}

func (mysql *MySql) AgentIDExists(agentId string) bool {
	countStr := mysql.monitorDataSelect("SELECT COUNT(*) FROM server WHERE name = ?", agentId)[0]
	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		panic(err)
	}
	return count > 0
}

func (mysql *MySql) getServerId(serverName string) string {
	res := mysql.monitorDataSelect("SELECT id FROM server WHERE name = ?", serverName)
	if len(res) == 0 {
		return ""
	}
	return res[0]
}

func (mysql *MySql) GetAgents() []string {
	return mysql.monitorDataSelect("SELECT DISTINCT name FROM server WHERE name != 'collector'")
}

func (mysql *MySql) GetCustomMetricNames(serverName string) []string {
	serverId := mysql.getServerId(serverName)
	return mysql.monitorDataSelect("SELECT DISTINCT log_type FROM monitor_log WHERE server_id = ? AND log_type NOT IN ('system', 'memory', 'swap', 'disks', 'processor', 'procUsage', 'networks', 'services', 'processes', 'memoryUsage', 'CpuUsage')", serverId)
}

func (mysql *MySql) GetLogFromDBCount(serverId string, logType string, count int64) []string {
	if logType == monitor.DISKS || logType == monitor.NETWORKS || logType == monitor.SERVICES {
		return mysql.monitorDataSelect("SELECT JSON_ARRAYAGG(log_text ORDER BY id) FROM monitor_log WHERE server_id = ? AND log_type = ? GROUP BY log_time ORDER BY log_time DESC LIMIT ?", serverId, logType, count)
	}
	return mysql.monitorDataSelect("SELECT log_text FROM monitor_log WHERE server_id = ? AND log_type = ? ORDER BY log_time DESC LIMIT ?", serverId, logType, count)
}

func (mysql *MySql) GetLogFromDB(serverName string, logType string, from int64, to int64, time int64) []string {
	serverId := mysql.getServerId(serverName)
	if from > 0 && to > 0 {
		return mysql.getLogFromDBInRange(serverId, logType, from, to)
	} else if time > 0 {
		return mysql.getLogFromDBAt(serverId, logType, time)
	} else {
		return mysql.GetLogFromDBCount(serverId, logType, 1)
	}
}

func (mysql *MySql) GetLogFromDBWithId(serverName string, logType string, logName string, from int64, to int64) [][]string {
	serverId := mysql.getServerId(serverName)
	if from > 0 && to > 0 {
		q := "SELECT id, log_text FROM monitor_log WHERE server_id = ? AND log_type = ?  AND log_name = ? AND log_time BETWEEN ? AND ? ORDER BY log_time"
		res, _ := mysql.Select(q, serverId, logType, logName, from, to)
		return res.Data
	} else {
		q := "SELECT id, log_text FROM monitor_log WHERE server_id = ? AND log_type = ? AND log_name = ? ORDER BY log_time DESC LIMIT 1"
		res, _ := mysql.Select(q, serverId, logType, logName)
		return res.Data
	}
}

func (mysql *MySql) getLogFromDBInRange(serverId string, logType string, from int64, to int64) []string {
	query := "SELECT log_text FROM monitor_log WHERE server_id = ? AND log_type = ? AND log_time BETWEEN ? AND ? ORDER BY log_time"
	// diff := to - from

	// if diff > 21600 && diff <= 172800 {
	// 	query = query + " AND STRFTIME('%M', DATETIME(log_time, 'unixepoch')) IN ('01', '30')"
	// }

	// if diff > 172800 {
	// 	query = query + " AND STRFTIME('%H%M', DATETIME(log_time, 'unixepoch')) IN ('0001', '0601', '1201', '1801')"
	// }

	if logType == monitor.DISKS || logType == monitor.NETWORKS || logType == monitor.SERVICES {
		query = "SELECT JSON_ARRAYAGG(log_text ORDER BY id) FROM monitor_log WHERE server_id = ? AND log_type = ? AND log_time BETWEEN ? AND ? GROUP BY log_time ORDER BY log_time"
	}

	return mysql.monitorDataSelect(query, serverId, logType, from, to)
}

func (mysql *MySql) getLogFromDBAt(serverId string, logType string, time int64) []string {
	return mysql.monitorDataSelect("SELECT log_text FROM monitor_log WHERE server_id = ? AND log_type = ? AND log_time = ?", serverId, logType, time)
}

func (mysql *MySql) GetAlertByStartEvent(logId string) []string {
	t, err := mysql.Select("SELECT * FROM alert WHERE start_log_id = ?", logId)
	if err != nil {
		logger.Log("error", err.Error())
	}
	if len(t.Data) == 0 {
		return nil
	}
	return t.Data[0]
}

func (mysql *MySql) AddAlert(alertStatus *alertstatus.AlertStatus) error {
	serverId := mysql.getServerId(alertStatus.Server)

	if len(serverId) == 0 {
		err := fmt.Errorf("server %s not registered", alertStatus.Server)
		logger.Log("ERROR", err.Error())
		return err
	}

	stmt, err := mysql.DB.Prepare("INSERT INTO alert (server_id, type, expected, actual, time, start_log_id) VALUES (?, ?, ?, ?, ?, ?);")

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	expectedValue := alertStatus.Alert.WarnThreshold

	if alertStatus.Type == alertstatus.Critical {
		expectedValue = alertStatus.Alert.CriticalThreshold
	}

	_, err = stmt.Exec(serverId, alertStatus.Type, expectedValue, alertStatus.Value, alertStatus.UnixTime, alertStatus.StartEvent)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

func (mysql *MySql) SetAlertEndLog(alertStatus *alertstatus.AlertStatus, startEventId string) error {
	serverId := mysql.getServerId(alertStatus.Server)

	if len(serverId) == 0 {
		err := fmt.Errorf("server %s not registered", alertStatus.Server)
		logger.Log("ERROR", err.Error())
		return err
	}

	stmt, err := mysql.DB.Prepare("UPDATE alert SET end_log_id = ? WHERE server_id = ? AND start_log_id = ?;")

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	_, err = stmt.Exec(alertStatus.StartEvent, serverId, startEventId)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

func (mysql *MySql) UpdateAlert(alertStatus *alertstatus.AlertStatus, startEventId string) error {
	serverId := mysql.getServerId(alertStatus.Server)

	if len(serverId) == 0 {
		err := fmt.Errorf("server %s not registered", alertStatus.Server)
		logger.Log("ERROR", err.Error())
		return err
	}

	stmt, err := mysql.DB.Prepare("UPDATE alert SET type = ?, expected = ?, actual = ? WHERE server_id = ? AND start_log_id = ?;")

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	expectedValue := alertStatus.Alert.WarnThreshold

	if alertStatus.Type == alertstatus.Critical {
		expectedValue = alertStatus.Alert.CriticalThreshold
	}

	_, err = stmt.Exec(alertStatus.Type, expectedValue, alertStatus.Value, serverId, startEventId)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

func (mysql *MySql) GetPreviousOpenAlert(alertStatus *alertstatus.AlertStatus) []string {
	serverId := mysql.getServerId(alertStatus.Server)
	q := "SELECT * FROM alert AS a JOIN monitor_log AS m ON a.start_log_id = m.id WHERE a.end_log_id IS NULL AND a.time < ? AND a.server_id = ? AND m.log_type = ?"

	if alertStatus.Alert.MetricName == monitor.DISKS || alertStatus.Alert.MetricName == monitor.NETWORKS || alertStatus.Alert.MetricName == monitor.SERVICES {
		q += " AND m.log_name = ?"
	}

	var t Table
	var err error

	switch alertStatus.Alert.MetricName {
	case monitor.DISKS:
		t, err = mysql.Select(q, alertStatus.UnixTime, serverId, alertStatus.Alert.MetricName, alertStatus.Alert.Disk)
	case monitor.SERVICES:
		t, err = mysql.Select(q, alertStatus.UnixTime, serverId, alertStatus.Alert.MetricName, alertStatus.Alert.Service)
	default:
		t, err = mysql.Select(q, alertStatus.UnixTime, serverId, alertStatus.Alert.MetricName)
	}

	if err != nil {
		logger.Log("error", "GetPreviousOpenAlert"+err.Error())
	}
	if len(t.Data) == 0 {
		return nil
	}
	return t.Data[0]
}

func (mysql *MySql) ClearAllAlertsWithNullEnd() error {
	q := "UPDATE alert SET end_log_id = 0 WHERE end_log_id IS NULL"

	stmt, err := mysql.DB.Prepare(q)

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	_, err = stmt.Exec()

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}
