package server

import (
	"encoding/json"
	"fmt"
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
		log.Fatal(http.ListenAndServeTLS(port, util.GetConfig().SSLCertFilePath, util.GetConfig().SSLKeyFilePath, router))
	} else {
		log.Fatal(http.ListenAndServe(port, router))
	}
}

func Run(port string) {
	fmt.Println("API running on..." + port)
	handleRequests(port)
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnSystem -- " + ip)
	system := monitor.GetSystem()
	json.NewEncoder(w).Encode(system)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnMemory -- " + ip)
	memory := monitor.GetMemory()
	json.NewEncoder(w).Encode(memory)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnSwap -- " + ip)
	swap := monitor.GetSwap()
	json.NewEncoder(w).Encode(swap)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnDisks -- " + ip)
	disks := monitor.GetDisks()
	json.NewEncoder(w).Encode(disks)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnProc -- " + ip)
	proc := monitor.GetProcessor()
	json.NewEncoder(w).Encode(proc)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnNetwork -- " + ip)
	network := monitor.GetNetwork()
	json.NewEncoder(w).Encode(network)
}

func returnMemUsage(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnMemUsage -- " + ip)
	memUsage := monitor.GetProcessesSortedByMem()
	json.NewEncoder(w).Encode(memUsage)
}
func returnCPUUsage(w http.ResponseWriter, r *http.Request) {
	ip, err := util.GetIncomingIPAddr(r)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Endpoint Hit: returnCPUUsage -- " + ip)
	cpuUsage := monitor.GetProcessesSortedByCPU()
	json.NewEncoder(w).Encode(cpuUsage)
}
