package main

import (
	"flag"
	"log"
	"os"
	"symon/monitor"
	"symon/server"
	"symon/util"
	"sync"
	"os/signal"
	//"syscall"
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

	collectDataPtr := flag.Bool("collect", false, "Collects and saves data to the sqlite DB")
	serverEnablePtr := flag.Bool("server", false, "Starts the server")

	flag.Parse()

	// setup signal catching
	sigs := make(chan os.Signal, 1)

	// catch all signals since not explicitly listing
	signal.Notify(sigs)
	//signal.Notify(sigs,syscall.SIGQUIT)

	// method invoked upon seeing signal
	go func() {
	  s := <-sigs
	  log.Printf("RECEIVED SIGNAL: %s",s)
	  os.Exit(1)
	}()

	if *collectDataPtr {
		monitor.Monitor()
	} else {
		var wg sync.WaitGroup
		if *serverEnablePtr {
			wg.Add(1)
			go func() {
				server.Run(":" + util.GetConfig().Port)
				wg.Done()
			}()
		}

		wg.Wait()
	}

}
