# SyMon
A simple system monitoring tool to monitor local and remote systems. 

## Usage

Before use, rename or copy `config-example.json` as `config.json` and modify required options

Then add the exec path with `-collect` parameter to crontab with required monitoring interval.

Ex:

```sh
# Collect monitoring data every minute
*/1 * * * * cd /root/source/SyMon && ./symon -collect > /dev/null 2>&1
```

If you are connecting to remote servers, rename or copy `remote-example.json` as `remote.json` and enter relevent server info. Multiple servers can be set up in the `remote.json`.

`key` file will be generated upon the first run. It will contain the API key to connect to the server.

### Email notifications
Make sure all the configurations are properly set in the config.json. 
Also make sure the connectivity to the SMTP server is allowed from system and network level.
Rename `.env-example` as `.env` with email creds. 

### Execution
* `-server` Start the server. Default `false`
* `-display` Shows the TUI with stats for local machine. Default `false`
* `-monitor=name` Shows the TUI with stats for selected remote server. Requires a valid `remote.json` file

### Keyboard shortcuts on TUI
* `d` Scroll down on disks 
* `D` Scroll up on disks
* `n` Scroll down on networks
* `N` Scroll up on networks
* `p` Switch between process list sorting (memory usage / cpu usage)
* `q` Quit TUI

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

## Screenshot
![](screenshots/screenshot_01.png)