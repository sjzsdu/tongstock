package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func useTemporaryHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestPIDRecordRoundTripAndLegacyCompatibility(t *testing.T) {
	home := useTemporaryHome(t)
	record := serverPIDRecord{
		PID:        12345,
		Executable: "/tmp/tongstock-server",
		StartedAt:  time.Now().Truncate(time.Second),
	}

	if err := writePIDRecord(record); err != nil {
		t.Fatalf("writePIDRecord() error = %v", err)
	}
	got, err := readPIDRecord()
	if err != nil {
		t.Fatalf("readPIDRecord() error = %v", err)
	}
	if got.PID != record.PID || got.Executable != record.Executable || !got.StartedAt.Equal(record.StartedAt) {
		t.Fatalf("readPIDRecord() = %#v, want %#v", got, record)
	}

	legacyPID := 54321
	if err := os.WriteFile(filepath.Join(home, ".tongstock", "server.pid"), []byte(strconv.Itoa(legacyPID)), 0644); err != nil {
		t.Fatalf("write legacy PID file: %v", err)
	}
	got, err = readPIDRecord()
	if err != nil {
		t.Fatalf("readPIDRecord() legacy error = %v", err)
	}
	if got.PID != legacyPID || got.Executable != "" {
		t.Fatalf("readPIDRecord() legacy = %#v, want PID %d", got, legacyPID)
	}
}

func TestProcessStatusRecognizesZombieAsExited(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer cmd.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		running, zombie := processStatus(cmd.Process.Pid)
		if running && zombie {
			if !waitForProcessExit(cmd.Process.Pid, 100*time.Millisecond) {
				t.Fatal("waitForProcessExit() did not treat zombie as exited")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("processStatus() did not recognize the exited child as a zombie")
}

func TestStopManagedServerWaitsAndClearsPID(t *testing.T) {
	useTemporaryHome(t)
	cmd := exec.Command("/bin/sh", "-c", `trap 'exit 0' TERM; while :; do sleep 0.1; done`)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	record := serverPIDRecord{
		PID:        cmd.Process.Pid,
		Executable: "/bin/sh",
		StartedAt:  time.Now(),
	}
	if err := writePIDRecord(record); err != nil {
		t.Fatalf("writePIDRecord() error = %v", err)
	}
	if err := stopManagedServer(); err != nil {
		t.Fatalf("stopManagedServer() error = %v", err)
	}
	if _, err := os.Stat(pidFilePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PID file still exists after stop: %v", err)
	}
	if !waitForProcessExit(record.PID, time.Second) {
		t.Fatalf("PID %d is still running after stop", record.PID)
	}
}

func TestManagedServerStartRestartStopLifecycle(t *testing.T) {
	home := useTemporaryHome(t)
	serverPath := filepath.Join(home, "tongstock-server")
	serverScript := []byte("#!/bin/sh\ntrap 'exit 0' TERM INT\nwhile :; do sleep 0.1; done\n")
	if err := os.WriteFile(serverPath, serverScript, 0755); err != nil {
		t.Fatalf("write fake server: %v", err)
	}

	previousFinder := serverFinder
	serverFinder = func() string { return serverPath }
	defer func() {
		serverFinder = previousFinder
		_ = stopManagedServer()
		setServiceState(false, 0)
	}()

	if err := startManagedServer(); err != nil {
		t.Fatalf("first startManagedServer() error = %v", err)
	}
	first, err := readPIDRecord()
	if err != nil {
		t.Fatalf("read first PID record: %v", err)
	}
	if running, pid := isServerRunning(); !running || pid != first.PID {
		t.Fatalf("first server status = running %v, PID %d; want PID %d", running, pid, first.PID)
	}

	if err := stopManagedServer(); err != nil {
		t.Fatalf("stop before restart error = %v", err)
	}
	if err := startManagedServer(); err != nil {
		t.Fatalf("restart startManagedServer() error = %v", err)
	}
	second, err := readPIDRecord()
	if err != nil {
		t.Fatalf("read second PID record: %v", err)
	}
	if second.PID == first.PID {
		t.Fatalf("restart reused PID %d", second.PID)
	}
	if running, _ := processStatus(first.PID); running {
		t.Fatalf("old PID %d is still running after restart", first.PID)
	}

	if err := stopManagedServer(); err != nil {
		t.Fatalf("final stopManagedServer() error = %v", err)
	}
	if _, err := os.Stat(pidFilePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PID file still exists after final stop: %v", err)
	}
}

func TestReapServerProcessClearsOnlyMatchingState(t *testing.T) {
	useTemporaryHome(t)
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	pid := cmd.Process.Pid
	if err := writePIDRecord(serverPIDRecord{PID: pid, Executable: "/bin/sh"}); err != nil {
		t.Fatalf("writePIDRecord() error = %v", err)
	}
	setServiceState(true, pid)

	reapServerProcess(cmd, pid)

	if _, err := os.Stat(pidFilePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PID file still exists after reap: %v", err)
	}
	mu.Lock()
	running, gotPID := serviceRunning, servicePID
	mu.Unlock()
	if running || gotPID != 0 {
		t.Fatalf("service state after reap = running %v, PID %d", running, gotPID)
	}
}
