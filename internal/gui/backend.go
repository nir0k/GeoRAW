package gui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nir0k/GeoRAW/internal/app"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Backend is bound to the Wails frontend.
type Backend struct {
	ctx context.Context
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

// PickGPX opens a file dialog filtered to GPX files.
func (b *Backend) PickGPX() (string, error) {
	ctx, err := b.currentCtx()
	if err != nil {
		return "", err
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: "Select GPX file",
		Filters: []runtime.FileFilter{
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
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: "Select photo folder",
	})
}

// ProcessRequest represents user input from the GUI.
type ProcessRequest struct {
	GPXPath    string `json:"gpxPath"`
	InputPath  string `json:"inputPath"`
	Recursive  bool   `json:"recursive"`
	LogLevel   string `json:"logLevel"`
	LogFile    string `json:"logFile"`
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

	offset, err := parseOffset(req.TimeOffset)
	if err != nil {
		return nil, err
	}

	opts := app.Options{
		GPXPath:      req.GPXPath,
		InputPath:    req.InputPath,
		Recursive:    req.Recursive,
		LogLevel:     req.LogLevel,
		LogFile:      req.LogFile,
		TimeOffset:   offset,
		AutoOffset:   req.AutoOffset,
		Overwrite:    req.Overwrite,
		PrintSummary: false,
	}

	return app.Run(ctx, opts)
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
