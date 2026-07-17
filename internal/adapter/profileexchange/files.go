package profileexchange

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const portableFileMode = 0o600

func ReadFile(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open profile file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect profile file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("selected profile source is not a regular file")
	}
	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("profile file exceeds the %d MiB limit", MaxFileSize/(1<<20))
	}

	data, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read profile file: %w", err)
	}
	if len(data) > MaxFileSize {
		return nil, fmt.Errorf("profile file exceeds the %d MiB limit", MaxFileSize/(1<<20))
	}
	return data, nil
}

func WriteFile(filename string, data []byte) error {
	if strings.TrimSpace(filename) == "" {
		return errors.New("profile export path is required")
	}
	if len(data) > MaxFileSize {
		return fmt.Errorf("profile export exceeds the %d MiB limit", MaxFileSize/(1<<20))
	}

	directory := filepath.Dir(filename)
	tempFile, err := os.CreateTemp(directory, ".shh-h-profiles-*.json")
	if err != nil {
		return fmt.Errorf("create temporary profile export: %w", err)
	}
	tempName := tempFile.Name()
	defer os.Remove(tempName)

	closeWithError := func(operation string, cause error) error {
		_ = tempFile.Close()
		return fmt.Errorf("%s profile export: %w", operation, cause)
	}
	if err := tempFile.Chmod(portableFileMode); err != nil {
		return closeWithError("protect temporary", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return closeWithError("write", err)
	}
	if err := tempFile.Sync(); err != nil {
		return closeWithError("sync", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close profile export: %w", err)
	}
	if err := os.Rename(tempName, filename); err != nil {
		return fmt.Errorf("replace profile export: %w", err)
	}
	if err := os.Chmod(filename, portableFileMode); err != nil {
		return fmt.Errorf("protect profile export: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync profile export directory: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
