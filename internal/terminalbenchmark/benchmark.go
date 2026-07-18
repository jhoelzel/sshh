package terminalbenchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"shh-h/internal/domain/profile"
)

const (
	EnvironmentEnabled    = "SHHH_TERMINAL_BENCHMARK"
	EnvironmentResultPath = "SHHH_TERMINAL_BENCHMARK_RESULT"
	EnvironmentFixture    = "SHHH_TERMINAL_BENCHMARK_FIXTURE"
	FixtureArgument       = "--shhh-terminal-benchmark-fixture"

	SchemaVersion                = 1
	PayloadBytes          uint64 = 10 * 1024 * 1024
	MaximumQueueBytes     uint64 = 1024 * 1024
	MaximumOutputMS              = 10_000
	MaximumIdleEchoP95MS         = 50
	MaximumFloodEchoP95MS        = 150
	MaximumResizeP95MS           = 150
	MaximumCloseMS               = 1_000
	MaximumProcessRSS     uint64 = 512 * 1024 * 1024
	MinimumSamples               = 20
)

type Configuration struct {
	Enabled                   bool   `json:"enabled"`
	ProcessID                 int    `json:"processId"`
	PayloadBytes              uint64 `json:"payloadBytes"`
	MaximumBackendQueueBytes  uint64 `json:"maximumBackendQueueBytes"`
	MaximumFrontendQueueBytes uint64 `json:"maximumFrontendQueueBytes"`
	MinimumLatencySamples     int    `json:"minimumLatencySamples"`
}

type ControllerDiagnostics struct {
	AcceptedSequence     uint64 `json:"acceptedSequence"`
	AcceptedBytes        uint64 `json:"acceptedBytes"`
	ConsumedSequence     uint64 `json:"consumedSequence"`
	ConsumedBytes        uint64 `json:"consumedBytes"`
	AcknowledgedSequence uint64 `json:"acknowledgedSequence"`
	PendingBytes         uint64 `json:"pendingBytes"`
	PeakPendingBytes     uint64 `json:"peakPendingBytes"`
	MaximumPendingBytes  uint64 `json:"maximumPendingBytes"`
	OutputFailed         bool   `json:"outputFailed"`
}

type BackendDiagnostics struct {
	NextSequence            uint64 `json:"nextSequence"`
	EmittedBytes            uint64 `json:"emittedBytes"`
	AcknowledgedSequence    uint64 `json:"acknowledgedSequence"`
	AcknowledgedBytes       uint64 `json:"acknowledgedBytes"`
	UnacknowledgedBytes     uint64 `json:"unacknowledgedBytes"`
	PendingChunks           int    `json:"pendingChunks"`
	PeakUnacknowledgedBytes uint64 `json:"peakUnacknowledgedBytes"`
	PeakPendingChunks       int    `json:"peakPendingChunks"`
	MaximumUnacknowledged   uint64 `json:"maximumUnacknowledged"`
}

type RuntimeMetrics struct {
	OperatingSystem string `json:"operatingSystem"`
	Architecture    string `json:"architecture"`
	GoVersion       string `json:"goVersion"`
	ProcessID       int    `json:"processId"`
}

type HostMetrics struct {
	Model                    string `json:"model"`
	Processor                string `json:"processor"`
	OperatingSystemVersion   string `json:"operatingSystemVersion"`
	MemoryBytes              uint64 `json:"memoryBytes"`
	ProcessTreePeakRSSBytes  uint64 `json:"processTreePeakRssBytes"`
	ProcessTreePeakProcesses int    `json:"processTreePeakProcesses"`
	WebKitPeakProcesses      int    `json:"webKitPeakProcesses"`
	RSSSamples               int    `json:"rssSamples"`
}

type Report struct {
	SchemaVersion              int                   `json:"schemaVersion"`
	StartedAt                  string                `json:"startedAt"`
	FinishedAt                 string                `json:"finishedAt"`
	PayloadBytes               uint64                `json:"payloadBytes"`
	OutputDurationMilliseconds float64               `json:"outputDurationMilliseconds"`
	IdleEchoMilliseconds       []float64             `json:"idleEchoMilliseconds"`
	FloodEchoMilliseconds      []float64             `json:"floodEchoMilliseconds"`
	ResizeMilliseconds         []float64             `json:"resizeMilliseconds"`
	IdleEchoP95Milliseconds    float64               `json:"idleEchoP95Milliseconds"`
	FloodEchoP95Milliseconds   float64               `json:"floodEchoP95Milliseconds"`
	ResizeP95Milliseconds      float64               `json:"resizeP95Milliseconds"`
	CloseDurationMilliseconds  float64               `json:"closeDurationMilliseconds"`
	Controller                 ControllerDiagnostics `json:"controller"`
	Backend                    BackendDiagnostics    `json:"backend"`
	Runtime                    RuntimeMetrics        `json:"runtime"`
	Host                       HostMetrics           `json:"host"`
	Passed                     bool                  `json:"passed"`
	Failures                   []string              `json:"failures"`
}

type Service struct {
	resultPath string
	executable string
}

func EnabledInEnvironment() bool {
	return strings.TrimSpace(os.Getenv(EnvironmentEnabled)) == "1"
}

func NewServiceFromEnvironment() (*Service, error) {
	enabled := strings.TrimSpace(os.Getenv(EnvironmentEnabled))
	resultPath := strings.TrimSpace(os.Getenv(EnvironmentResultPath))
	if enabled == "" && resultPath == "" {
		return nil, nil
	}
	if enabled != "1" {
		return nil, fmt.Errorf("%s must be exactly 1", EnvironmentEnabled)
	}
	if resultPath == "" {
		return nil, fmt.Errorf("%s is required", EnvironmentResultPath)
	}
	cleanResult, err := validateResultPath(resultPath)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate benchmark executable: %w", err)
	}
	return &Service{resultPath: cleanResult, executable: executable}, nil
}

func NewService(executable, resultPath string) (*Service, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, errors.New("benchmark executable is required")
	}
	cleanResult, err := validateResultPath(resultPath)
	if err != nil {
		return nil, err
	}
	return &Service{resultPath: cleanResult, executable: executable}, nil
}

func (service *Service) Configuration() Configuration {
	if service == nil {
		return Configuration{}
	}
	return Configuration{
		Enabled:                   true,
		ProcessID:                 os.Getpid(),
		PayloadBytes:              PayloadBytes,
		MaximumBackendQueueBytes:  MaximumQueueBytes,
		MaximumFrontendQueueBytes: MaximumQueueBytes,
		MinimumLatencySamples:     MinimumSamples,
	}
}

func (service *Service) Profile() (profile.Profile, error) {
	if service == nil {
		return profile.Profile{}, errors.New("terminal benchmark is disabled")
	}
	info, err := os.Stat(service.executable)
	if err != nil {
		return profile.Profile{}, fmt.Errorf("inspect benchmark executable: %w", err)
	}
	if info.IsDir() || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
		return profile.Profile{}, errors.New("benchmark executable is not executable")
	}
	return profile.Profile{
		ID: "terminal-benchmark", Name: "Terminal benchmark", Protocol: profile.ProtocolLocal,
		Shell: service.executable, Arguments: []string{FixtureArgument},
		Environment: map[string]string{EnvironmentFixture: "1"},
	}, nil
}

func (service *Service) Complete(report Report) (Report, error) {
	if service == nil {
		return Report{}, errors.New("terminal benchmark is disabled")
	}
	report.Runtime = RuntimeMetrics{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
		GoVersion:       runtime.Version(),
		ProcessID:       os.Getpid(),
	}
	evaluateFrontend(&report)
	if err := validateReport(report); err != nil {
		return Report{}, err
	}
	if err := WriteReportAtomic(service.resultPath, report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func EvaluateHost(report *Report) {
	if report == nil {
		return
	}
	report.Failures = removeFailurePrefix(report.Failures, "process tree ")
	if report.Host.RSSSamples < 1 {
		report.Failures = append(report.Failures, "process tree RSS was not sampled")
	} else if report.Host.ProcessTreePeakRSSBytes > MaximumProcessRSS {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"process tree RSS %d exceeded %d bytes", report.Host.ProcessTreePeakRSSBytes, MaximumProcessRSS,
		))
	}
	if report.Host.ProcessTreePeakProcesses < 2 {
		report.Failures = append(report.Failures, "process tree sampler did not observe the PTY fixture child")
	}
	if report.Host.WebKitPeakProcesses < 1 {
		report.Failures = append(report.Failures, "process tree sampler did not observe a benchmark-owned WebKit process")
	}
	report.Passed = len(report.Failures) == 0
}

func ReadReport(filename string) (Report, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, fmt.Errorf("decode terminal benchmark report: %w", err)
	}
	return report, nil
}

func WriteReportAtomic(filename string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode terminal benchmark report: %w", err)
	}
	data = append(data, '\n')
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create terminal benchmark report directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".terminal-benchmark-*.json")
	if err != nil {
		return fmt.Errorf("create terminal benchmark report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect terminal benchmark report: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write terminal benchmark report: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync terminal benchmark report: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close terminal benchmark report: %w", err)
	}
	if err := os.Rename(temporaryPath, filename); err != nil {
		return fmt.Errorf("replace terminal benchmark report: %w", err)
	}
	if err := os.Chmod(filename, 0o600); err != nil {
		return fmt.Errorf("protect terminal benchmark report: %w", err)
	}
	return nil
}

func evaluateFrontend(report *Report) {
	report.Failures = slices.Clone(report.Failures)
	report.OutputDurationMilliseconds = roundedMilliseconds(report.OutputDurationMilliseconds)
	report.CloseDurationMilliseconds = roundedMilliseconds(report.CloseDurationMilliseconds)
	for _, samples := range [][]float64{report.IdleEchoMilliseconds, report.FloodEchoMilliseconds, report.ResizeMilliseconds} {
		for index := range samples {
			samples[index] = roundedMilliseconds(samples[index])
		}
	}
	report.IdleEchoP95Milliseconds = percentile95(report.IdleEchoMilliseconds)
	report.FloodEchoP95Milliseconds = percentile95(report.FloodEchoMilliseconds)
	report.ResizeP95Milliseconds = percentile95(report.ResizeMilliseconds)

	checkSamples := func(name string, samples []float64) {
		if len(samples) < MinimumSamples {
			report.Failures = append(report.Failures, fmt.Sprintf("%s has %d samples; need %d", name, len(samples), MinimumSamples))
		}
	}
	checkSamples("idle input echo", report.IdleEchoMilliseconds)
	checkSamples("flood input echo", report.FloodEchoMilliseconds)
	checkSamples("resize", report.ResizeMilliseconds)
	if report.PayloadBytes != PayloadBytes {
		report.Failures = append(report.Failures, fmt.Sprintf("payload was %d bytes; need %d", report.PayloadBytes, PayloadBytes))
	}
	if report.OutputDurationMilliseconds > MaximumOutputMS {
		report.Failures = append(report.Failures, fmt.Sprintf("output duration %.2f ms exceeded %d ms", report.OutputDurationMilliseconds, MaximumOutputMS))
	}
	if report.IdleEchoP95Milliseconds > MaximumIdleEchoP95MS {
		report.Failures = append(report.Failures, fmt.Sprintf("idle input echo p95 %.2f ms exceeded %d ms", report.IdleEchoP95Milliseconds, MaximumIdleEchoP95MS))
	}
	if report.FloodEchoP95Milliseconds > MaximumFloodEchoP95MS {
		report.Failures = append(report.Failures, fmt.Sprintf("flood input echo p95 %.2f ms exceeded %d ms", report.FloodEchoP95Milliseconds, MaximumFloodEchoP95MS))
	}
	if report.ResizeP95Milliseconds > MaximumResizeP95MS {
		report.Failures = append(report.Failures, fmt.Sprintf("resize p95 %.2f ms exceeded %d ms", report.ResizeP95Milliseconds, MaximumResizeP95MS))
	}
	if report.CloseDurationMilliseconds > MaximumCloseMS {
		report.Failures = append(report.Failures, fmt.Sprintf("close duration %.2f ms exceeded %d ms", report.CloseDurationMilliseconds, MaximumCloseMS))
	}
	if report.Controller.OutputFailed {
		report.Failures = append(report.Failures, "xterm output parsing failed")
	}
	if report.Backend.EmittedBytes < PayloadBytes || report.Controller.AcceptedBytes < PayloadBytes {
		report.Failures = append(report.Failures, "10 MiB fixture payload did not traverse the complete terminal path")
	}
	if report.Controller.PendingBytes != 0 || report.Backend.UnacknowledgedBytes != 0 || report.Backend.PendingChunks != 0 {
		report.Failures = append(report.Failures, "terminal output did not drain before measurement")
	}
	if report.Controller.AcceptedBytes != report.Backend.EmittedBytes ||
		report.Controller.ConsumedBytes != report.Backend.AcknowledgedBytes ||
		report.Controller.AcceptedSequence != report.Backend.NextSequence ||
		report.Controller.AcknowledgedSequence != report.Backend.AcknowledgedSequence {
		report.Failures = append(report.Failures, "frontend and backend sequence or byte counters differ")
	}
	if report.Controller.PeakPendingBytes > MaximumQueueBytes || report.Controller.MaximumPendingBytes != MaximumQueueBytes {
		report.Failures = append(report.Failures, "frontend output queue exceeded or misreported its cap")
	}
	if report.Backend.PeakUnacknowledgedBytes > MaximumQueueBytes || report.Backend.MaximumUnacknowledged != MaximumQueueBytes {
		report.Failures = append(report.Failures, "backend output queue exceeded or misreported its cap")
	}
	report.Passed = len(report.Failures) == 0
}

func validateReport(report Report) error {
	if report.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported terminal benchmark report schema %d", report.SchemaVersion)
	}
	started, err := time.Parse(time.RFC3339Nano, report.StartedAt)
	if err != nil {
		return fmt.Errorf("invalid terminal benchmark start time: %w", err)
	}
	finished, err := time.Parse(time.RFC3339Nano, report.FinishedAt)
	if err != nil || finished.Before(started) {
		return errors.New("invalid terminal benchmark finish time")
	}
	for name, values := range map[string][]float64{
		"idle input echo":  report.IdleEchoMilliseconds,
		"flood input echo": report.FloodEchoMilliseconds,
		"resize":           report.ResizeMilliseconds,
	} {
		if len(values) > 1_000 {
			return fmt.Errorf("too many %s samples", name)
		}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 60_000 {
				return fmt.Errorf("invalid %s sample", name)
			}
		}
	}
	for _, value := range []float64{report.OutputDurationMilliseconds, report.CloseDurationMilliseconds} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 60_000 {
			return errors.New("invalid terminal benchmark duration")
		}
	}
	if len(report.Failures) > 20 {
		return errors.New("too many terminal benchmark failures")
	}
	for _, failure := range report.Failures {
		if len(failure) > 240 || strings.ContainsAny(failure, "\r\n") {
			return errors.New("invalid terminal benchmark failure")
		}
	}
	return nil
}

func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	index := int(math.Ceil(float64(len(ordered))*0.95)) - 1
	return ordered[max(index, 0)]
}

func roundedMilliseconds(value float64) float64 {
	return math.Round(value*1_000) / 1_000
}

func validateResultPath(value string) (string, error) {
	if !filepath.IsAbs(value) || filepath.Ext(value) != ".json" {
		return "", errors.New("terminal benchmark result must be an absolute JSON path")
	}
	clean := filepath.Clean(value)
	temporaryRoot, err := filepath.EvalSymlinks(filepath.Clean(os.TempDir()))
	if err != nil {
		return "", fmt.Errorf("resolve system temporary directory: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", errors.New("terminal benchmark result directory must already exist")
	}
	clean = filepath.Join(resolvedParent, filepath.Base(clean))
	relative, err := filepath.Rel(temporaryRoot, clean)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("terminal benchmark result must be inside the system temporary directory")
	}
	return clean, nil
}

func removeFailurePrefix(failures []string, prefix string) []string {
	result := failures[:0]
	for _, failure := range failures {
		if !strings.HasPrefix(failure, prefix) {
			result = append(result, failure)
		}
	}
	return result
}
