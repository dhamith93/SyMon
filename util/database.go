package util

import (
	"database/sql"

	// importing go-sqlite3
	_ "github.com/mattn/go-sqlite3"
)

// SaveLogToDB saves given monitor log to the sqlite DB
func SaveLogToDB(unixTime string, jsonStr string, logType string) {
	if GetConfig().SQLiteDBLoggingEnabled {
		database, _ := sql.Open("sqlite3", GetConfig().SQLiteDBPath)
		stmt, err := database.Prepare("CREATE TABLE IF NOT EXISTS monitor_log (save_time TEXT, log_type TEXT, log_text TEXT)")

		if err != nil {
			handleError(err)
		}

		stmt.Exec()

		stmt, err = database.Prepare("INSERT INTO monitor_log (save_time, log_type, log_text) VALUES (?, ?, ?)")

		if err != nil {
			handleError(err)
		}

		stmt.Exec(unixTime, logType, jsonStr)
	} else {
		Log("Error", "DB path is not specified")
	}
}

// GetLogFromDBCount returns log records of the given log type
func GetLogFromDBCount(logType string, count int) []string {
	return dbSelect("SELECT log_text FROM monitor_log WHERE log_type = ? ORDER BY save_time DESC LIMIT ?", logType, count)
}

// GetLogFromDB returns log records of the given log type in the given time/range
func GetLogFromDB(logType string, from int64, to int64, time int64) []string {
	if from > 0 && to > 0 {
		return getLogFromDBInRange(logType, from, to)
	} else if time > 0 {
		return getLogFromDBAt(logType, time)
	} else {
		return GetLogFromDBCount(logType, 1)
	}
}

func getLogFromDBInRange(logType string, from int64, to int64) []string {
	query := "SELECT log_text FROM monitor_log WHERE log_type = ?"
	diff := to - from

	if diff > 21600 && diff <= 172800 {
		query = query + " AND STRFTIME('%M', DATETIME(save_time, 'unixepoch')) IN ('01', '30')"
	}

	if diff > 172800 {
		query = query + " AND STRFTIME('%H%M', DATETIME(save_time, 'unixepoch')) IN ('0001', '0601', '1201', '1801')"
	}

	query = query + " AND save_time BETWEEN ? AND ? ORDER BY save_time"

	return dbSelect(query, logType, from, to)
}

func getLogFromDBAt(logType string, time int64) []string {
	return dbSelect("SELECT log_text FROM monitor_log WHERE log_type = ? AND save_time = ?", logType, time)
}

func dbSelect(query string, args ...interface{}) []string {
	database, err := sql.Open("sqlite3", GetConfig().SQLiteDBPath)
	if err != nil {
		handleError(err)
	}

	rows, err := database.Query(query, args...)
	if err != nil {
		handleError(err)
	}

	out := []string{}

	for rows.Next() {
		var logText string
		rows.Scan(&logText)
		out = append(out, logText)
	}

	defer database.Close()
	return out
}

func handleError(err error) {
	Log("ERROR", err.Error())
	panic(err)
}
