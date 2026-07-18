package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"shh-h/internal/terminalbenchmark"
)

const (
	burstSampleInterval = 50 * time.Millisecond
	soakSampleInterval  = time.Second
	soakRSSWarmupStart  = time.Minute
	soakRSSWarmupEnd    = 2 * time.Minute
	soakRSSEndWindow    = time.Minute
)

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
	var modeValue string
	defaultAppPath := "build/bin/shh-h.app/Contents/MacOS/shhh"
	if runtime.GOOS == "linux" {
		defaultAppPath = "build/bin/shhh-linux-smoke"
	}
	flag.StringVar(&appPath, "app", defaultAppPath, "packaged benchmark executable")
	flag.StringVar(&reportPath, "report", "", "final benchmark report")
	flag.DurationVar(&timeout, "timeout", 0, "maximum packaged benchmark duration")
	flag.StringVar(&modeValue, "mode", string(terminalbenchmark.ModeBurst), "benchmark mode: burst, smoke, or soak")
	flag.Parse()
	mode, err := terminalbenchmark.ParseMode(modeValue)
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
		if mode == terminalbenchmark.ModeSoak {
			timeout = terminalbenchmark.SoakDuration + 5*time.Minute
		}
	}
	if reportPath == "" {
		reportPath = "docs/benchmarks/m1-macos-arm64.json"
		if mode == terminalbenchmark.ModeSoak {
			reportPath = "docs/benchmarks/m1-macos-arm64-soak.json"
		} else if mode == terminalbenchmark.ModeSmoke {
			reportPath = filepath.Join(os.TempDir(), "m2-linux-amd64-smoke.json")
		}
	}

	switch runtime.GOOS {
	case "darwin":
		if mode == terminalbenchmark.ModeSmoke {
			return errors.New("Linux native smoke mode requires Linux")
		}
	case "linux":
		if mode != terminalbenchmark.ModeSmoke {
			return errors.New("Linux supports only native smoke mode")
		}
	default:
		return fmt.Errorf("packaged WebView benchmark does not support %s", runtime.GOOS)
	}
	appPath, err = filepath.Abs(appPath)
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
	command.Env = benchmarkEnvironment(home, rawReport, mode)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = io.MultiWriter(&output, os.Stderr)
	if err := command.Start(); err != nil {
		return fmt.Errorf("launch packaged benchmark: %w", err)
	}
	launchedAt := time.Now()

	processDone := make(chan error, 1)
	go func() { processDone <- command.Wait() }()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	sampleInterval := burstSampleInterval
	if mode == terminalbenchmark.ModeSoak {
		sampleInterval = soakSampleInterval
	}
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	var peakRSS uint64
	var peakProcesses int
	var peakWebKitProcesses int
	var samples int
	var rssReadings []rssReading
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
				rssReadings = append(rssReadings, rssReading{elapsed: time.Since(launchedAt), bytes: rss})
				peakRSS = max(peakRSS, rss)
				peakProcesses = max(peakProcesses, processCount)
				peakWebKitProcesses = max(peakWebKitProcesses, webKitCount)
			}
		case <-deadline.C:
			_ = command.Process.Kill()
			<-processDone
			if mode == terminalbenchmark.ModeSoak {
				host := sampledHostMetrics(peakRSS, peakProcesses, peakWebKitProcesses, samples)
				if salvageErr := salvageTimedOutSoak(rawReport, reportPath, host, rssReadings); salvageErr != nil {
					return fmt.Errorf("packaged benchmark exceeded %s; preserve partial report: %v\n%s", timeout, salvageErr, boundedOutput(output.String()))
				}
			}
			return fmt.Errorf("packaged benchmark exceeded %s\n%s", timeout, boundedOutput(output.String()))
		}
	}
	if processErr != nil {
		return fmt.Errorf("packaged benchmark exited unsuccessfully: %w\n%s", processErr, boundedOutput(output.String()))
	}

	host := sampledHostMetrics(peakRSS, peakProcesses, peakWebKitProcesses, samples)

	if mode == terminalbenchmark.ModeSoak {
		return completeSoak(rawReport, reportPath, output.String(), host, rssReadings)
	}
	if mode == terminalbenchmark.ModeSmoke {
		return completeLinuxSmoke(rawReport, reportPath, output.String(), host)
	}
	return completeBurst(rawReport, reportPath, output.String(), host)
}

func sampledHostMetrics(peakRSS uint64, peakProcesses, peakWebKitProcesses, samples int) terminalbenchmark.HostMetrics {
	host := collectHostMetrics()
	host.ProcessTreePeakRSSBytes = peakRSS
	host.ProcessTreePeakProcesses = peakProcesses
	host.WebKitPeakProcesses = peakWebKitProcesses
	host.RSSSamples = samples
	return host
}

func salvageTimedOutSoak(
	rawReport, reportPath string,
	host terminalbenchmark.HostMetrics,
	readings []rssReading,
) error {
	report, err := terminalbenchmark.ReadSoakReport(rawReport)
	if err != nil {
		return err
	}
	startRSS, endRSS, growthRSS, startSamples, endSamples := steadyStateRSS(readings)
	host.SteadyStateStartRSSBytes = startRSS
	host.SteadyStateEndRSSBytes = endRSS
	host.SteadyStateGrowthRSSBytes = growthRSS
	host.SteadyStateStartSamples = startSamples
	host.SteadyStateEndSamples = endSamples
	report.Host = host
	report.Failures = append(report.Failures, "packaged application did not exit before the host timeout")
	terminalbenchmark.EvaluateSoakHost(&report)
	return terminalbenchmark.WriteSoakReportAtomic(reportPath, report)
}

func completeBurst(rawReport, reportPath, processOutput string, host terminalbenchmark.HostMetrics) error {
	report, err := terminalbenchmark.ReadReport(rawReport)
	if err != nil {
		return fmt.Errorf("read packaged benchmark result: %w\n%s", err, boundedOutput(processOutput))
	}
	report.Host = host
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

func completeLinuxSmoke(rawReport, reportPath, processOutput string, host terminalbenchmark.HostMetrics) error {
	report, err := terminalbenchmark.ReadReport(rawReport)
	if err != nil {
		return fmt.Errorf("read packaged Linux smoke result: %w\n%s", err, boundedOutput(processOutput))
	}
	report.Host = host
	terminalbenchmark.EvaluateLinuxSmokeHost(&report)
	if err := terminalbenchmark.WriteReportAtomic(reportPath, report); err != nil {
		return err
	}

	fmt.Printf("Linux native smoke: WebKitGTK %s, %d process samples, %d WebKit helpers, %.1f MiB peak RSS\n",
		report.Host.WebKitGTKVersion,
		report.Host.RSSSamples,
		report.Host.WebKitPeakProcesses,
		float64(report.Host.ProcessTreePeakRSSBytes)/(1024*1024),
	)
	if !report.Passed {
		return fmt.Errorf("Linux native smoke failed: %s", strings.Join(report.Failures, "; "))
	}
	return nil
}

func completeSoak(
	rawReport, reportPath, processOutput string,
	host terminalbenchmark.HostMetrics,
	readings []rssReading,
) error {
	report, err := terminalbenchmark.ReadSoakReport(rawReport)
	if err != nil {
		return fmt.Errorf("read packaged terminal soak result: %w\n%s", err, boundedOutput(processOutput))
	}
	startRSS, endRSS, growthRSS, startSamples, endSamples := steadyStateRSS(readings)
	host.SteadyStateStartRSSBytes = startRSS
	host.SteadyStateEndRSSBytes = endRSS
	host.SteadyStateGrowthRSSBytes = growthRSS
	host.SteadyStateStartSamples = startSamples
	host.SteadyStateEndSamples = endSamples
	report.Host = host
	terminalbenchmark.EvaluateSoakHost(&report)
	if err := terminalbenchmark.WriteSoakReportAtomic(reportPath, report); err != nil {
		return err
	}

	fmt.Printf("terminal soak: %.1f minutes, %d sessions, %.1f MiB, echo p95 %.2f ms, close p95 %.2f ms, peak RSS %.1f MiB, RSS growth %.1f MiB\n",
		report.DurationMilliseconds/60_000,
		report.SessionCount,
		float64(report.TotalBytes)/(1024*1024),
		report.EchoP95Milliseconds,
		report.CloseP95Milliseconds,
		float64(report.Host.ProcessTreePeakRSSBytes)/(1024*1024),
		float64(report.Host.SteadyStateGrowthRSSBytes)/(1024*1024),
	)
	if !report.Passed {
		return fmt.Errorf("terminal soak failed: %s", strings.Join(report.Failures, "; "))
	}
	return nil
}

func benchmarkEnvironment(home, resultPath string, mode terminalbenchmark.Mode) []string {
	blocked := map[string]bool{
		"HOME":                                  true,
		terminalbenchmark.EnvironmentEnabled:    true,
		terminalbenchmark.EnvironmentResultPath: true,
		terminalbenchmark.EnvironmentFixture:    true,
		terminalbenchmark.EnvironmentMode:       true,
	}
	result := make([]string, 0, len(os.Environ())+4)
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
		terminalbenchmark.EnvironmentMode+"="+string(mode),
	)
}

type rssReading struct {
	elapsed time.Duration
	bytes   uint64
}

func steadyStateRSS(readings []rssReading) (start, end, growth uint64, startSamples, endSamples int) {
	if len(readings) == 0 {
		return 0, 0, 0, 0, 0
	}
	last := readings[len(readings)-1].elapsed
	var startValues []uint64
	var endValues []uint64
	for _, reading := range readings {
		if reading.elapsed >= soakRSSWarmupStart && reading.elapsed <= soakRSSWarmupEnd {
			startValues = append(startValues, reading.bytes)
		}
		if reading.elapsed >= last-soakRSSEndWindow {
			endValues = append(endValues, reading.bytes)
		}
	}
	start = medianRSS(startValues)
	end = medianRSS(endValues)
	if end > start {
		growth = end - start
	}
	return start, end, growth, len(startValues), len(endValues)
}

func medianRSS(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return ordered[middle-1]/2 + ordered[middle]/2 + (ordered[middle-1]%2+ordered[middle]%2)/2
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
	data, err := exec.Command("ps", "-axo", "pid=,ppid=,rss=,command=").Output()
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
	if strings.Contains(command, "/WebKit.framework/") && strings.Contains(command, "/com.apple.WebKit.") {
		return true
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	return strings.HasPrefix(base, "WebKit") && strings.HasSuffix(base, "Process")
}

func collectHostMetrics() terminalbenchmark.HostMetrics {
	if runtime.GOOS == "linux" {
		return collectLinuxHostMetrics()
	}
	return terminalbenchmark.HostMetrics{
		Model:                  commandValue("sysctl", "-n", "hw.model"),
		Processor:              firstValue(commandValue("sysctl", "-n", "machdep.cpu.brand_string"), commandValue("sysctl", "-n", "hw.machine")),
		OperatingSystemVersion: strings.TrimSpace(commandValue("sw_vers", "-productVersion") + " (" + commandValue("sw_vers", "-buildVersion") + ")"),
		MemoryBytes:            uintValue(commandValue("sysctl", "-n", "hw.memsize")),
	}
}

func collectLinuxHostMetrics() terminalbenchmark.HostMetrics {
	cpuInfo, _ := os.ReadFile("/proc/cpuinfo")
	memoryInfo, _ := os.ReadFile("/proc/meminfo")
	return terminalbenchmark.HostMetrics{
		Model:                  firstValue(fileValue("/sys/devices/virtual/dmi/id/product_name"), commandValue("uname", "-m")),
		Processor:              firstValue(colonValue(cpuInfo, "model name", "hardware"), commandValue("uname", "-m")),
		OperatingSystemVersion: firstValue(commandValue("uname", "-sr"), "unknown"),
		MemoryBytes:            memoryBytes(memoryInfo),
		WebKitGTKVersion:       commandValue("pkg-config", "--modversion", "webkit2gtk-4.1"),
	}
}

func fileValue(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func colonValue(data []byte, keys ...string) string {
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		for _, candidate := range keys {
			if strings.EqualFold(strings.TrimSpace(key), candidate) {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func memoryBytes(data []byte) uint64 {
	value := colonValue(data, "MemTotal")
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}
	kilobytes, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	return kilobytes * 1024
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
