package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/dhamith93/SyMon/internal/alertapi"
	"github.com/dhamith93/SyMon/internal/api"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/transport"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

type server struct {
	collector api.MonitorDataServiceClient
	alerts    alertapi.AlertServiceClient
}

// Run starts the server in given port
func Run(port string) {
	config := config.GetClient()

	collectorConn, err := transport.Dial(config.CollectorEndpoint, config.CollectorEndpointCACertPath)
	if err != nil {
		log.Fatal("cannot create collector client: ", err)
	}
	defer collectorConn.Close()

	alertConn, err := transport.Dial(config.AlertEndpoint, config.AlertEndpointCACertPath)
	if err != nil {
		log.Fatal("cannot create alert processor client: ", err)
	}
	defer alertConn.Close()

	s := &server{
		collector: api.NewMonitorDataServiceClient(collectorConn),
		alerts:    alertapi.NewAlertServiceClient(alertConn),
	}
	s.handleRequests(port)
}

func (s *server) handleRequests(port string) {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/agents", s.returnAgents)
	router.HandleFunc("/isup", s.returnIsUp)
	router.HandleFunc("/system", s.returnSystem)
	router.HandleFunc("/memory", s.returnMemory)
	router.HandleFunc("/swap", s.returnSwap)
	router.HandleFunc("/disks", s.returnDisks)
	router.HandleFunc("/proc", s.returnProc)
	router.HandleFunc("/network", s.returnNetwork)
	router.HandleFunc("/processes", s.returnProcesses)
	router.HandleFunc("/processor-usage-historical", s.returnProcHistorical)
	router.HandleFunc("/memory-historical", s.returnMemoryHistorical)
	router.HandleFunc("/disks-historical", s.returnDisksHistorical)
	router.HandleFunc("/services", s.returnServices)
	router.HandleFunc("/custom", s.returnCustom)
	router.HandleFunc("/custom-metric-names", s.returnCustomMetricNames)
	router.HandleFunc("/alerts", s.returnAlerts)
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./frontend/")))

	httpServer := http.Server{}
	httpServer.Addr = port
	httpServer.Handler = handlers.CompressHandler(router)
	httpServer.SetKeepAlivesEnabled(false)

	logger.Log("info", "API started on port "+port)
	log.Fatal(httpServer.ListenAndServe())
}

func (s *server) returnAgents(w http.ResponseWriter, r *http.Request) {
	s.handleRequestForMeta("agents", w, r)
}

func (s *server) returnIsUp(w http.ResponseWriter, r *http.Request) {
	s.handleRequestForPing(w, r)
}

func (s *server) returnSystem(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("system", w, r, false)
}

func (s *server) returnMemory(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("memory", w, r, false)
}

func (s *server) returnSwap(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("swap", w, r, false)
}

func (s *server) returnDisks(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("disks", w, r, false)
}

func (s *server) returnProc(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("procUsage", w, r, false)
}

func (s *server) returnNetwork(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("networks", w, r, false)
}

func (s *server) returnProcesses(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("processes", w, r, false)
}

func (s *server) returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("procUsage", w, r, false)
}

func (s *server) returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("memory-historical", w, r, false)
}

func (s *server) returnDisksHistorical(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("disks", w, r, false)
}

func (s *server) returnServices(w http.ResponseWriter, r *http.Request) {
	s.handleRequest("services", w, r, false)
}

func (s *server) returnCustom(w http.ResponseWriter, r *http.Request) {
	customMetricName, _ := parseGETForCustomMetricName(r)
	s.handleRequest(customMetricName, w, r, true)
}

func (s *server) returnCustomMetricNames(w http.ResponseWriter, r *http.Request) {
	s.handleRequestForMeta("customMetricNames", w, r)
}

func (s *server) returnAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverName, _ := parseGETForServerName(r)
	var out output
	received, err := s.getActiveAlerts(serverName)
	if err != nil {
		out.Status = "ERR"
		json.NewEncoder(w).Encode(&out)
		return
	}
	alertData, err := json.Marshal(received.Alerts)
	if err != nil {
		out.Status = "ERR"
		json.NewEncoder(w).Encode(&out)
		return
	}
	var data interface{}
	_ = json.Unmarshal(alertData, &data)
	out.Status = "OK"
	out.Data = data
	json.NewEncoder(w).Encode(&out)
}

func (s *server) handleRequest(logType string, w http.ResponseWriter, r *http.Request, isCustomMetric bool) {
	w.Header().Set("Content-Type", "application/json")
	serverName, _ := parseGETForServerName(r)
	at, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	received, err := s.getMonitorData(serverName, logType, from, to, at, isCustomMetric)
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

func (s *server) handleRequestForPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var out output
	out.Status = "OK"
	ctx, cancel := transport.Context()
	defer cancel()
	serverName, _ := parseGETForServerName(r)

	isUp, err := s.collector.IsUp(ctx, &api.ServerInfo{ServerName: serverName})

	if err != nil {
		out.Data = IsUp{IsUp: false}
		json.NewEncoder(w).Encode(&out)
		return
	}

	out.Data = IsUp{IsUp: isUp.IsUp}
	json.NewEncoder(w).Encode(&out)
}

func (s *server) handleRequestForMeta(metaType string, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var out output
	out.Status = "OK"
	ctx, cancel := transport.Context()
	defer cancel()

	var meta *api.Message
	var err error

	switch metaType {
	case "agents":
		meta, err = s.collector.HandleAgentIdsRequest(ctx, &api.Void{})
	case "customMetricNames":
		serverName, _ := parseGETForServerName(r)
		meta, err = s.collector.HandleCustomMetricNameRequest(ctx, &api.ServerInfo{ServerName: serverName})
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

func (s *server) getMonitorData(serverName string, logType string, from int64, to int64, at int64, isCustomMetric bool) (string, error) {
	ctx, cancel := transport.Context()
	defer cancel()
	monitorData, err := s.collector.HandleMonitorDataRequest(ctx, &api.MonitorDataRequest{ServerName: serverName, LogType: logType, From: from, To: to, Time: at, IsCustomMetric: isCustomMetric})
	if err != nil {
		// NotFound only means the query matched nothing, e.g. no services configured
		if status.Code(err) != codes.NotFound {
			logger.Log("error", "cannot get "+logType+" for "+serverName+": "+err.Error())
		}
		return "", err
	}
	return monitorData.MonitorData, nil
}

func (s *server) getActiveAlerts(serverName string) (*alertapi.AlertArray, error) {
	ctx, cancel := transport.Context()
	defer cancel()
	alerts, err := s.alerts.AlertRequest(ctx, &alertapi.Request{ServerName: serverName})
	if err != nil {
		logger.Log("error", "cannot get alerts for "+serverName+": "+err.Error())
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
