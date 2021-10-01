package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/dhamith93/SyMon/collector/internal/server"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
)

func main() {
	config := config.GetConfig("config.json")
	if config.LogFileEnabled {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	var removeAgentVal string
	initPtr := flag.Bool("init", false, "Initialize the collector")
	flag.StringVar(&removeAgentVal, "remove-agent", "", "Remove agent info from collector DB. Agent DB with monitor data is not deleted.")
	flag.Parse()

	if *initPtr {
		initCollector(config)
	} else if len(removeAgentVal) > 0 {
		removeAgent(removeAgentVal, config)
	} else {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			server.Run(":" + config.Port)
			wg.Done()
		}()
		wg.Wait()
	}

}

func removeAgent(removeAgentVal string, config config.Config) {
	fmt.Println("Removing agent " + removeAgentVal)
	var collectorDB *sql.DB
	var collectorErr error
	collectorDB, collectorErr = database.OpenDB(collectorDB, config.SQLiteDBPath+"/collector.db")
	if collectorErr != nil {
		fmt.Println(collectorErr.Error())
	} else {
		defer collectorDB.Close()
		if database.AgentIDExists(collectorDB, removeAgentVal) {
			err := database.RemoveAgent(collectorDB, removeAgentVal)
			if err != nil {
				fmt.Println(err.Error())
			}
		} else {
			fmt.Println("Agent ID " + removeAgentVal + " doesn't exists...")
		}
	}
}

func initCollector(config config.Config) {
	path := config.SQLiteDBPath + "/collector.db"
	db, err := database.CreateDB(path)
	if err != nil {
		fmt.Println(err.Error())
	}
	database.AddAgent(db, "collector", path)
	defer db.Close()
	fmt.Println("Generated Key: " + auth.GetKey())
}
