//go:build linux

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const agentOS = "linux"

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

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

func startServer(srv *http.Server, addr string) {
	log.Printf("rom-agent v%s listening on %s", version, addr)
	log.Fatal(srv.ListenAndServe())
}

func installService(exePath, addr string) error {
	return fmt.Errorf("service installation is not supported on Linux")
}

func uninstallService() error {
	return fmt.Errorf("service uninstall is not supported on Linux")
}

func doUpdate(serverURL string) error {
	return fmt.Errorf("one-click update is not supported on Linux")
}

func find7z() (string, error) {
	return exec.LookPath("7z")
}

func extract7z(src, destDir string) error {
	sevenZip, err := find7z()
	if err != nil {
		return fmt.Errorf("7z not found in PATH: %w", err)
	}
	cmd := exec.Command(sevenZip, "x", src, "-o"+destDir, "-y")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
