# talos-lxd-agent (experimental)

A custom [Talos Linux system extension](https://docs.siderolabs.com/talos/v1.13/build-and-extend-talos/custom-images-and-development/extension-services/)
that runs LXD's VM guest agent (`lxd-agent`) on Talos, creating the `/dev/lxd/sock`
DevLXD socket required by the [Canonical LXD CSI driver](https://github.com/canonical/lxd-csi-driver).

Talos ships no `lxd-agent` support (it is not a systemd guest and no official
extension exists), so the LXD CSI driver cannot provision filesystem volumes on
Talos VMs without this piece. This extension deliberately does **not** bundle a
copy of `lxd-agent`: the LXD host supplies a version-matched agent binary and
certificates through the VM's read-only `config` share, exactly like LXD does
for standard systemd guests.

## How it works

The extension deploys one Talos extension service (`ext-lxd-agent`) whose
container runs a small static Go loader (`cmd/lxd-agent-loader`):

1. Mounts a tmpfs on `/mnt` with shared propagation (see below).
2. Mounts LXD's `config` share (tag `config`) at `/run/lxd_agent/.mnt`
   using virtiofs, falling back to 9p.
3. Copies the share contents (agent binary, agent.conf, certificates) into
   `/run/lxd_agent`, then unmounts and removes the mountpoint.
4. Verifies `/run/lxd_agent/lxd-agent` exists and is executable.
5. `chdir`s to `/run/lxd_agent` (the agent expects `agent.conf` in its working
   directory) and `exec`s the host-supplied `lxd-agent` binary.

The service mounts host `/dev` and `/run` read-write with `rshared`
propagation, so:

- `/dev/lxd/sock` created by the agent appears on the Talos host (same
  devtmpfs instance), where the LXD CSI node plugin mounts it via hostPath.
- vsock (`/dev/vsock`), which the agent uses to reach the LXD host, is
  available.
- Agent state in `/run/lxd_agent` survives service restarts.

### Why the tmpfs on /mnt

The Talos root filesystem is read-only, so `/mnt/lxd-csi/<volume>` (the path
the CSI driver hardcodes for filesystem volumes) cannot be created on the
host directly. The service root is shared-propagated, so a tmpfs mounted at
`/mnt` inside the service's mount namespace propagates to the host. The
agent then creates `/mnt/lxd-csi/<volume>` on that shared tmpfs and mounts
LXD's virtiofs share there; those mounts propagate to the host, where the
kubelet and the CSI node plugin (hostPath with `mountPropagation:
Bidirectional`) pick them up. No `UserVolumeConfig` or
`machine.kubelet.extraMounts` are required.

## Requirements

- Talos `>= v1.13` (validated layout against v1.13.7).
- Kernel support for `virtiofs` (present as a nodev filesystem on the target
  nodes) and `vsock` (module present).
- LXD host with the `devlxd_volume_management` feature
  (`security.devlxd.management.volumes=true` on every Kubernetes VM) and the
  `auth_bearer_devlxd` extension (LXD 6.6+, or a 5.21.x build that ships
  both extensions) for CSI authorization.
- The agent binary always comes from the LXD host, so LXD and agent versions
  stay in sync automatically.

## Security notes

- Extension services on Talos run with all grantable capabilities and share
  the host network namespace; this service additionally bind-mounts host
  `/dev` and `/run` read-write. Treat the extension like any other
  root-equivalent workload.
- The executed `lxd-agent` binary and its certificates come from the LXD
  host via the config share. Trust is anchored in your LXD cluster, same as
  for any LXD guest.
- The LXD CSI bearer token is stored in 1Password (`op://homelab/lxd-csi/token`)
  and mounted into the CSI pods; it never appears in this repository.

## Build

```sh
make test      # go test + cross-compile check
make build     # OCI image (linux/amd64 by default)
make validate  # verify the extension image layout
```

For arm64: `make build PLATFORM=linux/arm64`.

## Publish

```sh
podman login ghcr.io
make push
```

## Deploy to the cluster

The Image Factory cannot bake custom extensions, so build a custom installer
that combines the official extensions from the existing schematic
(`iscsi-tools`, `tailscale`) with this one:

```sh
make installer   # builds + pushes the extension, then runs imager
make load        # loads _out/*installer*.tar into the local image store
podman push <loaded-installer-image> ghcr.io/danielbooth-cloud/talos-installer:0.1.0
```

`OFFICIAL_EXTENSIONS` in the `Makefile` must match your current schematic;
check the running versions with `talosctl -n <node> get extensions`.

Nodes pull the installer from a registry, so the reference must be reachable
from the nodes (ghcr.io, or a private registry over Tailscale). Then upgrade
each node in turn:

```sh
talosctl -n <node-ip> upgrade --image <registry>/<installer>:<tag> --force
```

Upgrading with the same Talos version but new extensions is supported; the
node reboots once. Roll nodes one at a time.

## Verify

```sh
talosctl -n <node-ip> service ext-lxd-agent
talosctl -n <node-ip> ls /dev/lxd/sock
talosctl -n <node-ip> ls /mnt/lxd-csi   # created on first volume mount
```

`talosctl service` should show `Running` and logs should end with
`starting host-supplied LXD agent`. Once the socket exists, the LXD CSI
Helmfile component (`lxd-csi` in `ops-cluster-apps`) can be deployed
normally.

## Troubleshooting

- `lxd-agent-loader: mount LXD config share ...` loop: the VM's `config`
  share is missing. Confirm the instance is managed by LXD and that
  `security.devlxd` is not disabled; virtiofs requires the host to export
  the share (LXD always does for VMs).
- `host-supplied LXD agent ... is not executable`: the config share was
  empty or truncated; check LXD host logs.
- `/mnt is already mounted, reusing it` is normal on service restarts; the
  propagated tmpfs from the previous run is reused.
- Certificate errors in agent logs after a LXD host upgrade: restart the
  service (`talosctl -n <node> service ext-lxd-agent restart`) so the loader
  re-copies the refreshed agent payload.
- The `/var/mnt/lxd-csi` directories found on the nodes are unused by this
  design (left over from earlier experiments) and can be removed.

## Repository layout

```text
cmd/lxd-agent-loader/       loader entrypoint (mount + copy + exec)
internal/payload/           recursive copy/clean helpers + tests
lxd-agent.yaml              Talos extension service spec
manifest.yaml               extension metadata consumed by Talos
Dockerfile                  builds the OCI system extension image
scripts/validate.sh         image layout verification
Makefile                    test/build/push/validate/installer targets
```
