#!/bin/sh
# Build the checked-out source; Herdr runs this once during installation.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
command -v go >/dev/null 2>&1 || { echo 'Pane Mover: install Go 1.26.8 or newer, then retry.' >&2; exit 1; }
# Use the installed compiler and this module, independent of a parent workspace.
export GOTOOLCHAIN=local GOWORK=off GOFLAGS= CGO_ENABLED=0
export GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)"
case "$GOOS" in darwin|linux) ;; *) echo 'Pane Mover supports macOS and Linux.' >&2; exit 1 ;; esac
mkdir -p bin
tmp=$(mktemp bin/.pane-mover.XXXXXX)
trap 'rm -f "$tmp"' EXIT
trap 'exit 1' HUP INT TERM
go build -mod=readonly -trimpath -buildvcs=false -o "$tmp" .
chmod 755 "$tmp"
mv -f "$tmp" bin/pane-mover
printf 'Built Pane Mover with %s\n' "$(go env GOVERSION)"
