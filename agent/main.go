package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/transport"
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

func main() {
	var name, value, unit string

	initPtr := flag.Bool("init", false, "Initialize agent")
	customPtr := flag.Bool("custom", false, "Send custom metrics")
	flag.StringVar(&name, "name", "", "Name of the metric")
	flag.StringVar(&unit, "unit", "", "Unit of the metric")
	flag.StringVar(&value, "value", "", "Value of the metric")
	enrollPtr := flag.Bool("enroll", false, "Enroll this host with a token from collector -enroll-token")
	token := flag.String("token", "", "With -enroll: the enrollment token")
	host := flag.String("host", "", "With -enroll: the host name to use, the machine's name by default")
	envFile := flag.String("env", config.DefaultEnvFile("agent"), "Settings file with KEY=value lines, loaded if it exists")
	flag.Parse()
	if err := config.LoadEnvFile(*envFile); err != nil {
		log.Fatal(err)
	}

	config := config.GetAgent()

	if config.LogFileEnabled {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	if *enrollPtr {
		if *host != "" {
			config.ServerId = *host
		}
		enroll(&config, *token)
		return
	}

	conn, err := transport.Dial(config.CollectorEndpoint, config.CollectorEndpointCACertPath, credentials(&config))
	if err != nil {
		log.Fatal("cannot create collector client: ", err)
	}
	defer conn.Close()
	client := api.NewMonitorDataServiceClient(conn)

	if *initPtr {
		initAgent(client, &config)
		return
	} else if *customPtr {
		if len(name) > 0 && len(value) > 0 && len(unit) > 0 {
			sendCustomMetric(client, name, unit, value, &config)
		} else {
			fmt.Println("Metric name, unit, and value all required")
		}
		return
	}

	interval := time.Duration(config.MonitorIntervalSeconds) * time.Second
	collector := monitor.NewCollector(&config)
	ticker := time.NewTicker(interval)
	tickerForPing := time.NewTicker(time.Minute)
	quit := make(chan struct{})
	quitForPing := make(chan bool)

	var wg sync.WaitGroup
	wg.Add(2)

	// Monitoring
	go func() {
		for {
			select {
			case <-ticker.C:
				// leave room so a slow collection does not run into the next tick
				ctx, cancel := context.WithTimeout(context.Background(), interval*3/4)
				monitorData := collector.CollectJSON(ctx)
				cancel()
				sendMonitorData(client, monitorData, &config)
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	// pinging
	go func() {
		for {
			select {
			case <-tickerForPing.C:
				sendPing(client, &config)
			case <-quitForPing:
				ticker.Stop()
				return
			}
		}
	}()

	wg.Wait()
	fmt.Println("Exiting")
}

// credentials returns the agent's own key once it is enrolled, and the
// shared SYMON_KEY before that
func credentials(config *config.Agent) grpccreds.PerRPCCredentials {
	key, err := os.ReadFile(config.AgentKeyPath)
	if err == nil && len(strings.TrimSpace(string(key))) > 0 {
		return transport.AgentKey(strings.TrimSpace(string(key)))
	}
	if !errors.Is(err, fs.ErrNotExist) && err != nil {
		logger.Log("error", "cannot read agent key, using SYMON_KEY: "+err.Error())
	}
	return transport.SharedKey()
}

// enroll trades a single use token for this host's own key and saves it
func enroll(config *config.Agent, token string) {
	if token == "" {
		fmt.Println("-enroll needs -token, get one with collector -enroll-token")
		os.Exit(1)
	}
	conn, err := transport.Dial(config.CollectorEndpoint, config.CollectorEndpointCACertPath, nil)
	if err != nil {
		fmt.Println("cannot create collector client: " + err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	ctx, cancel := transport.Context()
	defer cancel()
	response, err := api.NewMonitorDataServiceClient(conn).Enroll(ctx, &api.EnrollRequest{
		Token:    token,
		HostName: config.ServerId,
		Timezone: monitor.TimeZone(),
	})
	if err != nil {
		fmt.Println("cannot enroll: " + status.Convert(err).Message())
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(config.AgentKeyPath), 0755); err != nil {
		fmt.Println("cannot save agent key: " + err.Error())
		os.Exit(1)
	}
	if err := os.WriteFile(config.AgentKeyPath, []byte(response.AgentKey+"\n"), 0600); err != nil {
		fmt.Println("cannot save agent key: " + err.Error())
		os.Exit(1)
	}
	fmt.Printf("Enrolled as %s, key saved to %s\n", response.HostName, config.AgentKeyPath)
}

func initAgent(client api.MonitorDataServiceClient, config *config.Agent) {
	ctx, cancel := transport.Context()
	defer cancel()
	response, err := client.InitAgent(ctx, &api.ServerInfo{
		ServerName: config.ServerId,
		Timezone:   monitor.TimeZone(),
	})
	if err != nil {
		logger.Log("error", "error adding agent: "+err.Error())
		os.Exit(1)
	}
	fmt.Printf("%s \n", response.Body)
}

func sendPing(client api.MonitorDataServiceClient, config *config.Agent) {
	ctx, cancel := transport.Context()
	defer cancel()
	_, err := client.HandlePing(ctx, &api.ServerInfo{ServerName: config.ServerId})
	if err != nil {
		logger.Log("error", "error sending ping: "+err.Error())
	}
}

func sendMonitorData(client api.MonitorDataServiceClient, monitorData string, config *config.Agent) {
	ctx, cancel := transport.Context()
	defer cancel()
	_, err := client.HandleMonitorData(ctx, &api.MonitorData{MonitorData: monitorData})
	if err != nil {
		logger.Log("error", "error sending data: "+err.Error())
	}
}

func sendCustomMetric(client api.MonitorDataServiceClient, name string, unit string, value string, config *config.Agent) {
	customMetric := monitor.CustomMetric{
		Name:     name,
		Unit:     unit,
		Value:    value,
		Time:     strconv.FormatInt(time.Now().Unix(), 10),
		ServerId: config.ServerId,
	}
	jsonData, err := json.Marshal(&customMetric)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	ctx, cancel := transport.Context()
	defer cancel()
	_, err = client.HandleCustomMonitorData(ctx, &api.MonitorData{MonitorData: string(jsonData)})
	if err != nil {
		logger.Log("error", "error sending custom data: "+err.Error())
		os.Exit(1)
	}
}
