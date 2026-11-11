# Installing and running SyMon

This guide sets up SyMon on one Linux server and adds hosts to it. It covers upgrades, backups and the problems people run into most.

- [How the pieces fit](#how-the-pieces-fit)
- [Install the server](#install-the-server)
- [Add hosts](#add-hosts)
- [Alerts](#alerts)
- [Prometheus and Grafana](#prometheus-and-grafana)
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
| Agent | every monitored host | none | yes |

Agents send data to the collector on port 9000. The dashboard only talks to the collector. The collector sends alerts out itself, by email, to Slack, to PagerDuty or to a webhook, so it needs to reach those services. New hosts download the agent from the dashboard on port 8080, so a new host has to reach both ports.

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

The first `make pack-all` also creates `~/.config/symon/agent-signing.key` and signs the agent builds with it. Agents are built with its public key and only install updates from the dashboard that are signed with it, so keep the key on the machine you build on, never on the servers, and back it up. Without it, a new key means every agent needs one manual upgrade before the dashboard can update it again. `SIGNING_KEY=<file>` picks another location.

Copy the bundles to the server and extract them:

```sh
sudo mkdir -p /opt/symon /etc/symon
sudo tar -xf collector_linux_x86_64.tar.gz -C /opt/symon
sudo tar -xf client_linux_x86_64.tar.gz -C /opt/symon
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
sudo apt install -y postgresql-18 timescaledb-2-postgresql-18
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
# where new hosts download the agent, as they reach it, and where links
# in alert messages point
SYMON_DASHBOARD_URL=http://symon.example.lan:8080
```

Alert rules are edited on the dashboard, see [Alerts](#alerts).

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

The dashboard needs a login, and stays locked until the first user exists. Create one, which prints its password:

```sh
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -add-user admin
```

`-password-stdin` sets a password of your own instead, at least 12 characters. `-list-users`, `-reset-password <name>` and `-remove-user <name>` manage users. A login lasts 30 days, and a new password or a removed user logs that user out everywhere within a minute. After 10 wrong passwords a user name is locked for 15 minutes.

Put Caddy or nginx in front of the dashboard for HTTPS, so passwords and the session cookie are encrypted. The install script and agent downloads need no login, so new hosts can still enroll.

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
- **Containers.** Docker, Podman, containerd, LXC and systemd-nspawn containers are found from the cgroup tree. Names and images come from the Docker socket. For Podman, set `SYMON_CONTAINER_SOCKET=/run/podman/podman.sock`. Kubernetes pods are named from the container runtime, see [Kubernetes](#kubernetes).
- **Switching collectors off.** `SYMON_DISABLED_COLLECTORS=diskio,tcp,pressure,temps,containers` turns off any of those.
- **Custom metrics.** Send any number from a script or cron job, and it gets its own chart:

  ```sh
  sudo /opt/symon/agent/agent -custom -name=queue-length -unit=jobs -value=42
  ```

### Adding a host by hand

Where the install script cannot be used, for example on a host that cannot reach the dashboard, copy the agent from the client bundle's `downloads` folder. Write `/etc/symon/agent.env` with `SYMON_COLLECTOR_ENDPOINT=<server>:9000` and the shared `SYMON_KEY`. Then register the host once with `agent -init` and run `agent`.

## Alerts

Alert rules are edited on the dashboard's Rules page. The collector checks the rules that are on every 15 seconds, so a change applies right away, shows open alerts on the dashboard, and sends them to the [notification channels](#notifications) set up there.

A new install starts with one rule, "Host not reporting", which goes critical when any host sends nothing for 5 minutes. Rules can watch every host, including ones added later, or a list of hosts.

Only admins can change rules. Users are admins unless created with `-role viewer`, and `collector -set-role <name> -role viewer` changes it. Viewers see everything except the commands and output of [tasks](#tasks), and cannot change rules, update agents or run tasks.

### Rules as a file

The rules have the same format as the old `alerts.json`. On its first start after the upgrade to rules on the dashboard, the collector imports the file in `SYMON_ALERTS_CONFIG_PATH` once. After that the file is no longer read, and the setting can go. To keep rules in a file anyway, for example in Ansible, load it while the collector runs:

```sh
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -import-rules alerts.json
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -export-rules > alerts.json
```

`-import-rules` adds the rules and replaces the ones with the same name, and `-export-rules` prints the rules that are on. A file with a mistake is refused as a whole, with the line and column or the rule's name. Unknown fields count as mistakes, since they are almost always a misspelled one.

A rule looks like this:

```json
[
  {
    "Name": "Data disk",
    "Servers": ["*"],
    "MetricName": "disks",
    "Disk": "/dev/sda1",
    "Op": ">",
    "WarnThreshold": 80,
    "CriticalThreshold": 90,
    "TriggerIntveral": 120,
    "Template": "{subject}\n{serverName} {metricName} {value} {op} {expected} at {timestamp}\n{desc}",
    "Channels": ["ops email", "team slack"]
  }
]
```

| `MetricName` | Checks | Extra field |
|---|---|---|
| `procUsage` | CPU usage, % | |
| `memory` | memory used, % | |
| `swap` | swap used, % | |
| `disks` | disk space used, % | `Disk`, the device |
| `disk_forecast` | days until the disk is full, at its growth over the last week | `Disk`, the device |
| `services` | a service from the service list. `Op` `inactive` alerts when it stops, `active` when it runs | `Service`, the name from the service list |
| `ping` | host silent for longer than `TriggerIntveral` seconds | |
| `endpoint` | an HTTP check from the collector, no `Servers` | `Endpoint`, `Method`, `ExpectedHTTPCode`, `POSTBody`, `POSTContentType`, `CustomCACert`, `CertWarnDays`, `CertCriticalDays` |
| `kube_node`, `kube_workload`, `kube_pending`, `kube_restarts` | a Kubernetes cluster, see [Kubernetes](#kubernetes), no `Servers` | `Cluster`, `Namespace` |
| `device_down`, `device_port`, `device_metric` | an SNMP device, see [Devices](#devices-snmp), no `Servers` | `Device`, `Interface`, `DeviceMetric`, `Label` |
| any name, with `"IsCustom": true` | a custom metric | |

`Servers` lists host names, or `"*"` for every host. `Channels` names the [notification channels](#notifications) the rule's alerts go to; left out, they go to the default channels. `Op` is one of `>`, `<`, `>=`, `<=`, `==` or `!=`. A value has to stay past a threshold for `TriggerIntveral` seconds before the alert opens, and back to normal for as long before it resolves.

The collector requests each `endpoint` rule's URL every `SYMON_ENDPOINT_CHECK_INTERVAL` seconds (60 by default) and waits up to 30 seconds for an answer. `SYMON_ENABLE_ENDPOINT_MONITORING=false` stops that. A check passes when the status code is `ExpectedHTTPCode`, 200 if left out. The alert opens once checks have failed for `TriggerIntveral` seconds and resolves once they have passed for as long. Endpoint alerts belong to no host, and every check is kept as long as the 1 hour averages, for the response time and uptime charts on the dashboard's Endpoints page.

HTTPS endpoints also alert on their certificate: a warning when it expires in under `CertWarnDays` days (14 if left out) and critical under `CertCriticalDays` (3). 0 switches a level off. The Endpoints page shows when each certificate expires.

`disk_forecast` rules use `Op` `<`, for example a warning under 14 days and critical under 3. A forecast needs a day of history and steady growth, so a disk that fills and empties, like one with rotating logs, gets none. A disk that is not filling up counts as 365 days. The dashboard shows the forecast in the host's disk table and on a chart of where each filling disk is headed, and on the hosts page when a disk fills up within 30 days.

### Notifications

The collector sends every alert to notification channels when it opens, changes between warning and critical, and resolves. Admins set channels up on the dashboard: Alert rules, then Notification channels. Send test sends a message right away and shows what the service answered.

| Kind | Needs |
|---|---|
| Email | An SMTP server, its port (587 by default), a user and password if it asks for them, a from address and the addresses to send to. Port 465 uses TLS from the start, other ports switch to TLS when the server offers it, and a password is only sent over TLS. |
| Slack | A Slack app's bot token with the `chat:write` scope, and the channel, like `#ops`. Invite the app to the channel. Each alert is one message, updated when it changes and resolves. |
| PagerDuty | The integration key of an Events API v2 integration on a PagerDuty service. Each alert is one incident, resolved with the alert. |
| Webhook | A URL that gets every alert as JSON in a POST, and an optional header, like `Authorization: Bearer ...`. Any 2xx answer counts as delivered. |

Which channels hear about an alert:

- A rule picks channels in its editor. A rule that picks none goes to the channels marked **default**, which is how the built-in "Host not reporting" rule reaches someone.
- A channel can take **only critical alerts** and skip warnings.
- A resolved alert goes to the channels that heard about it.
- A channel that is off gets nothing.

Messages wait in the collector's database, so nothing is lost when a service is down or the collector restarts. A failed message is tried again after 30 seconds, 2, 10 and 30 minutes and an hour, then given up on. Messages about one alert go out in order. The Notifications page lists the newest messages, what became of them and the last error of each channel.

Set `SYMON_DASHBOARD_URL` on the collector to have messages link to the host's page.

The webhook body:

```json
{
  "event": "opened",
  "severity": "critical",
  "subject": "[Critical] CPU usage triggered on web1",
  "message": "web1 procUsage 97.20 > 95 at 2026-10-14 08:01:00 +0000 UTC",
  "host": "web1",
  "rule": "CPU usage",
  "metric": "procUsage",
  "target": "",
  "value": 97.2,
  "alertId": 42,
  "startedAt": "2026-10-14T08:01:00Z",
  "at": "2026-10-14T08:01:05Z",
  "link": "https://symon.example.com/hosts/web1"
}
```

`event` is `opened`, `changed`, `resolved` or `test`, and `severity` is `warning`, `critical`, `resolved` or `test`. `host` is empty for endpoint alerts, whose URL is in `target`, and for Kubernetes alerts, whose node or workload is in `target`.

Channel settings, including passwords, tokens, keys and webhook URLs, are kept in the collector's database. The dashboard never shows them again after saving: leave a secret empty when editing to keep it. Database backups contain them, so keep backups safe.

**Upgrading from the alert processor.** The collector now sends alerts itself, and the alert processor is gone. Add a channel on the dashboard for each service you used, with the credentials from `/etc/symon/alertprocessor.env`, then stop the alert processor and remove it: `sudo systemctl disable --now symon_alertprocessor`, `sudo rm /etc/systemd/system/symon_alertprocessor.service`. `SYMON_ALERT_ENDPOINT` can go from `collector.env`. Rules with the old `Email`, `Slack` or `Pagerduty` ticks keep working: they send to every channel of that kind. Saving such a rule on the dashboard turns the ticks into channel names. PagerDuty now uses an Events API v2 integration key instead of the REST API key.

## Tasks

A task is a command a host is willing to run when an admin asks for it on the dashboard, like restarting a service or clearing a cache. Each host has a Tasks page with its tasks, a Run button, and a history of who ran what and how it ended.

Tasks are listed on the host itself, in `/etc/symon/tasks.json`:

```json
[
  {
    "Name": "Restart nginx",
    "Description": "after a config change",
    "Command": "systemctl restart nginx"
  },
  {
    "Name": "Free disk space",
    "Command": "apt-get clean && journalctl --vacuum-time=7d",
    "TimeoutSeconds": 600
  }
]
```

```sh
sudo chown root:root /etc/symon/tasks.json
sudo chmod 644 /etc/symon/tasks.json
```

The tasks show up on the dashboard within a minute. The agent reads the file again for every run, so there is nothing to restart. [agent/tasks.json](../agent/tasks.json) is a sample.

- `Name` and `Command` are required. A name is unique on its host and up to 64 characters. `Description` is shown on the dashboard. `TimeoutSeconds` is how long a run may take: 300 unless set, 86400 at most.
- The command runs with `/bin/sh -c` as the user the agent runs as, which is root after the install script. It starts in `/`, gets no input, and does not see the agent's `SYMON_` settings.
- A host runs one task at a time.
- A run that goes over its timeout is killed, together with everything it started.
- Output shows on the dashboard while the task runs. The first 256 KB are kept.
- A run the agent does not pick up within 2 minutes is dropped, so nothing starts by surprise when a host comes back.
- A mistake in the file, like a misspelled field or a missing command, switches the host's tasks off until it is fixed. The agent's log says what is wrong: `journalctl -u symon_agent`.
- `SYMON_TASKS_PATH` in `/etc/symon/agent.env` points the agent at another file.
- The history is kept for 90 days. `SYMON_TASK_HISTORY_DAYS` in `/etc/symon/collector.env` changes that.

What keeps this safe:

- The dashboard can start the commands in the host's file, and cannot change them or add to them. The agent compares the command the admin confirmed with its file and refuses the run when they differ. So someone with a stolen dashboard password, or who got into the server, cannot run anything you did not list. They can run what you did list, so only list commands that are safe to run at any time.
- The agent ignores a tasks file that group or others can write to, or that belongs to neither root nor the agent's user. Whoever can write that file runs commands as the agent.
- Only admins see the commands and the output, and only admins start a task. Viewers see the tasks' names and the history.
- Every run is kept with the user who asked for it, and the collector logs it.

Tasks end when the agent stops, because systemd stops everything the agent started. So a task should not restart or upgrade the agent, and a job that has to outlive it belongs to systemd: `systemd-run --unit=nightly-backup /usr/local/bin/backup.sh`. Agent updates from the dashboard wait for a running task.

Tasks need an agent, collector and dashboard that know them. A host with an older agent shows no tasks.

## Kubernetes

SyMon watches a Kubernetes cluster whose nodes are machines the agent can be installed on, like kubeadm on VMs. Every node runs the agent like any other host, and it names the pods' containers. One node also reads the cluster's API and reports nodes, workloads, pods, restarts and warning events. The dashboard's Kubernetes page shows up once a cluster reports.

**1. An agent on every node.** Create one token for all the nodes, here three, and run the install command it prints on each:

```sh
sudo /opt/symon/collector_linux_x86_64/collector_linux_x86_64 -enroll-token -uses 3
```

Keep the hosts named after the machines, so the Kubernetes page links each node to its host page. The agent asks the container runtime's CRI socket which pod each container belongs to, and lists containers as `namespace/pod/container`, without the pause containers. That needs no Kubernetes access. containerd's socket is used by default. For CRI-O, add `SYMON_CRI_SOCKET=/var/run/crio/crio.sock` to `/etc/symon/agent.env`.

**2. A read-only account.** On a control-plane node, create the `symon-reader` service account from [kubernetes/symon-reader.yaml](kubernetes/symon-reader.yaml) and save its token for the agent:

```sh
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f symon-reader.yaml
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf -n symon get secret symon-reader-token -o jsonpath='{.data.token}' | base64 -d | sudo tee /etc/symon/kube.token > /dev/null
sudo chmod 600 /etc/symon/kube.token
```

The account may list nodes, pods, events, deployments, statefulsets and daemonsets, and nothing else. Its token does not expire. Deleting the `symon-reader-token` secret revokes it, and applying the file again makes a new one. The agent reads the token file on every round, so a new token works without a restart.

**3. Turn the reader on** on that node only, in `/etc/symon/agent.env`:

```sh
SYMON_KUBE_CLUSTER=homelab
SYMON_KUBE_API_URL=https://192.168.1.20:6443
```

`SYMON_KUBE_CLUSTER` is the cluster's name in SyMon. `SYMON_KUBE_API_URL` is the `server:` line of `/etc/kubernetes/admin.conf`. The agent checks the API's certificate against `/etc/kubernetes/pki/ca.crt` and reads the token from `/etc/symon/kube.token`. `SYMON_KUBE_CA_PATH` and `SYMON_KUBE_TOKEN_PATH` change those. Restart the agent with `sudo systemctl restart symon_agent`, and the cluster shows on the dashboard within a minute.

The reader lists the cluster every 30 seconds. The Kubernetes page shows:

- nodes, ready or not, with memory, disk or PID pressure, their version, CPU and memory
- deployments, statefulsets and daemonsets, with how many replicas are ready
- pods that are not running and ready, or restarted within the hour, with the reason, like CrashLoopBackOff or Unschedulable, and their CPU and memory from the node's agent
- charts of the pods by state and of container restarts over time
- every container restart of the last day, with why the run before ended, like OOMKilled
- warning events of the last hour

When the reader cannot list the cluster, for example with a wrong token, the page shows what the API said. When the reader stops reporting, the page says how old the state is.

**Alerts.** Four rule metrics watch a cluster. Each needs `Cluster`, the name from `SYMON_KUBE_CLUSTER`, and `Namespace` limits a rule to one namespace:

| `MetricName` | Checks each | Alerts when |
|---|---|---|
| `kube_node` | node | it is not Ready. `Namespace` does not apply |
| `kube_workload` | deployment, statefulset and daemonset | it has fewer replicas available than it wants |
| `kube_pending` | workload, and pod that belongs to none | its pods are stuck in Pending |
| `kube_restarts` | workload, and pod that belongs to none | its containers restarted more often in the last hour than the thresholds |

The first three take no thresholds and go critical once the problem has lasted `TriggerIntveral` seconds. Restart rules take `Op` and thresholds, like a warning above 3 and critical above 10. A message names the cluster and the node or workload, and what its pods report, like CrashLoopBackOff, Unschedulable or OOMKilled. An alert resolves when the problem is gone, and when its node or workload is deleted. A state older than 90 seconds is not checked, so alerts neither open nor resolve while the reader is down. The reader's host has its own "Host not reporting" alert.

```json
{
  "Name": "Crash loops",
  "MetricName": "kube_restarts",
  "Cluster": "homelab",
  "Namespace": "media",
  "Op": ">",
  "WarnThreshold": 3,
  "CriticalThreshold": 10,
  "TriggerIntveral": 60
}
```

**Moving the reader.** Only the host that first reported a cluster may report it. To read it from another node, take the `SYMON_KUBE_*` settings off the old node and restart its agent, press Remove cluster at the bottom of the Kubernetes page, then set up the new node as in steps 2 and 3. The cluster's charts and restart list start afresh.

**Upgrading.** Upgrade the collector and dashboard before the agents, as for any upgrade. An agent with `SYMON_KUBE_CLUSTER` that reports to an older collector logs that the collector is too old for clusters, and tries again every 10 minutes.

## Devices (SNMP)

SyMon polls switches, routers, UPSs, NAS boxes and other network gear over SNMP v2c. The agent on one host in their network reads them and reports to the collector, so the collector does not have to reach them. The dashboard's Devices page shows up once a device reports, with a page for each device.

**1. Check a device answers.** From the host that will poll it, with the net-snmp tools (`sudo apt install snmp`):

```sh
snmpwalk -v2c -c <community> 192.168.1.2 1.3.6.1.2.1.1
```

It should print the device's description and name. A timeout here means the agent will time out too: SNMP is off on the device, the community is wrong, or the device only answers some addresses.

**2. List the devices** on that host in `/etc/symon/snmp.json`, and make it readable by its owner only:

```json
[
  {
    "Name": "core-switch",
    "Address": "192.168.1.2",
    "Community": "public",
    "Profiles": ["interfaces"]
  },
  {
    "Name": "rack-ups",
    "Address": "192.168.1.3",
    "Community": "public",
    "Profiles": ["ups"]
  },
  {
    "Name": "nas",
    "Address": "192.168.1.4:161",
    "Community": "public",
    "Kind": "nas",
    "IntervalSeconds": 120,
    "Profiles": ["host", "interfaces"],
    "Metrics": [{ "Name": "system temperature", "OID": "1.3.6.1.4.1.6574.1.2.0", "Unit": "°C" }]
  }
]
```

```sh
sudo chmod 600 /etc/symon/snmp.json
```

Community strings work like passwords, so the agent refuses the file when group or others can read it, or when it belongs to neither root nor the agent's user. The communities never leave the host. The agent reads the file every round, so adding or removing a device needs no restart, and a new device shows on the dashboard within a minute. A mistake in the file stops all polling until it is fixed, and the agent logs which device and field is wrong. Unknown fields count as mistakes. `agent/snmp.json` is a sample.

| Field | |
|---|---|
| `Name` | the device on the dashboard and in alert rules. Up to 64 characters, without `/` |
| `Address` | an IP address or host name, with `:port` when it is not 161 |
| `Community` | the SNMP v2c community |
| `Profiles` | what to read: `interfaces`, `host` and `ups`, below |
| `Interfaces` | only these ports, by name. Left out, every physical port, without VLANs, loopbacks and tunnels |
| `Metrics` | other numbers to read, below |
| `Kind` | shown on the dashboard, like `nas` or `printer`. Left out, `ups`, `switch` or `server` from the profiles |
| `IntervalSeconds` | how often to poll this device, 10 to 3600. Left out, `SYMON_SNMP_INTERVAL_SECONDS` in `/etc/symon/agent.env`, 60 by default |

**Profiles.** Every device reports its description, name and uptime. On top of that:

- `interfaces` reads IF-MIB: traffic in and out in bits per second, errors per second, the link, the speed and the port's description. It uses the 64 bit counters when the device has them.
- `host` reads HOST-RESOURCES-MIB: CPU, memory used and space used on each volume. Most NAS boxes and Linux machines with snmpd have it, most switches do not.
- `ups` reads UPS-MIB, or APC's PowerNet MIB from an APC network card: on battery or line power, battery charge, minutes left, load, input voltage and battery temperature.

A profile the device does not have just shows nothing.

**Other numbers.** Each entry in `Metrics` reads one OID and gets its own chart. `Unit` is shown with it. `Scale` multiplies the value, like `0.1` for a device that counts in tenths of a degree. `"Counter": true` turns a number that only grows into a rate per second. The names cannot be the profiles' metric names below.

**The device's page** shows the UPS or system values when the device has them, and for ports a table with each port's link, speed, traffic and errors. Tick up to 8 ports to chart their traffic. It starts with the busiest ones, and the picked ports stay in the page's URL. An errors chart shows when a picked port had errors. When a device does not answer, the page says what the last poll said and keeps the last values it could read. When the polling agent stops, it says how old the last report is.

**Alerts.** Three rule metrics watch devices. Each needs `Device`, a name from the devices file, or `"*"` for every device, including ones added later:

| `MetricName` | Checks | Alerts when | Extra fields |
|---|---|---|---|
| `device_down` | each device | it does not answer | |
| `device_port` | one port of each device | the port is switched on and has no link | `Interface`, the port's name |
| `device_metric` | one value of each device | it is past the thresholds | `DeviceMetric`, and `Label` for one port, volume or line |

`device_down` and `device_port` take no thresholds and go critical once the problem has lasted `TriggerIntveral` seconds. `device_metric` takes `Op` and whole-number thresholds, and any metric name below or from `Metrics`. Without `Label`, each port or volume alerts on its own.

| Profile | Metric names |
|---|---|
| `interfaces` | `if_rx_bps` and `if_tx_bps` in bits per second, `if_rx_errors_ps`, `if_tx_errors_ps`, `if_up` (1 or 0), each with the port as label |
| `host` | `cpu_pct`, `mem_used_pct`, `storage_used_pct` with the volume as label |
| `ups` | `ups_on_battery` (1 or 0), `ups_charge_pct`, `ups_minutes_left`, `ups_load_pct`, `ups_input_volts`, `ups_battery_celsius` |

```json
{
  "Name": "UPS on battery",
  "MetricName": "device_metric",
  "Device": "rack-ups",
  "DeviceMetric": "ups_on_battery",
  "Op": "==",
  "WarnThreshold": 1,
  "CriticalThreshold": 1,
  "TriggerIntveral": 30
}
```

An alert resolves when the problem is gone, and when its device, port or value is no longer reported. A device that has not been reported for 3 of its intervals is not checked, so its alerts neither open nor resolve while the polling agent is down. That host has its own "Host not reporting" alert. With `SYMON_DASHBOARD_URL` set, messages link to the device's page.

**Moving the poller.** Only the host that first reported a device may report it. To poll it from another host, move its entry to that host's devices file and press Remove device at the bottom of the device's page. The new host takes it over on its next poll, and the device's charts start afresh. Removing the polling host from SyMon frees its devices the same way.

**Upgrading.** Upgrade the collector and dashboard before the agents, as for any upgrade. An agent with a devices file that reports to an older collector logs that the collector is too old for devices, and tries again every 10 minutes.

## Prometheus and Grafana

The dashboard serves every host's latest values at `/metrics` in the Prometheus format, so Grafana or an existing Prometheus can use them. Add it to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: symon
    scrape_interval: 15s
    static_configs:
      - targets: ["symon.example.lan:8080"]
```

Every value has a `host` label. Disks, interfaces, sensors, services and containers have their own labels too. Custom metrics sent in the last 2 days show as `symon_custom_metric` with `name` and `unit` labels, and `symon_custom_metric_timestamp_seconds` says when each was sent, so you can alert when a job stops reporting. `symon_up` is 0 for a host that stopped reporting, and its other values are left out until it reports again. `/metrics` needs no login unless `SYMON_CLIENT_METRICS_AUTH=true` is in `/etc/symon/client.env`. Then it takes a SyMon user's name and password as HTTP basic auth. Create a user for Prometheus and add it to the scrape config:

```yaml
    basic_auth:
      username: prometheus
      password_file: /etc/prometheus/symon-password
```

To switch `/metrics` off, add `SYMON_CLIENT_METRICS_ENABLED=false` to `/etc/symon/client.env`. Restart the dashboard after changing either.

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

Coming from a version without logins, the dashboard is locked after the upgrade until you create a user with `-add-user`, as in [Set up the dashboard](#4-set-up-the-dashboard).

Coming from a version with rules in `alerts.json`, the collector imports the file on its first start and adds the "Host not reporting" rule for every host. From then on rules are edited on the dashboard, see [Alerts](#alerts).

**Checking versions.** Every binary prints its version with `-version`, for example `/opt/symon/collector_linux_x86_64/collector_linux_x86_64 -version`, and logs it when it starts. The dashboard footer shows its own version, and the collector's too when they differ. The hosts page flags agents that are not on the dashboard's version.

**Hosts.** After the server upgrade, the hosts page offers to update every outdated agent, and each host's page has an Update agent button. Both are for admins. The agent picks the request up within a minute, downloads its build from the dashboard, and installs it only when it is signed with your key, runs on the host, and is the version asked for. Then it restarts, and the host's page shows the new version, or why it failed.

Agents from before this feature cannot update themselves yet. Run the install command on each of them once. No token is needed: the script sees the host is already enrolled, replaces the agent with the build the dashboard now serves, and keeps the key and the settings.

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
sudo systemctl disable --now symon_client symon_collector
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

**Kubernetes containers show as long IDs.** The agent cannot reach the CRI socket. `journalctl -u symon_agent` says why, once. Set `SYMON_CRI_SOCKET` for a runtime other than containerd.

**The Kubernetes page says it cannot read the cluster.** It shows what the API answered. A 401 means the token in `/etc/symon/kube.token` is wrong or its secret was deleted. A 403 means `symon-reader.yaml` was not applied. A certificate error means `SYMON_KUBE_CA_PATH` is not the cluster's CA, or `SYMON_KUBE_API_URL` uses a name the certificate does not cover.

**The cluster never shows up.** The reader logs why when the agent starts: `journalctl -u symon_agent | grep kubernetes`. Usually `SYMON_KUBE_API_URL` is not an `https://` URL, or there is no CA at `SYMON_KUBE_CA_PATH`.

**"another host reports this cluster".** Two nodes have `SYMON_KUBE_CLUSTER` set, or the reader moved. See [Moving the reader](#kubernetes).

**No devices show up.** On the polling host, `journalctl -u symon_agent | grep devices` says why: the file can be read by group or others, belongs to another user, or has a mistake. The agent logs it once, and again when it polls once more.

**A device shows as unreachable.** The page shows what the last poll said. A timeout means the device did not answer: SNMP is off, the community is wrong (most devices ignore a wrong one rather than refuse it), a firewall drops UDP port 161, or the device only answers some addresses. Try `snmpwalk` from the polling host, as in [Devices](#devices-snmp).

**A busy port shows too little traffic.** The device only has 32 bit counters, which wrap in about 34 seconds at 1 Gbps, so a poll a minute misses traffic. Set `IntervalSeconds` to 20 for that device.

**"another host polls this device".** Two hosts list the device, or the poller moved. See [Moving the poller](#devices-snmp).

**A container shows "host network".** It shares the host's network, so its traffic is already counted in the host's own network charts.

**The dashboard shows data only for part of a long range.** Longer ranges come from 1 minute and 1 hour averages, which are refreshed every few minutes. Recent data appears there shortly after it arrives.

**An agent update fails with "not signed with this agent's update key".** The dashboard's agent builds were signed with another key than the agent was built with, or not signed at all, for example after building on another machine. Rebuild with the original key, or run the install command on the host once to switch it to the new key.

**Forgot a password.** On the collector's host, `collector -reset-password <name>` prints a new one. Logged in users change their own on the Account page, which the user name in the header links to.

**The collector does not start after an upgrade, with an error about the alert rules.** On its first start it imports `SYMON_ALERTS_CONFIG_PATH`, and refuses a file with a mistake rather than dropping rules. Fix the line the error names, or unset the setting to start without the file's rules.

**"Too many failed logins".** That user name had 10 wrong passwords within 15 minutes. Wait 15 minutes, or reset the password.

## Settings reference

Every setting, with a comment on what it does, is in the component's `.env-example`, which also ships in each bundle:

- [collector/.env-example](../collector/.env-example)
- [client/.env-example](../client/.env-example)
- [agent/.env-example](../agent/.env-example)

TLS between the components is set up with the `*_TLS_*` and `*_CERT_PATH` settings. The collector needs a certificate and key, and agents and the dashboard need the CA file.
