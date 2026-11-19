#!/bin/sh
# Sets up a SyMon @VERSION@ server with Docker: TimescaleDB, the collector
# and the dashboard.
#
#   curl -fsSL https://github.com/dhamith93/SyMon/releases/latest/download/setup.sh | sh
#   curl -fsSL .../setup.sh | sh -s -- --dir /srv/symon --url https://monitor.example.com
#
# It writes compose.yml and .env with new secrets into ./symon (or --dir),
# starts everything, and creates the dashboard user admin with a new
# password, which it prints. --url is how people and hosts reach the
# dashboard, the server's first address on port 8080 unless given. Run it as
# a user who can use docker.
set -eu

VERSION='@VERSION@'
DIR=./symon
URL=''

usage() {
	echo "usage: setup.sh [--dir DIR] [--url http://host:8080]" >&2
	exit 2
}

fail() {
	echo "setup: $*" >&2
	exit 1
}

# all in a function, so nothing runs before the whole script has arrived
# through the pipe
main() {
while [ $# -gt 0 ]; do
	case "$1" in
	--dir)
		[ $# -ge 2 ] || usage
		DIR=$2
		shift 2
		;;
	--url)
		[ $# -ge 2 ] || usage
		URL=$2
		shift 2
		;;
	*) usage ;;
	esac
done

command -v docker >/dev/null 2>&1 || fail "Docker is not installed, see https://docs.docker.com/engine/install/"
docker compose version >/dev/null 2>&1 || fail "the docker compose plugin is not installed"
docker info >/dev/null 2>&1 || fail "cannot reach Docker. Run this as root, or as a user in the docker group."

if [ -e "$DIR/.env" ]; then
	fail "$DIR is set up already. To upgrade, change SYMON_VERSION in $DIR/.env, then run: cd $DIR && docker compose pull && docker compose up -d"
fi

if [ -z "$URL" ]; then
	address=$(hostname -I 2>/dev/null | awk '{print $1}')
	[ -n "$address" ] || address=$(hostname)
	case "$address" in *:*) address="[$address]" ;; esac
	URL="http://$address:8080"
fi
case "$URL" in
http://* | https://*) ;;
*) fail "--url must start with http:// or https://" ;;
esac
URL=${URL%/}

# 32 random bytes as hex, safe in a URL and in .env
secret() {
	od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

mkdir -p "$DIR"
cd "$DIR"

cat >compose.yml <<'COMPOSE'
@COMPOSE@
COMPOSE

umask 077
cat >.env <<ENV
# SyMon server settings for docker compose. Keep this file private: it holds
# the key the collector and dashboard share and the database password.
SYMON_VERSION=$VERSION
# how people and hosts reach the dashboard, used in alert links
SYMON_DASHBOARD_URL=$URL
SYMON_DASHBOARD_PORT=8080
SYMON_COLLECTOR_PORT=9000
SYMON_KEY=$(secret)
POSTGRES_PASSWORD=$(secret)
ENV

echo "Starting SyMon $VERSION in $(pwd). The first start downloads the images."
docker compose up -d --wait </dev/null || fail "SyMon did not start. See what happened with: cd $(pwd) && docker compose logs"

# stdin is the script itself when piped, so docker gets none of it
if docker compose exec -T collector ./collector -list-users </dev/null | grep -q '^No users yet'; then
	echo
	docker compose exec -T collector ./collector -add-user admin </dev/null
fi

cat <<DONE

SyMon is running at $URL

- Log in, open Hosts and press Add host. It gives you one command to run on
  each host you want to monitor.
- Hosts need to reach this server on port 8080 (the dashboard and agent
  downloads) and 9000 (the collector). Open both in the firewall.
- Put HTTPS in front of the dashboard, so passwords are not sent in the
  clear. The install guide has a Caddy example:
  https://github.com/dhamith93/SyMon/blob/main/docs/install.md
- Settings and secrets are in $(pwd)/.env. Back it up with the database.
DONE
}

main "$@"
