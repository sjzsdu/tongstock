package menubar

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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"
	"github.com/sjzsdu/tongstock/internal/serviceproc"
	"github.com/sjzsdu/tongstock/pkg/config"
)

//go:embed icon.png
var iconData []byte

var (
	serviceRunning  bool
	servicePID      int
	serviceExternal bool
	serviceConflict bool
	mu              sync.Mutex
	serviceOpMu     sync.Mutex
	serverFinder    = findServerCommand
	listenerFinder  = serviceproc.ListenerPIDs
	processFinder   = serviceproc.RecordForPID
	statusFinder    = serviceproc.ProcessStatus
	healthFinder    = queryServerHealth
)

const (
	gracefulStopTimeout = 8 * time.Second
	forcedStopTimeout   = 2 * time.Second
)

type serverPIDRecord = serviceproc.Record

type serverCommand struct {
	Executable string
	Args       []string
}

type serviceInspection struct {
	Running  bool
	External bool
	Conflict bool
	PID      int
	Record   serverPIDRecord
}

type serverHealth struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	PID     int    `json:"pid"`
}

// Run starts the TongStock menu bar application.
func Run() {
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
				baseURL := fmt.Sprintf("http://localhost:%d", configuredServerPort())
				waitForServer(baseURL+"/health", 10)
				openBrowser(baseURL)
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
	inspection := inspectService()
	return inspection.Running, inspection.PID
}

func inspectService() serviceInspection {
	if record, err := readPIDRecord(); err == nil {
		running, zombie := processStatus(record.PID)
		if running && !zombie && processMatchesRecord(record) {
			return serviceInspection{Running: true, PID: record.PID, Record: record}
		}
		removePIDRecord(record.PID)
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(pidFilePath())
	}

	port := configuredServerPort()
	listenerPIDs, listenerErr := listenerFinder(port)
	health, healthErr := healthFinder(port)

	candidates := append([]int(nil), listenerPIDs...)
	if healthErr == nil && health.Service == "tongstock" && health.PID > 0 {
		found := false
		for _, pid := range candidates {
			if pid == health.PID {
				found = true
				break
			}
		}
		if !found {
			candidates = append([]int{health.PID}, candidates...)
		}
	}

	for _, pid := range candidates {
		running, zombie := processStatus(pid)
		if !running || zombie {
			continue
		}
		record, err := processFinder(pid)
		if err == nil {
			return serviceInspection{
				Running:  true,
				External: true,
				PID:      pid,
				Record:   record,
			}
		}
	}

	if listenerErr == nil && len(listenerPIDs) > 0 {
		return serviceInspection{Conflict: true, PID: listenerPIDs[0]}
	}
	return serviceInspection{}
}

func configuredServerPort() int {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}
	port := cfg.Server.Port
	if port == 0 {
		return 8080
	}
	return port
}

func queryServerHealth(port int) (serverHealth, error) {
	client := &http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return serverHealth{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return serverHealth{}, fmt.Errorf("health returned HTTP %d", resp.StatusCode)
	}
	var health serverHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return serverHealth{}, err
	}
	if health.Status != "ok" {
		return serverHealth{}, fmt.Errorf("health status is %q", health.Status)
	}
	return health, nil
}

func updateMenuItems(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	mu.Lock()
	defer mu.Unlock()

	switch {
	case serviceRunning && serviceExternal:
		statusItem.SetTitle(fmt.Sprintf("External Service Running · PID %d", servicePID))
		startStopItem.SetTitle("Stop Service")
		startStopItem.SetTooltip("停止已验证的 TongStock 服务")
		startStopItem.Enable()
		restartItem.Enable()
	case serviceRunning:
		statusItem.SetTitle(fmt.Sprintf("Service Running · PID %d", servicePID))
		startStopItem.SetTitle("Stop Service")
		startStopItem.SetTooltip("停止服务")
		startStopItem.Enable()
		restartItem.Enable()
	case serviceConflict:
		statusItem.SetTitle(fmt.Sprintf("Port %d Occupied · PID %d", configuredServerPort(), servicePID))
		startStopItem.SetTitle("Start Service")
		startStopItem.SetTooltip("端口被非 TongStock 进程占用")
		startStopItem.Disable()
		restartItem.Disable()
	default:
		statusItem.SetTitle("Service Stopped")
		startStopItem.SetTitle("Start Service")
		startStopItem.SetTooltip("启动服务")
		startStopItem.Enable()
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
		setServiceInspection(inspectService())
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
		setServiceInspection(inspectService())
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Stop failed: %v", err))
		return
	}
	setServiceInspection(serviceInspection{})
	updateMenuItems(statusItem, startStopItem, restartItem)
}

func findServerCommand() serverCommand {
	// The unified tongstock binary starts a second copy of itself in server mode.
	exe, err := os.Executable()
	if err == nil {
		if command := unifiedServerCommand(exe); command.Executable != "" {
			return command
		}

		// Legacy tongstock-menubar builds still use a sibling tongstock-server.
		candidate := filepath.Join(filepath.Dir(exe), "tongstock-server")
		if _, err := os.Stat(candidate); err == nil {
			return serverCommand{Executable: candidate}
		}
	}

	// Compatibility fallbacks for older two-binary installations.
	home, _ := os.UserHomeDir()
	candidate := filepath.Join(home, "bin", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return serverCommand{Executable: candidate}
	}

	candidate = filepath.Join(home, ".local", "bin", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return serverCommand{Executable: candidate}
	}

	if path, err := exec.LookPath("tongstock-server"); err == nil {
		return serverCommand{Executable: path}
	}

	candidate = filepath.Join(home, "go", "bin", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return serverCommand{Executable: candidate}
	}

	candidate = filepath.Join(home, "Codes", "gos", "tongstock", "tongstock-server")
	if _, err := os.Stat(candidate); err == nil {
		return serverCommand{Executable: candidate}
	}

	return serverCommand{}
}

func unifiedServerCommand(executable string) serverCommand {
	if strings.TrimSuffix(filepath.Base(executable), filepath.Ext(executable)) != "tongstock" {
		return serverCommand{}
	}
	return serverCommand{Executable: executable, Args: []string{"server"}}
}

func restartService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	serviceOpMu.Lock()
	defer serviceOpMu.Unlock()

	statusItem.SetTitle("Restarting...")

	if err := stopManagedServer(); err != nil {
		setServiceInspection(inspectService())
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Restart failed: %v", err))
		return
	}

	if err := startManagedServer(); err != nil {
		setServiceInspection(inspectService())
		updateMenuItems(statusItem, startStopItem, restartItem)
		statusItem.SetTitle(fmt.Sprintf("Restart failed: %v", err))
		return
	}
	updateMenuItems(statusItem, startStopItem, restartItem)
}

func startManagedServer() error {
	inspection := inspectService()
	if inspection.Running {
		setServiceInspection(inspection)
		return nil
	}
	if inspection.Conflict {
		return fmt.Errorf("port %d is occupied by non-TongStock PID %d", configuredServerPort(), inspection.PID)
	}

	serverCmd := serverFinder()
	if serverCmd.Executable == "" {
		return errors.New("server command not found")
	}

	logFile, err := openServerLog()
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}

	cmd := exec.Command(serverCmd.Executable, serverCmd.Args...)
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
		Executable: serverCmd.Executable,
		Args:       append([]string(nil), serverCmd.Args...),
		StartedAt:  time.Now(),
	}
	if err := writePIDRecord(record); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("write PID file: %w", err)
	}

	setServiceInspection(serviceInspection{Running: true, PID: record.PID, Record: record})
	go reapServerProcess(cmd, record.PID)

	// Catch immediate failures such as an occupied port without delaying normal starts.
	time.Sleep(300 * time.Millisecond)
	if running, zombie := processStatus(record.PID); !running || zombie {
		removePIDRecord(record.PID)
		setServiceInspection(serviceInspection{})
		return fmt.Errorf("server exited immediately; see %s", serverLogPath())
	}

	return nil
}

func stopManagedServer() error {
	inspection := inspectService()
	if inspection.Conflict {
		return fmt.Errorf("refusing to stop non-TongStock PID %d on port %d", inspection.PID, configuredServerPort())
	}
	if !inspection.Running {
		return nil
	}
	record := inspection.Record

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
	setServiceInspection(serviceInspection{Running: running, PID: pid})
}

func setServiceInspection(inspection serviceInspection) {
	mu.Lock()
	serviceRunning = inspection.Running
	servicePID = inspection.PID
	serviceExternal = inspection.External
	serviceConflict = inspection.Conflict
	mu.Unlock()
}

func processStatus(pid int) (running bool, zombie bool) {
	return statusFinder(pid)
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
	return serviceproc.Matches(record)
}

func pidFilePath() string {
	return serviceproc.PIDFilePath()
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
	return serviceproc.Read()
}

func writePIDRecord(record serverPIDRecord) error {
	return serviceproc.Write(record)
}

func removePIDRecord(pid int) {
	serviceproc.RemoveIfPID(pid)
}

func monitorService(statusItem *systray.MenuItem, startStopItem *systray.MenuItem, restartItem *systray.MenuItem) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		setServiceInspection(inspectService())
		updateMenuItems(statusItem, startStopItem, restartItem)
		<-ticker.C
	}
}
