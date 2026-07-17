package buildinfo

import "testing"

func TestResolvePrefersLinkerMetadata(t *testing.T) {
	got := resolve(
		"1.4.2", "abc123", "2026-07-17T15:30:00Z", "false",
		vcsInfo{version: "v0.9.0", commit: "fallback", dirty: true, dirtyKnown: true},
		"go1.26.5", "darwin/arm64",
	)

	if got.Version != "1.4.2" || got.Commit != "abc123" || got.BuildDate != "2026-07-17T15:30:00Z" {
		t.Fatalf("linker metadata was not preserved: %#v", got)
	}
	if got.Dirty {
		t.Fatal("explicit clean linker metadata was replaced by the VCS fallback")
	}
	if got.GoVersion != "go1.26.5" || got.Platform != "darwin/arm64" {
		t.Fatalf("runtime metadata was not preserved: %#v", got)
	}
}

func TestResolveUsesGoVCSFallbacks(t *testing.T) {
	got := resolve(
		"", "", "", "",
		vcsInfo{version: "v2.0.0", commit: "def456", dirty: true, dirtyKnown: true},
		"go1.26.5", "linux/amd64",
	)

	if got.Version != "v2.0.0" || got.Commit != "def456" || got.BuildDate != unknownValue {
		t.Fatalf("unexpected fallback metadata: %#v", got)
	}
	if !got.Dirty {
		t.Fatal("expected Go VCS dirty state to be retained")
	}
}

func TestResolveHasStableDevelopmentDefaults(t *testing.T) {
	got := resolve("", "", "", "not-a-boolean", vcsInfo{}, "", "")

	if got.Version != developmentVersion || got.Commit != unknownValue || got.BuildDate != unknownValue {
		t.Fatalf("unexpected development defaults: %#v", got)
	}
	if got.Dirty || got.GoVersion != unknownValue || got.Platform != unknownValue {
		t.Fatalf("unexpected runtime defaults: %#v", got)
	}
}
