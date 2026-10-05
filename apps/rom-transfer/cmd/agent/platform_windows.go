//go:build windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

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
