package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"symon/monitor"

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
	fmt.Println("Endpoint Hit: returnSystem")
	system := monitor.GetSystem()
	json.NewEncoder(w).Encode(system)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Endpoint Hit: returnMemory")
	memory := monitor.GetMemory()
	json.NewEncoder(w).Encode(memory)
}
