package serviceproc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sjzsdu/tongstock/pkg/config"
)

// Record identifies a TongStock server process owned by the current user.
type Record struct {
	PID        int       `json:"pid"`
	Executable string    `json:"executable,omitempty"`
	Args       []string  `json:"args,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
}

func PIDFilePath() string {
	return filepath.Join(config.HomeDir(), "server.pid")
}

func CurrentRecord() Record {
	executable, _ := os.Executable()
	return Record{
		PID:        os.Getpid(),
		Executable: executable,
		Args:       append([]string(nil), os.Args[1:]...),
		StartedAt:  time.Now(),
	}
}

func Read() (Record, error) {
	data, err := os.ReadFile(PIDFilePath())
	if err != nil {
		return Record{}, err
	}

	var record Record
	if err := json.Unmarshal(data, &record); err == nil && record.PID > 0 {
		return record, nil
	}

	// Backward compatibility with the previous plain-text PID file.
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return Record{}, errors.New("invalid server PID file")
	}
	return Record{PID: pid}, nil
}

func Write(record Record) error {
	if record.PID <= 0 {
		return errors.New("invalid server PID")
	}
	if err := os.MkdirAll(config.HomeDir(), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmpPath := PIDFilePath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, PIDFilePath())
}

func RemoveIfPID(pid int) {
	record, err := Read()
	if err == nil && record.PID == pid {
		_ = os.Remove(PIDFilePath())
	}
}

func ProcessStatus(pid int) (running bool, zombie bool) {
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

func CommandLine(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func IsTongStockCommand(commandLine string) bool {
	fields := strings.Fields(commandLine)
	if len(fields) == 0 {
		return false
	}

	name := strings.TrimSuffix(filepath.Base(fields[0]), filepath.Ext(fields[0]))
	if name == "tongstock-server" {
		return true
	}
	if name != "tongstock" {
		return false
	}
	return len(fields) > 1 && fields[1] == "server"
}

func Matches(record Record) bool {
	commandLine, err := CommandLine(record.PID)
	if err != nil {
		return false
	}
	if record.Executable == "" {
		return IsTongStockCommand(commandLine)
	}

	fields := strings.Fields(commandLine)
	if len(fields) == 0 {
		return false
	}
	executableMatches := fields[0] == record.Executable ||
		filepath.Base(fields[0]) == filepath.Base(record.Executable)
	if !executableMatches {
		return false
	}
	for _, arg := range record.Args {
		found := false
		for _, actual := range fields[1:] {
			if actual == arg {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func ListenerPIDs(port int) ([]int, error) {
	out, err := exec.Command(
		"lsof", "-nP", "-tiTCP:"+strconv.Itoa(port), "-sTCP:LISTEN",
	).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(out) == 0 {
			return nil, nil
		}
		return nil, err
	}

	seen := make(map[int]bool)
	var pids []int
	for _, line := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids, nil
}

func RecordForPID(pid int) (Record, error) {
	commandLine, err := CommandLine(pid)
	if err != nil {
		return Record{}, err
	}
	if !IsTongStockCommand(commandLine) {
		return Record{}, fmt.Errorf("PID %d is not a TongStock server", pid)
	}
	fields := strings.Fields(commandLine)
	record := Record{PID: pid}
	if len(fields) > 0 {
		record.Executable = fields[0]
		record.Args = append([]string(nil), fields[1:]...)
	}
	return record, nil
}
