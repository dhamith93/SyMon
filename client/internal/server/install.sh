#!/bin/sh
# Installs or upgrades the SyMon agent on this host, served by the SyMon
# dashboard. Get a token on the collector host with: collector -enroll-token
#
#   curl -fsSL {{.Dashboard}}/install.sh | sudo sh -s -- --token <token> [--host <name>]
#   curl -fsSL {{.Dashboard}}/install.sh | sudo sh -s -- --uninstall
#
# Running it again on an enrolled host upgrades the agent and keeps its key.
set -eu

DASHBOARD='{{.Dashboard}}'
COLLECTOR='{{.Collector}}'
INSTALL_DIR=/opt/symon/agent
SETTINGS=/etc/symon/agent.env
KEY=/etc/symon/agent.key
UNIT=/etc/systemd/system/symon_agent.service

fail() {
	echo "error: $*" >&2
	exit 1
}

has() {
	command -v "$1" >/dev/null 2>&1
}

token=""
host=""
uninstall=no
while [ $# -gt 0 ]; do
	case "$1" in
	--token)
		[ $# -ge 2 ] || fail "--token needs a value"
		token="$2"
		shift 2
		;;
	--host)
		[ $# -ge 2 ] || fail "--host needs a value"
		host="$2"
		shift 2
		;;
	--uninstall)
		uninstall=yes
		shift
		;;
	*) fail "unknown option $1" ;;
	esac
done

[ "$(id -u)" -eq 0 ] || fail "run this as root, for example with sudo"

if [ "$uninstall" = yes ]; then
	if has systemctl; then
		systemctl disable --now symon_agent >/dev/null 2>&1 || true
	fi
	rm -f "$UNIT" "$KEY" "$SETTINGS"
	rm -rf "$INSTALL_DIR"
	if has systemctl; then
		systemctl daemon-reload
	fi
	echo "Removed the SyMon agent."
	echo "To remove the host and its key from SyMon, run on the collector host: collector -remove-agent <name>"
	exit 0
fi

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
armv6l | armv7l | armv8l) arch=arm ;;
*) fail "there is no agent build for $(uname -m)" ;;
esac

download() {
	if has curl; then
		curl -fsSL "$1" -o "$2"
	elif has wget; then
		wget -q "$1" -O "$2"
	else
		fail "curl or wget is needed"
	fi
}

echo "Downloading the agent for linux/$arch from $DASHBOARD"
mkdir -p "$INSTALL_DIR" /etc/symon
download "$DASHBOARD/downloads/agent-linux-$arch" "$INSTALL_DIR/agent.new" || fail "cannot download the agent"
chmod 755 "$INSTALL_DIR/agent.new"
mv "$INSTALL_DIR/agent.new" "$INSTALL_DIR/agent"

# settings are only written once, so later edits survive upgrades
if [ ! -f "$SETTINGS" ]; then
	{
		echo "SYMON_COLLECTOR_ENDPOINT=$COLLECTOR"
		echo "SYMON_MONITOR_INTERVAL_SECONDS=15"
		if [ -n "$host" ]; then
			echo "SYMON_SERVER_ID=$host"
		fi
	} >"$SETTINGS"
	chmod 640 "$SETTINGS"
fi

if [ -f "$KEY" ]; then
	echo "This host is already enrolled, upgrading the agent"
elif [ -n "$token" ]; then
	if [ -n "$host" ]; then
		"$INSTALL_DIR/agent" -env "$SETTINGS" -enroll -token "$token" -host "$host"
	else
		"$INSTALL_DIR/agent" -env "$SETTINGS" -enroll -token "$token"
	fi
else
	fail "this host is not enrolled yet, pass --token (get one with: collector -enroll-token)"
fi

if ! has systemctl; then
	echo "systemd was not found. Start the agent with: $INSTALL_DIR/agent -env $SETTINGS"
	exit 0
fi

cat >"$UNIT" <<EOF
[Unit]
Description=SyMon Agent
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/agent -env $SETTINGS
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable symon_agent >/dev/null 2>&1
systemctl restart symon_agent
echo "The agent is running. This host shows up on $DASHBOARD within a minute."
