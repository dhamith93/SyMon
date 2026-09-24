package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/store"
	"github.com/dhamith93/SyMon/internal/transport"
)

func main() {
	var removeAgentVal string
	var alertConfig []alerts.AlertConfig
	initPtr := flag.Bool("init", false, "Create the database schema and print a new SYMON_KEY")
	flag.StringVar(&removeAgentVal, "remove-agent", "", "Remove an agent. Its metrics are kept until retention drops them.")
	enrollTokenPtr := flag.Bool("enroll-token", false, "Print a token and command that enroll a new host")
	tokenHost := flag.String("host", "", "With -enroll-token: only this host name may use the token")
	tokenUses := flag.Int("uses", 1, "With -enroll-token: how many hosts the token can enroll")
	tokenTTL := flag.Duration("ttl", time.Hour, "With -enroll-token: how long the token is valid")
	envFile := flag.String("env", config.DefaultEnvFile("collector"), "Settings file with KEY=value lines, loaded if it exists")
	flag.Parse()
	if err := config.LoadEnvFile(*envFile); err != nil {
		log.Fatal(err)
	}

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

	ctx := context.Background()
	st, err := openStore(ctx, &config)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	if *initPtr {
		initCollector(ctx, st)
		return
	}
	if len(removeAgentVal) > 0 {
		removeAgent(ctx, st, removeAgentVal)
		return
	}
	if *enrollTokenPtr {
		printEnrollmentToken(ctx, st, &config, *tokenHost, *tokenUses, *tokenTTL)
		return
	}

	if err := st.Migrate(ctx); err != nil {
		log.Fatal("cannot update database schema: ", err)
	}
	if err := st.ApplyRetention(ctx); err != nil {
		log.Fatal("cannot set retention: ", err)
	}

	if alertConfig != nil {
		go handleAlerts(alertConfig, &config, st)
	}
	go purgeOldRecords(st)

	lis, err := net.Listen("tcp", ":"+config.Port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer, err := transport.NewServer(config.TLSEnabled, config.CertPath, config.KeyPath, api.AuthInterceptor(st))
	if err != nil {
		log.Fatal(err)
	}
	api.RegisterMonitorDataServiceServer(grpcServer, &api.Server{Store: st})
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}

func openStore(ctx context.Context, config *config.Collector) (*store.Store, error) {
	if len(config.DatabaseURL) == 0 {
		return nil, errors.New("SYMON_DATABASE_URL is not set")
	}

	retention := store.DefaultRetention
	if config.RetentionRawDays > 0 {
		retention.Raw = days(config.RetentionRawDays)
	}
	if config.RetentionMinuteDays > 0 {
		retention.Minute = days(config.RetentionMinuteDays)
	}
	if config.RetentionHourDays > 0 {
		retention.Hour = days(config.RetentionHourDays)
	}
	// the hourly rollups are built from the last 2 days of minute rollups
	if retention.Minute < days(3) {
		return nil, errors.New("SYMON_RETENTION_MINUTE_DAYS must be at least 3")
	}

	return store.Open(ctx, config.DatabaseURL, retention)
}

func days(n int) time.Duration {
	return time.Duration(n) * 24 * time.Hour
}

func initCollector(ctx context.Context, st *store.Store) {
	if err := st.Migrate(ctx); err != nil {
		fmt.Println("cannot create database schema: " + err.Error())
		os.Exit(1)
	}
	key := auth.GetKey(true)
	fmt.Printf("---\nSYMON_KEY: %s\n---\n", key)
}

func printEnrollmentToken(ctx context.Context, st *store.Store, config *config.Collector, host string, uses int, ttl time.Duration) {
	// the tables may be new, if the collector has not run since an upgrade
	if err := st.Migrate(ctx); err != nil {
		fmt.Println("cannot update database schema: " + err.Error())
		os.Exit(1)
	}
	token, expires, err := st.CreateEnrollmentToken(ctx, host, uses, ttl)
	if err != nil {
		fmt.Println("cannot create token: " + err.Error())
		os.Exit(1)
	}

	usable := "once"
	if uses > 1 {
		usable = fmt.Sprintf("by %d hosts", uses)
	}
	if host != "" {
		usable += " by " + host
	}
	fmt.Printf("Token usable %s until %s. On the new host run:\n\n", usable, expires.Local().Format("Jan 2 15:04"))
	fmt.Printf("  curl -fsSL --connect-timeout 10 %s/install.sh | sudo sh -s -- --token %s\n\n", dashboardURL(config), token)
	fmt.Println("The host downloads the agent from the dashboard, then sends its data to the collector.")
	fmt.Println("Run the same command again later to upgrade the agent.")
}

// dashboardURL is where new hosts download the agent from
func dashboardURL(config *config.Collector) string {
	if config.DashboardURL != "" {
		return strings.TrimRight(config.DashboardURL, "/")
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	return "http://" + hostname + ":8080"
}

func removeAgent(ctx context.Context, st *store.Store, name string) {
	fmt.Println("Removing agent " + name)
	err := st.RemoveHost(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		fmt.Println("Agent ID " + name + " doesn't exist")
		return
	}
	if err != nil {
		fmt.Println(err.Error())
	}
}
