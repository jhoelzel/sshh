package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"shh-h/internal/terminalbenchmark"
)

const sampleInterval = 50 * time.Millisecond

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var appPath string
	var reportPath string
	var timeout time.Duration
	flag.StringVar(&appPath, "app", "build/bin/shh-h.app/Contents/MacOS/shhh", "packaged benchmark executable")
	flag.StringVar(&reportPath, "report", "docs/benchmarks/m1-macos-arm64.json", "final benchmark report")
	flag.DurationVar(&timeout, "timeout", 45*time.Second, "maximum packaged benchmark duration")
	flag.Parse()

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("packaged WKWebView benchmark requires macOS, got %s", runtime.GOOS)
	}
	appPath, err := filepath.Abs(appPath)
	if err != nil {
		return fmt.Errorf("resolve benchmark app path: %w", err)
	}
	if info, err := os.Stat(appPath); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("benchmark app is not executable: %s", appPath)
	}
	reportPath, err = filepath.Abs(reportPath)
	if err != nil {
		return fmt.Errorf("resolve benchmark report path: %w", err)
	}

	runDirectory, err := os.MkdirTemp("", "shhh-terminal-benchmark-*")
	if err != nil {
		return fmt.Errorf("create benchmark directory: %w", err)
	}
	defer os.RemoveAll(runDirectory)
	home := filepath.Join(runDirectory, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create isolated benchmark home: %w", err)
	}
	rawReport := filepath.Join(runDirectory, "webview-result.json")
	baselineWebKit, err := webKitProcessIDs()
	if err != nil {
		return err
	}

	command := exec.Command(appPath)
	command.Env = benchmarkEnvironment(home, rawReport)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		return fmt.Errorf("launch packaged benchmark: %w", err)
	}

	processDone := make(chan error, 1)
	go func() { processDone <- command.Wait() }()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	var peakRSS uint64
	var peakProcesses int
	var peakWebKitProcesses int
	var samples int
	var processErr error

running:
	for {
		select {
		case processErr = <-processDone:
			break running
		case <-ticker.C:
			rss, processCount, webKitCount, sampleErr := sampleProcessTreeRSS(command.Process.Pid, baselineWebKit)
			if sampleErr == nil && rss > 0 {
				samples++
				peakRSS = max(peakRSS, rss)
				peakProcesses = max(peakProcesses, processCount)
				peakWebKitProcesses = max(peakWebKitProcesses, webKitCount)
			}
		case <-deadline.C:
			_ = command.Process.Kill()
			<-processDone
			return fmt.Errorf("packaged benchmark exceeded %s\n%s", timeout, boundedOutput(output.String()))
		}
	}
	if processErr != nil {
		return fmt.Errorf("packaged benchmark exited unsuccessfully: %w\n%s", processErr, boundedOutput(output.String()))
	}

	report, err := terminalbenchmark.ReadReport(rawReport)
	if err != nil {
		return fmt.Errorf("read packaged benchmark result: %w\n%s", err, boundedOutput(output.String()))
	}
	report.Host = collectHostMetrics()
	report.Host.ProcessTreePeakRSSBytes = peakRSS
	report.Host.ProcessTreePeakProcesses = peakProcesses
	report.Host.WebKitPeakProcesses = peakWebKitProcesses
	report.Host.RSSSamples = samples
	terminalbenchmark.EvaluateHost(&report)
	if err := terminalbenchmark.WriteReportAtomic(reportPath, report); err != nil {
		return err
	}

	fmt.Printf("terminal benchmark: output %.2f ms, idle p95 %.2f ms, flood p95 %.2f ms, resize p95 %.2f ms, close %.2f ms, peak RSS %.1f MiB\n",
		report.OutputDurationMilliseconds,
		report.IdleEchoP95Milliseconds,
		report.FloodEchoP95Milliseconds,
		report.ResizeP95Milliseconds,
		report.CloseDurationMilliseconds,
		float64(report.Host.ProcessTreePeakRSSBytes)/(1024*1024),
	)
	if !report.Passed {
		return fmt.Errorf("terminal benchmark failed: %s", strings.Join(report.Failures, "; "))
	}
	return nil
}

func benchmarkEnvironment(home, resultPath string) []string {
	blocked := map[string]bool{
		"HOME":                                  true,
		terminalbenchmark.EnvironmentEnabled:    true,
		terminalbenchmark.EnvironmentResultPath: true,
		terminalbenchmark.EnvironmentFixture:    true,
	}
	result := make([]string, 0, len(os.Environ())+3)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			result = append(result, item)
		}
	}
	return append(result,
		"HOME="+home,
		terminalbenchmark.EnvironmentEnabled+"=1",
		terminalbenchmark.EnvironmentResultPath+"="+resultPath,
	)
}

type processSample struct {
	pid     int
	ppid    int
	rssKB   uint64
	command string
}

func sampleProcessTreeRSS(rootPID int, baselineWebKit map[int]bool) (uint64, int, int, error) {
	data, err := processTable()
	if err != nil {
		return 0, 0, 0, err
	}
	return processTreeRSS(rootPID, data, baselineWebKit)
}

func processTreeRSS(rootPID int, data []byte, baselineWebKit map[int]bool) (uint64, int, int, error) {
	processes, err := parseProcessTable(data)
	if err != nil {
		return 0, 0, 0, err
	}
	selected := map[int]bool{rootPID: true}
	for changed := true; changed; {
		changed = false
		for _, process := range processes {
			if !selected[process.pid] && selected[process.ppid] {
				selected[process.pid] = true
				changed = true
			}
		}
	}
	var totalKB uint64
	processCount := 0
	webKitCount := 0
	for _, process := range processes {
		benchmarkWebKit := isWebKitHelper(process.command) && !baselineWebKit[process.pid]
		if selected[process.pid] || benchmarkWebKit {
			totalKB += process.rssKB
			processCount++
			if benchmarkWebKit {
				webKitCount++
			}
		}
	}
	if totalKB == 0 {
		return 0, 0, 0, errors.New("benchmark process was absent from process sample")
	}
	return totalKB * 1024, processCount, webKitCount, nil
}

func webKitProcessIDs() (map[int]bool, error) {
	data, err := processTable()
	if err != nil {
		return nil, err
	}
	processes, err := parseProcessTable(data)
	if err != nil {
		return nil, err
	}
	result := make(map[int]bool)
	for _, process := range processes {
		if isWebKitHelper(process.command) {
			result[process.pid] = true
		}
	}
	return result, nil
}

func processTable() ([]byte, error) {
	data, err := exec.Command("ps", "-axo", "pid=,ppid=,rss=,comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("sample process table: %w", err)
	}
	return data, nil
}

func parseProcessTable(data []byte) ([]processSample, error) {
	var processes []processSample
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		rss, rssErr := strconv.ParseUint(fields[2], 10, 64)
		if pidErr == nil && ppidErr == nil && rssErr == nil {
			processes = append(processes, processSample{
				pid: pid, ppid: ppid, rssKB: rss, command: strings.Join(fields[3:], " "),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return processes, nil
}

func isWebKitHelper(command string) bool {
	return strings.Contains(command, "/WebKit.framework/") && strings.Contains(command, "/com.apple.WebKit.")
}

func collectHostMetrics() terminalbenchmark.HostMetrics {
	return terminalbenchmark.HostMetrics{
		Model:                  commandValue("sysctl", "-n", "hw.model"),
		Processor:              firstValue(commandValue("sysctl", "-n", "machdep.cpu.brand_string"), commandValue("sysctl", "-n", "hw.machine")),
		OperatingSystemVersion: strings.TrimSpace(commandValue("sw_vers", "-productVersion") + " (" + commandValue("sw_vers", "-buildVersion") + ")"),
		MemoryBytes:            uintValue(commandValue("sysctl", "-n", "hw.memsize")),
	}
}

func commandValue(name string, arguments ...string) string {
	data, err := exec.Command(name, arguments...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func firstValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func uintValue(value string) uint64 {
	result, _ := strconv.ParseUint(value, 10, 64)
	return result
}

func boundedOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4_000 {
		return value[len(value)-4_000:]
	}
	return value
}
