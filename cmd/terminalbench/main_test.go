package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"shh-h/internal/terminalbenchmark"
)

func TestProcessTreeRSSIncludesOnlyRootAndDescendants(t *testing.T) {
	data := []byte(`
  1   0 100
 10   1 200
 11  10 300
 12  11 400
 20   1 500
 21  20 600
`)
	got, count, webKitCount, err := processTreeRSS(10, data, nil)
	if err != nil {
		t.Fatalf("calculate process tree RSS: %v", err)
	}
	if want := uint64((200 + 300 + 400) * 1024); got != want {
		t.Fatalf("process tree RSS = %d, want %d", got, want)
	}
	if count != 3 {
		t.Fatalf("process tree count = %d, want 3", count)
	}
	if webKitCount != 0 {
		t.Fatalf("WebKit process count = %d, want 0", webKitCount)
	}
}

func TestProcessTreeRSSRejectsMissingRoot(t *testing.T) {
	if _, _, _, err := processTreeRSS(99, []byte("1 0 100\n"), nil); err == nil {
		t.Fatal("missing benchmark root process was accepted")
	}
}

func TestProcessTreeRSSIncludesOnlyNewWebKitHelpers(t *testing.T) {
	data := []byte(`
 10 1 200 /tmp/shhh
 11 10 300 /tmp/shhh --fixture
 20 1 400 /System/Library/Frameworks/WebKit.framework/XPCServices/com.apple.WebKit.WebContent
 21 1 500 /System/Library/Frameworks/WebKit.framework/XPCServices/com.apple.WebKit.GPU
`)
	rss, count, webKitCount, err := processTreeRSS(10, data, map[int]bool{20: true})
	if err != nil {
		t.Fatalf("calculate WebKit process RSS: %v", err)
	}
	if want := uint64((200 + 300 + 500) * 1024); rss != want || count != 3 || webKitCount != 1 {
		t.Fatalf("usage = %d bytes, %d processes, %d WebKit; want %d, 3, 1", rss, count, webKitCount, want)
	}
}

func TestProcessTreeRSSRecognizesLinuxWebKitGTKHelpers(t *testing.T) {
	data := []byte(`
 10 1 200 /tmp/shhh-linux-smoke
 11 10 300 /tmp/shhh-linux-smoke --fixture
 20 10 400 /usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/WebKitWebProcess
 21 10 500 WebKitNetworkProcess
 22 1 600 WebKitWebDriver
`)
	rss, count, webKitCount, err := processTreeRSS(10, data, nil)
	if err != nil {
		t.Fatalf("calculate Linux WebKitGTK process RSS: %v", err)
	}
	if want := uint64((200 + 300 + 400 + 500) * 1024); rss != want || count != 4 || webKitCount != 2 {
		t.Fatalf("usage = %d bytes, %d processes, %d WebKit; want %d, 4, 2", rss, count, webKitCount, want)
	}
	if isWebKitHelper("WebKitWebDriver") {
		t.Fatal("WebKitWebDriver was classified as an application helper process")
	}
}

func TestLinuxHostParsers(t *testing.T) {
	cpu := []byte("processor : 0\nmodel name : Example CPU 9000\n")
	if got := colonValue(cpu, "model name", "hardware"); got != "Example CPU 9000" {
		t.Fatalf("processor = %q", got)
	}
	memory := []byte("MemTotal:       49152 kB\nMemFree: 1024 kB\n")
	if got, want := memoryBytes(memory), uint64(49152*1024); got != want {
		t.Fatalf("memory = %d bytes, want %d", got, want)
	}
}

func TestSteadyStateRSSUsesWarmAndFinalMedians(t *testing.T) {
	readings := []rssReading{
		{elapsed: 30 * time.Second, bytes: 10},
		{elapsed: 60 * time.Second, bytes: 100},
		{elapsed: 90 * time.Second, bytes: 110},
		{elapsed: 120 * time.Second, bytes: 120},
		{elapsed: 14 * time.Minute, bytes: 190},
		{elapsed: 14*time.Minute + 30*time.Second, bytes: 200},
		{elapsed: 15 * time.Minute, bytes: 210},
	}
	start, end, growth, startSamples, endSamples := steadyStateRSS(readings)
	if start != 110 || end != 200 || growth != 90 || startSamples != 3 || endSamples != 3 {
		t.Fatalf("steady RSS = %d, %d, %d, %d, %d; want 110, 200, 90, 3, 3", start, end, growth, startSamples, endSamples)
	}
}

func TestSteadyStateRSSDoesNotReportNegativeGrowth(t *testing.T) {
	readings := []rssReading{
		{elapsed: time.Minute, bytes: 200},
		{elapsed: 2 * time.Minute, bytes: 200},
		{elapsed: 14 * time.Minute, bytes: 100},
		{elapsed: 15 * time.Minute, bytes: 100},
	}
	_, _, growth, _, _ := steadyStateRSS(readings)
	if growth != 0 {
		t.Fatalf("steady RSS shrinkage reported as %d bytes of growth", growth)
	}
}

func TestTimedOutSoakPreservesPartialReport(t *testing.T) {
	directory := t.TempDir()
	raw := filepath.Join(directory, "raw.json")
	final := filepath.Join(directory, "final.json")
	now := time.Now().UTC()
	if err := terminalbenchmark.WriteSoakReportAtomic(raw, terminalbenchmark.SoakReport{
		SchemaVersion: terminalbenchmark.SoakSchemaVersion,
		StartedAt:     now.Format(time.RFC3339Nano), FinishedAt: now.Add(time.Second).Format(time.RFC3339Nano),
		Failures: []string{"frontend checkpoint failed"},
	}); err != nil {
		t.Fatalf("write partial soak report: %v", err)
	}
	if err := salvageTimedOutSoak(raw, final, terminalbenchmark.HostMetrics{}, nil); err != nil {
		t.Fatalf("salvage partial soak report: %v", err)
	}
	report, err := terminalbenchmark.ReadSoakReport(final)
	if err != nil {
		t.Fatalf("read salvaged soak report: %v", err)
	}
	if report.Passed || !containsText(report.Failures, "frontend checkpoint failed") ||
		!containsText(report.Failures, "did not exit before the host timeout") {
		t.Fatalf("salvaged failures = %#v", report.Failures)
	}
}

func TestCompleteLifecycleSmokeAddsNativeHostEvidence(t *testing.T) {
	directory := t.TempDir()
	raw := filepath.Join(directory, "raw-lifecycle.json")
	final := filepath.Join(directory, "final-lifecycle.json")
	now := time.Now().UTC()
	report := terminalbenchmark.LifecycleReport{
		SchemaVersion:             terminalbenchmark.LifecycleSchemaVersion,
		StartedAt:                 now.Format(time.RFC3339Nano),
		FinishedAt:                now.Add(time.Second).Format(time.RFC3339Nano),
		StartupObserved:           true,
		DomReadyObserved:          true,
		FrontendAttached:          true,
		TerminalOpened:            true,
		CloseRequested:            true,
		DecisionDelayMilliseconds: terminalbenchmark.MinimumLifecycleDecisionWait,
		ConfirmationRequested:     true,
		CloseAttempts: []terminalbenchmark.LifecycleCloseAttempt{
			{Sequence: 1, Prevented: true, LiveTerminalsBefore: 1, LiveTerminalsAfter: 1},
			{Sequence: 2, Prevented: false, LiveTerminalsBefore: 0, LiveTerminalsAfter: 0},
		},
		ShutdownCompleted: true,
		ShutdownSucceeded: true,
		Runtime: terminalbenchmark.RuntimeMetrics{
			OperatingSystem: runtime.GOOS,
			Architecture:    runtime.GOARCH,
			ProcessID:       1,
		},
	}
	if err := terminalbenchmark.WriteLifecycleReportAtomic(raw, report); err != nil {
		t.Fatalf("write raw lifecycle report: %v", err)
	}
	host := terminalbenchmark.HostMetrics{
		ProcessTreePeakProcesses: 2,
		WebKitPeakProcesses:      1,
		RSSSamples:               2,
	}
	if runtime.GOOS == "linux" {
		host.WebKitGTKVersion = terminalbenchmark.MinimumWebKitGTKVersion
	}
	if err := completeLifecycleSmoke(raw, final, "", host); err != nil {
		t.Fatalf("complete lifecycle smoke: %v", err)
	}
	completed, err := terminalbenchmark.ReadLifecycleReport(final)
	if err != nil {
		t.Fatalf("read completed lifecycle report: %v", err)
	}
	if !completed.Passed || completed.Host.ProcessTreePeakProcesses != 2 {
		t.Fatalf("completed lifecycle host report failed: %#v", completed)
	}
}

func containsText(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
