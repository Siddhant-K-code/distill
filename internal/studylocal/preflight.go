package studylocal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ProcessSnapshot struct {
	PID      int    `json:"pid"`
	ParentID int    `json:"parent_id"`
	RSSBytes int64  `json:"rss_bytes"`
	Command  string `json:"command"`
}

type HostSnapshot struct {
	SchemaVersion             string            `json:"schema_version"`
	CollectedAt               time.Time         `json:"collected_at"`
	OSVersion                 string            `json:"os_version"`
	OSBuild                   string            `json:"os_build"`
	Architecture              string            `json:"architecture"`
	Chip                      string            `json:"chip"`
	PhysicalMemoryBytes       int64             `json:"physical_memory_bytes"`
	VMAvailablePercent        float64           `json:"vm_available_percent"`
	MemoryPressureFreePercent float64           `json:"memory_pressure_free_percent"`
	SwapUsedBytes             int64             `json:"swap_used_bytes"`
	DiskFreeBytes             int64             `json:"disk_free_bytes"`
	PowerSource               string            `json:"power_source"`
	SandboxExecAvailable      bool              `json:"sandbox_exec_available"`
	XCTraceVersion            string            `json:"xctrace_version,omitempty"`
	MetalTraceAvailable       bool              `json:"metal_trace_available"`
	HeavyProcesses            []ProcessSnapshot `json:"heavy_processes"`
	Eligible                  bool              `json:"eligible"`
	Failures                  []string          `json:"failures"`
}

type systemProbe interface {
	Run(context.Context, string, ...string) ([]byte, error)
	StatFS(string) (int64, error)
	PID() int
}

type realSystemProbe struct{}

func (realSystemProbe) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=/dev/null", "LC_ALL=C", "LANG=C"}
	return command.Output()
}

func (realSystemProbe) StatFS(path string) (int64, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0, err
	}
	return int64(stats.Bavail) * int64(stats.Bsize), nil
}

func (realSystemProbe) PID() int { return os.Getpid() }

func CollectHostPreflight(ctx context.Context, outputParent string, limits IsolationLimits) (HostSnapshot, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return HostSnapshot{}, fmt.Errorf("local MLX execution requires darwin/arm64")
	}
	return collectHostPreflight(ctx, outputParent, limits, realSystemProbe{})
}

func collectHostPreflight(ctx context.Context, outputParent string, limits IsolationLimits, probe systemProbe) (HostSnapshot, error) {
	snapshot := HostSnapshot{SchemaVersion: SchemaVersion + "/host-preflight", CollectedAt: time.Now().UTC()}
	run := func(name string, args ...string) (string, error) {
		output, err := probe.Run(ctx, name, args...)
		if err != nil {
			return "", fmt.Errorf("%s: %w", filepath.Base(name), err)
		}
		return strings.TrimSpace(string(output)), nil
	}
	var err error
	if snapshot.OSVersion, err = run("/usr/bin/sw_vers", "-productVersion"); err != nil {
		return snapshot, err
	}
	if snapshot.OSBuild, err = run("/usr/bin/sw_vers", "-buildVersion"); err != nil {
		return snapshot, err
	}
	if snapshot.Architecture, err = run("/usr/bin/uname", "-m"); err != nil {
		return snapshot, err
	}
	if snapshot.Chip, err = run("/usr/sbin/sysctl", "-n", "machdep.cpu.brand_string"); err != nil {
		return snapshot, err
	}
	memory, err := run("/usr/sbin/sysctl", "-n", "hw.memsize")
	if err != nil {
		return snapshot, err
	}
	snapshot.PhysicalMemoryBytes, err = strconv.ParseInt(memory, 10, 64)
	if err != nil {
		return snapshot, fmt.Errorf("parse physical memory: %w", err)
	}
	vm, err := run("/usr/bin/vm_stat")
	if err != nil {
		return snapshot, err
	}
	snapshot.VMAvailablePercent, err = parseVMAvailablePercent(vm, snapshot.PhysicalMemoryBytes)
	if err != nil {
		return snapshot, err
	}
	pressure, err := run("/usr/bin/memory_pressure", "-Q")
	if err != nil {
		return snapshot, err
	}
	snapshot.MemoryPressureFreePercent, err = parseMemoryPressureFreePercent(pressure)
	if err != nil {
		return snapshot, err
	}
	swap, err := run("/usr/sbin/sysctl", "-n", "vm.swapusage")
	if err != nil {
		return snapshot, err
	}
	snapshot.SwapUsedBytes, err = parseSwapUsedBytes(swap)
	if err != nil {
		return snapshot, err
	}
	snapshot.DiskFreeBytes, err = probe.StatFS(outputParent)
	if err != nil {
		return snapshot, err
	}
	power, err := run("/usr/bin/pmset", "-g", "batt")
	if err != nil {
		return snapshot, err
	}
	if strings.Contains(power, "AC Power") {
		snapshot.PowerSource = "AC Power"
	} else {
		snapshot.PowerSource = "Battery Power"
	}
	sandboxProbe := `import socket
s = socket.socket()
try:
    s.connect(("127.0.0.1", 9))
except PermissionError:
    raise SystemExit(0)
except OSError:
    raise SystemExit(2)
raise SystemExit(1)`
	if _, err := run("/usr/bin/sandbox-exec", "-p", SandboxPolicy, "/usr/bin/python3", "-I", "-B", "-c", sandboxProbe); err == nil {
		snapshot.SandboxExecAvailable = true
	}
	if xctrace, err := run("/usr/bin/xcrun", "xctrace", "version"); err == nil {
		snapshot.XCTraceVersion = xctrace
		snapshot.MetalTraceAvailable = true
	}
	processes, err := run("/bin/ps", "-axo", "pid=,ppid=,rss=,comm=")
	if err != nil {
		return snapshot, err
	}
	allProcesses, err := parseProcesses(processes)
	if err != nil {
		return snapshot, err
	}
	excluded := ancestorPIDs(allProcesses, probe.PID())
	for _, process := range allProcesses {
		if process.RSSBytes >= limits.MaximumUnrelatedRSSBytes && !excluded[process.PID] {
			snapshot.HeavyProcesses = append(snapshot.HeavyProcesses, process)
		}
	}
	sort.Slice(snapshot.HeavyProcesses, func(i, j int) bool {
		if snapshot.HeavyProcesses[i].RSSBytes == snapshot.HeavyProcesses[j].RSSBytes {
			return snapshot.HeavyProcesses[i].PID < snapshot.HeavyProcesses[j].PID
		}
		return snapshot.HeavyProcesses[i].RSSBytes > snapshot.HeavyProcesses[j].RSSBytes
	})
	if snapshot.OSVersion != "27.0" || snapshot.OSBuild != "26A428" {
		snapshot.Failures = append(snapshot.Failures, "host_os_mismatch")
	}
	if snapshot.Architecture != limits.RequiredArchitecture || snapshot.Chip != limits.RequiredHardware ||
		snapshot.PhysicalMemoryBytes != limits.RequiredPhysicalMemoryBytes {
		snapshot.Failures = append(snapshot.Failures, "host_hardware_mismatch")
	}
	if snapshot.VMAvailablePercent < float64(limits.MinimumFreeMemoryPercent) ||
		snapshot.MemoryPressureFreePercent < float64(limits.MinimumFreeMemoryPercent) {
		snapshot.Failures = append(snapshot.Failures, "memory_pressure_exceeds_preflight_bound")
	}
	if snapshot.SwapUsedBytes > limits.MaximumSwapUsedBytes {
		snapshot.Failures = append(snapshot.Failures, "swap_exceeds_preflight_bound")
	}
	if snapshot.DiskFreeBytes < limits.MinimumDiskFreeBytes {
		snapshot.Failures = append(snapshot.Failures, "disk_below_preflight_bound")
	}
	if limits.RequireACPower && snapshot.PowerSource != "AC Power" {
		snapshot.Failures = append(snapshot.Failures, "ac_power_required")
	}
	if limits.RequireNetworkDenySandbox && !snapshot.SandboxExecAvailable {
		snapshot.Failures = append(snapshot.Failures, "sandbox_exec_unavailable")
	}
	if len(snapshot.HeavyProcesses) > 0 {
		snapshot.Failures = append(snapshot.Failures, "unrelated_high_memory_process")
	}
	snapshot.Eligible = len(snapshot.Failures) == 0
	return snapshot, nil
}

func parseVMAvailablePercent(output string, totalBytes int64) (float64, error) {
	pagePattern := regexp.MustCompile(`page size of ([0-9]+) bytes`)
	match := pagePattern.FindStringSubmatch(output)
	if len(match) != 2 || totalBytes <= 0 {
		return 0, fmt.Errorf("invalid vm_stat header")
	}
	pageBytes, _ := strconv.ParseInt(match[1], 10, 64)
	var pages int64
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Pages free", "Pages inactive", "Pages speculative":
			value = strings.TrimSuffix(strings.TrimSpace(value), ".")
			count, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid vm_stat page count")
			}
			pages += count
		}
	}
	return float64(pages*pageBytes) / float64(totalBytes) * 100, nil
}

func parseMemoryPressureFreePercent(output string) (float64, error) {
	pattern := regexp.MustCompile(`(?i)system-wide memory free percentage:\s*([0-9]+(?:\.[0-9]+)?)%`)
	match := pattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf("memory_pressure free percentage missing")
	}
	return strconv.ParseFloat(match[1], 64)
}

func parseSwapUsedBytes(output string) (int64, error) {
	pattern := regexp.MustCompile(`used = ([0-9]+(?:\.[0-9]+)?)([MG])`)
	match := pattern.FindStringSubmatch(output)
	if len(match) != 3 {
		return 0, fmt.Errorf("swap usage missing")
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, err
	}
	multiplier := float64(1 << 20)
	if match[2] == "G" {
		multiplier = 1 << 30
	}
	return int64(value * multiplier), nil
}

func parseProcesses(output string) ([]ProcessSnapshot, error) {
	var processes []ProcessSnapshot
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			return nil, fmt.Errorf("invalid process row")
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, err
		}
		parent, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, err
		}
		rssKB, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return nil, err
		}
		processes = append(processes, ProcessSnapshot{
			PID: pid, ParentID: parent, RSSBytes: rssKB * 1024, Command: strings.Join(fields[3:], " "),
		})
	}
	return processes, nil
}

func ancestorPIDs(processes []ProcessSnapshot, pid int) map[int]bool {
	parents := map[int]int{}
	for _, process := range processes {
		parents[process.PID] = process.ParentID
	}
	out := map[int]bool{}
	for pid > 0 && !out[pid] {
		out[pid] = true
		pid = parents[pid]
	}
	return out
}

func ReadHostSnapshot(data []byte) (HostSnapshot, error) {
	var snapshot HostSnapshot
	if err := strictDecode(data, &snapshot); err != nil {
		return HostSnapshot{}, err
	}
	return snapshot, nil
}

func encodeHostSnapshot(snapshot HostSnapshot) ([]byte, error) {
	return canonicalJSONFile(snapshot)
}

func hostSnapshotEqual(left, right HostSnapshot) bool {
	left.CollectedAt = time.Time{}
	right.CollectedAt = time.Time{}
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return bytes.Equal(leftBytes, rightBytes)
}
