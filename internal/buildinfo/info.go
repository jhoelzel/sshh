package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

const (
	developmentVersion = "0.1.0-dev"
	unknownValue       = "unknown"
)

// These values are set by release and development builds through Go linker
// flags. Empty values deliberately fall back to Go's embedded VCS metadata.
var (
	Version   string
	Commit    string
	BuildDate string
	Dirty     string
)

type Info struct {
	Version   string
	Commit    string
	BuildDate string
	Dirty     bool
	GoVersion string
	Platform  string
}

type vcsInfo struct {
	version    string
	commit     string
	dirty      bool
	dirtyKnown bool
}

func Current() Info {
	vcs, goVersion := readGoBuildInfo()
	return resolve(Version, Commit, BuildDate, Dirty, vcs, goVersion, runtime.GOOS+"/"+runtime.GOARCH)
}

func readGoBuildInfo() (vcsInfo, string) {
	result := vcsInfo{}
	goVersion := runtime.Version()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return result, goVersion
	}
	if strings.TrimSpace(info.GoVersion) != "" {
		goVersion = strings.TrimSpace(info.GoVersion)
	}
	if version := strings.TrimSpace(info.Main.Version); version != "" && version != "(devel)" {
		result.version = version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			result.commit = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			if value, err := strconv.ParseBool(strings.TrimSpace(setting.Value)); err == nil {
				result.dirty = value
				result.dirtyKnown = true
			}
		}
	}
	return result, goVersion
}

func resolve(version, commit, buildDate, dirty string, vcs vcsInfo, goVersion, platform string) Info {
	resolvedVersion := strings.TrimSpace(version)
	if resolvedVersion == "" {
		resolvedVersion = strings.TrimSpace(vcs.version)
	}
	if resolvedVersion == "" {
		resolvedVersion = developmentVersion
	}

	resolvedCommit := strings.TrimSpace(commit)
	if resolvedCommit == "" {
		resolvedCommit = strings.TrimSpace(vcs.commit)
	}
	if resolvedCommit == "" {
		resolvedCommit = unknownValue
	}

	resolvedBuildDate := strings.TrimSpace(buildDate)
	if resolvedBuildDate == "" {
		resolvedBuildDate = unknownValue
	}

	resolvedDirty, dirtyKnown := parseDirty(dirty)
	if !dirtyKnown && vcs.dirtyKnown {
		resolvedDirty = vcs.dirty
	}

	if strings.TrimSpace(goVersion) == "" {
		goVersion = unknownValue
	}
	if strings.TrimSpace(platform) == "" {
		platform = unknownValue
	}

	return Info{
		Version:   resolvedVersion,
		Commit:    resolvedCommit,
		BuildDate: resolvedBuildDate,
		Dirty:     resolvedDirty,
		GoVersion: strings.TrimSpace(goVersion),
		Platform:  strings.TrimSpace(platform),
	}
}

func parseDirty(value string) (bool, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return parsed, true
}
