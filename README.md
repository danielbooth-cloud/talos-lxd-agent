# talos-lxd-agent (experimental)

> [!WARNING]
> **AI-assisted development.** This extension was produced through heavy
> AI-assisted development with limited human review. It installs a
> root-equivalent Talos extension service that bind-mounts host `/dev` and
> `/run` read-write and mounts a shared tmpfs on `/mnt`. Read the code,
> test it on a non-production node first, and treat it as experimental
> software. No warranty of fitness for any purpose is provided.

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

1. Creates `/mnt` if it is missing (the scratch-based service rootfs ships
   none) and mounts a tmpfs on it with shared propagation (see below).
2. Mounts LXD's `config` share (tag `config`) at `/run/lxd_agent/.mnt`
   using virtiofs, falling back to 9p.
3. Copies the share contents (agent binary, agent.conf, certificates) into
   `/run/lxd_agent`, then unmounts and removes the mountpoint.
4. Verifies `/run/lxd_agent/lxd-agent` exists, is executable, and is
   statically linked (the scratch container has no dynamic loader; official
   LXD builds the agent statically, and a dynamic one fails loudly with an
   explanation instead of a bare exec error).
5. Logs that the payload is staged and exits (`restart: untilSuccess` keeps
   the service quiet afterwards). The binary is intentionally *not* executed
   here: Talos applies its default seccomp profile to extension-service
   containers, and that profile blocks `socket(AF_VSOCK)`, which the agent
   needs in order to listen on vsock:8443 — and extension specs have no
   seccomp override. The **lxd-agent DaemonSet**
   (`deploy/lxd-agent-daemonset.yaml`, deployed via the `lxd-agent` helmfile
   component) runs the staged payload as a privileged pod with
   `seccompProfile: Unconfined`.

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
host directly. The **lxd-agent DaemonSet pod** mounts a tmpfs on `/mnt`: its
`/mnt` volume uses `mountPropagation: Bidirectional`, so mounts created
inside the pod propagate to the host, where the kubelet and the CSI node
plugin (hostPath with `mountPropagation: Bidirectional`) pick them up. The
agent then creates `/mnt/lxd-csi/<volume>` on that shared tmpfs and mounts
LXD's virtiofs share there. No `UserVolumeConfig` or
`machine.kubelet.extraMounts` are required.

Note: the loader also mounts a tmpfs on `/mnt` inside the extension
service's own namespace (kept for the agent to find a writable `/mnt` if it
is ever exec'd there again), but that mount does **not** propagate to the
host — the pod is what establishes the shared host-visible tmpfs.

## Requirements

- Talos `>= v1.13` (validated layout against v1.13.7).
- Kernel support for `virtiofs` (present as a nodev filesystem on the target
  nodes) and `vsock` (module present).
- LXD host with the `devlxd_volume_management` feature
  (`security.devlxd.management.volumes=true` on every Kubernetes VM) and the
  `auth_bearer_devlxd` extension (LXD 6.6+, or a 5.21.x build that ships
  both extensions) for CSI authorization.
- `virtiofsd` available to the LXD host: LXD serves the VM config drive
  over virtiofs when it can and falls back to 9p otherwise. Talos kernels
  ship no 9p support, so the 9p fallback leaves the agent unable to start.
- The agent binary always comes from the LXD host, so LXD and agent versions
  stay in sync automatically.

## Security notes

- **Review before use.** This codebase was developed with heavy AI
  assistance; verify the loader logic and the `lxd-agent` trust model
  yourself before running it anywhere that matters.
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
talosctl -n <node-ip> service ext-lxd-agent   # STATE Finished once payload is staged
kubectl -n kube-system logs -l app.kubernetes.io/name=lxd-agent --tail=1
                                              # "starting host-supplied LXD agent"
talosctl -n <node-ip> ls /dev/lxd/sock
talosctl -n <node-ip> ls /mnt/lxd-csi         # created on first volume mount
```

Once the socket exists, the LXD CSI
Helmfile component (`lxd-csi` in `ops-cluster-apps`) can be deployed
normally.

## Troubleshooting

- `mount LXD config share: virtiofs: invalid argument; 9p: no such device`:
  no config share device is visible to the guest. LXD serves the config
  drive over virtiofs when it can and falls back to 9p otherwise (it logs
  "Cannot use virtio-fs for config drive, using 9p as a fallback", usually
  because `virtiofsd` is not installed on the LXD host). Talos kernels ship
  no 9p support, so the fallback cannot be mounted: install `virtiofsd` on
  the LXD host and restart the VM, then confirm with `lxc warning list` on
  the host and `talosctl -n <node> ls /sys/fs/virtio_fs` on the guest (the
  device tag is `config`).
- `host-supplied LXD agent ... is not executable`: the config share was
  empty or truncated; check LXD host logs.
- `host-supplied LXD agent ... is dynamically linked (interpreter ...)`: the
  binary LXD served in the config share needs a dynamic loader, which the
  scratch container does not provide. Official LXD builds `lxd-agent`
  statically (`CGO_ENABLED=0`, `-tags agent,netgo`), so this points at a
  custom agent build on the LXD host; rebuild it with CGO disabled.
- `/mnt is already mounted, reusing it` is normal on service restarts; the
  propagated tmpfs from the previous run is reused.
- Certificate errors in agent logs after a LXD host upgrade: re-stage the
  payload and restart the agent pods (`talosctl -n <node> service
  ext-lxd-agent restart`, then `kubectl -n kube-system rollout restart
  ds/lxd-agent`).
- `listen vsock ... operation not permitted` in loader logs: an old image
  still exec'ing the agent inside the extension container. Upgrade to a
  loader that only stages the payload and make sure the lxd-agent DaemonSet
  is deployed — the pod runs the agent with `seccompProfile: Unconfined`,
  which extension containers cannot set.
- `mounting shared tmpfs on /mnt` in lxd-agent pod logs (once per boot) is
  normal: the pod establishes the shared tmpfs that volume mounts
  propagate through.
- The `/var/mnt/lxd-csi` directories found on the nodes are unused by this
  design (left over from earlier experiments) and can be removed.

## Repository layout

```text
cmd/lxd-agent-loader/       loader entrypoint (mount + stage)
deploy/lxd-agent-daemonset.yaml  DaemonSet that runs the staged payload
internal/payload/           recursive copy/clean helpers + tests
lxd-agent.yaml              Talos extension service spec
manifest.yaml               extension metadata consumed by Talos
Dockerfile                  builds the OCI system extension image
scripts/validate.sh         image layout verification
Makefile                    test/build/push/validate/installer targets
```
