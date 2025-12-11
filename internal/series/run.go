package series

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nir0k/GeoRAW/internal/app"
	"github.com/nir0k/GeoRAW/internal/media"
	"github.com/nir0k/GeoRAW/internal/xmp"
	"github.com/nir0k/logger"
)

const (
	minSeriesLen             = 3
	maxGapDefault            = 1100 * time.Millisecond
	maxGapSequential         = 2200 * time.Millisecond
	evHDRThreshold   float64 = 0.7
)

type seriesJob struct {
	Path string
	Meta media.SeriesMetadata
	Seq  int
}

// Run is the main entry point for series detection/tagging.
func Run(ctx context.Context, opts Options) (*app.Summary, error) {
	return run(ctx, opts, nil)
}

// RunWithLogger allows piping logs into an in-memory buffer instead of a file.
func RunWithLogger(ctx context.Context, opts Options, buf *bytes.Buffer) (*app.Summary, error) {
	return run(ctx, opts, buf)
}

func run(ctx context.Context, opts Options, buf *bytes.Buffer) (*app.Summary, error) {
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

	infof("Starting series tagging with input=%s recursive=%t mode=%s overwrite=%t prefix=%s start=%d hdrTag=%s focusTag=%s",
		opts.InputPath, opts.Recursive, opts.Mode, opts.Overwrite, opts.Prefix, opts.StartIndex, opts.HDRTag, opts.FocusTag)

	files, err := media.CollectFiles(opts.InputPath, opts.Recursive)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found to process")
	}

	var (
		results   []app.FileResult
		processed int
		skipped   int
		unchanged int
		failed    int
		metaError int
	)

	jobs := make([]seriesJob, 0, len(files))
	for _, path := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".xmp" {
			continue
		}
		if !media.SupportedRaw(path) {
			warnf("Skipping non-RAW file: %s", path)
			skipped++
			results = append(results, app.FileResult{Path: path, Status: "skipped", Message: "Not a RAW file"})
			continue
		}

		meta, err := media.ReadSeriesMetadata(path)
		if err != nil {
			warnf("Failed to read metadata for %s: %v", path, err)
			metaError++
			results = append(results, app.FileResult{
				Path:    path,
				Status:  "meta_error",
				Message: err.Error(),
			})
			continue
		}

		if !isCanon(meta.CameraMake) {
			warnf("Skipping non-Canon file: %s (%s)", path, meta.CameraMake)
			skipped++
			results = append(results, app.FileResult{
				Path:    path,
				Status:  "skipped",
				Message: "Not a Canon RAW",
			})
			continue
		}

		jobs = append(jobs, seriesJob{
			Path: path,
			Meta: meta,
			Seq:  parseSequence(path),
		})
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("no Canon RAW files to process")
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Meta.CaptureTime.Equal(jobs[j].Meta.CaptureTime) {
			if jobs[i].Seq != jobs[j].Seq {
				return jobs[i].Seq < jobs[j].Seq
			}
			return jobs[i].Path < jobs[j].Path
		}
		return jobs[i].Meta.CaptureTime.Before(jobs[j].Meta.CaptureTime)
	})

	groups := buildGroups(jobs)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no candidate series found")
	}

	seriesIdx := opts.StartIndex
	for _, group := range groups {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if len(group) < minSeriesLen {
			for _, job := range group {
				skipped++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "skipped",
					Message: "Series too short",
				})
			}
			continue
		}

		seriesType := resolveType(group, opts)
		typeTag := opts.HDRTag
		if seriesType == ModeFocus {
			typeTag = opts.FocusTag
		}
		seriesID := fmt.Sprintf("%s_%05d", opts.Prefix, seriesIdx)
		seriesIdx++

		for _, job := range group {
			tags := []string{typeTag, seriesID}
			sidecar := xmp.SidecarPath(job.Path)

			wrote, err := xmp.MergeKeywords(sidecar, tags, opts.Overwrite)
			if errors.Is(err, xmp.ErrKeywordsAlreadyPresent) {
				infof("Series tags already present for %s", job.Path)
				unchanged++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "unchanged",
					Message: "Series tags already present",
				})
				continue
			}
			if err != nil {
				errorf("Failed to write sidecar for %s: %v", job.Path, err)
				failed++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "failed",
					Message: err.Error(),
				})
				continue
			}

			infof("Tagged %s as %s (%s) -> %s", job.Path, typeTag, seriesID, sidecar)
			if wrote {
				processed++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "processed",
					Message: fmt.Sprintf("%s [%s]", typeTag, seriesID),
				})
			} else {
				unchanged++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "unchanged",
					Message: "Sidecar unchanged",
				})
			}
		}
	}

	sum := &app.Summary{
		Processed: processed,
		Skipped:   skipped,
		Unchanged: unchanged,
		Failed:    failed,
		MetaError: metaError,
		Files:     results,
	}

	if opts.PrintSummary {
		fmt.Printf("Finished. processed=%d skipped=%d unchanged=%d failed=%d meta_errors=%d\n", processed, skipped, unchanged, failed, metaError)
	}
	infof("Finished. processed=%d skipped=%d unchanged=%d failed=%d meta_errors=%d", processed, skipped, unchanged, failed, metaError)
	return sum, nil
}

func isCanon(makeStr string) bool {
	return strings.Contains(strings.ToLower(makeStr), "canon")
}

func buildGroups(jobs []seriesJob) [][]seriesJob {
	if len(jobs) == 0 {
		return nil
	}
	var groups [][]seriesJob
	current := []seriesJob{jobs[0]}

	for i := 1; i < len(jobs); i++ {
		prev := current[len(current)-1]
		next := jobs[i]
		if sameSeries(prev, next) {
			current = append(current, next)
			continue
		}
		groups = append(groups, current)
		current = []seriesJob{next}
	}
	groups = append(groups, current)
	return groups
}

func sameSeries(prev, next seriesJob) bool {
	gap := next.Meta.CaptureTime.Sub(prev.Meta.CaptureTime)
	allowed := maxGapDefault

	if prev.Seq >= 0 && next.Seq >= 0 {
		diff := next.Seq - prev.Seq
		if diff != 1 {
			return false
		}
		allowed = maxGapSequential
	}

	return gap >= 0 && gap <= allowed
}

func resolveType(group []seriesJob, opts Options) Mode {
	if opts.Mode == ModeHDR || opts.Mode == ModeFocus {
		return opts.Mode
	}

	for _, job := range group {
		if job.Meta.FocusBr {
			return ModeFocus
		}
	}

	evValues := make([]float64, 0, len(group))
	for _, job := range group {
		if job.Meta.ExposureTime <= 0 || job.Meta.FNumber <= 0 {
			continue
		}
		evValues = append(evValues, ev(job.Meta))
	}

	if len(evValues) == 0 {
		return ModeHDR
	}
	sort.Float64s(evValues)
	rangeEv := evValues[len(evValues)-1] - evValues[0]
	if rangeEv >= evHDRThreshold {
		return ModeHDR
	}
	return ModeFocus
}

func ev(meta media.SeriesMetadata) float64 {
	if meta.ExposureTime <= 0 || meta.FNumber <= 0 {
		return 0
	}
	ev := math.Log2((meta.FNumber * meta.FNumber) / meta.ExposureTime)
	if meta.ISO > 0 {
		ev -= math.Log2(float64(meta.ISO) / 100.0)
	}
	return ev
}

func parseSequence(path string) int {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "" {
		return -1
	}
	i := len(name) - 1
	for ; i >= 0; i-- {
		if name[i] < '0' || name[i] > '9' {
			break
		}
	}
	if i == len(name)-1 {
		return -1
	}
	num := name[i+1:]
	val, err := strconv.Atoi(num)
	if err != nil {
		return -1
	}
	return val
}
