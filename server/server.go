package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"symon/monitor"
	"symon/util"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
)

func checkAuth(endpoint func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header["Token"] != nil {
			token, err := jwt.Parse(r.Header["Token"][0], func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("Error with method")
				}
				return []byte(util.GetKey()), nil
			})

			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode("Unauthorized")
				util.Log("Auth Error", err.Error())
			}

			if token.Valid {
				endpoint(w, r)
			}
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode("Unauthorized")
			ip, _ := util.GetIncomingIPAddr(r)
			util.Log("Auth Error", "Unauthorized request from "+ip)
		}
	})
}

func handleRequests(port string) {
	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/isup", checkAuth(returnIsUp))
	router.Handle("/system", checkAuth(returnSystem))
	router.Handle("/memory", checkAuth(returnMemory))
	router.Handle("/swap", checkAuth(returnSwap))
	router.Handle("/disks", checkAuth(returnDisks))
	router.Handle("/proc", checkAuth(returnProc))
	router.Handle("/network", checkAuth(returnNetwork))
	router.Handle("/memusage", checkAuth(returnMemUsage))
	router.Handle("/cpuusage", checkAuth(returnCPUUsage))
	router.Handle("/processor-usage-historical", checkAuth(returnProcHistorical))
	router.Handle("/memory-historical", checkAuth(returnMemoryHistorical))
	router.Handle("/services", checkAuth(returnServices))

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

func returnIsUp(w http.ResponseWriter, r *http.Request) {
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	data := map[string]string{
		"time": unixTime,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func returnSystem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	system := monitor.System{}
	data := util.GetLogFromDBCount("system", 1)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &system)
	}
	json.NewEncoder(w).Encode(&system)
}

func returnMemory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	memory := monitor.Memory{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("memory", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &memory)
	}
	json.NewEncoder(w).Encode(&memory)
}

func returnSwap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	swap := monitor.Swap{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("swap", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &swap)
	}
	json.NewEncoder(w).Encode(&swap)
}

func returnDisks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	disks := []monitor.Disk{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("disks", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &disks)
	}
	json.NewEncoder(w).Encode(&disks)
}

func returnProc(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	proc := monitor.Processor{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("processor", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &proc)
	}
	json.NewEncoder(w).Encode(&proc)
}

func returnNetwork(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	network := []monitor.Network{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("network", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &network)
	}
	json.NewEncoder(w).Encode(&network)
}

func returnMemUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	memUsage := []monitor.Process{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("memUsage", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &memUsage)
	}
	json.NewEncoder(w).Encode(&memUsage)
}

func returnCPUUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cpuUsage := []monitor.Process{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("cpuUsage", from, to, time)
	if len(data) > 0 {
		_ = json.Unmarshal([]byte(data[0]), &cpuUsage)
	}
	json.NewEncoder(w).Encode(&cpuUsage)
}

func returnProcHistorical(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	procs := []monitor.ProcessorUsage{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)
	data := util.GetLogFromDB("processor", from, to, time)

	dataString := util.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &procs)
	json.NewEncoder(w).Encode(&procs)
}

func returnMemoryHistorical(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	memories := []monitor.Memory{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)

	data := util.GetLogFromDB("memory", from, to, time)

	dataString := util.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &memories)
	json.NewEncoder(w).Encode(&memories)
}

func returnServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	services := []monitor.Service{}
	data := []string{}
	time, _ := parseGETForTime(r)
	from, to, _ := parseGETForDates(r)

	if (from != 0 && to != 0) || time != 0 {
		data = util.GetLogFromDB("services", from, to, time)
	} else {
		data = util.GetLogFromDBCount("services", len(util.GetConfig().Services))
	}

	dataString := util.StringArrToJSONArr(data)

	_ = json.Unmarshal([]byte(dataString), &services)
	json.NewEncoder(w).Encode(&services)
}

func parseGETForTime(r *http.Request) (int64, error) {
	timeArr, ok := r.URL.Query()["time"]

	if !ok {
		return 0, fmt.Errorf("Error parsing GET vars")
	}

	timeInt, err := strconv.ParseInt(timeArr[0], 10, 64)

	if err != nil {
		return 0, fmt.Errorf("Error parsing GET vars")
	}

	return timeInt, nil
}

func parseGETForDates(r *http.Request) (int64, int64, error) {
	from, okFrom := r.URL.Query()["from"]
	to, okTo := r.URL.Query()["to"]

	if !okFrom || !okTo {
		return 0, 0, fmt.Errorf("Error parsing GET vars")
	}

	fromTime, err1 := strconv.ParseInt(from[0], 10, 64)
	toTime, err2 := strconv.ParseInt(to[0], 10, 64)

	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("Error parsing GET vars")
	}

	return fromTime, toTime, nil
}
