package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Agents struct {
	AgentIDs []string
}

type output struct {
	Status string
	Data   interface{}
}

type IsUp struct {
	IsUp bool
}

// Run starts the server in given port
func Run(port string) {
	handleRequests(port)
}

func handleRequests(port string) {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/agents", returnAgents)
	router.HandleFunc("/isup", returnIsUp)
	router.HandleFunc("/system", returnSystem)
	router.HandleFunc("/memory", returnMemory)
	router.HandleFunc("/swap", returnSwap)
	router.HandleFunc("/disks", returnDisks)
	router.HandleFunc("/proc", returnProc)
	router.HandleFunc("/network", returnNetwork)
	router.HandleFunc("/processes", returnProcesses)
	router.HandleFunc("/processor-usage-historical", returnProcHistorical)
	router.HandleFunc("/memory-historical", returnMemoryHistorical)
	router.HandleFunc("/disks-historical", returnDisksHistorical)
	router.HandleFunc("/services", returnServices)
	router.HandleFunc("/custom", returnCustom)
	router.HandleFunc("/custom-metric-names", returnCustomMetricNames)
	router.HandleFunc("/alerts", returnAlerts)
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./frontend/")))

	server := http.Server{}
	server.Addr = port
	server.Handler = handlers.CompressHandler(router)
	server.SetKeepAlivesEnabled(false)

	logger.Log("info", "API started on port "+port)
	log.Fatal(server.ListenAndServe())
}

func returnAgents(w http.ResponseWriter, r *http.Request) {
	handleRequestForMeta("agents", w, r)
}

func returnIsUp(w http.ResponseWriter, r *http.Request) {
	handleRequestForPing(w, r)
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	handleRequest("system", w, r, false)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	handleRequest("memory", w, r, false)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	handleRequest("swap", w, r, false)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	handleRequest("disks", w, r, false)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	handleRequest("procUsage", w, r, false)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	handleRequest("networks", w, r, false)
}

func returnProcesses(w http.ResponseWriter, r *http.Request) {
	handleRequest("processes", w, r, false)
}

func returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	handleRequest("procUsage", w, r, false)
}

func returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	handleRequest("memory-historical", w, r, false)
}

func returnDisksHistorical(w http.ResponseWriter, r *http.Request) {
	handleRequest("disks", w, r, false)
}

func returnServices(w http.ResponseWriter, r *http.Request) {
	handleRequest("services", w, r, false)
}

func returnCustom(w http.ResponseWriter, r *http.Request) {
	customMetricName, _ := parseGETForCustomMetricName(r)
	handleRequest(customMetricName, w, r, true)
}

func returnCustomMetricNames(w http.ResponseWriter, r *http.Request) {
	handleRequestForMeta("customMetricNames", w, r)
}

func returnAlerts(w http.ResponseWriter, r *http.Request) {
	// handleRequestForMeta("customMetricNames", w, r)
	config := config.GetClient()
	serverName, _ := parseGETForServerName(r)
	received, _ := getActiveAlerts(serverName, &config)
	alertData, err := json.Marshal(received.Alerts)
	var data interface{}
	var out output
	if err != nil {
		out.Status = "ERR"
		json.NewEncoder(w).Encode(&out)
		return
	}
	out.Status = "OK"
	_ = json.Unmarshal(alertData, &data)
	out.Data = data
	json.NewEncoder(w).Encode(&out)
}

func handleRequest(logType string, w http.ResponseWriter, r *http.Request, isCustomMetric bool) {
	w.Header().Set("Content-Type", "application/json")
	config := config.GetClient()
	serverName, _ := parseGETForServerName(r)
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	received, err := getMonitorData(serverName, logType, from, to, time, &config, isCustomMetric)
	var data interface{}
	var out output
	out.Status = "OK"
	if err != nil {
		out.Status = "ERR"
		json.NewEncoder(w).Encode(&out)
		return
	}
	_ = json.Unmarshal([]byte(received), &data)
	out.Data = data
	json.NewEncoder(w).Encode(&out)
}

func handleRequestForPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	config := config.GetClient()
	var out output
	out.Status = "OK"
	conn, c, ctx, cancel := createClient(&config)
	defer conn.Close()
	defer cancel()
	serverName, _ := parseGETForServerName(r)

	isUp, err := c.IsUp(ctx, &api.ServerInfo{ServerName: serverName})

	if err != nil {
		out.Data = IsUp{IsUp: false}
		json.NewEncoder(w).Encode(&out)
		return
	}

	out.Data = IsUp{IsUp: isUp.IsUp}
	json.NewEncoder(w).Encode(&out)
}

func handleRequestForMeta(metaType string, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	config := config.GetClient()
	var out output
	out.Status = "OK"
	conn, c, ctx, cancel := createClient(&config)
	defer conn.Close()
	defer cancel()

	var meta *api.Message
	var err error

	switch metaType {
	case "agents":
		meta, err = c.HandleAgentIdsRequest(ctx, &api.Void{})
	case "customMetricNames":
		serverName, _ := parseGETForServerName(r)
		meta, err = c.HandleCustomMetricNameRequest(ctx, &api.ServerInfo{ServerName: serverName})
	default:
		break
	}

	if err != nil {
		out.Status = "ERR"
		out.Data = err.Error()
		json.NewEncoder(w).Encode(&out)
		return
	}

	var data interface{}
	_ = json.Unmarshal([]byte(meta.Body), &data)
	out.Data = data
	json.NewEncoder(w).Encode(&out)
}

func generateToken() string {
	token, err := auth.GenerateJWT()
	if err != nil {
		logger.Log("error", "error generating token: "+err.Error())
		os.Exit(1)
	}
	return token
}

func createClient(config *config.Client) (*grpc.ClientConn, api.MonitorDataServiceClient, context.Context, context.CancelFunc) {
	var (
		conn     *grpc.ClientConn
		tlsCreds credentials.TransportCredentials
		err      error
	)

	if len(config.CollectorEndpointCACertPath) > 0 {
		tlsCreds, err = loadTLSCreds(config.CollectorEndpointCACertPath)
		if err != nil {
			log.Fatal("cannot load TLS credentials: ", err)
		}
		conn, err = grpc.Dial(config.CollectorEndpoint, grpc.WithTransportCredentials(tlsCreds))
	} else {
		conn, err = grpc.Dial(config.CollectorEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if err != nil {
		logger.Log("error", "connection error: "+err.Error())
		os.Exit(1)
	}
	c := api.NewMonitorDataServiceClient(conn)
	token := generateToken()
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{"jwt": token})), time.Second*10)
	return conn, c, ctx, cancel
}

func getMonitorData(serverName string, logType string, from int64, to int64, time int64, config *config.Client, isCustomMetric bool) (string, error) {
	conn, c, ctx, cancel := createClient(config)
	defer conn.Close()
	defer cancel()
	monitorData, err := c.HandleMonitorDataRequest(ctx, &api.MonitorDataRequest{ServerName: serverName, LogType: logType, From: from, To: to, Time: time, IsCustomMetric: isCustomMetric})
	if err != nil {
		logger.Log("error", "error sending data: "+err.Error())
		return "", err
	}
	return monitorData.MonitorData, nil
}

func getActiveAlerts(serverName string, config *config.Client) (*alertapi.AlertArray, error) {
	var (
		conn     *grpc.ClientConn
		tlsCreds credentials.TransportCredentials
		err      error
	)

	if len(config.AlertEndpointCACertPath) > 0 {
		tlsCreds, err = loadTLSCreds(config.AlertEndpointCACertPath)
		if err != nil {
			log.Fatal("cannot load TLS credentials: ", err)
		}
		conn, err = grpc.Dial(config.AlertEndpoint, grpc.WithTransportCredentials(tlsCreds))
	} else {
		conn, err = grpc.Dial(config.AlertEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if err != nil {
		logger.Log("error", "connection error: "+err.Error())
		os.Exit(1)
	}
	token := generateToken()
	c := alertapi.NewAlertServiceClient(conn)
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.New(map[string]string{"jwt": token})), time.Second*10)
	defer conn.Close()
	defer cancel()

	alerts, err := c.AlertRequest(ctx, &alertapi.Request{ServerName: serverName})

	if err != nil {
		logger.Log("error", "error sending data: "+err.Error())
		return &alertapi.AlertArray{}, err
	}
	return alerts, nil
}

func parseGETForTime(r *http.Request) (int64, error) {
	timeArr, ok := r.URL.Query()["time"]

	if !ok {
		return 0, fmt.Errorf("error parsing get vars")
	}

	timeInt, err := strconv.ParseInt(timeArr[0], 10, 64)

	if err != nil {
		return 0, fmt.Errorf("error parsing get vars")
	}

	return timeInt, nil
}

func parseGETForDates(r *http.Request) (int64, int64, error) {
	from, okFrom := r.URL.Query()["from"]
	to, okTo := r.URL.Query()["to"]

	if !okFrom || !okTo {
		return 0, 0, fmt.Errorf("error parsing get vars")
	}

	fromTime, err1 := strconv.ParseInt(from[0], 10, 64)
	toTime, err2 := strconv.ParseInt(to[0], 10, 64)

	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("error parsing get vars")
	}

	return fromTime, toTime, nil
}

func parseGETForServerName(r *http.Request) (string, error) {
	serverIdArr, ok := r.URL.Query()["serverId"]
	if !ok || len(serverIdArr) == 0 {
		logger.Log("ERROR", "cannot parse for server ID")
		return "", fmt.Errorf("cannot parse for server id")
	}
	return serverIdArr[0], nil
}

func parseGETForCustomMetricName(r *http.Request) (string, error) {
	customMetricNameArr, ok := r.URL.Query()["custom-metric"]

	if !ok {
		return "", fmt.Errorf("error parsing get vars")
	}

	if len(customMetricNameArr) == 0 {
		return "", fmt.Errorf("error parsing get vars")
	}

	return customMetricNameArr[0], nil
}

func loadTLSCreds(path string) (credentials.TransportCredentials, error) {
	cert, err := ioutil.ReadFile(path)
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
