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
	log.Fatal(http.ListenAndServe(port, router))
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
