package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/systats"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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

	if *initPtr {
		initAgent(&config)
		return
	} else if *customPtr {
		if len(name) > 0 && len(value) > 0 && len(unit) > 0 {
			sendCustomMetric(name, unit, value, &config)
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
				sendMonitorData(monitorData, &config)
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
				sendPing(&config)
			case <-quitForPing:
				ticker.Stop()
				return
			}
		}
	}()

	wg.Wait()
	fmt.Println("Exiting")
}

func initAgent(config *config.Agent) {
	conn, c, ctx, cancel := createClient(config)
	if conn == nil {
		logger.Log("error", "error creating connection")
		return
	}
	defer conn.Close()
	defer cancel()
	syStats := systats.New()
	response, err := c.InitAgent(ctx, &api.ServerInfo{
		ServerName: config.ServerId,
		Timezone:   monitor.GetSystem(&syStats).TimeZone,
	})
	if err != nil {
		logger.Log("error", "error adding agent: "+err.Error())
		os.Exit(1)
	}
	fmt.Printf("%s \n", response.Body)
}

func sendPing(config *config.Agent) {
	conn, c, ctx, cancel := createClient(config)
	if conn == nil {
		logger.Log("error", "error creating connection")
		return
	}
	defer conn.Close()
	defer cancel()
	_, err := c.HandlePing(ctx, &api.ServerInfo{ServerName: config.ServerId})
	if err != nil {
		logger.Log("error", "error sending ping: "+err.Error())
	}
}

func sendMonitorData(monitorData string, config *config.Agent) {
	conn, c, ctx, cancel := createClient(config)
	if conn == nil {
		logger.Log("error", "error creating connection")
		return
	}
	defer conn.Close()
	defer cancel()
	_, err := c.HandleMonitorData(ctx, &api.MonitorData{MonitorData: monitorData})
	if err != nil {
		logger.Log("error", "error sending data: "+err.Error())
	}
}

func sendCustomMetric(name string, unit string, value string, config *config.Agent) {
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
	conn, c, ctx, cancel := createClient(config)
	if conn == nil {
		logger.Log("error", "error creating connection")
		return
	}
	defer conn.Close()
	defer cancel()
	_, err = c.HandleCustomMonitorData(ctx, &api.MonitorData{MonitorData: string(jsonData)})
	if err != nil {
		logger.Log("error", "error sending custom data: "+err.Error())
		os.Exit(1)
	}
}

func generateToken() string {
	token, err := auth.GenerateJWT()
	if err != nil {
		logger.Log("error", "error generating token: "+err.Error())
		os.Exit(1)
	}
	return token
}

func createClient(config *config.Agent) (*grpc.ClientConn, api.MonitorDataServiceClient, context.Context, context.CancelFunc) {
	var (
		conn     *grpc.ClientConn
		tlsCreds credentials.TransportCredentials
		err      error
	)

	if len(config.CollectorEndpointCACertPath) > 0 {
		tlsCreds, err = loadTLSCreds(config)
		if err != nil {
			log.Fatal("cannot load TLS credentials: ", err)
		}
		conn, err = grpc.Dial(config.CollectorEndpoint, grpc.WithTransportCredentials(tlsCreds))
	} else {
		conn, err = grpc.Dial(config.CollectorEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if err != nil {
		logger.Log("error", "connection error: "+err.Error())
		return nil, nil, nil, nil
	}
	c := api.NewMonitorDataServiceClient(conn)
	token := generateToken()
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{"jwt": token})), time.Second*10)
	return conn, c, ctx, cancel
}

func loadTLSCreds(config *config.Agent) (credentials.TransportCredentials, error) {
	cert, err := ioutil.ReadFile(config.CollectorEndpointCACertPath)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(cert) {
		return nil, fmt.Errorf("failed to add server CA cert")
	}

	tlsConfig := &tls.Config{
		RootCAs: certPool,
	}

	return credentials.NewTLS(tlsConfig), nil
}
