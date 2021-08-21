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
	if config.GetConfig("config.json").LogFileEnabled {
		file, err := os.OpenFile(config.GetConfig("config.json").LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	var initAgentVal string
	initPtr := flag.Bool("init", false, "Initialize the collector")
	flag.StringVar(&initAgentVal, "init-agent", "", "Register agent")
	flag.Parse()

	config := config.GetConfig("config.json")

	if *initPtr {
		initCollector(config)
	} else if len(initAgentVal) > 0 {
		initAgent(initAgentVal, config)
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

func initAgent(initAgentVal string, config config.Config) {
	fmt.Println("Initializing agent for " + initAgentVal)
	path := config.SQLiteDBPath + "/" + initAgentVal + ".db"

	var collectorDB *sql.DB
	var collectorErr error
	collectorDB, collectorErr = database.OpenDB(collectorDB, config.SQLiteDBPath+"/collector.db")

	if collectorErr != nil {
		fmt.Println(collectorErr.Error())
	} else {
		defer collectorDB.Close()
		if !database.AgentIDExists(collectorDB, initAgentVal) {
			_, err := database.CreateDB(path)
			if err != nil {
				fmt.Println(err.Error())
			} else {
				err := database.AddAgent(collectorDB, initAgentVal, path)
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		} else {
			fmt.Println("Agent ID " + initAgentVal + " exists...")
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
