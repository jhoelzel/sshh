package textfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxBytes = 16 * 1024 * 1024
	fileMode = 0o600
)

func WriteAtomic(filename string, data []byte) error {
	if strings.TrimSpace(filename) == "" {
		return errors.New("text export path is required")
	}
	if len(data) > MaxBytes {
		return fmt.Errorf("text export exceeds the %d MiB limit", MaxBytes/(1<<20))
	}

	directory := filepath.Dir(filename)
	temporary, err := os.CreateTemp(directory, ".shh-h-terminal-*.txt")
	if err != nil {
		return fmt.Errorf("create temporary text export: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	closeWithError := func(operation string, cause error) error {
		_ = temporary.Close()
		return fmt.Errorf("%s text export: %w", operation, cause)
	}
	if err := temporary.Chmod(fileMode); err != nil {
		return closeWithError("protect temporary", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return closeWithError("write", err)
	}
	if err := temporary.Sync(); err != nil {
		return closeWithError("sync", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close text export: %w", err)
	}
	if err := os.Rename(temporaryPath, filename); err != nil {
		return fmt.Errorf("replace text export: %w", err)
	}
	if err := os.Chmod(filename, fileMode); err != nil {
		return fmt.Errorf("protect text export: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync text export directory: %w", err)
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
