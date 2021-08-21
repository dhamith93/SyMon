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

* `.../system` 
* `.../memory` 
* `.../swap` 
* `.../disks` 
* `.../proc` 
* `.../network` 
* `.../memusage` 
* `.../cpuusage` 
* `.../processor-usage-historical` 
* `.../memory-historical` 