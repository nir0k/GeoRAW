package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options represents user-provided CLI parameters.
type Options struct {
	GPXPath    string
	InputPath  string
	Recursive  bool
	LogLevel   string
	LogFile    string
	TimeOffset time.Duration
	AutoOffset bool
	Overwrite  bool
	PrintSummary bool
}

// Validate performs basic validation and assigns defaults where needed.
func (o *Options) Validate() error {
	o.GPXPath = strings.TrimSpace(o.GPXPath)
	o.InputPath = strings.TrimSpace(o.InputPath)
	o.LogLevel = strings.TrimSpace(o.LogLevel)
	o.LogFile = strings.TrimSpace(o.LogFile)

	if o.GPXPath == "" {
		return fmt.Errorf("GPX path is required")
	}
	if o.InputPath == "" {
		return fmt.Errorf("input path is required")
	}
	if o.LogLevel == "" {
		o.LogLevel = "info"
	}
	if o.LogFile == "" {
		defaultPath, err := defaultLogPath()
		if err != nil {
			return err
		}
		o.LogFile = defaultPath
	}
	return nil
}

func defaultLogPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	dir := filepath.Dir(exe)
	// When running via `go run`, executable resides in temp; prefer current working dir then.
	if strings.HasPrefix(dir, os.TempDir()) {
		cwd, err := os.Getwd()
		if err == nil {
			dir = cwd
		}
	}
	return filepath.Join(dir, "georaw.log"), nil
}
