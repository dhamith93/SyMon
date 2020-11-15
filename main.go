package main

import (
	"flag"
	"log"
	"os"
	"symon/display"
	"symon/server"
	"symon/util"
	"sync"
)

func main() {
	if util.GetConfig().LogFileEnabled {
		file, err := os.OpenFile(util.GetConfig().LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		log.SetOutput(file)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	displayEnablePtr := flag.Bool("display", false, "Show monitoring stats")
	serverEnablePtr := flag.Bool("server", true, "Starts the server")
	monitorPtr := flag.String("monitor", "", "Name of the server to monitor")

	flag.Parse()

	if *serverEnablePtr {
		go func() {
			server.Run(":" + util.GetConfig().Port)
			wg.Done()
		}()
	}

	if *displayEnablePtr && *monitorPtr != "" {
		display.Show("self")
	}

	if *monitorPtr != "" {
		display.Show(*monitorPtr)
	}
}
