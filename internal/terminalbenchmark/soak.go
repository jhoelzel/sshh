package terminalbenchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	SoakSchemaVersion                        = 1
	SoakSessionCount                         = 8
	SoakDuration                             = 15 * time.Minute
	SoakDurationMilliseconds          uint64 = uint64(SoakDuration / time.Millisecond)
	SoakHeartbeat                            = 5 * time.Second
	SoakHeartbeatMilliseconds         uint64 = uint64(SoakHeartbeat / time.Millisecond)
	SoakOutputInterval                       = 250 * time.Millisecond
	SoakOutputChunkBytes              uint64 = 16_000
	SoakMinimumPayloadBytesPerSession        = uint64(SoakDuration/SoakOutputInterval) * SoakOutputChunkBytes * 3 / 4
	SoakMinimumEchoSamples                   = 1_200
	SoakMinimumVisibilitySwitches            = 120
	MaximumSoakEchoP95MS                     = 150
	MaximumSoakCloseP95MS                    = 1_000
	MaximumSoakProcessRSS             uint64 = 1024 * 1024 * 1024
	MaximumSoakRSSGrowth              uint64 = 96 * 1024 * 1024
	MinimumSoakRSSSamples                    = 600
	MinimumSoakSteadyStateSamples            = 30
)

type SoakSessionReport struct {
	Index                     int                   `json:"index"`
	CloseDurationMilliseconds float64               `json:"closeDurationMilliseconds"`
	Controller                ControllerDiagnostics `json:"controller"`
	Backend                   BackendDiagnostics    `json:"backend"`
}

type SoakReport struct {
	SchemaVersion        int                 `json:"schemaVersion"`
	StartedAt            string              `json:"startedAt"`
	FinishedAt           string              `json:"finishedAt"`
	DurationMilliseconds float64             `json:"durationMilliseconds"`
	SessionCount         int                 `json:"sessionCount"`
	VisibilitySwitches   int                 `json:"visibilitySwitches"`
	TotalBytes           uint64              `json:"totalBytes"`
	EchoMilliseconds     []float64           `json:"echoMilliseconds"`
	EchoP95Milliseconds  float64             `json:"echoP95Milliseconds"`
	CloseP95Milliseconds float64             `json:"closeP95Milliseconds"`
	Sessions             []SoakSessionReport `json:"sessions"`
	Runtime              RuntimeMetrics      `json:"runtime"`
	Host                 HostMetrics         `json:"host"`
	Passed               bool                `json:"passed"`
	Failures             []string            `json:"failures"`
}

func (service *Service) CompleteSoak(report SoakReport) (SoakReport, error) {
	if service == nil {
		return SoakReport{}, errors.New("terminal benchmark is disabled")
	}
	if service.mode != ModeSoak {
		return SoakReport{}, errors.New("terminal soak benchmark is not configured")
	}
	report.Runtime = RuntimeMetrics{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
		GoVersion:       runtime.Version(),
		ProcessID:       os.Getpid(),
	}
	evaluateSoakFrontend(&report)
	if err := validateSoakReport(report); err != nil {
		return SoakReport{}, err
	}
	if err := WriteSoakReportAtomic(service.resultPath, report); err != nil {
		return SoakReport{}, err
	}
	return report, nil
}

func EvaluateSoakHost(report *SoakReport) {
	if report == nil {
		return
	}
	report.Failures = removeFailurePrefixes(report.Failures, "process tree ", "steady-state ")
	if report.Host.RSSSamples < MinimumSoakRSSSamples {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"process tree RSS has %d samples; need %d", report.Host.RSSSamples, MinimumSoakRSSSamples,
		))
	}
	if report.Host.ProcessTreePeakRSSBytes > MaximumSoakProcessRSS {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"process tree RSS %d exceeded %d bytes", report.Host.ProcessTreePeakRSSBytes, MaximumSoakProcessRSS,
		))
	}
	if report.Host.ProcessTreePeakProcesses < SoakSessionCount+1 {
		report.Failures = append(report.Failures, "process tree sampler did not observe every PTY fixture child")
	}
	if report.Host.WebKitPeakProcesses < 1 {
		report.Failures = append(report.Failures, "process tree sampler did not observe a benchmark-owned WebKit process")
	}
	if report.Host.SteadyStateStartSamples < MinimumSoakSteadyStateSamples ||
		report.Host.SteadyStateEndSamples < MinimumSoakSteadyStateSamples {
		report.Failures = append(report.Failures, "steady-state RSS windows have too few samples")
	} else if report.Host.SteadyStateGrowthRSSBytes > MaximumSoakRSSGrowth {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"steady-state RSS growth %d exceeded %d bytes", report.Host.SteadyStateGrowthRSSBytes, MaximumSoakRSSGrowth,
		))
	}
	report.Passed = len(report.Failures) == 0
}

func ReadSoakReport(filename string) (SoakReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return SoakReport{}, err
	}
	var report SoakReport
	if err := json.Unmarshal(data, &report); err != nil {
		return SoakReport{}, fmt.Errorf("decode terminal soak report: %w", err)
	}
	return report, nil
}

func WriteSoakReportAtomic(filename string, report SoakReport) error {
	return writeJSONAtomic(filename, report)
}

func evaluateSoakFrontend(report *SoakReport) {
	report.Failures = slices.Clone(report.Failures)
	report.DurationMilliseconds = roundedMilliseconds(report.DurationMilliseconds)
	for index := range report.EchoMilliseconds {
		report.EchoMilliseconds[index] = roundedMilliseconds(report.EchoMilliseconds[index])
	}
	report.EchoP95Milliseconds = percentile95(report.EchoMilliseconds)
	closeSamples := make([]float64, 0, len(report.Sessions))
	for index := range report.Sessions {
		report.Sessions[index].CloseDurationMilliseconds = roundedMilliseconds(
			report.Sessions[index].CloseDurationMilliseconds,
		)
		closeSamples = append(closeSamples, report.Sessions[index].CloseDurationMilliseconds)
	}
	report.CloseP95Milliseconds = percentile95(closeSamples)

	if report.DurationMilliseconds < float64(SoakDurationMilliseconds) {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"soak duration %.2f ms was shorter than %d ms", report.DurationMilliseconds, SoakDurationMilliseconds,
		))
	}
	if len(report.EchoMilliseconds) < SoakMinimumEchoSamples {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"soak input echo has %d samples; need %d", len(report.EchoMilliseconds), SoakMinimumEchoSamples,
		))
	}
	if report.EchoP95Milliseconds > MaximumSoakEchoP95MS {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"soak input echo p95 %.2f ms exceeded %d ms", report.EchoP95Milliseconds, MaximumSoakEchoP95MS,
		))
	}
	if report.CloseP95Milliseconds > MaximumSoakCloseP95MS {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"soak close p95 %.2f ms exceeded %d ms", report.CloseP95Milliseconds, MaximumSoakCloseP95MS,
		))
	}
	if report.VisibilitySwitches < SoakMinimumVisibilitySwitches {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"soak switched visible sessions %d times; need %d", report.VisibilitySwitches, SoakMinimumVisibilitySwitches,
		))
	}
	if report.SessionCount != SoakSessionCount || len(report.Sessions) != SoakSessionCount {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"soak reported %d sessions and %d snapshots; need %d", report.SessionCount, len(report.Sessions), SoakSessionCount,
		))
	}

	seen := make(map[int]bool, len(report.Sessions))
	var totalBytes uint64
	for _, session := range report.Sessions {
		if session.Index < 0 || session.Index >= SoakSessionCount || seen[session.Index] {
			report.Failures = append(report.Failures, "soak session indexes are invalid or duplicated")
			continue
		}
		seen[session.Index] = true
		totalBytes += session.Controller.AcceptedBytes
		evaluateSoakSession(&report.Failures, session)
	}
	if report.TotalBytes != totalBytes {
		report.Failures = append(report.Failures, "soak total bytes differ from the per-session counters")
	}
	report.Passed = len(report.Failures) == 0
}

func evaluateSoakSession(failures *[]string, session SoakSessionReport) {
	controller := session.Controller
	backend := session.Backend
	if controller.OutputFailed {
		*failures = append(*failures, fmt.Sprintf("session %d xterm output parsing failed", session.Index))
	}
	if controller.AcceptedBytes < SoakMinimumPayloadBytesPerSession {
		*failures = append(*failures, fmt.Sprintf(
			"session %d traversed %d bytes; need at least %d", session.Index, controller.AcceptedBytes, SoakMinimumPayloadBytesPerSession,
		))
	}
	if controller.PendingBytes != 0 || backend.UnacknowledgedBytes != 0 || backend.PendingChunks != 0 {
		*failures = append(*failures, fmt.Sprintf("session %d output did not drain", session.Index))
	}
	if controller.AcceptedBytes != controller.ConsumedBytes ||
		controller.AcceptedBytes != backend.EmittedBytes ||
		controller.AcceptedBytes != backend.AcknowledgedBytes ||
		controller.AcceptedSequence != controller.ConsumedSequence ||
		controller.AcceptedSequence != controller.AcknowledgedSequence ||
		controller.AcceptedSequence != backend.NextSequence ||
		controller.AcceptedSequence != backend.AcknowledgedSequence {
		*failures = append(*failures, fmt.Sprintf("session %d frontend and backend counters differ", session.Index))
	}
	if controller.PeakPendingBytes > MaximumQueueBytes || controller.MaximumPendingBytes != MaximumQueueBytes {
		*failures = append(*failures, fmt.Sprintf("session %d frontend output queue exceeded or misreported its cap", session.Index))
	}
	if backend.PeakUnacknowledgedBytes > MaximumQueueBytes || backend.MaximumUnacknowledged != MaximumQueueBytes {
		*failures = append(*failures, fmt.Sprintf("session %d backend output queue exceeded or misreported its cap", session.Index))
	}
	if session.CloseDurationMilliseconds > MaximumSoakCloseP95MS {
		*failures = append(*failures, fmt.Sprintf("session %d close exceeded %d ms", session.Index, MaximumSoakCloseP95MS))
	}
}

func validateSoakReport(report SoakReport) error {
	if report.SchemaVersion != SoakSchemaVersion {
		return fmt.Errorf("unsupported terminal soak report schema %d", report.SchemaVersion)
	}
	started, err := time.Parse(time.RFC3339Nano, report.StartedAt)
	if err != nil {
		return fmt.Errorf("invalid terminal soak start time: %w", err)
	}
	finished, err := time.Parse(time.RFC3339Nano, report.FinishedAt)
	if err != nil || finished.Before(started) {
		return errors.New("invalid terminal soak finish time")
	}
	if math.IsNaN(report.DurationMilliseconds) || math.IsInf(report.DurationMilliseconds, 0) ||
		report.DurationMilliseconds < 0 || report.DurationMilliseconds > float64(2*time.Hour/time.Millisecond) {
		return errors.New("invalid terminal soak duration")
	}
	if len(report.EchoMilliseconds) > 5_000 {
		return errors.New("too many terminal soak echo samples")
	}
	for _, value := range report.EchoMilliseconds {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 60_000 {
			return errors.New("invalid terminal soak echo sample")
		}
	}
	if len(report.Sessions) > SoakSessionCount {
		return errors.New("too many terminal soak session reports")
	}
	for _, session := range report.Sessions {
		if math.IsNaN(session.CloseDurationMilliseconds) || math.IsInf(session.CloseDurationMilliseconds, 0) ||
			session.CloseDurationMilliseconds < 0 || session.CloseDurationMilliseconds > 60_000 {
			return errors.New("invalid terminal soak close duration")
		}
	}
	if len(report.Failures) > 40 {
		return errors.New("too many terminal soak failures")
	}
	for _, failure := range report.Failures {
		if len(failure) > 240 || strings.ContainsAny(failure, "\r\n") {
			return errors.New("invalid terminal soak failure")
		}
	}
	return nil
}

func removeFailurePrefixes(failures []string, prefixes ...string) []string {
	result := failures[:0]
	for _, failure := range failures {
		remove := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(failure, prefix) {
				remove = true
				break
			}
		}
		if !remove {
			result = append(result, failure)
		}
	}
	return result
}
