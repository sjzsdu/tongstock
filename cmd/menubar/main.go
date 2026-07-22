package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

//go:embed icon.png
var iconData []byte

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTooltip("TongStock 股票分析工作台")

	// Try to load icon
setIcon()

	// Status item
	pid := os.Getpid()
	statusItem := systray.AddMenuItem(fmt.Sprintf("Service Running · PID %d", pid), "")
	statusItem.Disable()

	systray.AddSeparator()

	// Open Console
	openConsole := systray.AddMenuItem("Open Console", "在浏览器中打开 TongStock")

	// About
	aboutItem := systray.AddMenuItem("About TongStock", "关于 TongStock")

	systray.AddSeparator()

	// Restart Service
	restartItem := systray.AddMenuItem("Restart Service", "重启服务")

	// Quit
	quitItem := systray.AddMenuItem("Quit Menu Bar", "退出菜单栏")

	// Handle events
	go func() {
		for {
			select {
			case <-openConsole.ClickedCh:
				ensureServerRunning(statusItem)
				waitForServer("http://localhost:8080/health", 10)
				openBrowser("http://localhost:8080")
			case <-aboutItem.ClickedCh:
				openBrowser("https://github.com/sjzsdu/tongstock")
			case <-restartItem.ClickedCh:
				restartService(statusItem)
			case <-quitItem.ClickedCh:
				systray.Quit()
				os.Exit(0)
			}
		}
	}()

	// Monitor service health
	go monitorService(statusItem)
}

func onExit() {}

func setIcon() {
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
		return
	}
	// Fallback: try loading from file
	iconPaths := []string{
		"assets/icon.png",
		filepath.Join(os.Getenv("HOME"), ".tongstock/icon.png"),
	}
	for _, p := range iconPaths {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			systray.SetIcon(data)
			return
		}
	}
}

func getDefaultIcon() []byte {
	// Minimal 16x16 PNG icon (a simple "T" letter)
	// This is a placeholder - replace with actual icon
	return nil
}

func openBrowser(rawURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		u, _ := url.Parse(rawURL)
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u.String())
	}
	if cmd != nil {
		cmd.Start()
	}
}

func waitForServer(url string, maxSeconds int) {
	for i := 0; i < maxSeconds; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(time.Second)
	}
}

func ensureServerRunning(statusItem *systray.MenuItem) {
	// Check if server is already running via PID file
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	data, err := os.ReadFile(pidFile)
	if err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil {
			proc, err := os.FindProcess(pid)
			if err == nil && proc.Signal(syscall.Signal(0)) == nil {
				statusItem.SetTitle(fmt.Sprintf("Service Running · PID %d", pid))
				return // already running
			}
		}
	}

	// Try to find tongstock-server in PATH or known locations
	serverBin := findServerBinary()
	if serverBin == "" {
		statusItem.SetTitle("Server not found")
		return
	}

	cmd := exec.Command(serverBin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		statusItem.SetTitle("Start failed")
		return
	}
	pid := cmd.Process.Pid
	os.MkdirAll(filepath.Dir(pidFile), 0755)
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
	statusItem.SetTitle(fmt.Sprintf("Service Running · PID %d", pid))
}

func findServerBinary() string {
	// 1. Look next to the menubar binary
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "tongstock-server")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 2. Look in ~/bin
	home := os.Getenv("HOME")
	candidate := filepath.Join(home, "bin", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// 3. Look in ~/.local/bin
	candidate = filepath.Join(home, ".local", "bin", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// 4. Check PATH
	if path, err := exec.LookPath("tongstock-server"); err == nil {
		return path
	}

	// 5. Look in project source directory (common dev location)
	candidate = filepath.Join(home, "Codes", "gos", "tongstock-worktree", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ""
}

func restartService(statusItem *systray.MenuItem) {
	statusItem.SetTitle("Restarting...")

	// Kill existing server process
	killExistingProcess()

	// Find the server binary
	serverBin := findServerBinary()
	if serverBin == "" {
		statusItem.SetTitle("Server binary not found")
		statusItem.Enable()
		return
	}

	// Start new server process
	cmd := exec.Command(serverBin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		statusItem.SetTitle("Restart failed")
		statusItem.Enable()
		return
	}

	pid := cmd.Process.Pid
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	os.MkdirAll(filepath.Dir(pidFile), 0755)
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
	statusItem.SetTitle(fmt.Sprintf("Service Running · PID %d", pid))
	statusItem.Enable()
}

func killExistingProcess() {
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	proc.Signal(syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	os.Remove(pidFile)
}

func monitorService(statusItem *systray.MenuItem) {
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			continue
		}
		pidStr := strings.TrimSpace(string(data))
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		// Check if process is alive
		proc, err := os.FindProcess(pid)
		if err != nil {
			statusItem.SetTitle("Service Stopped")
			continue
		}
		// On macOS, FindProcess always succeeds, need to check with kill -0
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			statusItem.SetTitle("Service Stopped")
		} else {
			statusItem.SetTitle(fmt.Sprintf("Service Running · PID %d", pid))
		}
	}
}
