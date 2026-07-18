//go:build windows

package localpty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func resolveWindowsShell(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		resolved, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("find configured Windows shell %q: %w", configured, err)
		}
		return resolved, nil
	}

	candidates := []string{"pwsh.exe"}
	if systemRoot := strings.TrimSpace(os.Getenv("SystemRoot")); systemRoot != "" {
		candidates = append(candidates, filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
	}
	candidates = append(candidates, "powershell.exe")
	if commandPrompt := strings.TrimSpace(os.Getenv("ComSpec")); commandPrompt != "" {
		candidates = append(candidates, commandPrompt)
	}
	candidates = append(candidates, "cmd.exe", "wsl.exe")

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToUpper(candidate)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", errors.New("no supported Windows shell found; install PowerShell, Command Prompt, or WSL")
}

func resolveWindowsWorkingDirectory(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find Windows home directory: %w", err)
	}
	return home, nil
}

func windowsProcessParameters(command string, arguments []string, directory string) (*uint16, []uint16, *uint16, error) {
	application, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode local shell path: %w", err)
	}
	commandLine, err := windows.UTF16FromString(windows.ComposeCommandLine(append([]string{command}, arguments...)))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode local shell command line: %w", err)
	}
	var currentDirectory *uint16
	if directory != "" {
		currentDirectory, err = windows.UTF16PtrFromString(directory)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("encode local shell working directory: %w", err)
		}
	}
	return application, commandLine, currentDirectory, nil
}

func mergeWindowsEnvironment(base, overrides []string) []string {
	type environmentValue struct {
		name  string
		value string
	}
	values := make(map[string]environmentValue, len(base)+len(overrides))
	apply := func(items []string) {
		for _, item := range items {
			name, value, ok := splitWindowsEnvironment(item)
			if !ok {
				continue
			}
			values[strings.ToUpper(name)] = environmentValue{name: name, value: value}
		}
	}
	apply(base)
	apply(overrides)

	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.name+"="+value.value)
	}
	sort.Slice(result, func(left, right int) bool {
		leftName, _, _ := splitWindowsEnvironment(result[left])
		rightName, _, _ := splitWindowsEnvironment(result[right])
		leftKey := strings.ToUpper(leftName)
		rightKey := strings.ToUpper(rightName)
		if leftKey == rightKey {
			return leftName < rightName
		}
		return leftKey < rightKey
	})
	return result
}

func splitWindowsEnvironment(item string) (string, string, bool) {
	searchFrom := 0
	if strings.HasPrefix(item, "=") {
		searchFrom = 1
	}
	separator := strings.IndexByte(item[searchFrom:], '=')
	if separator < 0 {
		return "", "", false
	}
	separator += searchFrom
	if separator == 0 {
		return "", "", false
	}
	return item[:separator], item[separator+1:], true
}

func createWindowsEnvironmentBlock(environment []string) ([]uint16, error) {
	block := make([]uint16, 0)
	for _, item := range environment {
		if strings.IndexByte(item, 0) >= 0 {
			return nil, errors.New("environment contains a null byte")
		}
		block = append(block, utf16.Encode([]rune(item))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	if len(environment) == 0 {
		block = append(block, 0)
	}
	return block, nil
}
