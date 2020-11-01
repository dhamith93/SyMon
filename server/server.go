package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"symon/monitor"

	"github.com/gorilla/mux"
)

func handleRequests() {
	myRouter := mux.NewRouter().StrictSlash(true)
	myRouter.HandleFunc("/system", returnSystem)
	log.Fatal(http.ListenAndServe(":5000", myRouter))
}

func Run() {
	fmt.Println("API started...")
	handleRequests()
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Endpoint Hit: returnSystem")
	system := monitor.GetSystem()
	json.NewEncoder(w).Encode(system)
}
