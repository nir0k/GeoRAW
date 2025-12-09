package gui

import (
	"context"
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

// PickGPX opens a file dialog filtered to GPX files.
func (b *Backend) PickGPX() (string, error) {
	return runtime.OpenFileDialog(b.ctx, runtime.OpenDialogOptions{
		Title: "Select GPX file",
		Filters: []runtime.FileFilter{
			{DisplayName: "GPX", Pattern: "*.gpx"},
		},
	})
}

// PickFolder opens a directory chooser.
func (b *Backend) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(b.ctx, runtime.OpenDialogOptions{
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
	TimeOffset int64  `json:"timeOffsetSeconds"`
	AutoOffset bool   `json:"autoOffset"`
	Overwrite  bool   `json:"overwrite"`
}

// Process executes the geotagging workflow using existing CLI logic.
func (b *Backend) Process(req ProcessRequest) (*app.Summary, error) {
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	opts := app.Options{
		GPXPath:    req.GPXPath,
		InputPath:  req.InputPath,
		Recursive:  req.Recursive,
		LogLevel:   req.LogLevel,
		LogFile:    req.LogFile,
		TimeOffset: time.Duration(req.TimeOffset) * time.Second,
		AutoOffset: req.AutoOffset,
		Overwrite:  req.Overwrite,
		PrintSummary: false,
	}

	return app.Run(ctx, opts)
}
