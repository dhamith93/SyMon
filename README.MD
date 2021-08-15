# SyMon
A simple system monitoring tool to monitor server HW stats. 

## Usage

In progress...

### Agent 
Collects metrics and send out to collector

#### AGENT ENV VARS
SYMON_SERVER_ID

### Collector
`./collector --init` creates a key

`./collector --init-agent <server-id>` creates db for agent

### API
In request header add `Key => {API_KEY}`

* `.../system` Returns system info
* `.../memory` Returns memory info
* `.../swap` Returns swap info
* `.../disks` returns json array of disk info
* `.../proc` Return CPU info
* `.../network` Return json array of network interface info
* `.../memusage` Return json array of 10 processes using most memory
* `.../cpuusage` Return json array of 10 processes using most CPU
* `.../processor-usage-historical` Return json array of ~100 historical CPU usage data points
* `.../memory-historical` Return json array of ~100 historical memory usage data points