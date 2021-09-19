# SyMon
A simple system monitoring tool to monitor server HW stats. 

[![Go](https://github.com/dhamith93/SyMon/actions/workflows/go.yml/badge.svg)](https://github.com/dhamith93/SyMon/actions/workflows/go.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/dhamith93/SyMon)](https://goreportcard.com/report/github.com/dhamith93/SyMon)

## Collecting metrics and features
* System info
* CPU info
* Memory/SWAP info
* CPU/Memory usage graphs for past hour
* Disk usage info
* Primary network interface bandwidth metrics
* Network usage info
* Services Active/Inactive
* 10 most CPU/Memory using processes
* Custom timeseries metrics
* Search metrics by date range
* Point-in-time metrics loading

## Usage

### Collector
Change the config-example.json file and save it as config.json.

Init Collector by running `./collector --init`. It will create a key file. 

Copy the key file and place the key file with agents/clients.

Run the collector by executing `./collector`

Agents can be added to collector by running `./collector --init-agent <server-id>`

### Agents
Change the config-example.json file and save it as config.json. Server-Id should be unique between servers.

Run the agent.

### Clients
This is used as the frontend of the application. Change the config-example.json file and save it as config.json. 

Run the client and load localhost:port from a browser. Default localhost:9000.

### Custom metrics
Custom timeseries data can be added using the agent as below. Current server time will be taken as the time.

```bash
./agent -custom -name=test -unit=byte -value=1.0
```

## Todo
* Disk I/O graphs
* Alerts
* Dark mode