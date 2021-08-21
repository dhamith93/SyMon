package server

import (
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

	"github.com/gorilla/mux"
)

type Agents struct {
	AgentIDs []string
}

func Run(port string) {
	config := config.GetConfig("config.json")
	handleRequests(port, config)
}

func handleRequests(port string, config config.Config) {
	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/collect", auth.CheckAuth(returnCollect))
	router.Handle("/agents", auth.CheckAuth(returnAgents))
	router.Handle("/system", auth.CheckAuth(returnSystem))
	router.Handle("/memory", auth.CheckAuth(returnMemory))
	router.Handle("/swap", auth.CheckAuth(returnSwap))
	router.Handle("/disks", auth.CheckAuth(returnDisks))
	router.Handle("/proc", auth.CheckAuth(returnProc))
	router.Handle("/network", auth.CheckAuth(returnNetwork))
	router.Handle("/memusage", auth.CheckAuth(returnMemUsage))
	router.Handle("/cpuusage", auth.CheckAuth(returnCPUUsage))
	router.Handle("/processor-usage-historical", auth.CheckAuth(returnProcHistorical))
	router.Handle("/memory-historical", auth.CheckAuth(returnMemoryHistorical))
	router.Handle("/services", auth.CheckAuth(returnServices))

	server := http.Server{}
	server.Addr = port
	server.Handler = router
	server.SetKeepAlivesEnabled(false)

	if config.SSLEnabled && config.SSLCertFilePath != "" && config.SSLKeyFilePath != "" {
		logger.Log("info", "[SSL] API started on port "+port)
		log.Fatal(server.ListenAndServeTLS(config.SSLCertFilePath, config.SSLKeyFilePath))
	} else {
		logger.Log("info", "API started on port "+port)
		log.Fatal(server.ListenAndServe())
	}
}

func returnCollect(w http.ResponseWriter, r *http.Request) {
	logger.Log("info", "API HIT "+r.RemoteAddr+" "+time.Now().String())
	body, _ := ioutil.ReadAll(r.Body)
	var monitorData = monitor.MonitorData{}
	err := json.Unmarshal(body, &monitorData)
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
		logger.Log("info", "API FINISHED "+r.RemoteAddr+" "+time.Now().String())
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

func returnSystem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	system := monitor.System{}
	db, err := getDB(r)
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	defer db.Close()
	data := database.GetLogFromDBCount(db, "system", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &system)
	}
	json.NewEncoder(w).Encode(&system)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	memory := monitor.Memory{}
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
	data := database.GetLogFromDB(db, "memory", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &memory)
	}
	json.NewEncoder(w).Encode(&memory)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	swap := monitor.Swap{}
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
	data := database.GetLogFromDB(db, "swap", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &swap)
	}
	json.NewEncoder(w).Encode(&swap)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	disks := []monitor.Disk{}
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
	data := database.GetLogFromDB(db, "disks", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &disks)
	}
	json.NewEncoder(w).Encode(&disks)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	proc := monitor.Processor{}
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
	data := database.GetLogFromDB(db, "processor", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &proc)
	}
	json.NewEncoder(w).Encode(&proc)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	network := []monitor.Network{}
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
	data := database.GetLogFromDB(db, "networks", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &network)
	}
	json.NewEncoder(w).Encode(&network)
}

func returnMemUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	memUsage := []monitor.Process{}
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
	data := database.GetLogFromDB(db, "memoryUsage", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &memUsage)
	}
	json.NewEncoder(w).Encode(&memUsage)
}

func returnCPUUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cpuUsage := []monitor.Process{}
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
	data := database.GetLogFromDB(db, "CpuUsage", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &cpuUsage)
	}
	json.NewEncoder(w).Encode(&cpuUsage)
}

func returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	procs := []monitor.ProcessorUsage{}
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
	data := database.GetLogFromDB(db, "processor", from, to, time)

	dataString := stringops.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &procs)
	json.NewEncoder(w).Encode(&procs)
}

func returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	memories := []monitor.Memory{}
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

	data := database.GetLogFromDB(db, "memory", from, to, time)

	dataString := stringops.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &memories)
	json.NewEncoder(w).Encode(&memories)
}

func returnServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	services := []monitor.Service{}
	db, err := getDB(r)
	if err != nil {
		logger.Log("ERROR", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode("")
		return
	}
	defer db.Close()
	data := []string{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)

	if (from != 0 && to != 0) || time != 0 {
		data = database.GetLogFromDB(db, "services", from, to, time)
	} else {
		countArr, ok := r.URL.Query()["count"]
		count, err := strconv.ParseInt(countArr[0], 10, 64)
		if ok && err == nil {
			data = database.GetLogFromDBCount(db, "services", count)
		}
	}

	dataString := stringops.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &services)
	json.NewEncoder(w).Encode(&services)
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
