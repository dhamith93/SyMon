package main

import (
    "encoding/json"
    "fmt"
    "symon/config"
    "symon/monitor"
)

func main() {
    conf := config.Config{MonitorInterval: 5, LogFileEnabled: true, LogFilePath: "/path/to", DBPath: "/path/to"}

    byteArray, err := json.Marshal(conf)
    if err != nil {
        fmt.Println(err)
    }

    fmt.Printf("%+v\n", conf)
    fmt.Println(string(byteArray))

    system := monitor.GetSystem()

    fmt.Printf("%+v\n", system)
}
