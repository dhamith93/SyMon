package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dhamith93/SyMon/internal/config"
	"github.com/dhamith93/SyMon/internal/logger"
	"github.com/dhamith93/SyMon/internal/send"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type Agents struct {
	AgentIDs []string
}

type output struct {
	Status string
	Data   interface{}
}

// Run starts the server in given port
func Run(port string) {
	handleRequests(port)
}

func handleRequests(port string) {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/agents", returnAgents)
	// router.HandleFunc("/isup", returnIsUp)
	router.HandleFunc("/system", returnSystem)
	router.HandleFunc("/memory", returnMemory)
	router.HandleFunc("/swap", returnSwap)
	router.HandleFunc("/disks", returnDisks)
	router.HandleFunc("/proc", returnProc)
	router.HandleFunc("/network", returnNetwork)
	router.HandleFunc("/memusage", returnMemUsage)
	router.HandleFunc("/cpuusage", returnCPUUsage)
	router.HandleFunc("/processor-usage-historical", returnProcHistorical)
	router.HandleFunc("/memory-historical", returnMemoryHistorical)
	router.HandleFunc("/services", returnServices)
	router.HandleFunc("/custom", returnCustom)
	router.HandleFunc("/custom-metric-names", returnCustomMetricNames)
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./public/")))

	server := http.Server{}
	server.Addr = port
	server.Handler = handlers.CompressHandler(router)
	server.SetKeepAlivesEnabled(false)

	logger.Log("info", "API started on port "+port)
	log.Fatal(server.ListenAndServe())
}

func returnAgents(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/agents"
	handleRequest(url, w)
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/system?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/memory?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/swap?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/disks?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/proc?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/network?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnMemUsage(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/memusage?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnCPUUsage(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/cpuusage?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/processor-usage-historical?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/memory-historical?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnServices(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/services?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnCustom(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/custom?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func returnCustomMetricNames(w http.ResponseWriter, r *http.Request) {
	url := config.GetConfig("config.json").MonitorEndpoint + "/custom-metric-names?" + r.URL.Query().Encode()
	handleRequest(url, w)
}

func handleRequest(url string, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	var data interface{}
	res, _, str := send.SendGet(url)
	var out output
	if res {
		out.Status = "OK"
	} else {
		out.Status = "ERR"
	}
	_ = json.Unmarshal([]byte(str), &data)
	out.Data = data
	json.NewEncoder(w).Encode(&out)
}
