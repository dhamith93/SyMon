package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/transport"
	"github.com/dhamith93/systats"
)

func main() {
	config := config.GetAgent()

	if config.LogFileEnabled {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	var name, value, unit string

	initPtr := flag.Bool("init", false, "Initialize agent")
	customPtr := flag.Bool("custom", false, "Send custom metrics")
	flag.StringVar(&name, "name", "", "Name of the metric")
	flag.StringVar(&unit, "unit", "", "Unit of the metric")
	flag.StringVar(&value, "value", "", "Value of the metric")
	flag.Parse()

	conn, err := transport.Dial(config.CollectorEndpoint, config.CollectorEndpointCACertPath)
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

	ticker := time.NewTicker(time.Duration(config.MonitorIntervalSeconds) * time.Second)
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
				monitorData := monitor.MonitorAsJSON(&config)
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

func initAgent(client api.MonitorDataServiceClient, config *config.Agent) {
	ctx, cancel := transport.Context()
	defer cancel()
	syStats := systats.New()
	response, err := client.InitAgent(ctx, &api.ServerInfo{
		ServerName: config.ServerId,
		Timezone:   monitor.GetSystem(&syStats).TimeZone,
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
