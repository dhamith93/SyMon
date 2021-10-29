package database

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"github.com/dhamith93/SyMon/internal/fileops"
	"github.com/dhamith93/SyMon/internal/logger"
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
		log.Fatalf("cannot connect to mysql database %v", mysql.SqlErr)
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

func (mysql *MySql) SaveLogToDB(serverName string, unixTime string, jsonStr string, logType string) error {
	serverId := mysql.getServerId(serverName)
	if len(serverId) == 0 {
		err := fmt.Errorf("server %s not registered", serverName)
		logger.Log("ERROR", err.Error())
		return err
	}

	stmt, err := mysql.DB.Prepare("INSERT INTO monitor_log (server_id, log_time, log_type, log_text) VALUES (?, ?, ?, ?)")

	if err != nil {
		mysql.SqlErr = err
		logger.Log("ERROR", err.Error())
		return err
	}

	defer stmt.Close()

	_, err = stmt.Exec(serverId, unixTime, logType, jsonStr)

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

func (mysql *MySql) GetCustomMetricNames() []string {
	return mysql.monitorDataSelect("SELECT DISTINCT log_type FROM monitor_log WHERE log_type NOT IN ('system', 'memory', 'swap', 'disks', 'processor', 'procUsage', 'networks', 'services', 'processes', 'memoryUsage', 'CpuUsage')")
}

func (mysql *MySql) GetLogFromDBCount(serverId string, logType string, count int64) []string {
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

func (mysql *MySql) getLogFromDBInRange(serverId string, logType string, from int64, to int64) []string {
	query := "SELECT log_text FROM monitor_log WHERE server_id = ? AND log_type = ?"
	// diff := to - from

	// if diff > 21600 && diff <= 172800 {
	// 	query = query + " AND STRFTIME('%M', DATETIME(log_time, 'unixepoch')) IN ('01', '30')"
	// }

	// if diff > 172800 {
	// 	query = query + " AND STRFTIME('%H%M', DATETIME(log_time, 'unixepoch')) IN ('0001', '0601', '1201', '1801')"
	// }

	query = query + " AND log_time BETWEEN ? AND ? ORDER BY log_time"

	return mysql.monitorDataSelect(query, serverId, logType, from, to)
}

func (mysql *MySql) getLogFromDBAt(serverId string, logType string, time int64) []string {
	return mysql.monitorDataSelect("SELECT log_text FROM monitor_log WHERE server_id = ? AND log_type = ? AND log_time = ?", serverId, logType, time)
}
