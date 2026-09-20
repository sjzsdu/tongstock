package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sjzsdu/tongstock/internal/serverapp"
	"github.com/sjzsdu/tongstock/internal/serviceproc"
	"github.com/sjzsdu/tongstock/pkg/config"
	"github.com/spf13/cobra"
)

var (
	serverDaemon bool
)

// serverCmd is the root command for server management.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "TongStock HTTP 服务管理",
	Long:  `启动、停止、查看状态或重启 TongStock HTTP 服务。不带子命令时默认以前台模式运行。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if serverDaemon {
			return startDaemon()
		}
		return serverapp.Run()
	},
}

// serverStartCmd starts the server in the background (daemon mode).
var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "后台启动服务",
	Long:  `在后台启动 TongStock HTTP 服务，自动写入 PID 文件以便后续管理。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startDaemon()
	},
}

// serverStopCmd stops the running server.
var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "停止服务",
	Long:  `优雅停止正在运行的 TongStock 服务（先 SIGTERM，超时后 SIGKILL）。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return stopServer()
	},
}

// serverStatusCmd shows the current server status.
var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看服务状态",
	Long:  `显示 TongStock 服务的运行状态、PID 和端口信息。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showStatus()
	},
}

// serverRestartCmd restarts the server.
var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "重启服务",
	Long:  `停止并重新启动 TongStock 服务。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return restartServer()
	},
}

func init() {
	serverCmd.PersistentFlags().BoolVar(&serverDaemon, "daemon", false, "以后台模式运行")
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverRestartCmd)
	rootCmd.AddCommand(serverCmd)
}

// --- Daemon management functions ---

func startDaemon() error {
	// Check if already running
	inspection := inspectServer()
	if inspection.Running {
		fmt.Printf("服务已在运行中 (PID %d)\n", inspection.PID)
		return nil
	}
	if inspection.Conflict {
		return fmt.Errorf("端口 %d 被非 TongStock 进程占用 (PID %d)", configuredPort(), inspection.PID)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// Open log file
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(home, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	logPath := filepath.Join(home, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	// Start the process in a new session (setsid)
	cmd := exec.Command(exe, "server")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("启动服务失败: %w", err)
	}
	logFile.Close()

	// Write PID record
	record := serviceproc.Record{
		PID:        cmd.Process.Pid,
		Executable: exe,
		Args:       []string{"server"},
		StartedAt:  time.Now(),
	}
	if err := serviceproc.Write(record); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("记录服务进程失败: %w", err)
	}

	// Wait briefly to catch immediate failures
	time.Sleep(500 * time.Millisecond)
	running, zombie := serviceproc.ProcessStatus(record.PID)
	if !running || zombie {
		serviceproc.RemoveIfPID(record.PID)
		return fmt.Errorf("服务启动后立即退出，请检查日志: %s", logPath)
	}

	fmt.Printf("服务已启动 (PID %d)\n", record.PID)
	fmt.Printf("日志: %s\n", logPath)
	return nil
}

func stopServer() error {
	inspection := inspectServer()
	if inspection.Conflict {
		return fmt.Errorf("端口 %d 被非 TongStock 进程占用 (PID %d)，拒绝停止", configuredPort(), inspection.PID)
	}
	if !inspection.Running {
		fmt.Println("服务未运行")
		return nil
	}

	proc, err := os.FindProcess(inspection.PID)
	if err != nil {
		return fmt.Errorf("查找进程失败: %w", err)
	}

	// Try graceful stop first
	fmt.Printf("正在停止服务 (PID %d)...\n", inspection.PID)
	if err := proc.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("发送 SIGTERM 失败: %w", err)
	}

	// Wait for graceful exit
	deadline := time.Now().Add(8 * time.Second)
	for {
		running, zombie := serviceproc.ProcessStatus(inspection.PID)
		if !running || zombie {
			serviceproc.RemoveIfPID(inspection.PID)
			fmt.Println("服务已停止")
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill
	fmt.Println("优雅停止超时，强制终止...")
	if err := proc.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("强制终止失败: %w", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		running, _ := serviceproc.ProcessStatus(inspection.PID)
		if !running {
			serviceproc.RemoveIfPID(inspection.PID)
			fmt.Println("服务已强制停止")
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("PID %d 未退出", inspection.PID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func showStatus() error {
	inspection := inspectServer()
	if inspection.Conflict {
		fmt.Printf("⚠ 服务未运行，但端口 %d 被其他进程占用 (PID %d)\n", configuredPort(), inspection.PID)
		return nil
	}
	if !inspection.Running {
		fmt.Println("服务未运行")
		return nil
	}

	port := configuredPort()
	source := "TongStock 管理"
	if inspection.External {
		source = "外部进程"
	}

	fmt.Printf("✓ 服务运行中\n")
	fmt.Printf("  PID:    %d\n", inspection.PID)
	fmt.Printf("  端口:   %d\n", port)
	fmt.Printf("  来源:   %s\n", source)
	if inspection.Record.StartedAt.IsZero() {
		fmt.Printf("  启动:   未知\n")
	} else {
		fmt.Printf("  启动:   %s\n", inspection.Record.StartedAt.Format("2006-01-02 15:04:05"))
	}

	// Try to get health info
	health, err := queryHealth(port)
	if err == nil {
		fmt.Printf("  状态:   %s\n", health.Status)
		fmt.Printf("  服务:   %s\n", health.Service)
	}

	return nil
}

func restartServer() error {
	fmt.Println("正在重启服务...")

	if err := stopServer(); err != nil {
		// Ignore "not running" errors
		if !strings.Contains(err.Error(), "未运行") {
			return fmt.Errorf("停止服务失败: %w", err)
		}
	}

	// Brief pause
	time.Sleep(500 * time.Millisecond)

	return startDaemon()
}

// --- Helper types and functions ---

type serverInspection struct {
	Running  bool
	External bool
	Conflict bool
	PID      int
	Record   serviceproc.Record
}

type serverHealth struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	PID     int    `json:"pid"`
}

func inspectServer() serverInspection {
	// Check PID file first
	if record, err := serviceproc.Read(); err == nil {
		running, zombie := serviceproc.ProcessStatus(record.PID)
		if running && !zombie && serviceproc.Matches(record) {
			return serverInspection{Running: true, PID: record.PID, Record: record}
		}
		serviceproc.RemoveIfPID(record.PID)
	}

	// Fallback: check port and health
	port := configuredPort()
	listenerPIDs, _ := serviceproc.ListenerPIDs(port)
	health, healthErr := queryHealth(port)

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
		running, zombie := serviceproc.ProcessStatus(pid)
		if !running || zombie {
			continue
		}
		record, err := serviceproc.RecordForPID(pid)
		if err == nil {
			return serverInspection{Running: true, External: true, PID: pid, Record: record}
		}
	}

	if len(listenerPIDs) > 0 {
		return serverInspection{Conflict: true, PID: listenerPIDs[0]}
	}
	return serverInspection{}
}

func configuredPort() int {
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

func queryHealth(port int) (serverHealth, error) {
	// Use simple HTTP request without importing net/http
	// to avoid circular dependencies
	cmd := exec.Command("curl", "-s", "-m", "1", fmt.Sprintf("http://127.0.0.1:%d/health", port))
	out, err := cmd.Output()
	if err != nil {
		return serverHealth{}, err
	}

	var health serverHealth
	// Simple JSON parse without importing encoding/json
	text := strings.TrimSpace(string(out))
	if !strings.Contains(text, "\"status\"") {
		return serverHealth{}, fmt.Errorf("invalid health response")
	}

	// Extract status
	if idx := strings.Index(text, "\"status\""); idx >= 0 {
		rest := text[idx+8:]
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			rest = rest[colonIdx+1:]
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "\"") {
				rest = rest[1:]
				if endIdx := strings.Index(rest, "\""); endIdx >= 0 {
					health.Status = rest[:endIdx]
				}
			}
		}
	}

	// Extract service
	if idx := strings.Index(text, "\"service\""); idx >= 0 {
		rest := text[idx+9:]
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			rest = rest[colonIdx+1:]
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, "\"") {
				rest = rest[1:]
				if endIdx := strings.Index(rest, "\""); endIdx >= 0 {
					health.Service = rest[:endIdx]
				}
			}
		}
	}

	// Extract pid
	if idx := strings.Index(text, "\"pid\""); idx >= 0 {
		rest := text[idx+5:]
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			rest = rest[colonIdx+1:]
			rest = strings.TrimSpace(rest)
			endIdx := 0
			for endIdx < len(rest) && rest[endIdx] >= '0' && rest[endIdx] <= '9' {
				endIdx++
			}
			if endIdx > 0 {
				fmt.Sscanf(rest[:endIdx], "%d", &health.PID)
			}
		}
	}

	return health, nil
}
