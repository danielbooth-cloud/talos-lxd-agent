//go:build !linux

package main

import "fmt"

func mountMntWorkspace() error {
	return nil
}

func mountConfig(_, _ string) (string, error) {
	return "", fmt.Errorf("LXD config-share mounting is supported only on Linux")
}

func unmountConfig(_ string) error {
	return nil
}
