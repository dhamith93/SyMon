package main

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/pkg/memdb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	)
	if err != nil {
		logger.Log("error", "memdb: "+err.Error())
	}

	s := alertapi.Server{
		Database: &notificationTracker,
	}
	lis, err := net.Listen("tcp", ":5999")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(authInterceptor))
	alertapi.RegisterAlertServiceServer(grpcServer, &s)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
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
