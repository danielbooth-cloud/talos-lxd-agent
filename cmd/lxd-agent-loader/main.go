package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/danielbooth-cloud/talos-lxd-agent/internal/payload"
)

const (
	configTag  = "config"
	runtimeDir = "/run/lxd_agent"
	mountDir   = "/run/lxd_agent/.mnt"
	agentPath  = "/run/lxd_agent/lxd-agent"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC | log.Lmsgprefix)
	log.SetPrefix("lxd-agent-loader: ")

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := mountMntWorkspace(); err != nil {
		return fmt.Errorf("prepare /mnt mount workspace: %w", err)
	}

	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}

	// A service restart can leave an old mount behind. Detach it before
	// recreating the staging directory.
	if err := unmountConfig(mountDir); err != nil {
		return fmt.Errorf("detach stale config mount: %w", err)
	}

	if err := payload.Clean(runtimeDir, filepath.Base(mountDir)); err != nil {
		return fmt.Errorf("clean stale agent payload: %w", err)
	}

	if err := os.MkdirAll(mountDir, 0o700); err != nil {
		return fmt.Errorf("create config mountpoint: %w", err)
	}

	mountType, err := mountConfig(configTag, mountDir)
	if err != nil {
		return fmt.Errorf("mount LXD config share: %w", err)
	}

	log.Printf("mounted LXD config share using %s", mountType)

	copyErr := payload.CopyTree(mountDir, runtimeDir)
	unmountErr := unmountConfig(mountDir)
	removeErr := os.Remove(mountDir)

	if copyErr != nil {
		return fmt.Errorf("copy LXD agent payload: %w", copyErr)
	}

	if unmountErr != nil {
		return fmt.Errorf("unmount LXD config share: %w", unmountErr)
	}

	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return fmt.Errorf("remove config mountpoint: %w", removeErr)
	}

	info, err := os.Stat(agentPath)
	if err != nil {
		return fmt.Errorf("locate host-supplied LXD agent: %w", err)
	}

	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("host-supplied LXD agent %q is not executable", agentPath)
	}

	if err := os.Chdir(runtimeDir); err != nil {
		return fmt.Errorf("change working directory to %q: %w", runtimeDir, err)
	}

	log.Printf("starting host-supplied LXD agent")

	return syscall.Exec(agentPath, []string{agentPath}, os.Environ())
}
