#!/bin/sh
# Starts a release. Run where the signing key is, never in CI: builds the
# agents from the pushed tag, signs them, uploads them to a draft GitHub
# release and starts the release workflow, which builds everything else from
# the same tag, checks the signatures and publishes.
#
#   make release-agents VERSION=v4.5.0
#
# Needs gh logged in with the repo and workflow scopes.
set -eu

VERSION=${1:?usage: release-agents.sh vX.Y.Z}
KEY=${SIGNING_KEY:-$HOME/.config/symon/agent-signing.key}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

fail() {
	echo "$*" >&2
	exit 1
}

echo "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$' || fail "VERSION must look like v4.5.0 or v4.5.0-rc.1"
[ -f "$KEY" ] || fail "there is no signing key at $KEY"
command -v gh >/dev/null || fail "gh is needed, https://cli.github.com"
gh auth status >/dev/null 2>&1 || fail "gh is not logged in, run: gh auth login"

# the tag has to be pushed, so CI builds the same source
tagged=$(git rev-parse -q --verify "refs/tags/$VERSION^{commit}") || fail "there is no tag $VERSION"
pushed=$(git ls-remote origin "refs/tags/$VERSION^{}" | cut -f1)
[ -n "$pushed" ] || pushed=$(git ls-remote origin "refs/tags/$VERSION" | cut -f1)
[ "$tagged" = "$pushed" ] || fail "the tag is not on GitHub as it is here, push it with: git push origin $VERSION"
if gh release view "$VERSION" >/dev/null 2>&1; then
	fail "there is a release $VERSION already"
fi

# agents carry the public key, and CI checks the signatures with the one in the repo
public=$(cat "$KEY.pub")
[ "$public" = "$(cat deploy/agent-signing.pub)" ] || fail "$KEY.pub is not the key in deploy/agent-signing.pub"

work=$(mktemp -d)
trap 'git worktree remove --force "$work/src" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
git worktree add --detach "$work/src" "$VERSION" >/dev/null
mkdir "$work/agents"

LDFLAGS="-X github.com/dhamith93/SyMon/internal/version.Version=$VERSION -X github.com/dhamith93/SyMon/internal/update.PublicKey=$public"
for arch in amd64 arm64 arm; do
	# GOARM=6 also runs on ARMv7, so one arm build covers every Raspberry Pi
	(cd "$work/src" && CGO_ENABLED=0 GOOS=linux GOARCH=$arch GOARM=6 \
		go build -trimpath -buildvcs=false -ldflags "$LDFLAGS" -o "$work/agents/agent-linux-$arch" ./agent)
done
agents="$work/agents/agent-linux-amd64 $work/agents/agent-linux-arm64 $work/agents/agent-linux-arm"
# shellcheck disable=SC2086 # the list is split on purpose
(cd "$work/src" && go run ./tools/sign sign -key "$KEY" $agents && go run ./tools/sign verify -pub deploy/agent-signing.pub $agents)

prerelease=""
case "$VERSION" in *-rc.*) prerelease="--prerelease" ;; esac
# shellcheck disable=SC2086
gh release create "$VERSION" --draft $prerelease --verify-tag --title "$VERSION" --generate-notes "$work"/agents/*
gh workflow run release.yml --ref "$VERSION" -f tag="$VERSION"
echo "Started the release workflow for $VERSION. Follow it with: gh run watch"
