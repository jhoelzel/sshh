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
