package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/dhamith93/SyMon/collector/internal/config"
	"github.com/dhamith93/SyMon/collector/internal/server"
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
	flag.StringVar(&removeAgentVal, "remove-agent", "", "Remove agent info from collector DB. Agent monitor data is not deleted.")
	flag.Parse()

	if *initPtr {
		initCollector(&config)
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
	mysql := getMySQLConnection(&config)
	defer mysql.Close()

	if mysql.SqlErr != nil {
		fmt.Println(mysql.SqlErr.Error())
		return
	}

	if !mysql.AgentIDExists(removeAgentVal) {
		fmt.Println("Agent ID " + removeAgentVal + " doesn't exists...")
		return
	}

	err := mysql.RemoveAgent(removeAgentVal)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func initCollector(config *config.Config) {
	mysql := getMySQLConnection(config)
	defer mysql.Close()
	err := mysql.Init()
	if err != nil {
		fmt.Println(err.Error())
	}
}

func getMySQLConnection(c *config.Config) database.MySql {
	mysql := database.MySql{}
	password := os.Getenv("SYMON_MYSQL_PSWD")
	mysql.Connect(c.MySQLUserName, password, c.MySQLHost, c.MySQLDatabaseName, false)
	return mysql
}
