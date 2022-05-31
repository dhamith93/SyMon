package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/dhamith93/SyMon/collector/internal/config"
	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func main() {
	var removeAgentVal, configPath, alertConfigPath string
	var alertConfig []alerts.AlertConfig
	initPtr := flag.Bool("init", false, "Initialize the collector")
	flag.StringVar(&removeAgentVal, "remove-agent", "", "Remove agent info from collector DB. Agent monitor data is not deleted.")
	flag.StringVar(&configPath, "config", "", "Path to config.json.")
	flag.StringVar(&alertConfigPath, "alerts", "", "Path to alerts json file.")
	flag.Parse()

	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("cannot load config.json: %v", err)
	}

	config := config.GetConfig(configPath)
	if config.LogFileEnabled {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	err := godotenv.Load()
	if err != nil {
		logger.Log("Error", "Error loading .env file")
	}

	if len(alertConfigPath) > 0 {
		if _, err := os.Stat(alertConfigPath); errors.Is(err, os.ErrNotExist) {
			logger.Log("cannot load alert config: ", err.Error())
		}
		alertConfig = alerts.GetAlertConfig(alertConfigPath)
	}

	if *initPtr {
		initCollector(&config)
	} else if len(removeAgentVal) > 0 {
		removeAgent(removeAgentVal, config)
	} else {

		if alertConfig != nil {
			mysql := getMySQLConnection(&config)
			defer mysql.Close()
			go handleAlerts(alertConfig, &config, &mysql)
		}

		lis, err := net.Listen("tcp", ":"+config.Port)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		s := api.Server{}
		grpcServer := grpc.NewServer(grpc.UnaryInterceptor(authInterceptor))
		api.RegisterMonitorDataServiceServer(grpcServer, &s)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %s", err)
		}
	}
}

func authInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	meta, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		logger.Log("error", "cannot parse meta")
		return nil, status.Error(codes.Unauthenticated, "INTERNAL_SERVER_ERROR")
	}
	if len(meta["jwt"]) != 1 {
		logger.Log("error", "cannot parse meta - token empty")
		return nil, status.Error(codes.Unauthenticated, "token empty")
	}
	if !auth.ValidToken(meta["jwt"][0]) {
		logger.Log("error", "auth error")
		return nil, status.Error(codes.PermissionDenied, "invalid auth token")
	}
	return handler(ctx, req)
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
