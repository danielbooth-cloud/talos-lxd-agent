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
// The Talos root filesystem is read-only, so /mnt/lxd-csi cannot be created
// on the host directly. Mounting a tmpfs at /mnt inside this service's mount
// namespace propagates the mount to the host (the service root is shared),
// and everything created beneath it afterwards is visible in both
// namespaces.
func mountMntWorkspace() error {
	info, err := os.Stat("/mnt")
	if err != nil {
		return fmt.Errorf("stat /mnt: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("/mnt is not a directory")
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

	p9Err := syscall.Mount(source, target, "9p", syscall.MS_RDONLY, "access=0,trans=virtio,size=1048576")
	if p9Err == nil {
		return "9p", nil
	}

	return "", fmt.Errorf("virtiofs: %w; 9p: %v", virtiofsErr, p9Err)
}

func unmountConfig(target string) error {
	err := syscall.Unmount(target, syscall.MNT_DETACH)
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOENT) {
		return nil
	}

	return err
}
