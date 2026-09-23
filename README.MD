# SyMon

[![Go](https://github.com/dhamith93/SyMon/actions/workflows/go.yml/badge.svg)](https://github.com/dhamith93/SyMon/actions/workflows/go.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/dhamith93/SyMon)](https://goreportcard.com/report/github.com/dhamith93/SyMon)

SyMon is a simple Linux system monitoring tool written in Go. Primarily created for monitoring smaller servers, home PCs, and devices like Raspberry Pi, but it is now extended to support full fledged system level monitoring on Linux systems. SyMon also supports custom time series data collection. Which can be used to collect sensor, or application output/performance data. 

Currently, it gathers following metrics under the monitoring interval given in the configuration file. 

* System Info
* CPU Info
* Memory/SWAP info
* CPU/Memory usage graphs for past hour (default) or custom time range
* Disk usage and usage graphs
* Network usage info and graphs
* Endpoint monitoring with support for GET and POST requests
* Whether a service is running or not
* 10 processes running on the system sorted by CPU or Memory usage
* Custom metrics (time series data) collection and graphs
* Showing metrics graphs in custom time/date ranges
* Point-in-time data loading of processes for CPU/Memory usage 
* Alert setup for CPU/Memory/SWAP/Disk/Services/Custom metrics 
    * Push alerts to user via Email, PagerDuty, or Slack
* Custom data retention period
* Disk IO and network throughput, TCP connection states, pressure stall information (PSI) and temperature sensors

### Screenshots

<img src="png/01.png" width="400" />
<img src="png/02.png" width="400" />
<img src="png/03.png" width="400" />
<img src="png/04.png" width="400" />
<img src="png/05.png" width="400" />
<img src="png/06.png" width="400" />
<img src="png/07.png" width="400" />

## Customizations
Communication between components (agents, collector, frontend, and alertprocessor) is done using gRPC. A custom service can be set up to read or push data into any of the components if needed. [API](internal/api/api.proto), [Alert API](internal/alertapi/alertapi.proto)

## Components
SyMon has four major components users can use to collect metrics and process alerts. 

[![](png/symon_components.png)](png/symon_components.png)

### Agent
Agent has to be installed in the system you want to monitor. Once it is installed and configured, it will collect and send out metrics to the Collector in a given collection interval. It also can be used to send out custom metrics to the Collector. Custom metrics can be used to collect application, process data, or sensor data periodically through cron job or script/application.

### Collector
The Collector is a required component of SyMon. It stores the data sent by agents in TimescaleDB (PostgreSQL with the TimescaleDB extension). Once set up, it can collect monitor data from multiple servers. It also checks alerts given by the alert config file. If an alert is raised, status changed, or resolved it will push them to the Alert processor. Raw data is kept for 7 days, 1 minute averages for 30 days and 1 hour averages for a year by default. These can be changed with the `SYMON_RETENTION_*` environment variables. 

### Alert Processor
Alert processor is an optional component of SyMon. You can skip the Alert processor if you wish only to collect and visualize metrics and custom data. However, if you wish to set up alerts, you can use the Alert processor to push out incoming alerts or alert status changes to Email, PagerDuty, and Slack. The UI element, or the Client component of SyMon also uses the Alert processor to get raised alerts. 

### Client
Client component is the user interface component of SyMon which is accessible from browsers. It shows every host with its status, usage and open alerts, and a page per host with charts for CPU (overall and per core), load, memory, pressure, disks, disk IO, network, TCP connections and temperatures, plus the top processes at any point in time. Charts share one time range, from 15 minutes to 30 days or a custom window, and dragging across a chart zooms in. There is also an alerts page with open alerts and history, and a page for each host's custom metrics. The dashboard follows the system light or dark theme, and every chart can be switched to a table. 

Currently the Client does not require any user authentication, but authentication and HTTPS can be set up using Apache or nginx. Which is recommended if this is used in public networks. 

## Local development

`docker compose up --build` starts TimescaleDB, the Collector, the Alert processor, the Client on http://localhost:8080 and one Agent. The Agent reports on its own container, not the host. The stack is for development only.

Building from source needs Go 1.26 or newer. `make build-all` builds every component, and `go test ./...` runs the tests.

## Installing and usage

### TLS support

Communication between Agent, Collector, Alertprocessor, and Client can be encrypted using TLS. Respective `.env-example` files can be referred to set up the server certificate, server private key (Collector, Alertprocessor), and CA file (Agents, Collector, Client) related environment variables.

Server certificate and the private key is needed with Collector and Alertprocessor if TLS is used. Agents and Clients need the CA file to communicate with them.

Collector also needs the CA file of the Alertprocessor's server certificate if TLS is used with the Alertprocessor to push the alerts to ther processor.

Communication between the Client and end user should be encrypted using Apache/nginx.

### Collector

First step is to install PostgreSQL with the [TimescaleDB extension](https://docs.timescale.com/self-hosted/latest/install/) and create a database and a user that owns it. A superuser then has to run `CREATE EXTENSION IF NOT EXISTS timescaledb;` in that database. Once it is set up, environment variables as per the `.env-example` file should be set up with the required configurations. `SYMON_DATABASE_URL` is the connection string, for example `postgres://symon:password@localhost:5432/symon`. `alerts.json` should be used to set up alerting rules. Setting up alert rules is optional and can be skipped if alerting is not used. 

Once everything is set up, you have to initialize the collector by running the below command. It will create the database tables and print the `key` string. Set it as the `SYMON_KEY` environment variable for every component. The collector also updates the database schema by itself when it starts.

`./collector_linux_x86_64 --init`

Once initialized, you can run the collector by running below command. It is best to set this up as a service. Example can be find [here](custom_scripts).

`./collector_linux_x86_64`

### Agents
The easiest way to add a host is to enroll it. On the collector host, create a token:

`sudo ./collector_linux_x86_64 -enroll-token`

It prints a command like the one below. Run it on the new host. It downloads the agent from the dashboard, enrolls the host, and installs and starts the agent as a systemd service.

`curl -fsSL http://symon:8080/install.sh | sudo sh -s -- --token <token>`

* Tokens work once and expire after an hour. `-uses 10` lets one token enroll ten hosts, `-ttl 24h` keeps it valid longer and `-host web-3` limits it to one host name.
* The host name is the machine's name unless `--host <name>` is passed to the script.
* Each host gets its own key in `/etc/symon/agent.key`, so the shared `SYMON_KEY` is not needed on it. Data sent with that key is always stored under that host.
* Running the command again on an enrolled host upgrades the agent. `--uninstall` removes it, and `collector -remove-agent <name>` removes the host and its key from SyMon.
* Agents are downloaded from the Client, which serves the builds `make pack-client` puts in its `downloads` folder (Linux amd64, arm64 and 32 bit ARM). Agents reach the collector through the dashboard's host name on the collector's port, unless `SYMON_CLIENT_AGENT_COLLECTOR_ENDPOINT` says otherwise.

An agent can also be set up by hand with the shared `SYMON_KEY`: fill in the variables from `.env-example`, register the host with `./agent_linux_x86_64 -init`, and run the agent, preferably as a service.

Custom metrics sent with `agent -custom` use the host's key too, so run them as root on enrolled hosts.

### Custom metrics
Custom time series data can be added using the agent as below. Current server time will be taken as the time. Running the below command with the parameters will send out the data to the Collector. This can be done manually, using a cron job, or through an application. 

`./agent_linux_x86_64 -custom -name=test -unit=byte -value=1.0`

`./agent_linux_x86_64 -custom -name='active-http-requests' -unit='requests' -value=$(netstat | grep -c https)`

### Alerts
This is an optional part of SyMon. It is only needed if you need to process alerts and push them out to Email, PagerDuty, and Slack. 

Set up the environment variables as shown in `.env-example` file with configurations and API credentials for PagerDuty/Slack and run the Alert processor.

`./alertprocessor_linux_x86_64`

### Clients
To run the Client, fill the relevent environment variables as shown in `.env-example` and start the Client with the below command. The dashboard is built into the binary. You can use Apache, or nginx to set up SSL and user authentication if needed.

`./client_linux_x86_64`

Building the Client from source needs Node.js 22 or newer for the dashboard: `make build-client` runs `make web` first. `npm run dev` in `client/web` serves the dashboard with live reload against a Client running on port 8080.

### Running as systemd services
`custom_scripts` has a unit per component. They expect each release bundle extracted to `/opt/symon` (for example `/opt/symon/collector_linux_x86_64/`) and read their settings from `/etc/symon/<component>.env`. Those files use plain `KEY=value` lines, without the `export` used in the `.env-example` files. The collector, alert processor and client run as a `symon` system user. The agent runs as root so it can read every process and check services.

### API documentation
The Client exposes a JSON API under `/api/v1`. Times are unix seconds. Errors return a JSON body `{"error": "..."}` with a 4xx or 5xx status.

* `GET /api/v1/fleet`
    * Every host with its status, latest usage and number of open alerts
* `GET /api/v1/hosts/{host}`
    * The host's latest snapshot as the agent sent it, with `up` and `lastSeen`
* `GET /api/v1/hosts/{host}/series?metric=cpu&from=&to=`
    * A metric over time. `from` and `to` default to the last hour. `label` keeps one series (a mount point, interface, sensor or custom metric name), `maxPoints` sets the most points per series (default 1000) and `max=1` returns the peak of each bucket instead of the average
    * Metrics: `cpu`, `cpu_core`, `load1`, `load5`, `load15`, `memory`, `memory_used`, `swap`, `swap_used`, `psi_cpu`, `psi_memory`, `psi_memory_full`, `psi_io`, `psi_io_full`, `tcp_established`, `tcp_time_wait`, `tcp_close_wait`, `tcp_listen`, `tcp_total`, `disk_used`, `disk_inodes`, `disk_read`, `disk_write`, `disk_util`, `net_rx`, `net_tx`, `temperature`, `custom`
* `GET /api/v1/hosts/{host}/processes?at=`
    * Top processes by CPU and by memory at or before `at`, or the latest
* `GET /api/v1/hosts/{host}/custom-metrics`
    * Names of the host's custom metrics
* `GET /api/v1/alerts?host=&open=1&from=&to=`
    * Alerts, newest first. `open=1` leaves out resolved ones
