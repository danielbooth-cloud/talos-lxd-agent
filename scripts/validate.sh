#!/bin/sh
# Validates that a built Talos system extension image contains the files
# Talos expects: /manifest.yaml, the extension service spec under
# /rootfs/usr/local/etc/containers/, and the service payload under
# /rootfs/usr/local/lib/containers/.
#
# It also asserts the image is linux/amd64 and that the shipped loader is a
# static amd64 ELF, so a wrong --platform build fails loudly instead of
# silently producing an unusable extension.
#
# Usage: validate.sh <image> [container-tool] [expected-arch]
set -eu

image=$1
container_tool=${2:-podman}
expected_arch=${3:-amd64}

cid=$("$container_tool" create "$image")
cleanup() {
	"$container_tool" rm "$cid" >/dev/null 2>&1 || true
	rm -rf "$tmpdir"
}
tmpdir=$(mktemp -d)
trap cleanup EXIT

"$container_tool" export "$cid" >"$tmpdir/image.tar"

for path in \
	manifest.yaml \
	rootfs/usr/local/etc/containers/lxd-agent.yaml \
	rootfs/usr/local/lib/containers/lxd-agent/lxd-agent-loader
do
	if ! tar -tf "$tmpdir/image.tar" | grep -qx "$path"; then
		echo "Extension image is missing required file: $path" >&2
		exit 1
	fi
done

image_arch=$("$container_tool" image inspect "$image" --format '{{.Architecture}}')
if [ "$image_arch" != "$expected_arch" ]; then
	echo "Extension image architecture is $image_arch, expected $expected_arch" >&2
	exit 1
fi

tar -xf "$tmpdir/image.tar" -C "$tmpdir" rootfs/usr/local/lib/containers/lxd-agent/lxd-agent-loader
loader="$tmpdir/rootfs/usr/local/lib/containers/lxd-agent/lxd-agent-loader"

if ! file "$loader" | grep -q 'ELF 64-bit LSB executable, x86-64'; then
	echo "Loader is not an x86-64 ELF binary: $(file "$loader")" >&2
	exit 1
fi

if ! file "$loader" | grep -q 'statically linked'; then
	echo "Loader is not statically linked: $(file "$loader")" >&2
	exit 1
fi

echo "Extension image layout OK (linux/$expected_arch, static amd64 loader)"
