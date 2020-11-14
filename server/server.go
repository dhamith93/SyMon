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

	if util.GetConfig().SSLEnabled && util.GetConfig().SSLCertFilePath != "" && util.GetConfig().SSLKeyFilePath != "" {
		util.Log("info", "[SSL] API started on port "+port)
		log.Fatal(http.ListenAndServeTLS(port, util.GetConfig().SSLCertFilePath, util.GetConfig().SSLKeyFilePath, router))
	} else {
		util.Log("info", "API started on port "+port)
		log.Fatal(http.ListenAndServe(port, router))
	}
}

func Run(port string) {
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
	system := monitor.GetSystem()
	json.NewEncoder(w).Encode(system)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	memory := monitor.GetMemory()
	json.NewEncoder(w).Encode(memory)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	swap := monitor.GetSwap()
	json.NewEncoder(w).Encode(swap)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	disks := monitor.GetDisks()
	json.NewEncoder(w).Encode(disks)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	proc := monitor.GetProcessor()
	json.NewEncoder(w).Encode(proc)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	network := monitor.GetNetwork()
	json.NewEncoder(w).Encode(network)
}

func returnMemUsage(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	memUsage := monitor.GetProcessesSortedByMem()
	json.NewEncoder(w).Encode(memUsage)
}

func returnCPUUsage(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	cpuUsage := monitor.GetProcessesSortedByCPU()
	json.NewEncoder(w).Encode(cpuUsage)
}
