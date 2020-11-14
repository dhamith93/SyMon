package main

import (
	"log"
	"os"
	"symon/server"
	"symon/util"
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

	server.Run(":" + util.GetConfig().Port)
}
