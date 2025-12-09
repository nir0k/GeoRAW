package gui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nir0k/GeoRAW/internal/app"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Backend is bound to the Wails frontend.
type Backend struct {
	ctx context.Context

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	logBuf  *bytes.Buffer
}

// OnStartup stores the Wails context.
func (b *Backend) OnStartup(ctx context.Context) {
	b.ctx = ctx
}

func (b *Backend) currentCtx() (context.Context, error) {
	if b == nil {
		return nil, errors.New("backend is not initialized yet")
	}
	if b.ctx == nil {
		return nil, errors.New("UI is not ready yet")
	}
	return b.ctx, nil
}

// Cancel stops the current processing (if any).
func (b *Backend) Cancel() error {
	b.mu.Lock()
	cancel := b.cancel
	b.mu.Unlock()

	if cancel == nil {
		return errors.New("nothing to cancel")
	}
	cancel()
	return nil
}

// PickGPX opens a file dialog filtered to GPX files.
func (b *Backend) PickGPX() (string, error) {
	ctx, err := b.currentCtx()
	if err != nil {
		return "", err
	}
	return wruntime.OpenFileDialog(ctx, wruntime.OpenDialogOptions{
		Title: "Select GPX file",
		Filters: []wruntime.FileFilter{
			{DisplayName: "GPX", Pattern: "*.gpx"},
		},
	})
}

// PickFolder opens a directory chooser.
func (b *Backend) PickFolder() (string, error) {
	ctx, err := b.currentCtx()
	if err != nil {
		return "", err
	}
	return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{
		Title: "Select photo folder",
	})
}

// PickFiles opens a multi-file chooser for photos.
func (b *Backend) PickFiles() ([]string, error) {
	ctx, err := b.currentCtx()
	if err != nil {
		return nil, err
	}
	res, err := wruntime.OpenMultipleFilesDialog(ctx, wruntime.OpenDialogOptions{
		Title: "Select photos",
		Filters: []wruntime.FileFilter{
			{DisplayName: "RAW photos", Pattern: "*.cr2;*.cr3;*.arw;*.nef;*.raf;*.dng;*.rw2;*.orf;*.pef"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	return res, err
}

// ClearLogs resets the in-memory log buffer.
func (b *Backend) ClearLogs() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.logBuf != nil {
		b.logBuf.Reset()
	}
}

// GetLogs returns the current in-memory log.
func (b *Backend) GetLogs() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.logBuf == nil {
		return "", nil
	}
	return b.logBuf.String(), nil
}

// SaveLog asks for a path and writes the in-memory log to disk.
func (b *Backend) SaveLog() (string, error) {
	ctx, err := b.currentCtx()
	if err != nil {
		return "", err
	}
	logStr, err := b.GetLogs()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(logStr) == "" {
		return "", errors.New("log is empty")
	}

	target, err := wruntime.SaveFileDialog(ctx, wruntime.SaveDialogOptions{
		Title:           "Save log",
		DefaultFilename: "georaw.log",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Log", Pattern: "*.log"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if target == "" {
		return "", nil
	}
	if err := os.WriteFile(target, []byte(logStr), 0o644); err != nil {
		return "", err
	}
	return target, nil
}

// OpenFolder opens a directory in the system file manager.
func (b *Backend) OpenFolder(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("path is empty")
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// ProcessRequest represents user input from the GUI.
type ProcessRequest struct {
	GPXPath    string `json:"gpxPath"`
	InputPath  string `json:"inputPath"`
	Recursive  bool   `json:"recursive"`
	LogLevel   string `json:"logLevel"`
	TimeOffset string `json:"timeOffset"`
	AutoOffset bool   `json:"autoOffset"`
	Overwrite  bool   `json:"overwrite"`
}

// Process executes the geotagging workflow using existing CLI logic.
func (b *Backend) Process(req ProcessRequest) (*app.Summary, error) {
	ctx, err := b.currentCtx()
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return nil, errors.New("already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	b.running = true
	b.cancel = cancel
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		if b.cancel != nil {
			b.cancel()
		}
		b.running = false
		b.cancel = nil
		b.mu.Unlock()
	}()

	offset, err := parseOffset(req.TimeOffset)
	if err != nil {
		return nil, err
	}

	// Attach in-memory log buffer
	buf := &bytes.Buffer{}
	b.mu.Lock()
	b.logBuf = buf
	b.mu.Unlock()

	opts := app.Options{
		GPXPath:      req.GPXPath,
		InputPath:    req.InputPath,
		Recursive:    req.Recursive,
		LogLevel:     req.LogLevel,
		LogFile:      "",
		TimeOffset:   offset,
		AutoOffset:   req.AutoOffset,
		Overwrite:    req.Overwrite,
		PrintSummary: false,
	}

	return app.RunWithLogger(runCtx, opts, buf)
}

// parseOffset accepts human-friendly strings like "1h30m", "01:30:00", "-15m", "+90s".
func parseOffset(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	// Try Go duration first.
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}

	// Support ±HH:MM or ±HH:MM:SS.
	sign := 1
	if strings.HasPrefix(raw, "-") {
		sign = -1
		raw = strings.TrimPrefix(raw, "-")
	} else if strings.HasPrefix(raw, "+") {
		raw = strings.TrimPrefix(raw, "+")
	}

	parts := strings.Split(raw, ":")
	if len(parts) == 2 || len(parts) == 3 {
		var h, m, s int64
		var err error

		h, err = parseComponent(parts[0], 23)
		if err != nil {
			return 0, err
		}
		m, err = parseComponent(parts[1], 59)
		if err != nil {
			return 0, err
		}
		if len(parts) == 3 {
			s, err = parseComponent(parts[2], 59)
			if err != nil {
				return 0, err
			}
		}
		total := time.Duration(sign) * (time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)
		return total, nil
	}

	return 0, fmt.Errorf("invalid time offset format: %q", raw)
}

func parseComponent(v string, max int64) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	val, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid time component %q", v)
	}
	if val < 0 || val > max {
		return 0, fmt.Errorf("time component %q is out of range", v)
	}
	return val, nil
}
