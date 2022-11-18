package main

import (
	"sync"

	"github.com/dhamith93/SyMon/client/internal/server"
	"github.com/dhamith93/SyMon/internal/config"
)

func main() {
	config := config.GetClient()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		server.Run(":" + config.Port)
		wg.Done()
	}()
	wg.Wait()
}
