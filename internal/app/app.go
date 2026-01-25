package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/nir0k/GeoRAW/internal/gpx"
	"github.com/nir0k/GeoRAW/internal/media"
	"github.com/nir0k/GeoRAW/internal/xmp"
	"github.com/nir0k/logger"
)

// FileResult describes per-file outcome.
type FileResult struct {
	Path    string `json:"path"`
	Status  string `json:"status"`  // processed, unchanged, skipped, out_of_track, meta_error, failed
	Message string `json:"message"` // optional details
}

// Summary collects overall stats and per-file results.
type Summary struct {
	Processed  int          `json:"processed"`
	Skipped    int          `json:"skipped"`
	Unchanged  int          `json:"unchanged"`
	OutOfTrack int          `json:"out_of_track"`
	Failed     int          `json:"failed"`
	MetaError  int          `json:"meta_errors"`
	Files      []FileResult `json:"files"`
}

// Run is the main entry point for the workflow.
func Run(ctx context.Context, opts Options) (*Summary, error) {
	return run(ctx, opts, nil)
}

// RunWithLogger allows piping logs into an in-memory buffer instead of a file.
func RunWithLogger(ctx context.Context, opts Options, buf *bytes.Buffer) (*Summary, error) {
	return run(ctx, opts, buf)
}

func run(ctx context.Context, opts Options, buf *bytes.Buffer) (*Summary, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	cfg := logger.LogConfig{
		FilePath:       opts.LogFile,
		Format:         "standard",
		FileLevel:      opts.LogLevel,
		ConsoleLevel:   opts.LogLevel,
		ConsoleOutput:  buf != nil,
		EnableRotation: true,
		RotationConfig: logger.RotationConfig{
			MaxSize:    25,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		},
	}
	logInstance, err := logger.NewLogger(cfg)
	if err != nil {
		return nil, err
	}
	if buf != nil {
		logInstance.Config.ConsoleOutput = true
		logInstance.ConsoleLogger = log.New(buf, "", 0)
	}

	infof := logInstance.Infof
	warnf := logInstance.Warningf
	errorf := logInstance.Errorf

	infof("Starting GeoRAW with GPX=%s input=%s recursive=%t offset=%s autoOffset=%t overwrite=%t", opts.GPXPath, opts.InputPath, opts.Recursive, opts.TimeOffset, opts.AutoOffset, opts.Overwrite)

	track, err := gpx.LoadTrack(opts.GPXPath)
	if err != nil {
		return nil, err
	}
	start, end := track.Bounds()
	infof("GPX track loaded with %d points (%s .. %s)", track.PointCount(), start.Format(time.RFC3339), end.Format(time.RFC3339))

	files, err := media.CollectFiles(opts.InputPath, opts.Recursive)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found to process")
	}

	totalFiles := 0
	for _, path := range files {
		if strings.EqualFold(filepath.Ext(path), ".xmp") {
			continue
		}
		totalFiles++
	}
	progressTotal := totalFiles * 2
	progressDone := 0
	reportProgress := func() {
		if opts.Progress == nil || progressTotal == 0 {
			return
		}
		if progressDone > progressTotal {
			progressDone = progressTotal
		}
		opts.Progress(progressDone, progressTotal)
	}
	advance := func(step int) {
		if step <= 0 {
			return
		}
		progressDone += step
		reportProgress()
	}
	reportProgress()

	var (
		processed int
		skipped   int
		failed    int
		unchanged int
		outTrack  int
		metaError int
		results   []FileResult
	)

	jobs := make([]photoJob, 0, len(files))

	for _, path := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".xmp" {
			// Ignore sidecars silently; they may co-exist with RAWs.
			continue
		}
		if !media.SupportedRaw(path) {
			warnf("Skipping non-RAW file: %s", path)
			skipped++
			results = append(results, FileResult{
				Path:   path,
				Status: "skipped",
			})
			advance(2)
			continue
		}

		meta, err := media.ReadMetadata(path)
		if err != nil {
			warnf("Failed to read metadata for %s: %v", path, err)
			metaError++
			results = append(results, FileResult{
				Path:    path,
				Status:  "meta_error",
				Message: err.Error(),
			})
			advance(2)
			continue
		}

		jobs = append(jobs, photoJob{
			Path: path,
			Meta: meta,
		})
		advance(1)
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("no RAW files to process")
	}

	effectiveOffset := opts.TimeOffset
	if effectiveOffset == 0 && opts.AutoOffset {
		offset, samples, err := detectOffset(track, jobs)
		if err != nil {
			warnf("Auto offset detection failed, using 0s: %v", err)
		} else {
			effectiveOffset = offset
			infof("Auto-detected time offset: %s using %d samples", effectiveOffset, samples)
		}
	} else if !opts.AutoOffset {
		infof("Auto offset disabled, using manual offset: %s", effectiveOffset)
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		capture := job.Meta.CaptureTime.Add(effectiveOffset).UTC()
		coord, err := track.CoordinateAt(capture)
		if err != nil {
			if errors.Is(err, gpx.ErrTimestampOutOfBounds) {
				warnf("Capture time outside GPX coverage for %s (%s): %v", job.Path, capture.Format(time.RFC3339), err)
				outTrack++
				results = append(results, FileResult{
					Path:    job.Path,
					Status:  "out_of_track",
					Message: err.Error(),
				})
				advance(1)
				continue
			}
			errorf("No matching GPX point for %s (%s): %v", job.Path, capture.Format(time.RFC3339), err)
			failed++
			results = append(results, FileResult{
				Path:    job.Path,
				Status:  "failed",
				Message: err.Error(),
			})
			advance(1)
			continue
		}

		sidecarPath := xmp.SidecarPath(job.Path)
		wrote, err := xmp.MergeAndWrite(sidecarPath, coord, capture, opts.Overwrite)
		if errors.Is(err, xmp.ErrGPSAlreadyPresent) {
			infof("Skipping already geotagged sidecar %s (use --overwrite-gps to replace)", sidecarPath)
			unchanged++
			results = append(results, FileResult{
				Path:    job.Path,
				Status:  "unchanged",
				Message: "GPS already present",
			})
			advance(1)
			continue
		}
		if err != nil {
			errorf("Failed to write sidecar for %s: %v", job.Path, err)
			failed++
			results = append(results, FileResult{
				Path:    job.Path,
				Status:  "failed",
				Message: err.Error(),
			})
			advance(1)
			continue
		}

		infof("Geotagged %s (%s %s, %s) -> %s [lat=%.6f lon=%.6f alt=%v]",
			job.Path,
			job.Meta.CameraMake,
			job.Meta.CameraModel,
			capture.Format(time.RFC3339),
			sidecarPath,
			coord.Latitude,
			coord.Longitude,
			altText(coord.Altitude),
		)
		if wrote {
			processed++
			results = append(results, FileResult{
				Path:    job.Path,
				Status:  "processed",
				Message: sidecarPath,
			})
		} else {
			unchanged++
			results = append(results, FileResult{
				Path:    job.Path,
				Status:  "unchanged",
				Message: "Sidecar existed",
			})
		}
		advance(1)
	}

	sum := &Summary{
		Processed:  processed,
		Skipped:    skipped,
		Unchanged:  unchanged,
		OutOfTrack: outTrack,
		Failed:     failed,
		MetaError:  metaError,
		Files:      results,
	}
	summary := fmt.Sprintf("Finished. processed=%d skipped=%d unchanged=%d out_of_track=%d failed=%d meta_errors=%d", processed, skipped, unchanged, outTrack, failed, metaError)
	if opts.PrintSummary {
		fmt.Println(summary)
	}
	infof("%s", summary)
	return sum, nil
}

func altText(val *float64) string {
	if val == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2fm", *val)
}
