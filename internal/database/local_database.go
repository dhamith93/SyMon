package database

import (
	"database/sql"
	"os"

	// importing go-sqlite3
	"github.com/dhamith93/SyMon/internal/logger"
	_ "github.com/mattn/go-sqlite3"
)

// OpenDB opens a SQLite DB file
func OpenDB(db *sql.DB, path string) (*sql.DB, error) {
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
	diff := to - from

	if diff > 21600 && diff <= 172800 {
		query = query + " AND STRFTIME('%M', DATETIME(save_time, 'unixepoch')) IN ('01', '30')"
	}

	if diff > 172800 {
		query = query + " AND STRFTIME('%H%M', DATETIME(save_time, 'unixepoch')) IN ('0001', '0601', '1201', '1801')"
	}

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

	defer database.Close()
	return out
}

func handleError(err error) {
	logger.Log("ERROR", err.Error())
}
