package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/dhamith93/SyMon/internal/database"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	user := flag.String("user", "", "username of the database")
	password := flag.String("password", "", "password of the database user")
	host := flag.String("host", "", "host and port ex. 127.0.0.1:3306")
	dbName := flag.String("database", "", "database name")
	sqlitePath := flag.String("sqlite-path", "", "sqlite db path")
	serverName := flag.String("server-name", "", "server name")

	flag.Parse()

	if len(*user) == 0 || len(*password) == 0 || len(*host) == 0 || len(*dbName) == 0 || len(*sqlitePath) == 0 || len(*serverName) == 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	var sqlite *sql.DB
	var err error

	sqlite, err = database.OpenDB(sqlite, *sqlitePath)
	if err != nil {
		if err != nil {
			log.Fatalf("error %v", err)
		}
	}
	defer sqlite.Close()

	sqliteData, err := sqliteSelect(sqlite, "SELECT * FROM monitor_log")
	if err != nil {
		log.Fatalf("error %v", err)
	}

	mysql := database.MySql{}
	mysql.Connect(*user, *password, *host, *dbName, false)
	defer mysql.Close()

	for _, row := range sqliteData {
		err := mysql.SaveLogToDB(*serverName, row[0], row[2], row[1], "")
		if err != nil {
			log.Fatalf("error %v", err)
		}
	}
}

func sqliteSelect(database *sql.DB, query string, args ...interface{}) ([][]string, error) {
	output := make([][]string, 0)
	row, err := database.Query(query, args...)
	if err != nil {
		return output, err
	}
	defer row.Close()

	columns, err := row.Columns()
	if err != nil {
		return output, err
	}

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

	return output, nil
}
