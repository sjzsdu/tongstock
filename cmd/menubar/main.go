package main

import (
	_ "embed"
	"encoding/json"
	"errors"
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
	"github.com/sjzsdu/tongstock/pkg/config"
)

//go:embed icon.png
var iconData []byte

var (
	serviceRunning bool
	servicePID     int
	mu             sync.Mutex
	serviceOpMu    sync.Mutex
	serverFinder   = findServerBinary
)

const (
	gracefulStopTimeout = 8 * time.Second
	forcedStopTimeout   = 2 * time.Second
)

type serverPIDRecord struct {
	PID        int       `json:"pid"`
	Executable string    `json:"executable,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
}

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
	record, err := readPIDRecord()
	if err != nil {
		return false, 0
	}

	running, zombie := processStatus(record.PID)
	if !running || zombie {
		removePIDRecord(record.PID)
		return false, 0
	}

	if !processMatchesRecord(record) {
		removePIDRecord(record.PID)
		return false, 0
	}

	return true, record.PID
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
	if running, _ := isServerRunning(); running {
		return
	}

	startService(statusItem, startStopItem, restartItem)
}

func startService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	serviceOpMu.Lock()
	defer serviceOpMu.Unlock()

	statusItem.SetTitle("Starting...")

	if err := startManagedServer(); err != nil {
		setServiceState(false, 0)
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Start failed: %v", err))
		return
	}
	updateMenuItems(statusItem, startStopItem, restartItem)
}

func stopService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	serviceOpMu.Lock()
	defer serviceOpMu.Unlock()

	statusItem.SetTitle("Stopping...")

	if err := stopManagedServer(); err != nil {
		running, pid := isServerRunning()
		setServiceState(running, pid)
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Stop failed: %v", err))
		return
	}
	setServiceState(false, 0)
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
	home, _ := os.UserHomeDir()
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
	serviceOpMu.Lock()
	defer serviceOpMu.Unlock()

	statusItem.SetTitle("Restarting...")

	if err := stopManagedServer(); err != nil {
		running, pid := isServerRunning()
		setServiceState(running, pid)
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Restart failed: %v", err))
		return
	}

	if err := startManagedServer(); err != nil {
		setServiceState(false, 0)
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Restart failed: %v", err))
		return
	}
	updateMenuItems(statusItem, startStopItem, restartItem)
}

func startManagedServer() error {
	if running, pid := isServerRunning(); running {
		setServiceState(true, pid)
		return nil
	}

	serverBin := serverFinder()
	if serverBin == "" {
		return errors.New("server binary not found")
	}

	logFile, err := openServerLog()
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}

	cmd := exec.Command(serverBin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	logFile.Close()

	record := serverPIDRecord{
		PID:        cmd.Process.Pid,
		Executable: serverBin,
		StartedAt:  time.Now(),
	}
	if err := writePIDRecord(record); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("write PID file: %w", err)
	}

	setServiceState(true, record.PID)
	go reapServerProcess(cmd, record.PID)

	// Catch immediate failures such as an occupied port without delaying normal starts.
	time.Sleep(300 * time.Millisecond)
	if running, zombie := processStatus(record.PID); !running || zombie {
		removePIDRecord(record.PID)
		setServiceState(false, 0)
		return fmt.Errorf("server exited immediately; see %s", serverLogPath())
	}

	return nil
}

func stopManagedServer() error {
	record, err := readPIDRecord()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read PID file: %w", err)
	}

	running, zombie := processStatus(record.PID)
	if !running || zombie {
		removePIDRecord(record.PID)
		return nil
	}
	if !processMatchesRecord(record) {
		removePIDRecord(record.PID)
		return fmt.Errorf("PID %d is not the recorded TongStock server", record.PID)
	}

	proc, err := os.FindProcess(record.PID)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if waitForProcessExit(record.PID, gracefulStopTimeout) {
		removePIDRecord(record.PID)
		return nil
	}

	if err := proc.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("force stop PID %d: %w", record.PID, err)
	}
	if !waitForProcessExit(record.PID, forcedStopTimeout) {
		return fmt.Errorf("PID %d did not exit", record.PID)
	}
	removePIDRecord(record.PID)
	return nil
}

func reapServerProcess(cmd *exec.Cmd, pid int) {
	_ = cmd.Wait()
	removePIDRecord(pid)

	mu.Lock()
	if servicePID == pid {
		serviceRunning = false
		servicePID = 0
	}
	mu.Unlock()
}

func setServiceState(running bool, pid int) {
	mu.Lock()
	serviceRunning = running
	servicePID = pid
	mu.Unlock()
}

func processStatus(pid int) (running bool, zombie bool) {
	if pid <= 0 {
		return false, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false, false
	}

	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Z") {
		return true, true
	}
	return true, false
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		running, zombie := processStatus(pid)
		if !running || zombie {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func processMatchesRecord(record serverPIDRecord) bool {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(record.PID)).Output()
	if err != nil {
		return false
	}
	commandLine := strings.TrimSpace(string(out))
	if record.Executable != "" {
		return strings.Contains(commandLine, record.Executable) ||
			strings.Contains(commandLine, filepath.Base(record.Executable))
	}
	return strings.Contains(commandLine, "tongstock-server") ||
		(strings.Contains(commandLine, "tongstock") && strings.Contains(commandLine, "server"))
}

func pidFilePath() string {
	return filepath.Join(config.HomeDir(), "server.pid")
}

func serverLogPath() string {
	return filepath.Join(config.HomeDir(), "server.log")
}

func openServerLog() (*os.File, error) {
	if err := os.MkdirAll(config.HomeDir(), 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(serverLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}

func readPIDRecord() (serverPIDRecord, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return serverPIDRecord{}, err
	}

	var record serverPIDRecord
	if err := json.Unmarshal(data, &record); err == nil && record.PID > 0 {
		return record, nil
	}

	// Backward compatibility with the previous plain-text PID file.
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return serverPIDRecord{}, errors.New("invalid server PID file")
	}
	return serverPIDRecord{PID: pid}, nil
}

func writePIDRecord(record serverPIDRecord) error {
	if err := os.MkdirAll(config.HomeDir(), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmpPath := pidFilePath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, pidFilePath())
}

func removePIDRecord(pid int) {
	record, err := readPIDRecord()
	if err == nil && record.PID == pid {
		_ = os.Remove(pidFilePath())
	}
}

func monitorService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		running, pid := isServerRunning()
		setServiceState(running, pid)
		updateMenuItems(statusItem, startStopItem, restartItem)
		<-ticker.C
	}
}
