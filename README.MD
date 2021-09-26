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

Agents can be removed by running `./collector --remove-agent <server-id>`

### Agents
Change the config-example.json file and save it as config.json. Server-Id should be unique between servers.

Run the agent.

### Clients
This is used as the frontend of the application. Change the config-example.json file and save it as config.json. 

Run the client and load localhost:port from a browser. Default localhost:9000.

It is recommended to use the client app internally.

### Custom metrics
Custom timeseries data can be added using the agent as below. Current server time will be taken as the time.

```bash
./agent -custom -name=test -unit=byte -value=1.0

# example
./agent -custom -name='active-http-requests' -unit='requests' -value=$(netstat | grep -c https)
```

This can be automated using a cron or a script

### API documentation
#### End points
* `/system`
    * Returns system info
* `/memory`
    * Returns memory info/usage as an array
```json
    ["timestamp", "used%", "total", "used", "free"]
```
* `/swap`
    * Returns swap info/usage as an array
```json
    ["timestamp", "used%", "total", "used", "free"]
```
* `/disks`
    * Return disk info/usage
```json
	{
	    Time: "time", 
	    Disks: [["file system", "mount point", "type", "size", "free", "used", "used%", "inodes", "inodes free", "inodes used", "inodes used%"]]
	}
```
* `/network`
    * Return disk info/usage
```json
	{
	    Time: "time", 
	    Networks: [["ip", "interface", "rx", "tx"]]
	}
```
* `/processes`
    * Returns list of processes using most CPU/RAM (10 each)
```json

    {
        CPU: [
            ["pid", "cpu", "mem", "command"],
            ["pid", "cpu", "mem", "command"]
        ],
		Memory: [
            ["pid", "cpu", "mem", "command"],
            ["pid", "cpu", "mem", "command"]
        ]
    }
```
* `/processor-usage-historical`
    * CPU load avg
```json
    ["timestamp", "load%"]
```
* `/memory-historical`
    * Memory usage
```json
    ["timestamp", "used%", "total", "used", "free"]
```
* `/services`
    * List of services enabled to monitor in agent's config.json
```json
    [{"Name":"name","Running":true,"Time":"1632680550"},]
```
* `/custom-metric-names`
    * Names of custom metrics
```json
    {CustomMetrics: ["active-http-requests",]}
```
* `/custom?custom-metric=name`
    * Custom metric values
```json
    [{"Name":"active-http-requests","ServerId":"id","Time":"time","Unit":"unit","Value":"x"},]
```
* Required GET parameters
    * `serverId=` ID of the server
* Optional GET parameters
    * `from=[unix timestamp]&to=[unix timestamp]` values within a time range
    * `time=[unix timestamp]` value(s) at a certain time (depends on availability of data)

#### API Authenticatication 
* Client app
    * Since the API call authentication is handled through the client app, API calls via the client app can be used without any authentication. 

* Collector app
    * To use with collector app, API calls must have JWT in header derived from the key file. 

## Todo
* Disk I/O graphs
* Alerts
* Dark mode
* Automatic data deletion past retention period