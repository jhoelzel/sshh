package profileexchange

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	profiledomain "shh-h/internal/domain/profile"
)

const (
	CurrentVersion = 1
	MaxFileSize    = 2 << 20
	MaxProfiles    = 1000
	maxWarnings    = 500
)

type Format string

const (
	FormatJSON    Format = "shh-h JSON"
	FormatOpenSSH Format = "OpenSSH config"
)

type ParseResult struct {
	Format   Format
	Profiles []profiledomain.Profile
	Warnings []string
}

type portableDocument struct {
	Version  int               `json:"version"`
	Profiles []portableProfile `json:"profiles"`
}

// portableProfile intentionally has no IDs, timestamps, or dedicated
// credential fields. User-authored profile values are otherwise preserved.
type portableProfile struct {
	Name             string                       `json:"name"`
	Protocol         profiledomain.Protocol       `json:"protocol"`
	Host             string                       `json:"host,omitempty"`
	Port             int                          `json:"port,omitempty"`
	Username         string                       `json:"username,omitempty"`
	Authentication   profiledomain.Authentication `json:"authentication,omitempty"`
	IdentityFile     string                       `json:"identityFile,omitempty"`
	Shell            string                       `json:"shell,omitempty"`
	Arguments        []string                     `json:"arguments,omitempty"`
	WorkingDirectory string                       `json:"workingDirectory,omitempty"`
	Environment      map[string]string            `json:"environment,omitempty"`
	Tags             []string                     `json:"tags,omitempty"`
	Group            string                       `json:"group,omitempty"`
	Favorite         bool                         `json:"favorite,omitempty"`
}

func Encode(profiles []profiledomain.Profile) ([]byte, error) {
	if len(profiles) > MaxProfiles {
		return nil, fmt.Errorf("cannot export %d profiles; limit is %d", len(profiles), MaxProfiles)
	}
	items := make([]profiledomain.Profile, len(profiles))
	copy(items, profiles)
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.ToLower(items[i].Name)
		right := strings.ToLower(items[j].Name)
		if left == right {
			return items[i].Name < items[j].Name
		}
		return left < right
	})

	document := portableDocument{Version: CurrentVersion, Profiles: make([]portableProfile, 0, len(items))}
	for _, item := range items {
		document.Profiles = append(document.Profiles, portableFromDomain(item))
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode portable profiles: %w", err)
	}
	return append(data, '\n'), nil
}

func Parse(filename string, data []byte) (ParseResult, error) {
	if len(data) > MaxFileSize {
		return ParseResult{}, fmt.Errorf("profile file exceeds the %d MiB limit", MaxFileSize/(1<<20))
	}
	trimmed := bytes.TrimSpace(data)
	if strings.EqualFold(filepath.Ext(filename), ".json") || (len(trimmed) > 0 && trimmed[0] == '{') {
		return parseJSON(trimmed)
	}
	return parseOpenSSH(data)
}

func parseJSON(data []byte) (ParseResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document portableDocument
	if err := decoder.Decode(&document); err != nil {
		return ParseResult{}, fmt.Errorf("decode portable profiles: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ParseResult{}, err
	}
	if document.Version != CurrentVersion {
		return ParseResult{}, fmt.Errorf("unsupported portable profile version %d", document.Version)
	}
	if len(document.Profiles) > MaxProfiles {
		return ParseResult{}, fmt.Errorf("profile file contains %d entries; limit is %d", len(document.Profiles), MaxProfiles)
	}

	profiles := make([]profiledomain.Profile, 0, len(document.Profiles))
	for _, item := range document.Profiles {
		profiles = append(profiles, item.domain())
	}
	return ParseResult{Format: FormatJSON, Profiles: profiles, Warnings: []string{}}, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing portable profile data: %w", err)
	}
	return errors.New("portable profile file contains more than one JSON value")
}

func portableFromDomain(item profiledomain.Profile) portableProfile {
	return portableProfile{
		Name: item.Name, Protocol: item.Protocol, Host: item.Host, Port: item.Port,
		Username: item.Username, Authentication: item.Authentication, IdentityFile: item.IdentityFile,
		Shell: item.Shell, Arguments: append([]string(nil), item.Arguments...),
		WorkingDirectory: item.WorkingDirectory, Environment: cloneMap(item.Environment),
		Tags: append([]string(nil), item.Tags...), Group: item.Group, Favorite: item.Favorite,
	}
}

func (item portableProfile) domain() profiledomain.Profile {
	return profiledomain.Profile{
		Name: item.Name, Protocol: item.Protocol, Host: item.Host, Port: item.Port,
		Username: item.Username, Authentication: item.Authentication, IdentityFile: item.IdentityFile,
		Shell: item.Shell, Arguments: append([]string(nil), item.Arguments...),
		WorkingDirectory: item.WorkingDirectory, Environment: cloneMap(item.Environment),
		Tags: append([]string(nil), item.Tags...), Group: item.Group, Favorite: item.Favorite,
	}
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

type sshDirective struct {
	line     int
	keyword  string
	values   []string
	patterns []string
}

type sshAlias struct {
	name string
	line int
}

func parseOpenSSH(data []byte) (ParseResult, error) {
	warnings := newWarningCollector()
	directives := make([]sshDirective, 0)
	aliases := make([]sshAlias, 0)
	seenAliases := make(map[string]struct{})
	var patterns []string
	inMatch := false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), MaxFileSize)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		words, err := splitSSHWords(scanner.Text())
		if err != nil {
			warnings.addf("line %d: %v", lineNumber, err)
			continue
		}
		if len(words) == 0 {
			continue
		}
		keyword, values := splitSSHDirective(words)
		lowerKeyword := strings.ToLower(keyword)
		switch lowerKeyword {
		case "host":
			inMatch = false
			if len(values) == 0 {
				patterns = nil
				warnings.addf("line %d: Host requires at least one pattern", lineNumber)
				continue
			}
			patterns = append([]string(nil), values...)
			for _, value := range values {
				if strings.HasPrefix(value, "!") || strings.ContainsAny(value, "*?") {
					if value != "*" && !strings.HasPrefix(value, "!") {
						warnings.addf("line %d: wildcard Host pattern %q is settings-only and was not imported", lineNumber, value)
					}
					continue
				}
				key := strings.ToLower(value)
				if _, exists := seenAliases[key]; exists {
					warnings.addf("line %d: duplicate Host alias %q was imported once", lineNumber, value)
					continue
				}
				seenAliases[key] = struct{}{}
				aliases = append(aliases, sshAlias{name: value, line: lineNumber})
			}
		case "match":
			inMatch = true
			patterns = nil
			warnings.addf("line %d: Match blocks are not imported", lineNumber)
		default:
			if inMatch {
				warnings.addf("line %d: directive %q inside a Match block was skipped", lineNumber, keyword)
				continue
			}
			directives = append(directives, sshDirective{
				line: lineNumber, keyword: lowerKeyword, values: append([]string(nil), values...),
				patterns: append([]string(nil), patterns...),
			})
			if !isSupportedSSHDirective(lowerKeyword) {
				warnings.addf("line %d: unsupported directive %q was ignored", lineNumber, keyword)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, fmt.Errorf("read OpenSSH config: %w", err)
	}
	if len(aliases) > MaxProfiles {
		return ParseResult{}, fmt.Errorf("OpenSSH config contains %d concrete hosts; limit is %d", len(aliases), MaxProfiles)
	}
	if len(aliases) == 0 {
		warnings.addf("no concrete Host entries were found")
	}

	profiles := make([]profiledomain.Profile, 0, len(aliases))
	for _, alias := range aliases {
		candidate, ok := evaluateSSHAlias(alias, directives, warnings)
		if ok {
			profiles = append(profiles, candidate)
		}
	}
	return ParseResult{Format: FormatOpenSSH, Profiles: profiles, Warnings: warnings.list()}, nil
}

func splitSSHDirective(words []string) (string, []string) {
	if index := strings.IndexByte(words[0], '='); index >= 0 {
		keyword := words[0][:index]
		values := make([]string, 0, len(words))
		values = append(values, words[0][index+1:])
		values = append(values, words[1:]...)
		return keyword, values
	}
	return words[0], words[1:]
}

func splitSSHWords(line string) ([]string, error) {
	words := make([]string, 0)
	var current strings.Builder
	var quote byte
	escaped := false
	hasToken := false

	flush := func() {
		if hasToken {
			words = append(words, current.String())
			current.Reset()
			hasToken = false
		}
	}
	for index := 0; index < len(line); index++ {
		character := line[index]
		if escaped {
			current.WriteByte(character)
			hasToken = true
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			hasToken = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteByte(character)
			}
			hasToken = true
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
			hasToken = true
		case '#':
			flush()
			return words, nil
		case ' ', '\t', '\r':
			flush()
		default:
			current.WriteByte(character)
			hasToken = true
		}
	}
	if escaped {
		return nil, errors.New("unfinished escape sequence")
	}
	if quote != 0 {
		return nil, errors.New("unterminated quoted value")
	}
	flush()
	return words, nil
}

func isSupportedSSHDirective(keyword string) bool {
	switch keyword {
	case "hostname", "user", "port", "identityfile":
		return true
	default:
		return false
	}
}

func evaluateSSHAlias(alias sshAlias, directives []sshDirective, warnings *warningCollector) (profiledomain.Profile, bool) {
	profile := profiledomain.Profile{
		Name: alias.name, Protocol: profiledomain.ProtocolSSH, Host: alias.name,
		Port: 22, Authentication: profiledomain.AuthenticationAuto,
	}
	set := make(map[string]bool)
	valid := true
	for _, directive := range directives {
		if !hostPatternsMatch(directive.patterns, alias.name) {
			continue
		}
		switch directive.keyword {
		case "hostname":
			if !set[directive.keyword] {
				value, ok := oneSSHValue(alias.name, directive, warnings)
				if !ok {
					valid = false
					continue
				}
				profile.Host = value
				set[directive.keyword] = true
			}
		case "user":
			if !set[directive.keyword] {
				value, ok := oneSSHValue(alias.name, directive, warnings)
				if !ok {
					valid = false
					continue
				}
				profile.Username = value
				set[directive.keyword] = true
			}
		case "port":
			if !set[directive.keyword] {
				value, ok := oneSSHValue(alias.name, directive, warnings)
				if !ok {
					valid = false
					continue
				}
				port, err := strconv.Atoi(value)
				if err != nil || port < 1 || port > 65535 {
					warnings.addf("line %d: Host %q has invalid Port %q and was skipped", directive.line, alias.name, value)
					valid = false
					continue
				}
				profile.Port = port
				set[directive.keyword] = true
			}
		case "identityfile":
			value, ok := oneSSHValue(alias.name, directive, warnings)
			if !ok {
				continue
			}
			if strings.EqualFold(value, "none") {
				continue
			}
			if strings.Contains(value, "%") || (strings.HasPrefix(value, "~") && value != "~" && !strings.HasPrefix(value, "~/")) {
				warnings.addf("line %d: Host %q uses an IdentityFile path expansion that shh-h cannot resolve; the path was ignored", directive.line, alias.name)
				continue
			}
			if !set[directive.keyword] {
				profile.IdentityFile = value
				set[directive.keyword] = true
			} else {
				warnings.addf("line %d: Host %q has another IdentityFile; only the first was imported", directive.line, alias.name)
			}
		case "proxyjump", "proxycommand":
			if set[directive.keyword] {
				continue
			}
			set[directive.keyword] = true
			if len(directive.values) == 1 && strings.EqualFold(directive.values[0], "none") {
				continue
			}
			warnings.addf("line %d: Host %q was skipped because %s cannot be represented safely", directive.line, alias.name, directive.keyword)
			valid = false
		}
	}
	if !valid {
		warnings.addf("line %d: Host %q was not imported because its effective connection settings are invalid", alias.line, alias.name)
		return profiledomain.Profile{}, false
	}
	return profile, true
}

func oneSSHValue(alias string, directive sshDirective, warnings *warningCollector) (string, bool) {
	if len(directive.values) != 1 || strings.TrimSpace(directive.values[0]) == "" {
		warnings.addf("line %d: Host %q has malformed %s; the directive requires exactly one non-empty value", directive.line, alias, directive.keyword)
		return "", false
	}
	return directive.values[0], true
}

func hostPatternsMatch(patterns []string, alias string) bool {
	if len(patterns) == 0 {
		return true
	}
	matched := false
	for _, rawPattern := range patterns {
		negated := strings.HasPrefix(rawPattern, "!")
		pattern := strings.TrimPrefix(rawPattern, "!")
		if !wildcardMatch(strings.ToLower(pattern), strings.ToLower(alias)) {
			continue
		}
		if negated {
			return false
		}
		matched = true
	}
	return matched
}

func wildcardMatch(pattern, value string) bool {
	valueRunes := []rune(value)
	previous := make([]bool, len(valueRunes)+1)
	previous[0] = true
	for _, patternRune := range []rune(pattern) {
		current := make([]bool, len(valueRunes)+1)
		switch patternRune {
		case '*':
			current[0] = previous[0]
			for index := 1; index <= len(valueRunes); index++ {
				current[index] = previous[index] || current[index-1]
			}
		case '?':
			for index := 1; index <= len(valueRunes); index++ {
				current[index] = previous[index-1]
			}
		default:
			for index := 1; index <= len(valueRunes); index++ {
				current[index] = previous[index-1] && valueRunes[index-1] == patternRune
			}
		}
		previous = current
	}
	return previous[len(valueRunes)]
}

type warningCollector struct {
	items     []string
	seen      map[string]struct{}
	truncated bool
}

func newWarningCollector() *warningCollector {
	return &warningCollector{seen: make(map[string]struct{})}
}

func (collector *warningCollector) addf(format string, values ...any) {
	message := fmt.Sprintf(format, values...)
	if _, exists := collector.seen[message]; exists {
		return
	}
	collector.seen[message] = struct{}{}
	if len(collector.items) < maxWarnings {
		collector.items = append(collector.items, message)
	} else {
		collector.truncated = true
	}
}

func (collector *warningCollector) list() []string {
	items := append([]string(nil), collector.items...)
	if collector.truncated {
		items = append(items, "Additional diagnostics were omitted after the 500-item safety limit.")
	}
	return items
}
