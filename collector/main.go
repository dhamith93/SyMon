package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/transport"
)

func main() {
	var removeAgentVal string
	var alertConfig []alerts.AlertConfig
	initPtr := flag.Bool("init", false, "Initialize the collector")
	flag.StringVar(&removeAgentVal, "remove-agent", "", "Remove agent info from collector DB. Agent monitor data is not deleted.")
	flag.Parse()

	config := config.GetCollector()
	if config.LogFileEnabled {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	if len(config.AlertsFilePath) > 0 {
		if _, err := os.Stat(config.AlertsFilePath); errors.Is(err, os.ErrNotExist) {
			logger.Log("cannot load alert config: ", err.Error())
		}
		alertConfig = alerts.GetAlertConfig(config.AlertsFilePath)
	}

	if *initPtr {
		initCollector(&config)
	} else if len(removeAgentVal) > 0 {
		removeAgent(removeAgentVal, &config)
	} else {

		mysql := getMySQLConnection(&config, false)
		defer mysql.Close()

		if alertConfig != nil {
			go handleAlerts(alertConfig, &config, &mysql)
		}

		go handleDataPurge(&config, &mysql)

		lis, err := net.Listen("tcp", ":"+config.Port)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		s := api.Server{}
		grpcServer, err := transport.NewServer(config.TLSEnabled, config.CertPath, config.KeyPath)
		if err != nil {
			log.Fatal(err)
		}

		api.RegisterMonitorDataServiceServer(grpcServer, &s)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %s", err)
		}
	}
}

func removeAgent(removeAgentVal string, config *config.Collector) {
	fmt.Println("Removing agent " + removeAgentVal)
	mysql := getMySQLConnection(config, false)
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

func initCollector(config *config.Collector) {
	mysql := getMySQLConnection(config, true)
	defer mysql.Close()
	err := mysql.Init()
	if err != nil {
		fmt.Println(err.Error())
	}
	key := auth.GetKey(true)
	os.Setenv("SYMON_KEY", key)
	fmt.Printf("---\nSYMON_KEY: %s\n---\n", key)
}

func getMySQLConnection(c *config.Collector, isMultiStatement bool) database.MySql {
	mysql := database.MySql{}
	mysql.Connect(c.MySQLUserName, c.MySQLPassword, c.MySQLHost, c.MySQLDatabaseName, isMultiStatement)
	return mysql
}
