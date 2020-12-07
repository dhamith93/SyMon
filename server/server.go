package server

import (
	"encoding/json"
	"log"
	"net/http"
	"symon/monitor"
	"symon/util"

	"github.com/gorilla/mux"
)

func handleRequests(port string) {
	router := mux.NewRouter().StrictSlash(true)
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

	server := http.Server{}
	server.Addr = port
	server.Handler = router
	server.SetKeepAlivesEnabled(false)

	if util.GetConfig().SSLEnabled && util.GetConfig().SSLCertFilePath != "" && util.GetConfig().SSLKeyFilePath != "" {
		util.Log("info", "[SSL] API started on port "+port)
		log.Fatal(server.ListenAndServeTLS(util.GetConfig().SSLCertFilePath, util.GetConfig().SSLKeyFilePath))
	} else {
		util.Log("info", "API started on port "+port)
		log.Fatal(server.ListenAndServe())
	}
}

// Run starts the server in given port
func Run(port string) {
	util.GetKey()
	handleRequests(port)
}

func isAuthorized(w http.ResponseWriter, r *http.Request) bool {
	if len(r.Header["Key"]) != 0 && r.Header["Key"][0] == util.GetKey() {
		return true
	}
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode("")
	ip, _ := util.GetIncomingIPAddr(r)
	util.Log("warn", "Unauthorized request from "+ip)
	return false
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	system := monitor.System{}
	data := util.GetLogFromDB("system", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &system)
	}
	json.NewEncoder(w).Encode(&system)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	memory := monitor.Memory{}
	data := util.GetLogFromDB("memory", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &memory)
	}
	json.NewEncoder(w).Encode(&memory)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	swap := monitor.Swap{}
	data := util.GetLogFromDB("swap", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &swap)
	}
	json.NewEncoder(w).Encode(&swap)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	disks := []monitor.Disk{}
	data := util.GetLogFromDB("disks", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &disks)
	}
	json.NewEncoder(w).Encode(&disks)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	proc := monitor.Processor{}
	data := util.GetLogFromDB("processor", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &proc)
	}
	json.NewEncoder(w).Encode(&proc)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	network := []monitor.Network{}
	data := util.GetLogFromDB("network", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &network)
	}
	json.NewEncoder(w).Encode(&network)
}

func returnMemUsage(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	memUsage := []monitor.Process{}
	data := util.GetLogFromDB("memUsage", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &memUsage)
	}
	json.NewEncoder(w).Encode(&memUsage)
}

func returnCPUUsage(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	cpuUsage := []monitor.Process{}
	data := util.GetLogFromDB("cpuUsage", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &cpuUsage)
	}
	json.NewEncoder(w).Encode(&cpuUsage)
}

func returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	procs := []monitor.ProcessorUsage{}
	data := util.GetLogFromDB("processor", 100)

	dataString := util.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &procs)
	json.NewEncoder(w).Encode(&procs)
}

func returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	memories := []monitor.Memory{}
	data := util.GetLogFromDB("memory", 100)

	dataString := util.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &memories)
	json.NewEncoder(w).Encode(&memories)
}

func returnServices(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	services := []monitor.Service{}
	data := util.GetLogFromDB("services", len(util.GetConfig().Services))

	dataString := util.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &services)
	json.NewEncoder(w).Encode(&services)
}
