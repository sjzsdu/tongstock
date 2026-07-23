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
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

//go:embed icon.png
var iconData []byte

var (
	serviceRunning bool
	servicePID     int
	mu             sync.Mutex
)

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTooltip("TongStock 股票分析工作台")

	setIcon()

	// Status item
	statusItem := systray.AddMenuItem("Service Stopped", "")
	statusItem.Disable()

	systray.AddSeparator()

	// Open Console
	openConsole := systray.AddMenuItem("Open Console", "在浏览器中打开 TongStock")

	// About
	aboutItem := systray.AddMenuItem("About TongStock", "关于 TongStock")

	systray.AddSeparator()

	// Start/Stop Service (dynamically updated)
	startStopItem := systray.AddMenuItem("Start Service", "启动服务")

	// Restart Service
	restartItem := systray.AddMenuItem("Restart Service", "重启服务")
	restartItem.Disable()

	systray.AddSeparator()

	// Quit
	quitItem := systray.AddMenuItem("Quit Menu Bar", "退出菜单栏")

	// Handle events
	go func() {
		for {
			select {
			case <-openConsole.ClickedCh:
				ensureServerRunning(statusItem, startStopItem, restartItem)
				waitForServer("http://localhost:8080/health", 10)
				openBrowser("http://localhost:8080")
			case <-aboutItem.ClickedCh:
				openBrowser("https://github.com/sjzsdu/tongstock")
			case <-startStopItem.ClickedCh:
				mu.Lock()
				running := serviceRunning
				mu.Unlock()
				if running {
					stopService(statusItem, startStopItem, restartItem)
				} else {
					startService(statusItem, startStopItem, restartItem)
				}
			case <-restartItem.ClickedCh:
				restartService(statusItem, startStopItem, restartItem)
			case <-quitItem.ClickedCh:
				systray.Quit()
				os.Exit(0)
			}
		}
	}()

	// Monitor service health
	go monitorService(statusItem, startStopItem, restartItem)
}

func onExit() {}

func setIcon() {
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
		return
	}
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

func isServerRunning() (bool, int) {
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false, 0
	}
	return true, pid
}

func updateMenuItems(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	mu.Lock()
	defer mu.Unlock()

	if serviceRunning {
		statusItem.SetTitle(fmt.Sprintf("Service Running · PID %d", servicePID))
		startStopItem.SetTitle("Stop Service")
		startStopItem.SetTooltip("停止服务")
		restartItem.Enable()
	} else {
		statusItem.SetTitle("Service Stopped")
		startStopItem.SetTitle("Start Service")
		startStopItem.SetTooltip("启动服务")
		restartItem.Disable()
	}
}

func ensureServerRunning(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	mu.Lock()
	running := servicePID
	mu.Unlock()

	if running > 0 {
		return
	}

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
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	os.MkdirAll(filepath.Dir(pidFile), 0755)
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)

	mu.Lock()
	serviceRunning = true
	servicePID = pid
	mu.Unlock()

	updateMenuItems(statusItem, startStopItem, restartItem)
}

func startService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	statusItem.SetTitle("Starting...")

	serverBin := findServerBinary()
	if serverBin == "" {
		statusItem.SetTitle("Server binary not found")
		return
	}

	cmd := exec.Command(serverBin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		statusItem.SetTitle("Start failed")
		return
	}

	pid := cmd.Process.Pid
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	os.MkdirAll(filepath.Dir(pidFile), 0755)
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)

	mu.Lock()
	serviceRunning = true
	servicePID = pid
	mu.Unlock()

	updateMenuItems(statusItem, startStopItem, restartItem)
}

func stopService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	statusItem.SetTitle("Stopping...")

	killExistingProcess()

	mu.Lock()
	serviceRunning = false
	servicePID = 0
	mu.Unlock()

	updateMenuItems(statusItem, startStopItem, restartItem)
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

	// 5. Look in go/bin (where make install puts it)
	candidate = filepath.Join(home, "go", "bin", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// 6. Look in project source directory (common dev location)
	candidate = filepath.Join(home, "Codes", "gos", "tongstock", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return ""
}

func restartService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	statusItem.SetTitle("Restarting...")

	killExistingProcess()

	serverBin := findServerBinary()
	if serverBin == "" {
		statusItem.SetTitle("Server binary not found")
		mu.Lock()
		serviceRunning = false
		servicePID = 0
		mu.Unlock()
		updateMenuItems(statusItem, startStopItem, restartItem)
		return
	}

	cmd := exec.Command(serverBin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		statusItem.SetTitle("Restart failed")
		mu.Lock()
		serviceRunning = false
		servicePID = 0
		mu.Unlock()
		updateMenuItems(statusItem, startStopItem, restartItem)
		return
	}

	pid := cmd.Process.Pid
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	os.MkdirAll(filepath.Dir(pidFile), 0755)
	os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)

	mu.Lock()
	serviceRunning = true
	servicePID = pid
	mu.Unlock()

	updateMenuItems(statusItem, startStopItem, restartItem)
}

func killExistingProcess() {
	// First try: PID file method (for processes started via menubar)
	pidFile := filepath.Join(os.Getenv("HOME"), ".tongstock", "server.pid")
	if data, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				proc.Signal(syscall.SIGTERM)
				time.Sleep(500 * time.Millisecond)
				os.Remove(pidFile)
				return
			}
		}
		// If we get here, PID file existed but was invalid; fall through to cleanup
		os.Remove(pidFile) // Remove stale PID file
	}

	// Fallback: Direct process search (for manually started or orphaned processes)
	cmd := exec.Command("pgrep", "tongstock-server")
	if out, err := cmd.CombinedOutput(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			if pid, err := strconv.Atoi(line); err == nil {
				if proc, err := os.FindProcess(pid); err == nil {
					proc.Signal(syscall.SIGTERM)
					time.Sleep(500 * time.Millisecond)
				}
			}
		}
	}
	// Clean up PID file after fallback attempt
	os.Remove(pidFile)
}

func monitorService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		running, pid := isServerRunning()

		mu.Lock()
		serviceRunning = running
		servicePID = pid
		mu.Unlock()

		updateMenuItems(statusItem, startStopItem, restartItem)
	}
}
