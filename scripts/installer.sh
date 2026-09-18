#!/bin/sh
# Builds a custom Talos installer with imager, combining the official
# extensions from the current schematic with the custom LXD agent extension.
#
# Registry credentials are staged into a container-readable file and passed
# through DOCKER_CONFIG, because the imager container does not run as root and
# cannot read a 0600 root-owned auth file mounted at /root/.docker/config.json.
#
# Usage: installer.sh <out-dir-abs> <container-tool> <auth-file>
#                      <imager-image> <arch> <base-installer> <extension>...
set -eu

out_dir=$1
shift
container_tool=$1
shift
auth_file=$1
shift
imager=$1
shift
arch=$1
shift
base_installer=$1
shift

mkdir -p "$out_dir"

auth_dir=""
auth_mount=""
auth_env=""
cleanup() {
	if [ -n "$auth_dir" ]; then
		rm -rf "$auth_dir"
	fi
}
trap cleanup EXIT INT TERM

if [ "$auth_file" != "-" ] && [ -f "$auth_file" ]; then
	auth_dir=$(mktemp -d)
	cp "$auth_file" "$auth_dir/config.json"
	chmod 0755 "$auth_dir"
	chmod 0644 "$auth_dir/config.json"
	auth_mount="--volume $auth_dir:/auth:ro"
	auth_env="--env DOCKER_CONFIG=/auth"
fi

ext_args=""
for ext in "$@"; do
	ext_args="$ext_args --system-extension-image $ext"
done

# The variables hold deliberately space-separated flag lists.
# shellcheck disable=SC2086
if "$container_tool" run --rm $auth_mount $auth_env --volume "$out_dir:/out" \
	"$imager" installer \
	--arch "$arch" \
	--platform metal \
	--base-installer-image "$base_installer" \
	$ext_args \
	--output /out
then
	exit 0
fi

echo "installer build failed" >&2
exit 1
