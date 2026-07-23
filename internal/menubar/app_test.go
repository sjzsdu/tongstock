package menubar

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
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

func isolateServiceDiscovery(t *testing.T) {
	t.Helper()
	oldListenerFinder := listenerFinder
	oldHealthFinder := healthFinder
	listenerFinder = func(port int) ([]int, error) { return nil, nil }
	healthFinder = func(port int) (serverHealth, error) {
		return serverHealth{}, errors.New("not running")
	}
	t.Cleanup(func() {
		listenerFinder = oldListenerFinder
		healthFinder = oldHealthFinder
	})
}

func copyTestServerBinary(t *testing.T, dir string) string {
	t.Helper()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	serverPath := filepath.Join(dir, "tongstock-server")
	binaryData, err := os.ReadFile(testBinary)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	if err := os.WriteFile(serverPath, binaryData, 0755); err != nil {
		t.Fatalf("write helper server: %v", err)
	}
	return serverPath
}

func TestPIDRecordRoundTripAndLegacyCompatibility(t *testing.T) {
	home := useTemporaryHome(t)
	record := serverPIDRecord{
		PID:        12345,
		Executable: "/tmp/tongstock-server",
		Args:       []string{"server"},
		StartedAt:  time.Now().Truncate(time.Second),
	}

	if err := writePIDRecord(record); err != nil {
		t.Fatalf("writePIDRecord() error = %v", err)
	}
	got, err := readPIDRecord()
	if err != nil {
		t.Fatalf("readPIDRecord() error = %v", err)
	}
	if got.PID != record.PID || got.Executable != record.Executable || len(got.Args) != 1 || got.Args[0] != "server" || !got.StartedAt.Equal(record.StartedAt) {
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

func TestUnifiedServerCommandUsesSameExecutable(t *testing.T) {
	command := unifiedServerCommand("/Applications/TongStock/tongstock")
	if command.Executable != "/Applications/TongStock/tongstock" {
		t.Fatalf("Executable = %q", command.Executable)
	}
	if len(command.Args) != 1 || command.Args[0] != "server" {
		t.Fatalf("Args = %#v, want [server]", command.Args)
	}

	if command := unifiedServerCommand("/Applications/TongStock/tongstock-menubar"); command.Executable != "" {
		t.Fatalf("legacy executable unexpectedly selected unified mode: %#v", command)
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
	isolateServiceDiscovery(t)
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
	isolateServiceDiscovery(t)
	serverPath := copyTestServerBinary(t, home)

	previousFinder := serverFinder
	serverFinder = func() serverCommand {
		return serverCommand{
			Executable: serverPath,
			Args:       []string{"-test.run=TestTongStockServerHelper"},
		}
	}
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

func TestInspectServiceDiscoversExternalTongStock(t *testing.T) {
	useTemporaryHome(t)
	oldListenerFinder := listenerFinder
	oldProcessFinder := processFinder
	oldStatusFinder := statusFinder
	oldHealthFinder := healthFinder
	listenerFinder = func(port int) ([]int, error) { return []int{4242}, nil }
	processFinder = func(pid int) (serverPIDRecord, error) {
		return serverPIDRecord{PID: pid, Executable: "/tmp/tongstock-server"}, nil
	}
	statusFinder = func(pid int) (bool, bool) { return true, false }
	healthFinder = func(port int) (serverHealth, error) {
		return serverHealth{Status: "ok"}, nil
	}
	defer func() {
		listenerFinder = oldListenerFinder
		processFinder = oldProcessFinder
		statusFinder = oldStatusFinder
		healthFinder = oldHealthFinder
	}()

	inspection := inspectService()
	if !inspection.Running || !inspection.External || inspection.Conflict || inspection.PID != 4242 {
		t.Fatalf("inspectService() = %#v", inspection)
	}
}

func TestInspectServiceMarksUnrelatedListenerAsConflict(t *testing.T) {
	useTemporaryHome(t)
	oldListenerFinder := listenerFinder
	oldProcessFinder := processFinder
	oldStatusFinder := statusFinder
	oldHealthFinder := healthFinder
	listenerFinder = func(port int) ([]int, error) { return []int{5252}, nil }
	processFinder = func(pid int) (serverPIDRecord, error) {
		return serverPIDRecord{}, fmt.Errorf("not TongStock")
	}
	statusFinder = func(pid int) (bool, bool) { return true, false }
	healthFinder = func(port int) (serverHealth, error) {
		return serverHealth{}, errors.New("not TongStock")
	}
	defer func() {
		listenerFinder = oldListenerFinder
		processFinder = oldProcessFinder
		statusFinder = oldStatusFinder
		healthFinder = oldHealthFinder
	}()

	inspection := inspectService()
	if inspection.Running || !inspection.Conflict || inspection.PID != 5252 {
		t.Fatalf("inspectService() = %#v", inspection)
	}
}

func TestStopManagedServerStopsExternalTongStockProcess(t *testing.T) {
	home := useTemporaryHome(t)
	healthBefore := healthFinder
	listenerBefore := listenerFinder
	healthFinder = func(port int) (serverHealth, error) {
		return serverHealth{}, errors.New("legacy health response")
	}

	serverPath := copyTestServerBinary(t, home)

	cmd := exec.Command(serverPath, "-test.run=TestTongStockServerHelper")
	cmd.Env = append(os.Environ(), "TONGSTOCK_TEST_SERVER_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start external helper: %v", err)
	}
	listenerFinder = func(port int) ([]int, error) { return []int{cmd.Process.Pid}, nil }
	defer func() {
		listenerFinder = listenerBefore
		healthFinder = healthBefore
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if inspection := inspectService(); inspection.Running && inspection.External {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("external TongStock helper was not discovered")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := stopManagedServer(); err != nil {
		t.Fatalf("stopManagedServer() external error = %v", err)
	}
	if !waitForProcessExit(cmd.Process.Pid, time.Second) {
		t.Fatalf("external PID %d still running after stop", cmd.Process.Pid)
	}
}

func TestStopManagedServerNeverKillsUnrelatedListener(t *testing.T) {
	useTemporaryHome(t)
	cmd := exec.Command("/bin/sh", "-c", `trap 'exit 0' TERM; while :; do sleep 0.1; done`)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start unrelated helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGKILL)
		_ = cmd.Wait()
	}()

	listenerBefore := listenerFinder
	healthBefore := healthFinder
	listenerFinder = func(port int) ([]int, error) { return []int{cmd.Process.Pid}, nil }
	healthFinder = func(port int) (serverHealth, error) {
		return serverHealth{}, errors.New("not TongStock")
	}
	defer func() {
		listenerFinder = listenerBefore
		healthFinder = healthBefore
	}()

	if err := stopManagedServer(); err == nil {
		t.Fatal("stopManagedServer() unexpectedly accepted unrelated listener")
	}
	running, zombie := processStatus(cmd.Process.Pid)
	if !running || zombie {
		t.Fatalf("unrelated PID %d was stopped", cmd.Process.Pid)
	}
}

func TestTongStockServerHelper(t *testing.T) {
	if filepath.Base(os.Args[0]) != "tongstock-server" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	<-signals
	os.Exit(0)
}

func TestReapServerProcessClearsOnlyMatchingState(t *testing.T) {
	useTemporaryHome(t)
	isolateServiceDiscovery(t)
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
