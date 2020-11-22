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

// GetLogFromDB returns log records of the given log type
func GetLogFromDB(logType string, count int) []string {
	database, _ := sql.Open("sqlite3", GetConfig().SQLiteDBPath)
	rows, _ := database.Query("SELECT log_text FROM monitor_log WHERE log_type = ? ORDER BY save_time DESC LIMIT ?", logType, count)
	out := []string{}

	for rows.Next() {
		var logText string
		rows.Scan(&logText)
		out = append(out, logText)
	}

	return out
}

func handleError(err error) {
	Log("ERROR", err.Error())
	panic(err)
}
