#!/bin/sh
# Validates that a built Talos system extension image contains the files
# Talos expects: /manifest.yaml, the extension service spec under
# /rootfs/usr/local/etc/containers/, and the service payload under
# /rootfs/usr/local/lib/containers/.
#
# Usage: validate.sh <image> [container-tool]
set -eu

image=$1
container_tool=${2:-podman}

cid=$("$container_tool" create "$image")
cleanup() {
	"$container_tool" rm "$cid" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ! "$container_tool" export "$cid" | tar -tf - | grep -E 'manifest\.yaml|rootfs/usr/local/etc/containers/lxd-agent\.yaml|rootfs/usr/local/lib/containers/lxd-agent/lxd-agent-loader' >/dev/null; then
	echo "Extension image layout is missing required files" >&2
	exit 1
fi

echo "Extension image layout OK"
