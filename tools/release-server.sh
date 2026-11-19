#!/bin/sh
# Builds the server side of a release from the checked out tag, around the
# agents release-agents.sh built and signed: the dashboard, the collector and
# client for amd64 and arm64, their bundles, setup.sh and SHA256SUMS.
#
#   tools/release-server.sh v4.5.0
#
# Needs the signed agents in dist/agents. Writes the binaries the images are
# made from to dist/linux_<arch> and every release asset to dist/upload.
# Bundles keep the names make pack-* uses, so the install guide's paths and
# the systemd units work for both.
set -eu

VERSION=${1:?usage: release-server.sh vX.Y.Z}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
DIST=dist

for arch in amd64 arm64 arm; do
	for file in "agent-linux-$arch" "agent-linux-$arch.sig"; do
		if [ ! -f "$DIST/agents/$file" ]; then
			echo "there is no $DIST/agents/$file, the signed agents come from tools/release-agents.sh" >&2
			exit 1
		fi
	done
done

rm -rf "$DIST/linux_amd64" "$DIST/linux_arm64" "$DIST/bundle" "$DIST/upload"
mkdir -p "$DIST/bundle" "$DIST/upload"

(cd client/web && npm ci && npm run build)

LDFLAGS="-X github.com/dhamith93/SyMon/internal/version.Version=$VERSION"
for arch in amd64 arm64; do
	out="$DIST/linux_$arch"
	mkdir -p "$out"
	for component in collector client; do
		CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=false -ldflags "$LDFLAGS" -o "$out/$component" "./$component"
	done

	# the names make pack-* uses, x86_64 for amd64
	name=$arch
	[ "$arch" = amd64 ] && name=x86_64
	for component in collector client; do
		dir="${component}_linux_$name"
		mkdir -p "$DIST/bundle/$dir"
		cp "$out/$component" "$DIST/bundle/$dir/$dir"
		cp "$component/.env-example" "$DIST/bundle/$dir/"
		sed "s/_linux_x86_64/_linux_$name/g" "custom_scripts/symon_$component.service" >"$DIST/bundle/$dir/symon_$component.service"
	done
	cp collector/alerts.json "$DIST/bundle/collector_linux_$name/"
	mkdir -p "$DIST/bundle/client_linux_$name/downloads"
	cp "$DIST"/agents/agent-linux-* "$DIST/bundle/client_linux_$name/downloads/"
	for component in collector client; do
		tar -czf "$DIST/upload/${component}_linux_$name.tar.gz" -C "$DIST/bundle" "${component}_linux_$name"
	done
done

cp "$DIST"/agents/agent-linux-* "$DIST/upload/"
cp deploy/compose.yml "$DIST/upload/compose.yml"
# setup.sh carries the version and the compose file, so it is one download
awk -v version="$VERSION" '
	/^@COMPOSE@$/ { while ((getline line < "deploy/compose.yml") > 0) print line; next }
	{ gsub(/@VERSION@/, version); print }
' deploy/setup.sh >"$DIST/upload/setup.sh"
chmod 755 "$DIST/upload/setup.sh"

checksums() {
	if command -v sha256sum >/dev/null; then sha256sum -- "$@"; else shasum -a 256 -- "$@"; fi
}
(cd "$DIST/upload" && checksums * >../SHA256SUMS)
mv "$DIST/SHA256SUMS" "$DIST/upload/SHA256SUMS"
echo "release assets in $DIST/upload:"
ls -l "$DIST/upload"
