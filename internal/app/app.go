package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nir0k/GeoRAW/internal/gpx"
	"github.com/nir0k/GeoRAW/internal/media"
	"github.com/nir0k/GeoRAW/internal/xmp"
	"github.com/nir0k/logger"
)

// Run is the main entry point for the CLI workflow.
func Run(ctx context.Context, opts Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	cfg := logger.LogConfig{
		FilePath:       opts.LogFile,
		Format:         "standard",
		FileLevel:      opts.LogLevel,
		ConsoleLevel:   opts.LogLevel,
		ConsoleOutput:  opts.ConsoleLog,
		EnableRotation: true,
		RotationConfig: logger.RotationConfig{
			MaxSize:    25,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		},
	}
	if err := logger.InitLogger(cfg); err != nil {
		return err
	}

	logger.Infof("Starting GeoRAW with GPX=%s input=%s recursive=%t offset=%s autoOffset=%t overwrite=%t", opts.GPXPath, opts.InputPath, opts.Recursive, opts.TimeOffset, opts.AutoOffset, opts.Overwrite)

	track, err := gpx.LoadTrack(opts.GPXPath)
	if err != nil {
		return err
	}
	start, end := track.Bounds()
	logger.Infof("GPX track loaded with %d points (%s .. %s)", track.PointCount(), start.Format(time.RFC3339), end.Format(time.RFC3339))

	files, err := media.CollectFiles(opts.InputPath, opts.Recursive)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files found to process")
	}

	var (
		processed int
		skipped   int
		failed    int
		unchanged int
		outTrack  int
		metaError int
	)

	jobs := make([]photoJob, 0, len(files))

	for _, path := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".xmp" {
			// Ignore sidecars silently; they may co-exist with RAWs.
			continue
		}
		if !media.SupportedRaw(path) {
			logger.Warningf("Skipping non-RAW file: %s", path)
			skipped++
			continue
		}

		meta, err := media.ReadMetadata(path)
		if err != nil {
			logger.Warningf("Failed to read metadata for %s: %v", path, err)
			metaError++
			continue
		}

		jobs = append(jobs, photoJob{
			Path: path,
			Meta: meta,
		})
	}

	if len(jobs) == 0 {
		return fmt.Errorf("no RAW files to process")
	}

	effectiveOffset := opts.TimeOffset
	if effectiveOffset == 0 && opts.AutoOffset {
		offset, samples, err := detectOffset(track, jobs)
		if err != nil {
			logger.Warningf("Auto offset detection failed, using 0s: %v", err)
		} else {
			effectiveOffset = offset
			logger.Infof("Auto-detected time offset: %s using %d samples", effectiveOffset, samples)
		}
	} else if !opts.AutoOffset {
		logger.Infof("Auto offset disabled, using manual offset: %s", effectiveOffset)
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		capture := job.Meta.CaptureTime.Add(effectiveOffset).UTC()
		coord, err := track.CoordinateAt(capture)
		if err != nil {
			if errors.Is(err, gpx.ErrTimestampOutOfBounds) {
				logger.Warningf("Capture time outside GPX coverage for %s (%s): %v", job.Path, capture.Format(time.RFC3339), err)
				outTrack++
				continue
			}
			logger.Errorf("No matching GPX point for %s (%s): %v", job.Path, capture.Format(time.RFC3339), err)
			failed++
			continue
		}

		sidecarPath := xmp.SidecarPath(job.Path)
		wrote, err := xmp.MergeAndWrite(sidecarPath, coord, capture, opts.Overwrite)
		if errors.Is(err, xmp.ErrGPSAlreadyPresent) {
			logger.Infof("Skipping already geotagged sidecar %s (use --overwrite-gps to replace)", sidecarPath)
			unchanged++
			continue
		}
		if err != nil {
			logger.Errorf("Failed to write sidecar for %s: %v", job.Path, err)
			failed++
			continue
		}

		logger.Infof("Geotagged %s (%s %s, %s) -> %s [lat=%.6f lon=%.6f alt=%v]",
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
		} else {
			unchanged++
		}
	}

	logger.Infof("Finished. processed=%d skipped=%d unchanged=%d out_of_track=%d failed=%d meta_errors=%d", processed, skipped, unchanged, outTrack, failed, metaError)
	return nil
}

func altText(val *float64) string {
	if val == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2fm", *val)
}
