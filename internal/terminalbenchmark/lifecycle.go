package terminalbenchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	LifecycleSchemaVersion       = 1
	MinimumLifecycleDecisionWait = 200
	maximumLifecycleAttempts     = 8
)

const (
	lifecyclePhaseFrontendAttached = "frontend-attached"
	lifecyclePhaseTerminalOpened   = "terminal-opened"
	lifecyclePhaseCloseRequested   = "close-requested"
	lifecyclePhaseConfirming       = "confirming"
	lifecyclePhaseFailed           = "failed"
)

type LifecycleCloseAttempt struct {
	Sequence            int    `json:"sequence"`
	RecordedAt          string `json:"recordedAt"`
	Prevented           bool   `json:"prevented"`
	LiveTerminalsBefore int    `json:"liveTerminalsBefore"`
	LiveTerminalsAfter  int    `json:"liveTerminalsAfter"`
}

type LifecycleReport struct {
	SchemaVersion             int                     `json:"schemaVersion"`
	StartedAt                 string                  `json:"startedAt"`
	FinishedAt                string                  `json:"finishedAt"`
	StartupObserved           bool                    `json:"startupObserved"`
	DomReadyObserved          bool                    `json:"domReadyObserved"`
	FrontendAttached          bool                    `json:"frontendAttached"`
	TerminalOpened            bool                    `json:"terminalOpened"`
	CloseRequested            bool                    `json:"closeRequested"`
	DecisionDelayMilliseconds int                     `json:"decisionDelayMilliseconds"`
	ConfirmationRequested     bool                    `json:"confirmationRequested"`
	FrontendFailed            bool                    `json:"frontendFailed"`
	CloseAttempts             []LifecycleCloseAttempt `json:"closeAttempts"`
	DroppedCloseAttempts      int                     `json:"droppedCloseAttempts"`
	ShutdownCompleted         bool                    `json:"shutdownCompleted"`
	ShutdownSucceeded         bool                    `json:"shutdownSucceeded"`
	Runtime                   RuntimeMetrics          `json:"runtime"`
	Host                      HostMetrics             `json:"host"`
	Passed                    bool                    `json:"passed"`
	Failures                  []string                `json:"failures"`
}

type lifecycleRecorder struct {
	mu     sync.Mutex
	report LifecycleReport
}

func newLifecycleRecorder() *lifecycleRecorder {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return &lifecycleRecorder{report: LifecycleReport{
		SchemaVersion: LifecycleSchemaVersion,
		StartedAt:     now,
		FinishedAt:    now,
		CloseAttempts: []LifecycleCloseAttempt{},
		Failures:      []string{},
	}}
}

func (service *Service) RecordLifecycleStartup(observed bool) error {
	return service.updateLifecycle(func(report *LifecycleReport) {
		report.StartupObserved = observed
	})
}

func (service *Service) RecordLifecycleDomReady() error {
	return service.updateLifecycle(func(report *LifecycleReport) {
		report.DomReadyObserved = true
	})
}

func (service *Service) RecordLifecycleBeforeClose(prevented bool, liveBefore, liveAfter int) error {
	if liveBefore < 0 || liveAfter < 0 {
		return errors.New("native lifecycle terminal counts must not be negative")
	}
	return service.updateLifecycle(func(report *LifecycleReport) {
		sequence := len(report.CloseAttempts) + report.DroppedCloseAttempts + 1
		if len(report.CloseAttempts) >= maximumLifecycleAttempts {
			report.DroppedCloseAttempts++
			return
		}
		report.CloseAttempts = append(report.CloseAttempts, LifecycleCloseAttempt{
			Sequence:            sequence,
			RecordedAt:          time.Now().UTC().Format(time.RFC3339Nano),
			Prevented:           prevented,
			LiveTerminalsBefore: liveBefore,
			LiveTerminalsAfter:  liveAfter,
		})
	})
}

func (service *Service) RecordLifecycleShutdown(succeeded bool) error {
	return service.updateLifecycle(func(report *LifecycleReport) {
		report.ShutdownCompleted = true
		report.ShutdownSucceeded = succeeded
		report.Runtime = RuntimeMetrics{
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
			GoVersion:       runtime.Version(),
			ProcessID:       os.Getpid(),
		}
		evaluateLifecycle(report)
	})
}

func (service *Service) recordLifecycleProgress(phase string, completed int) error {
	phase = strings.TrimSpace(phase)
	if completed < 0 || completed > 5_000 {
		return errors.New("invalid terminal lifecycle progress")
	}
	switch phase {
	case lifecyclePhaseFrontendAttached, lifecyclePhaseTerminalOpened, lifecyclePhaseCloseRequested,
		lifecyclePhaseConfirming, lifecyclePhaseFailed:
	default:
		return fmt.Errorf("unsupported terminal lifecycle phase %q", phase)
	}
	return service.updateLifecycle(func(report *LifecycleReport) {
		switch phase {
		case lifecyclePhaseFrontendAttached:
			report.FrontendAttached = completed == 1
		case lifecyclePhaseTerminalOpened:
			report.TerminalOpened = completed == 1
		case lifecyclePhaseCloseRequested:
			report.CloseRequested = completed == 1
		case lifecyclePhaseConfirming:
			report.ConfirmationRequested = true
			report.DecisionDelayMilliseconds = completed
		case lifecyclePhaseFailed:
			report.FrontendFailed = true
		}
	})
}

func (service *Service) updateLifecycle(update func(*LifecycleReport)) error {
	if service == nil || service.mode != ModeLifecycle {
		return nil
	}
	if service.lifecycle == nil {
		return errors.New("terminal lifecycle recorder is unavailable")
	}
	service.lifecycle.mu.Lock()
	defer service.lifecycle.mu.Unlock()
	update(&service.lifecycle.report)
	service.lifecycle.report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return WriteLifecycleReportAtomic(service.resultPath, service.lifecycle.report)
}

func EvaluateLifecycleHost(report *LifecycleReport) {
	if report == nil {
		return
	}
	evaluateLifecycle(report)
	if report.Host.RSSSamples < 1 {
		report.Failures = append(report.Failures, "native host process tree RSS was not sampled")
	}
	if report.Host.ProcessTreePeakProcesses < 2 {
		report.Failures = append(report.Failures, "native host process sampler did not observe the PTY fixture child")
	}
	if report.Host.WebKitPeakProcesses < 1 {
		report.Failures = append(report.Failures, "native host process sampler did not observe a WebView helper")
	}
	if report.Runtime.OperatingSystem == "linux" && !versionAtLeast(report.Host.WebKitGTKVersion, MinimumWebKitGTKVersion) {
		report.Failures = append(report.Failures, fmt.Sprintf(
			"native host WebKitGTK version %q is below %s", report.Host.WebKitGTKVersion, MinimumWebKitGTKVersion,
		))
	}
	report.Passed = len(report.Failures) == 0
}

func evaluateLifecycle(report *LifecycleReport) {
	failures := make([]string, 0, 16)
	if report.SchemaVersion != LifecycleSchemaVersion {
		failures = append(failures, fmt.Sprintf("lifecycle report schema was %d; need %d", report.SchemaVersion, LifecycleSchemaVersion))
	}
	if !report.StartupObserved {
		failures = append(failures, "Wails OnStartup was not observed")
	}
	if !report.DomReadyObserved {
		failures = append(failures, "Wails OnDomReady was not observed")
	}
	if !report.FrontendAttached {
		failures = append(failures, "benchmark frontend did not attach")
	}
	if !report.TerminalOpened {
		failures = append(failures, "benchmark frontend did not open a live PTY")
	}
	if !report.CloseRequested {
		failures = append(failures, "frontend did not receive the consolidated close request")
	}
	if report.DecisionDelayMilliseconds < MinimumLifecycleDecisionWait {
		failures = append(failures, fmt.Sprintf(
			"close decision remained alive for %d ms; need at least %d ms",
			report.DecisionDelayMilliseconds, MinimumLifecycleDecisionWait,
		))
	}
	if !report.ConfirmationRequested {
		failures = append(failures, "frontend did not confirm coordinated shutdown")
	}
	if report.FrontendFailed {
		failures = append(failures, "frontend reported a lifecycle smoke failure")
	}
	if report.DroppedCloseAttempts != 0 {
		failures = append(failures, "native close attempt report exceeded its bound")
	}
	if len(report.CloseAttempts) != 2 {
		failures = append(failures, fmt.Sprintf("native close attempts were %d; need exactly 2", len(report.CloseAttempts)))
	} else {
		first := report.CloseAttempts[0]
		second := report.CloseAttempts[1]
		if first.Sequence != 1 || !first.Prevented || first.LiveTerminalsBefore != 1 || first.LiveTerminalsAfter != 1 {
			failures = append(failures, "first native close was not prevented with one live terminal retained")
		}
		if second.Sequence != 2 || second.Prevented || second.LiveTerminalsBefore != 0 || second.LiveTerminalsAfter != 0 {
			failures = append(failures, "confirmed native close did not proceed after terminal shutdown")
		}
	}
	if !report.ShutdownCompleted {
		failures = append(failures, "Wails OnShutdown was not observed")
	} else if !report.ShutdownSucceeded {
		failures = append(failures, "application services failed during OnShutdown")
	}
	if report.Runtime.OperatingSystem == "" || report.Runtime.Architecture == "" || report.Runtime.ProcessID <= 0 {
		failures = append(failures, "native runtime identity was not recorded")
	}
	report.Failures = failures
	report.Passed = len(failures) == 0
}

func ReadLifecycleReport(filename string) (LifecycleReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return LifecycleReport{}, err
	}
	var report LifecycleReport
	if err := json.Unmarshal(data, &report); err != nil {
		return LifecycleReport{}, fmt.Errorf("decode terminal lifecycle report: %w", err)
	}
	if report.SchemaVersion != LifecycleSchemaVersion {
		return LifecycleReport{}, fmt.Errorf("unsupported terminal lifecycle report schema %d", report.SchemaVersion)
	}
	return report, nil
}

func WriteLifecycleReportAtomic(filename string, report LifecycleReport) error {
	return writeJSONAtomic(filename, report)
}
