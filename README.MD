# SyMon
A simple system monitoring tool to monitor server HW stats. 

## Collecting metrics
* System info
* CPU info
* Memory/SWAP info
* CPU/Memory usage graphs for past hour
* Disk usage info
* Primary network interface bandwidth metrics
* Network usage info
* Services Active/Inactive
* 10 most CPU/Memory using processes

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

## Todo
* Disk I/O graphs
* Custom metrics
* Alerts
* Dark mode
* Search metrics by date range