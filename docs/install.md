# Installing and running SyMon

This guide sets up SyMon on one Linux server and adds hosts to it. It covers upgrades, backups and the problems people run into most.

- [How the pieces fit](#how-the-pieces-fit)
- [Install the server](#install-the-server)
- [Add hosts](#add-hosts)
- [Alerts](#alerts)
- [Upgrades](#upgrades)
- [Backups](#backups)
- [Uninstall](#uninstall)
- [Troubleshooting](#troubleshooting)
- [Settings reference](#settings-reference)

## How the pieces fit

| Component | Runs on | Port | Needed |
|---|---|---|---|
| Collector | the server | 9000, from every host | yes |
| Client (dashboard) | the server | 8080, from browsers and new hosts | yes |
| TimescaleDB | the server, or any PostgreSQL host | 5432, from the collector only | yes |
| Alert processor | the server | 5999, from the collector only | only for email, Slack or PagerDuty |
| Agent | every monitored host | none | yes |

Agents send data to the collector on port 9000. The dashboard only talks to the collector. New hosts download the agent from the dashboard on port 8080, so a new host has to reach both ports.

## Install the server

These steps use Ubuntu 24.04. Any systemd based Linux works, with its own package names.

### 1. Build the release bundles

On a machine with Go 1.26 and Node.js 22 or newer:

```sh
git clone https://github.com/dhamith93/SyMon.git
cd SyMon
make pack-all
```

`release/` then holds one bundle per component. The client bundle includes the agent builds that new hosts download: Linux amd64, arm64 and 32 bit ARM (every Raspberry Pi).

Copy the bundles to the server and extract them:

```sh
sudo mkdir -p /opt/symon /etc/symon
sudo tar -xf collector_linux_x86_64.tar.gz -C /opt/symon
sudo tar -xf client_linux_x86_64.tar.gz -C /opt/symon
sudo tar -xf alertprocessor_linux_x86_64.tar.gz -C /opt/symon   # only for alerts
sudo useradd --system --no-create-home --shell /usr/sbin/nologin symon
```

### 2. Install TimescaleDB

Follow the [TimescaleDB install guide](https://docs.timescale.com/self-hosted/latest/install/) for your distribution. On Ubuntu that is roughly:

```sh
sudo apt install -y gnupg postgresql-common apt-transport-https lsb-release wget
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y
echo "deb https://packagecloud.io/timescale/timescaledb/ubuntu/ $(lsb_release -cs) main" \
  | sudo tee /etc/apt/sources.list.d/timescaledb.list
wget -qO- https://packagecloud.io/timescale/timescaledb/gpgkey \
  | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/timescaledb.gpg
sudo apt update
sudo apt install -y timescaledb-2-postgresql-18
sudo timescaledb-tune --quiet --yes
sudo systemctl restart postgresql
```

Create a database and a user for SyMon. The extension has to be created by a superuser:

```sh
sudo -u postgres psql <<'SQL'
CREATE USER symon WITH PASSWORD 'change-me';
CREATE DATABASE symon OWNER symon;
\c symon
CREATE EXTENSION IF NOT EXISTS timescaledb;
SQL
```

### 3. Set up the collector

Settings live in `/etc/symon/<component>.env`, one `KEY=value` per line, without `export`. Every component reads its own file when it starts. `-env <file>` points it somewhere else.

`/etc/symon/collector.env`:

```sh
SYMON_PORT=9000
SYMON_DATABASE_URL=postgres://symon:change-me@localhost:5432/symon?sslmode=disable
# where new hosts download the agent, as they reach it
SYMON_DASHBOARD_URL=http://symon.example.lan:8080
SYMON_ALERTS_CONFIG_PATH=/etc/symon/alerts.json
# only with the alert processor
SYMON_ALERT_ENDPOINT=localhost:5999
```

Alert rules go in `/etc/symon/alerts.json`, described under [Alerts](#alerts). The bundle has an example. Leave `SYMON_ALERTS_CONFIG_PATH` out to run without alerts.

Create the database tables and a shared key:

```sh
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -init
```

It prints `SYMON_KEY: ...`. Add that line as `SYMON_KEY=...` to every file in `/etc/symon`. The components use it to talk to each other. Hosts added by enrollment get their own key and do not need it.

Then protect the settings, since they hold the key and the database password:

```sh
sudo chown root:symon /etc/symon/*.env
sudo chmod 640 /etc/symon/*.env
```

### 4. Set up the dashboard

`/etc/symon/client.env`:

```sh
SYMON_CLIENT_PORT=8080
SYMON_CLIENT_COLLECTOR_ENDPOINT=localhost:9000
SYMON_KEY=...
```

The dashboard has no login. Keep it on a private network, or put nginx or Apache in front of it for HTTPS and a password.

### 5. Start everything

The systemd units are in [custom_scripts](../custom_scripts) in the repository. Copy them to the server, then:

```sh
sudo cp custom_scripts/symon_collector.service custom_scripts/symon_client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now symon_collector symon_client
```

Open port 8080 to browsers and new hosts and port 9000 to monitored hosts. With ufw:

```sh
sudo ufw allow 8080/tcp
sudo ufw allow 9000/tcp
```

The dashboard is now at `http://<server>:8080`. It stays empty until a host is added.

## Add hosts

On the server, create an enrollment token:

```sh
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -enroll-token
```

It prints a command. Run it on the host you want to monitor:

```sh
curl -fsSL --connect-timeout 10 http://symon.example.lan:8080/install.sh | sudo sh -s -- --token <token>
```

The script downloads the agent for the host's CPU, enrolls it, and starts it as the `symon_agent` systemd service. The host appears on the dashboard within a minute.

- A token works once and expires after an hour. `-uses 10` lets one token enroll ten hosts, `-ttl 24h` keeps it valid longer, and `-host web-3` limits it to one host name.
- The host is named after the machine unless you add `--host <name>` to the script.
- The agent keeps its own key in `/etc/symon/agent.key`. Data sent with that key is always stored under that host.
- Agent settings are in `/etc/symon/agent.env`. The script writes the file once and never overwrites it, so your edits survive upgrades. Restart the agent after changing it: `sudo systemctl restart symon_agent`.

### What the agent collects

Out of the box: CPU (overall and per core), load, memory and swap, disks, disk IO, network, TCP connections, pressure stall, temperatures, the top processes, and running containers.

- **Services.** Point `SYMON_SERVICE_LIST_PATH` at a JSON file listing systemd services to watch, like [agent/services.json](../agent/services.json).
- **Containers.** Docker, Podman, containerd, LXC and systemd-nspawn containers are found from the cgroup tree. Names and images come from the Docker socket. For Podman, set `SYMON_CONTAINER_SOCKET=/run/podman/podman.sock`.
- **Switching collectors off.** `SYMON_DISABLED_COLLECTORS=diskio,tcp,pressure,temps,containers` turns off any of those.
- **Custom metrics.** Send any number from a script or cron job, and it gets its own chart:

  ```sh
  sudo /opt/symon/agent/agent -custom -name=queue-length -unit=jobs -value=42
  ```

### Adding a host by hand

Where the install script cannot be used, for example on a host that cannot reach the dashboard, copy the agent from the client bundle's `downloads` folder. Write `/etc/symon/agent.env` with `SYMON_COLLECTOR_ENDPOINT=<server>:9000` and the shared `SYMON_KEY`. Then register the host once with `agent -init` and run `agent`.

## Alerts

The collector checks the rules in `SYMON_ALERTS_CONFIG_PATH` and shows open alerts on the dashboard. To also send them by email, Slack or PagerDuty, run the alert processor.

A rule looks like this:

```json
[
  {
    "Name": "Data disk",
    "Servers": ["web-1", "web-2"],
    "MetricName": "disks",
    "Disk": "/dev/sda1",
    "Op": ">",
    "WarnThreshold": 80,
    "CriticalThreshold": 90,
    "TriggerIntveral": 120,
    "Template": "{subject}\n{serverName} {metricName} {value} {op} {expected} at {timestamp}\n{desc}",
    "Email": false,
    "Slack": true,
    "SlackChannel": "#ops",
    "Pagerduty": false
  }
]
```

| `MetricName` | Checks | Extra field |
|---|---|---|
| `procUsage` | CPU usage, % | |
| `memory` | memory used, % | |
| `swap` | swap used, % | |
| `disks` | disk space used, % | `Disk`, the device |
| `services` | a service from the service list. `Op` `inactive` alerts when it stops, `active` when it runs | `Service`, the name from the service list |
| `ping` | host silent for longer than `TriggerIntveral` seconds | |
| `endpoint` | an HTTP check from the collector | `Endpoint`, `Method`, `ExpectedHTTPCode`, `POSTBody`, `POSTContentType` |
| any name, with `"IsCustom": true` | a custom metric | |

`Op` is one of `>`, `<`, `>=`, `<=`, `==` or `!=`. A value has to stay past a threshold for `TriggerIntveral` seconds before the alert opens, and back to normal for as long before it resolves. Endpoint checks need `SYMON_ENABLE_ENDPOINT_MONITORING=true` on the collector.

The collector reads the rules when it starts, so restart it after editing them.

### The alert processor

`/etc/symon/alertprocessor.env` holds the port, the shared key, and the credentials for each channel you use (see [alertprocessor/.env-example](../alertprocessor/.env-example)):

```sh
SYMON_ALERT_PORT=5999
SYMON_KEY=...
SLACK_TOKEN=xoxb-...
EMAIL_HOST=smtp.example.com
EMAIL_PORT=587
EMAIL_USER=...
EMAIL_PASSWORD=...
EMAIL_FROM=symon@example.com
ALERT_EMAIL_LIST=ops@example.com
```

```sh
sudo cp custom_scripts/symon_alertprocessor.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now symon_alertprocessor
```

## Upgrades

Upgrade the server first, then the hosts.

**Server.** Build the new bundles and extract them over the old ones. Settings in `/etc/symon` are not touched. Then restart:

```sh
sudo systemctl stop symon_client symon_collector
sudo tar -xf collector_linux_x86_64.tar.gz -C /opt/symon
sudo tar -xf client_linux_x86_64.tar.gz -C /opt/symon
sudo systemctl start symon_collector symon_client
```

The collector updates the database schema by itself when it starts. Back up the database first (see below) if you want a way back, because schema changes are not undone by going back to an older build.

**Hosts.** Run the install command again on each host. No token is needed: the script sees the host is already enrolled, replaces the agent with the build the dashboard now serves, and keeps the key and the settings.

```sh
curl -fsSL --connect-timeout 10 http://symon.example.lan:8080/install.sh | sudo sh
```

## Backups

Everything SyMon knows is in the database, apart from the files in `/etc/symon`.

```sh
sudo -u postgres pg_dump -Fc symon > symon.dump
sudo tar -czf symon-etc.tar.gz /etc/symon
```

TimescaleDB needs two extra calls when restoring into a fresh database:

```sh
sudo -u postgres psql -d symon -c "CREATE EXTENSION IF NOT EXISTS timescaledb; SELECT timescaledb_pre_restore();"
sudo -u postgres pg_restore -d symon symon.dump
sudo -u postgres psql -d symon -c "SELECT timescaledb_post_restore();"
```

Data is kept for a limited time, so a backup is mostly about the host list, keys and alert history. By default raw data is kept for 7 days, 1 minute averages for 30 days and 1 hour averages for a year. The `SYMON_RETENTION_*` settings change that.

## Uninstall

**A host.** On the host:

```sh
curl -fsSL --connect-timeout 10 http://symon.example.lan:8080/install.sh | sudo sh -s -- --uninstall
```

Then remove it from the dashboard and revoke its key on the server:

```sh
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -remove-agent <name>
```

Its history stays until retention drops it.

**The server.**

```sh
sudo systemctl disable --now symon_client symon_collector symon_alertprocessor
sudo rm /etc/systemd/system/symon_*.service
sudo rm -rf /opt/symon /etc/symon
sudo -u postgres dropdb symon
sudo -u postgres dropuser symon
```

## Troubleshooting

**The install command prints nothing, or times out.** The host cannot reach the dashboard. Check it with `curl -I http://<server>:8080/install.sh`. If the name does not resolve, use the server's IP address, and set `SYMON_DASHBOARD_URL` to the IP so the printed command uses it too. If it hangs, a firewall is blocking port 8080.

**The host enrolled but never shows up.** The agent cannot reach the collector on port 9000. See `journalctl -u symon_agent`. The agent reaches the collector through the dashboard's host name. If that is wrong for your network, set `SYMON_CLIENT_AGENT_COLLECTOR_ENDPOINT=<address>:9000` on the dashboard and enroll again, or fix `SYMON_COLLECTOR_ENDPOINT` in the host's `/etc/symon/agent.env`.

**"unknown agent key, enroll this host again".** The host was removed with `-remove-agent`, or the database was replaced. Delete `/etc/symon/agent.key` on the host and run the install command with a new token.

**The collector fails on start with an error about a missing TimescaleDB function or extension.** The TimescaleDB extension is missing from the database, or the database URL sets a `search_path` without `public`. Run `CREATE EXTENSION timescaledb;` as a superuser, and keep `public` in any `search_path`.

**No containers on a host that runs them.** Check `journalctl -u symon_agent` for the reason, which is logged once. Containers show without names when the Docker socket cannot be read. Point `SYMON_CONTAINER_SOCKET` at the right socket for Podman or a rootless Docker.

**A container shows "host network".** It shares the host's network, so its traffic is already counted in the host's own network charts.

**The dashboard shows data only for part of a long range.** Longer ranges come from 1 minute and 1 hour averages, which are refreshed every few minutes. Recent data appears there shortly after it arrives.

## Settings reference

Every setting, with a comment on what it does, is in the component's `.env-example`, which also ships in each bundle:

- [collector/.env-example](../collector/.env-example)
- [client/.env-example](../client/.env-example)
- [agent/.env-example](../agent/.env-example)
- [alertprocessor/.env-example](../alertprocessor/.env-example)

TLS between the components is set up with the `*_TLS_*` and `*_CERT_PATH` settings. The collector and alert processor need a certificate and key. Agents, the dashboard and the collector (for the alert processor) need the CA file.
