//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const agentOS = "windows"

func expandPath(path string) string {
	var b strings.Builder
	i := 0
	for i < len(path) {
		if path[i] == '%' {
			j := strings.Index(path[i+1:], "%")
			if j >= 0 {
				key := path[i+1 : i+1+j]
				if val := os.Getenv(key); val != "" {
					b.WriteString(val)
				} else {
					b.WriteString("%" + key + "%")
				}
				i = i + 1 + j + 1
				continue
			}
		}
		b.WriteByte(path[i])
		i++
	}
	return b.String()
}

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func freeSpace(path string) (uint64, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	r1, _, e1 := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if r1 == 0 {
		return 0, e1
	}
	return freeBytesAvailable, nil
}

func configFilePath() string {
	appdata := os.Getenv("APPDATA")
	return filepath.Join(appdata, "rom-agent", "config.json")
}

func defaultDownloadsDir() string {
	return filepath.Join(os.Getenv("USERPROFILE"), "Downloads")
}

func defaultBrowseRoot() string {
	profile := os.Getenv("USERPROFILE")
	if profile == "" {
		return `C:\`
	}
	return profile
}
