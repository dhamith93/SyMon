package database

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"

	// importing go-sqlite3
	"github.com/dhamith93/SyMon/internal/fileops"
	"github.com/dhamith93/SyMon/internal/logger"
	_ "github.com/mattn/go-sqlite3"
)

// OpenDB opens a SQLite DB file
func OpenDB(db *sql.DB, path string) (*sql.DB, error) {
	if !fileops.IsFile(path) {
		return db, fmt.Errorf("cannot find db for agent")
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return db, err
	}
	return db, nil
}

// CreateDB create and return SQLite DB
func CreateDB(dbName string) (*sql.DB, error) {
	file, err := os.Create(dbName)
	if err != nil {
		return nil, err
	}
	file.Close()
	db, _ := sql.Open("sqlite3", dbName)
	return db, nil
}

// SaveLogToDB saves given monitor log to the sqlite DB
func SaveLogToDB(database *sql.DB, unixTime string, jsonStr string, logType string) error {
	stmt, err := database.Prepare("CREATE TABLE IF NOT EXISTS monitor_log (save_time TEXT, log_type TEXT, log_text TEXT)")

	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}
	defer stmt.Close()
	stmt.Exec()

	stmt, err = database.Prepare("INSERT INTO monitor_log (save_time, log_type, log_text) VALUES (?, ?, ?)")

	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}

	_, err = stmt.Exec(unixTime, logType, jsonStr)

	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

// AddAgent adds the agent to the given DB
func AddAgent(database *sql.DB, agentId string, path string) error {
	stmt, err := database.Prepare("CREATE TABLE IF NOT EXISTS agent (agent_id TEXT, db_path TEXT)")

	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}
	defer stmt.Close()
	stmt.Exec()

	stmt, err = database.Prepare("INSERT INTO agent (agent_id, db_path) VALUES (?, ?)")

	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}

	_, err = stmt.Exec(agentId, path)

	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}

	return nil
}

// RemoveAgent removes agent info
func RemoveAgent(database *sql.DB, agentId string) error {
	stmt, err := database.Prepare("DELETE FROM agent WHERE agent_id = ?")
	if err != nil {
		logger.Log("ERROR", err.Error())
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(agentId)
	return err
}

// AgenIDExists checks if agent is added to the db
func AgentIDExists(database *sql.DB, agentId string) bool {
	countStr := dbSelect(database, "SELECT COUNT(*) FROM agent WHERE agent_id = ?", agentId)[0]
	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		panic(err)
	}
	return count > 0
}

func GetAgents(database *sql.DB) []string {
	return dbSelect(database, "SELECT DISTINCT agent_id FROM agent WHERE agent_id != 'collector'")
}

func GetCustomMetricNames(database *sql.DB) []string {
	return dbSelect(database, "SELECT DISTINCT log_type FROM monitor_log WHERE log_type NOT IN ('system', 'memory', 'swap', 'disks', 'processor', 'procUsage', 'networks', 'services', 'processes', 'memoryUsage', 'CpuUsage')")
}

// GetDBPathForAgent returns the db path of the given agent
func GetDBPathForAgent(database *sql.DB, agentId string) string {
	if AgentIDExists(database, agentId) {
		return dbSelect(database, "SELECT db_path FROM agent WHERE agent_id = ?", agentId)[0]
	}
	return ""
}

// GetLogFromDBCount returns log records of the given log type
func GetLogFromDBCount(database *sql.DB, logType string, count int64) []string {
	return dbSelect(database, "SELECT log_text FROM monitor_log WHERE log_type = ? ORDER BY save_time DESC LIMIT ?", logType, count)
}

// GetLogFromDB returns log records of the given log type in the given time/range
func GetLogFromDB(database *sql.DB, logType string, from int64, to int64, time int64) []string {
	if from > 0 && to > 0 {
		return getLogFromDBInRange(database, logType, from, to)
	} else if time > 0 {
		return getLogFromDBAt(database, logType, time)
	} else {
		return GetLogFromDBCount(database, logType, 1)
	}
}

func getLogFromDBInRange(database *sql.DB, logType string, from int64, to int64) []string {
	query := "SELECT log_text FROM monitor_log WHERE log_type = ?"
	// diff := to - from

	// if diff > 21600 && diff <= 172800 {
	// 	query = query + " AND STRFTIME('%M', DATETIME(save_time, 'unixepoch')) IN ('01', '30')"
	// }

	// if diff > 172800 {
	// 	query = query + " AND STRFTIME('%H%M', DATETIME(save_time, 'unixepoch')) IN ('0001', '0601', '1201', '1801')"
	// }

	query = query + " AND save_time BETWEEN ? AND ? ORDER BY save_time"

	return dbSelect(database, query, logType, from, to)
}

func getLogFromDBAt(database *sql.DB, logType string, time int64) []string {
	return dbSelect(database, "SELECT log_text FROM monitor_log WHERE log_type = ? AND save_time = ?", logType, time)
}

func dbSelect(database *sql.DB, query string, args ...interface{}) []string {
	rows, err := database.Query(query, args...)

	if err != nil {
		handleError(err)
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

func handleError(err error) {
	logger.Log("ERROR", err.Error())
}
