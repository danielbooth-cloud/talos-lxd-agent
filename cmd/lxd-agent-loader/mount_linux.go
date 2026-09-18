//go:build linux

package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"syscall"
)

// mountMntWorkspace ensures /mnt is a shared mount so that LXD agent volume
// mounts created under /mnt/lxd-csi propagate to the Talos host, where the
// LXD CSI node plugin (and kubelet) expect to find them.
//
// The service rootfs is built FROM scratch and does not ship a /mnt
// directory, so the mountpoint is created on the writable container
// filesystem before anything is mounted on it.
//
// The Talos root filesystem is read-only, so /mnt/lxd-csi cannot be created
// on the host directly. Mounting a tmpfs at /mnt inside this service's mount
// namespace propagates the mount to the host (the service root is shared),
// and everything created beneath it afterwards is visible in both
// namespaces.
func mountMntWorkspace() error {
	// The scratch-based rootfs has no /mnt; this is a no-op if the
	// directory already exists (e.g. a reused containerd snapshot).
	if err := os.MkdirAll("/mnt", 0o755); err != nil {
		return fmt.Errorf("create /mnt mountpoint: %w", err)
	}

	if err := syscall.Mount("", "/mnt", "tmpfs", 0, "mode=0755"); err != nil {
		// EBUSY means /mnt is already a mountpoint, most likely the
		// propagated tmpfs from a previous service run. The agent can
		// use it as-is.
		if errors.Is(err, syscall.EBUSY) {
			log.Printf("/mnt is already mounted, reusing it")

			return nil
		}

		return fmt.Errorf("mount tmpfs on /mnt: %w", err)
	}

	// Make the new tmpfs shared explicitly; it should already have
	// inherited shared propagation from the service root.
	if err := syscall.Mount("", "/mnt", "", syscall.MS_SHARED, ""); err != nil {
		return fmt.Errorf("mark /mnt shared: %w", err)
	}

	return nil
}

func mountConfig(source, target string) (string, error) {
	virtiofsErr := syscall.Mount(source, target, "virtiofs", syscall.MS_RDONLY, "")
	if virtiofsErr == nil {
		return "virtiofs", nil
	}

	// 9p fallback for LXD hosts that serve the config share over 9p. The
	// 9p filesystem is not available on stock Talos kernels (no 9p modules
	// are shipped), so this path only works on kernels with 9p support.
	p9Err := syscall.Mount(source, target, "9p", syscall.MS_RDONLY,
		"trans=virtio,version=9p2000.L,msize=1048576,access=0")
	if p9Err == nil {
		return "9p", nil
	}

	return "", fmt.Errorf("virtiofs: %w; 9p: %v (%s)", virtiofsErr, p9Err, configShareHint())
}

// configShareHint explains the common cause of both config share mounts
// failing: the LXD host served the config share over 9p (typically because
// virtiofsd is not installed there), and the guest cannot mount 9p either.
func configShareHint() string {
	entries, err := os.ReadDir("/sys/fs/virtio_fs")
	if err != nil || len(entries) == 0 {
		return "no virtiofs device is attached to the guest; check the LXD host for the warning " +
			"\"Cannot use virtio-fs for config drive, using 9p as a fallback\" " +
			"(install virtiofsd on the LXD host and restart the VM; Talos kernels ship no 9p support)"
	}

	return "a virtiofs device is attached but its tag did not match"
}

func unmountConfig(target string) error {
	err := syscall.Unmount(target, syscall.MNT_DETACH)
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT) {
		return nil
	}

	return err
}
