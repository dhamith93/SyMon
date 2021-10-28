package server

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dhamith93/SyMon/internal/auth"
	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/database"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/monitor"
	"github.com/dhamith93/SyMon/internal/stringops"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Agents struct {
	AgentIDs []string
}

type CustomMetrics struct {
	CustomMetrics []string
}

func Run(port string) {
	config := config.GetConfig("config.json")
	handleRequests(port, config)
}

func handleRequests(port string, config config.Config) {
	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/collect", auth.CheckAuth(returnCollect))
	router.Handle("/collect-custom", auth.CheckAuth(returnCollectCustom))
	router.Handle("/collect-init", auth.CheckAuth(returnInit))
	router.Handle("/custom", auth.CheckAuth(returnCustom))
	router.Handle("/custom-metric-names", auth.CheckAuth(returnCustomMetricNames))
	router.Handle("/agents", auth.CheckAuth(returnAgents))
	router.Handle("/system", auth.CheckAuth(returnSystem))
	router.Handle("/memory", auth.CheckAuth(returnMemory))
	router.Handle("/swap", auth.CheckAuth(returnSwap))
	router.Handle("/disks", auth.CheckAuth(returnDisks))
	router.Handle("/proc", auth.CheckAuth(returnProc))
	router.Handle("/network", auth.CheckAuth(returnNetwork))
	router.Handle("/processes", auth.CheckAuth(returnProcesses))
	router.Handle("/processor-usage-historical", auth.CheckAuth(returnProcHistorical))
	router.Handle("/memory-historical", auth.CheckAuth(returnMemoryHistorical))
	router.Handle("/services", auth.CheckAuth(returnServices))

	server := http.Server{}
	server.Addr = port
	server.Handler = handlers.CompressHandler(router)
	server.SetKeepAlivesEnabled(false)

	if config.SSLEnabled && config.SSLCertFilePath != "" && config.SSLKeyFilePath != "" {
		logger.Log("info", "[SSL] API started on port "+port)
		log.Fatal(server.ListenAndServeTLS(config.SSLCertFilePath, config.SSLKeyFilePath))
	} else {
		logger.Log("info", "API started on port "+port)
		log.Fatal(server.ListenAndServe())
	}
}

func returnInit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	serverIdArr, ok := r.URL.Query()["serverId"]
	timeZoneArr, ok2 := r.URL.Query()["timezone"]

	if !ok || !ok2 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode("cannot parse server id/timezone")
		return
	}

	err := initAgent(serverIdArr[0], timeZoneArr[0], config.GetConfig("config.json"))

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode("agent added")
}

func returnCollect(w http.ResponseWriter, r *http.Request) {
	// logger.Log("info", "API HIT "+r.RemoteAddr+" "+time.Now().String())
	gunzip, err := gzip.NewReader(r.Body)
	if err != nil {
		log.Println("error unzip: ", err)
	}
	defer gunzip.Close()
	body, _ := ioutil.ReadAll(gunzip)
	var monitorData = monitor.MonitorData{}
	err = json.Unmarshal(body, &monitorData)
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)

	data := map[string]string{
		"time": unixTime,
	}

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		data["status"] = "ERROR"
		data["error"] = err.Error()
	} else {
		err := HandleMonitorData(monitorData)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			data["status"] = "ERROR"
			data["error"] = err.Error()
		} else {
			w.WriteHeader(http.StatusOK)
			data["status"] = "OK"
		}
		// logger.Log("info", "API FINISHED "+r.RemoteAddr+" "+time.Now().String())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func returnCollectCustom(w http.ResponseWriter, r *http.Request) {
	gunzip, err := gzip.NewReader(r.Body)
	if err != nil {
		log.Println("error unzip: ", err)
	}
	defer gunzip.Close()
	body, _ := ioutil.ReadAll(gunzip)
	var customMetric = monitor.CustomMetric{}
	err = json.Unmarshal(body, &customMetric)
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)

	data := map[string]string{
		"time": unixTime,
	}

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		data["status"] = "ERROR"
		data["error"] = err.Error()
	} else {
		err := HandleCustomMetric(customMetric)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			data["status"] = "ERROR"
			data["error"] = err.Error()
		} else {
			w.WriteHeader(http.StatusOK)
			data["status"] = "OK"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func returnAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var db *sql.DB
	db, err := database.OpenDB(db, config.GetConfig("config.json").SQLiteDBPath+"/collector.db")
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	defer db.Close()
	agents := Agents{}
	agents.AgentIDs = database.GetAgents(db)
	json.NewEncoder(w).Encode(&agents)
}

func returnCustomMetricNames(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	db, err := getDB(r)
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	defer db.Close()
	customMetrics := CustomMetrics{}
	customMetrics.CustomMetrics = database.GetCustomMetricNames(db)
	json.NewEncoder(w).Encode(&customMetrics)
}

func returnCustom(w http.ResponseWriter, r *http.Request) {
	customMetricName, err := parseGETForCustomMetricName(r)
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	sendResponseAsArray(w, r, customMetricName, false, []monitor.CustomMetric{})
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, r, "system", monitor.System{})
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, r, "memory", []string{})
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, r, "swap", []string{})
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, r, "disks", monitor.Disk{})
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, r, "processor", monitor.Processor{})
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	sendResponseAsArray(w, r, "networks", true, [][]monitor.Network{})
}

func returnProcesses(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, r, "processes", monitor.Process{})
}

func returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	sendResponseAsArray(w, r, "procUsage", true, []string{})
}

func returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	sendResponseAsArray(w, r, "memory", true, []string{})
}

func returnServices(w http.ResponseWriter, r *http.Request) {
	sendResponseAsArray(w, r, "services", false, []monitor.Service{})
}

func sendResponseAsArray(w http.ResponseWriter, r *http.Request, logType string, convertToJsonArr bool, iface ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	db, err := getDB(r)
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	defer db.Close()
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := database.GetLogFromDB(db, logType, from, to, time)
	if convertToJsonArr || (to != 0 && from != 0) {
		dataString := stringops.StringArrToJSONArr(data)
		_ = json.Unmarshal([]byte(dataString), &iface)
	} else {
		if len(data) > 0 && to == 0 {
			_ = json.Unmarshal([]byte(data[0]), &iface)
		}
	}
	json.NewEncoder(w).Encode(&iface)
}

func sendResponse(w http.ResponseWriter, r *http.Request, logType string, iface interface{}) {
	w.Header().Set("Content-Type", "application/json")
	db, err := getDB(r)
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	defer db.Close()
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := database.GetLogFromDB(db, logType, from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &iface)
	}
	json.NewEncoder(w).Encode(&iface)
}

func initAgent(agentId string, timezone string, config config.Config) error {
	logger.Log("info", "Initializing agent for "+agentId)

	mysql := database.MySql{}
	mysql.Connect()
	defer mysql.Close()

	if mysql.AgentIDExists(agentId) {
		logger.Log("error", "agent id "+agentId+" exists")
		return fmt.Errorf("agent id " + agentId + " exists")
	}

	err := mysql.AddAgent(agentId, timezone)
	if err != nil {
		logger.Log("error", err.Error())
		return fmt.Errorf("error adding agent")
	}

	return nil
}

func getDB(r *http.Request) (*sql.DB, error) {
	var db *sql.DB
	var err error
	serverIdArr, ok := r.URL.Query()["serverId"]
	if ok {
		db, err = database.OpenDB(db, config.GetConfig("config.json").SQLiteDBPath+"/"+serverIdArr[0]+".db")
		if err != nil {
			return nil, err
		}
		return db, nil
	}
	return nil, fmt.Errorf("cannot parse server id")
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
