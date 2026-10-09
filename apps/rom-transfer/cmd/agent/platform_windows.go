//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const agentOS = "windows"

const (
	svcName        = "rom-agent"
	svcDisplayName = "ROM Transfer Agent"
	svcDesc        = "Serves local file access to the ROM Transfer web app"
)

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
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
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

// ── Windows service ───────────────────────────────────────────────────────────

type agentSvc struct{ srv *http.Server }

func (s *agentSvc) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	go s.srv.ListenAndServe()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.srv.Shutdown(ctx)
			return false, 0
		}
	}
	return false, 0
}

func startServer(srv *http.Server, addr string) {
	ok, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("cannot detect service mode: %v", err)
	}
	if ok {
		elog, _ := eventlog.Open(svcName)
		if elog != nil {
			elog.Info(1, fmt.Sprintf("rom-agent v%s starting on %s", version, addr))
			defer elog.Close()
		}
		if err := svc.Run(svcName, &agentSvc{srv: srv}); err != nil {
			if elog != nil {
				elog.Error(1, err.Error())
			}
			log.Fatalf("service run failed: %v", err)
		}
		return
	}
	selfInstall(addr)
}

func selfInstall(addr string) {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot determine exe path: %v", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		log.Fatalf("cannot connect to service manager — run as Administrator: %v", err)
	}
	defer m.Disconnect()

	// Stop and remove any existing installation.
	if s, err := m.OpenService(svcName); err == nil {
		log.Println("Stopping existing service...")
		s.Control(svc.Stop)
		for i := 0; i < 20; i++ {
			if st, err := s.Query(); err != nil || st.State == svc.Stopped {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		s.Delete()
		s.Close()
		eventlog.Remove(svcName)
		log.Println("Removed existing service.")
	}

	s, err := m.CreateService(svcName, exe, mgr.Config{
		DisplayName: svcDisplayName,
		Description: svcDesc,
		StartType:   mgr.StartAutomatic,
	}, "-addr", addr)
	if err != nil {
		log.Fatalf("install failed: %v", err)
	}
	defer s.Close()
	eventlog.InstallAsEventCreate(svcName, eventlog.Error|eventlog.Warning|eventlog.Info)

	if err := s.Start(); err != nil {
		log.Fatalf("service registered but failed to start: %v", err)
	}
	log.Printf("ROM Transfer Agent v%s installed and running on %s", version, addr)
	log.Println("The service will start automatically on boot. You can close this window.")
}

func doUpdate(serverURL string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine exe path: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	newExe := filepath.Join(exeDir, "rom-agent-update.exe")

	resp, err := http.Get(serverURL + "/downloads/rom-agent.exe")
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}
	f, err := os.Create(newExe)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(newExe)
		return fmt.Errorf("download write failed: %w", err)
	}
	f.Close()

	batPath := filepath.Join(exeDir, "rom-agent-update.bat")
	bat := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak >nul
sc query rom-agent >nul 2>&1
if errorlevel 1 goto replace
sc stop rom-agent
:waitstop
sc query rom-agent | find "STOPPED" >nul
if errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto waitstop
)
:replace
move /y "%s" "%s"
sc query rom-agent >nul 2>&1
if not errorlevel 1 sc start rom-agent
del "%%~f0"
`, newExe, exePath)

	if err := os.WriteFile(batPath, []byte(bat), 0755); err != nil {
		os.Remove(newExe)
		return fmt.Errorf("cannot write update script: %w", err)
	}

	cmd := exec.Command("cmd", "/c", batPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000 | 0x00000200, // CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
	}
	return cmd.Start()
}

func find7z() (string, error) {
	if p, err := exec.LookPath("7z"); err == nil {
		return p, nil
	}
	candidates := []string{
		`C:\Program Files\7-Zip\7z.exe`,
		`C:\Program Files (x86)\7-Zip\7z.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("7z not found; install 7-Zip from https://www.7-zip.org/")
}

func extract7z(src, destDir string) error {
	sevenZip, err := find7z()
	if err != nil {
		return err
	}
	cmd := exec.Command(sevenZip, "x", src, "-o"+destDir, "-y")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z extraction failed: %w\n%s", err, out)
	}
	return nil
}

func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("cannot connect to service manager: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(svcName)
	if err != nil {
		return fmt.Errorf("service %q not found", svcName)
	}
	defer s.Close()
	eventlog.Remove(svcName)
	return s.Delete()
}
