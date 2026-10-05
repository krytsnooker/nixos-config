//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

func freeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func configFilePath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "rom-agent", "config.json")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "rom-agent", "config.json")
}

func defaultDownloadsDir() string {
	return filepath.Join(os.Getenv("HOME"), "Downloads")
}

func defaultBrowseRoot() string {
	home := os.Getenv("HOME")
	if home == "" {
		return "/"
	}
	return home
}
