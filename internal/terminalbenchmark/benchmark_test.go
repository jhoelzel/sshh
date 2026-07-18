package terminalbenchmark

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServiceCompletesContentFreePassingReport(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "result.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	service, err := NewService(executable, resultPath)
	if err != nil {
		t.Fatalf("create benchmark service: %v", err)
	}
	profile, err := service.Profile()
	if err != nil {
		t.Fatalf("create benchmark profile: %v", err)
	}
	if profile.Shell != executable || len(profile.Arguments) != 1 || profile.Arguments[0] != FixtureArgument ||
		profile.Environment[EnvironmentFixture] != "1" {
		t.Fatalf("unexpected benchmark profile: %#v", profile)
	}

	report := passingReport()
	completed, err := service.Complete(report)
	if err != nil {
		t.Fatalf("complete benchmark: %v", err)
	}
	if !completed.Passed || len(completed.Failures) != 0 {
		t.Fatalf("passing benchmark failed: %#v", completed.Failures)
	}
	if completed.Runtime.OperatingSystem != runtime.GOOS || completed.Runtime.ProcessID != os.Getpid() {
		t.Fatalf("runtime identity was not recorded: %#v", completed.Runtime)
	}
	written, err := ReadReport(resultPath)
	if err != nil {
		t.Fatalf("read benchmark report: %v", err)
	}
	if !written.Passed || written.IdleEchoP95Milliseconds != 10 || written.ResizeP95Milliseconds != 90 {
		t.Fatalf("unexpected written benchmark report: %#v", written)
	}
	info, err := os.Stat(resultPath)
	if err != nil {
		t.Fatalf("stat benchmark report: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("benchmark report mode = %o, want 600", info.Mode().Perm())
	}
}

func TestEvaluationRejectsBudgetAndMemoryRegressions(t *testing.T) {
	report := passingReport()
	report.OutputDurationMilliseconds = MaximumOutputMS + 1
	report.Controller.PeakPendingBytes = MaximumQueueBytes + 1
	evaluateFrontend(&report)
	if report.Passed || !containsFailure(report.Failures, "output duration") || !containsFailure(report.Failures, "frontend output queue") {
		t.Fatalf("frontend regression was not rejected: %#v", report.Failures)
	}

	report = passingReport()
	evaluateFrontend(&report)
	report.Host = HostMetrics{
		ProcessTreePeakRSSBytes: MaximumProcessRSS + 1, ProcessTreePeakProcesses: 2,
		WebKitPeakProcesses: 1, RSSSamples: 10,
	}
	EvaluateHost(&report)
	if report.Passed || !containsFailure(report.Failures, "process tree RSS") {
		t.Fatalf("memory regression was not rejected: %#v", report.Failures)
	}
}

func TestServiceCompletesPassingSoakReport(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "soak.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	service, err := NewServiceWithMode(executable, resultPath, ModeSoak)
	if err != nil {
		t.Fatalf("create soak service: %v", err)
	}
	if config := service.Configuration(); config.Mode != ModeSoak ||
		config.SoakDurationMilliseconds != SoakDurationMilliseconds || config.SoakSessionCount != SoakSessionCount {
		t.Fatalf("unexpected soak configuration: %#v", config)
	}

	report := passingSoakReport()
	completed, err := service.CompleteSoak(report)
	if err != nil {
		t.Fatalf("complete terminal soak: %v", err)
	}
	if !completed.Passed || len(completed.Failures) != 0 {
		t.Fatalf("passing soak failed: %#v", completed.Failures)
	}
	if completed.EchoP95Milliseconds != 20 || completed.CloseP95Milliseconds != 100 {
		t.Fatalf("unexpected soak percentiles: %#v", completed)
	}
	written, err := ReadSoakReport(resultPath)
	if err != nil {
		t.Fatalf("read terminal soak: %v", err)
	}
	if !written.Passed || written.TotalBytes != report.TotalBytes {
		t.Fatalf("unexpected written soak: %#v", written)
	}
	if _, err := service.Complete(passingReport()); err == nil {
		t.Fatal("soak service accepted a burst report")
	}
}

func TestSoakEvaluationRejectsCounterLatencyAndMemoryRegressions(t *testing.T) {
	report := passingSoakReport()
	report.EchoMilliseconds[0] = MaximumSoakEchoP95MS + 1
	for index := 1; index < 100; index++ {
		report.EchoMilliseconds[index] = MaximumSoakEchoP95MS + 1
	}
	report.Sessions[0].Controller.ConsumedBytes--
	evaluateSoakFrontend(&report)
	if report.Passed || !containsFailure(report.Failures, "input echo p95") || !containsFailure(report.Failures, "counters differ") {
		t.Fatalf("soak frontend regressions were not rejected: %#v", report.Failures)
	}

	report = passingSoakReport()
	evaluateSoakFrontend(&report)
	report.Host = HostMetrics{
		ProcessTreePeakRSSBytes:   MaximumSoakProcessRSS + 1,
		ProcessTreePeakProcesses:  SoakSessionCount + 1,
		WebKitPeakProcesses:       1,
		RSSSamples:                MinimumSoakRSSSamples,
		SteadyStateStartRSSBytes:  100,
		SteadyStateEndRSSBytes:    100 + MaximumSoakRSSGrowth + 1,
		SteadyStateGrowthRSSBytes: MaximumSoakRSSGrowth + 1,
		SteadyStateStartSamples:   MinimumSoakSteadyStateSamples,
		SteadyStateEndSamples:     MinimumSoakSteadyStateSamples,
	}
	EvaluateSoakHost(&report)
	if report.Passed || !containsFailure(report.Failures, "process tree RSS") || !containsFailure(report.Failures, "steady-state RSS growth") {
		t.Fatalf("soak host regressions were not rejected: %#v", report.Failures)
	}
}

func TestResultPathMustBePrivateTemporaryJSON(t *testing.T) {
	for _, path := range []string{"relative.json", filepath.Join(os.TempDir(), "result.txt"), filepath.Join(string(filepath.Separator), "result.json")} {
		if _, err := NewService("/bin/sh", path); err == nil {
			t.Fatalf("accepted unsafe benchmark result path %q", path)
		}
	}
}

func TestEnabledEnvironmentUsesTheSameTrimmedFlagAsServiceLoading(t *testing.T) {
	t.Setenv(EnvironmentEnabled, " 1 ")
	if !EnabledInEnvironment() {
		t.Fatal("trimmed benchmark launch flag was not recognized")
	}
	t.Setenv(EnvironmentEnabled, "true")
	if EnabledInEnvironment() {
		t.Fatal("noncanonical benchmark launch flag was accepted")
	}
}

func TestEnvironmentSelectsOnlySupportedBenchmarkModes(t *testing.T) {
	t.Setenv(EnvironmentEnabled, "1")
	t.Setenv(EnvironmentResultPath, filepath.Join(t.TempDir(), "result.json"))
	t.Setenv(EnvironmentMode, string(ModeSoak))
	service, err := NewServiceFromEnvironment()
	if err != nil || service.Configuration().Mode != ModeSoak {
		t.Fatalf("load soak mode: %#v, %v", service, err)
	}
	t.Setenv(EnvironmentMode, "unbounded")
	if _, err := NewServiceFromEnvironment(); err == nil {
		t.Fatal("unsupported benchmark mode was accepted")
	}
}

func TestFloodWritesExactDeterministicPayload(t *testing.T) {
	var output bytes.Buffer
	var writes sync.Mutex
	completed := false
	writeFlood(&output, &writes, 16_123, func() { completed = true })
	if !completed || output.Len() != 16_123 {
		t.Fatalf("flood wrote %d bytes and completed=%t", output.Len(), completed)
	}
	if !strings.HasPrefix(output.String(), strings.Repeat("x", 78)+"\r\n") {
		t.Fatal("flood payload is not the expected line-oriented fixture")
	}
}

func TestFixtureRequiresExplicitAuthorization(t *testing.T) {
	t.Setenv(EnvironmentFixture, "")
	handled, err := RunFixtureIfRequested([]string{FixtureArgument})
	if !handled || err == nil {
		t.Fatalf("unauthorized fixture result = handled %t, error %v", handled, err)
	}
	handled, err = RunFixtureIfRequested([]string{"--version"})
	if handled || err != nil {
		t.Fatalf("ordinary argument result = handled %t, error %v", handled, err)
	}
}

func passingReport() Report {
	started := time.Now().UTC()
	return Report{
		SchemaVersion: SchemaVersion,
		StartedAt:     started.Format(time.RFC3339Nano), FinishedAt: started.Add(time.Second).Format(time.RFC3339Nano),
		PayloadBytes: PayloadBytes, OutputDurationMilliseconds: 1_000,
		IdleEchoMilliseconds: repeatSample(10), FloodEchoMilliseconds: repeatSample(20), ResizeMilliseconds: repeatSample(90),
		CloseDurationMilliseconds: 100,
		Controller: ControllerDiagnostics{
			AcceptedSequence: 10, AcceptedBytes: PayloadBytes + 100, ConsumedSequence: 10, ConsumedBytes: PayloadBytes + 100,
			AcknowledgedSequence: 10, PeakPendingBytes: 10_000, MaximumPendingBytes: MaximumQueueBytes,
		},
		Backend: BackendDiagnostics{
			NextSequence: 10, EmittedBytes: PayloadBytes + 100, AcknowledgedSequence: 10, AcknowledgedBytes: PayloadBytes + 100,
			PeakUnacknowledgedBytes: 10_000, PeakPendingChunks: 2, MaximumUnacknowledged: MaximumQueueBytes,
		},
	}
}

func passingSoakReport() SoakReport {
	started := time.Now().UTC()
	report := SoakReport{
		SchemaVersion:        SoakSchemaVersion,
		StartedAt:            started.Format(time.RFC3339Nano),
		FinishedAt:           started.Add(SoakDuration).Format(time.RFC3339Nano),
		DurationMilliseconds: float64(SoakDurationMilliseconds),
		SessionCount:         SoakSessionCount,
		VisibilitySwitches:   SoakMinimumVisibilitySwitches,
		EchoMilliseconds:     make([]float64, SoakMinimumEchoSamples),
		Sessions:             make([]SoakSessionReport, SoakSessionCount),
	}
	for index := range report.EchoMilliseconds {
		report.EchoMilliseconds[index] = 20
	}
	bytes := SoakMinimumPayloadBytesPerSession + 1_000
	for index := range report.Sessions {
		report.Sessions[index] = SoakSessionReport{
			Index: index, CloseDurationMilliseconds: 100,
			Controller: ControllerDiagnostics{
				AcceptedSequence: 1_000, AcceptedBytes: bytes, ConsumedSequence: 1_000, ConsumedBytes: bytes,
				AcknowledgedSequence: 1_000, PeakPendingBytes: 10_000, MaximumPendingBytes: MaximumQueueBytes,
			},
			Backend: BackendDiagnostics{
				NextSequence: 1_000, EmittedBytes: bytes, AcknowledgedSequence: 1_000, AcknowledgedBytes: bytes,
				PeakUnacknowledgedBytes: 10_000, PeakPendingChunks: 2, MaximumUnacknowledged: MaximumQueueBytes,
			},
		}
		report.TotalBytes += bytes
	}
	return report
}

func repeatSample(value float64) []float64 {
	result := make([]float64, MinimumSamples)
	for index := range result {
		result[index] = value
	}
	return result
}

func containsFailure(failures []string, fragment string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, fragment) {
			return true
		}
	}
	return false
}
