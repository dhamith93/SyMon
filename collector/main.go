package main

import (
	"context"
	"crypto/tls"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
		var grpcServer *grpc.Server

		if config.TLSEnabled {
			tlsCreds, err := loadTLSCreds(&config)
			if err != nil {
				log.Fatal("cannot load TLS credentials: ", err)
				log.Fatalf("failed to load TLS cert %s, key %s: %v", config.KeyPath, config.KeyPath, err)
			}
			grpcServer = grpc.NewServer(
				grpc.Creds(tlsCreds),
				grpc.UnaryInterceptor(authInterceptor),
			)
		} else {
			grpcServer = grpc.NewServer(grpc.UnaryInterceptor(authInterceptor))
		}

		api.RegisterMonitorDataServiceServer(grpcServer, &s)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %s", err)
		}
	}
}

func loadTLSCreds(config *config.Collector) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(config.CertPath, config.KeyPath)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}
	return credentials.NewTLS(tlsConfig), nil
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
