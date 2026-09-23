package main

import (
	"flag"
	"log"
	"sync"

	"github.com/dhamith93/SyMon/client/internal/server"
	"github.com/dhamith93/SyMon/internal/config"
)

func main() {
	envFile := flag.String("env", config.DefaultEnvFile("client"), "Settings file with KEY=value lines, loaded if it exists")
	flag.Parse()
	if err := config.LoadEnvFile(*envFile); err != nil {
		log.Fatal(err)
	}

	config := config.GetClient()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		server.Run(":" + config.Port)
		wg.Done()
	}()
	wg.Wait()
}
