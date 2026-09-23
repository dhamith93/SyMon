package main

import (
	"log"
	"net"
	"os"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/transport"
	"github.com/dhamith93/SyMon/pkg/memdb"
)

func main() {
	config := config.GetAlertProcessor()

	if config.LogFileEnabled {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	notificationTracker := memdb.CreateDatabase("notification_tracker")
	err := notificationTracker.Create(
		"alert",
		memdb.Col{Name: "server_name", Type: memdb.String},
		memdb.Col{Name: "metric_type", Type: memdb.String},
		memdb.Col{Name: "metric_name", Type: memdb.String},
		memdb.Col{Name: "log_id", Type: memdb.Int64},
		memdb.Col{Name: "subject", Type: memdb.String},
		memdb.Col{Name: "content", Type: memdb.String},
		memdb.Col{Name: "status", Type: memdb.Int},
		memdb.Col{Name: "timestamp", Type: memdb.String},
		memdb.Col{Name: "resolved", Type: memdb.Bool},
		memdb.Col{Name: "pg_incident_id", Type: memdb.String},
		memdb.Col{Name: "slack_msg_ts", Type: memdb.String},
	)
	if err != nil {
		logger.Log("error", "memdb: "+err.Error())
	}

	s := alertapi.Server{
		Database: &notificationTracker,
	}
	lis, err := net.Listen("tcp", ":"+config.Port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer, err := transport.NewServer(config.TLSEnabled, config.CertPath, config.KeyPath)
	if err != nil {
		log.Fatal(err)
	}

	alertapi.RegisterAlertServiceServer(grpcServer, &s)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
