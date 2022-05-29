# SyMon
A system monitoring and alerting tool to monitor server HW stats. Currently supports Linux systems. 

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
* Email and PagerDuty alert notifications

## Components
* Agent
    * Collects system metrics and pushes to the collector
* Collector
    * Collects system metrics from agents and stores them in MySQL/MariaDB database
    * Monitors the metrics for alerts. Sends alerts to alertprocessor
* Alertprocessor
    * Handles incoming alerts from collector
        * Sends alert to
            * Email addresses
            * PagerDuty
* Client
    * The user interface of the SyMon
    * Displays monitoring data through a web UI
    * Provides an API to get the server metrics

## Usage

### Collector
Install or set up a MySQL/MariaDB database for the collector. 

Change the config-example.json file and save it as config.json. Set up the env var `SYMON_MYSQL_PSWD` with the MySQL/MariaDB user name.

Init Collector by running `./collector --init`. It will create a key file. 

Copy the key file and place the key file with agents/clients.

Run the collector by executing `./collector`

Agents can be removed by running `./collector --remove-agent <server-id>`

### Agents
Change the config-example.json file and save it as config.json. Server-Id should be unique between servers.

Once the `config.json` file is configured, run the below command to register the agent with the collector.

```bash
./agent -init
```

Once the agent is registered, run the agent in background.

### Custom metrics
Custom timeseries data can be added using the agent as below. Current server time will be taken as the time.

```bash
./agent -custom -name=test -unit=byte -value=1.0

# example
./agent -custom -name='active-http-requests' -unit='requests' -value=$(netstat | grep -c https)
```

This can be automated using a cron or a script

### Alerts
SyMon alertprocessor supports internal alert management as well as email and PagerDuty alert notification management. 

[alerts.json](https://github.com/dhamith93/SyMon/blob/master/collector/alerts.json) file can be given when launching the collector as a set of rules to process and push alerts to the Alertprocessor. 

`./collector --config=config.json --alerts=alerts.json`

Alertprocessor holds and distributes alerts via email or as PagerDuty Incidents. [.env](https://github.com/dhamith93/SyMon/blob/master/alertprocessor/.env-example) file has to be set with Email and/or PagerDuty credentials to use that functionality. 

Client, or the frontend of SyMon also use the Alertprocessor to retrieve the ongoing alerts. 

### Clients
This is used as the frontend of the application. Change the config-example.json file and save it as config.json. 

Run the client and load localhost:port from a browser. Default localhost:9000.

It is recommended to use the client app internally.


### API documentation

#### API Authenticatication 
* Client app
    * Since the API call authentication is handled through the client app, API calls via the client app can be used without any authentication. 
#### End points
* `/system`
    * Returns system info
* `/memory`
    * Returns memory info/usage
* `/swap`
    * Returns swap info/usage
* `/disks`
    * Return disk info/usage per disk
* `/network`
    * Return disk info/usage per interface
* `/processes`
    * Returns list of processes using most CPU/RAM 
* `/processor-usage-historical`
    * CPU load avg
* `/memory-historical`
    * Memory usage
* `/services`
    * List of services enabled to monitor in agent's config.json
* `/custom-metric-names`
    * Names of custom metrics
* `/custom?custom-metric=name`
    * Custom metric values
* Required GET parameters
    * `serverId=` ID of the server
* Optional GET parameters
    * `from=[unix timestamp]&to=[unix timestamp]` values within a time range
    * `time=[unix timestamp]` value(s) at a certain time (depends on availability of data)

## Todo
* Automatic data deletion past retention period