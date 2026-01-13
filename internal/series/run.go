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
	Path      string
	Meta      media.SeriesMetadata
	Seq       int
	ForceType *Mode
}

type hdrHint struct {
	Path  string
	Meta  media.SeriesMetadata
	Seq   int
	IsRaw bool
}

type seriesGroup struct {
	Jobs       []seriesJob
	ForcedType *Mode
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

	extraTags := parseExtraTags(opts.ExtraTags)
	infof("Starting series tagging with input=%s recursive=%t mode=%s overwrite=%t prefix=%s start=%d extraTags=%q",
		opts.InputPath, opts.Recursive, opts.Mode, opts.Overwrite, opts.Prefix, opts.StartIndex, strings.Join(extraTags, ","))

	files, err := media.CollectFiles(opts.InputPath, opts.Recursive)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found to process")
	}

	total := 0
	for _, path := range files {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".xmp" {
			continue
		}
		if isHDRMergedCandidate(ext) {
			continue
		}
		total++
	}
	completed := 0
	reportProgress := func(done int) {
		if opts.Progress == nil || total == 0 {
			return
		}
		opts.Progress(done, total)
	}
	reportProgress(0)

	var (
		results   []app.FileResult
		processed int
		skipped   int
		unchanged int
		failed    int
		metaError int
		hints     []hdrHint
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
		if isHDRMergedCandidate(ext) {
			meta, err := media.ReadSeriesMetadata(path)
			if err != nil {
				warnf("Failed to read metadata for %s: %v", path, err)
				continue
			}
			if !isCanon(meta.CameraMake) {
				continue
			}
			hints = append(hints, hdrHint{
				Path:  path,
				Meta:  meta,
				Seq:   parseSequence(path),
				IsRaw: false,
			})
			continue
		}
		if !media.SupportedRaw(path) {
			warnf("Skipping non-RAW file: %s", path)
			skipped++
			results = append(results, app.FileResult{Path: path, Status: "skipped", Message: "Not a RAW file"})
			completed++
			reportProgress(completed)
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
			completed++
			reportProgress(completed)
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
			completed++
			reportProgress(completed)
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

	hdrGroups, assigned := detectHDRGroups(hints, jobs, warnf)

	autoJobs := make([]seriesJob, 0, len(jobs))
	for _, job := range jobs {
		if _, ok := assigned[job.Path]; ok {
			continue
		}
		autoJobs = append(autoJobs, job)
	}

	groups := append(hdrGroups, buildGroups(autoJobs)...)
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
		if len(group.Jobs) < minSeriesLen {
			for _, job := range group.Jobs {
				skipped++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "skipped",
					Message: "Series too short",
				})
				completed++
				reportProgress(completed)
			}
			continue
		}

		typeTag := seriesTypeTag
		if group.ForcedType == nil && !shouldTagHDR(group.Jobs, opts) {
			for _, job := range group.Jobs {
				skipped++
				results = append(results, app.FileResult{
					Path:    job.Path,
					Status:  "skipped",
					Message: "Not detected as HDR",
				})
				completed++
				reportProgress(completed)
			}
			continue
		}
		seriesID := fmt.Sprintf("%s_%05d", opts.Prefix, seriesIdx)
		seriesIdx++

		for _, job := range group.Jobs {
			tags := make([]string, 0, 2+len(extraTags))
			tags = append(tags, typeTag, seriesID)
			tags = append(tags, extraTags...)
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
				completed++
				reportProgress(completed)
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
				completed++
				reportProgress(completed)
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
			completed++
			reportProgress(completed)
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

func parseExtraTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
}

func buildGroups(jobs []seriesJob) []seriesGroup {
	if len(jobs) == 0 {
		return nil
	}
	var groups []seriesGroup
	current := []seriesJob{jobs[0]}

	for i := 1; i < len(jobs); i++ {
		prev := current[len(current)-1]
		next := jobs[i]
		if sameSeries(prev, next) {
			current = append(current, next)
			continue
		}
		groups = append(groups, seriesGroup{Jobs: current})
		current = []seriesJob{next}
	}
	groups = append(groups, seriesGroup{Jobs: current})
	return groups
}

func detectHDRGroups(hints []hdrHint, jobs []seriesJob, warnf func(string, ...interface{})) ([]seriesGroup, map[string]struct{}) {
	if len(hints) == 0 || len(jobs) == 0 {
		return nil, nil
	}
	sort.Slice(hints, func(i, j int) bool {
		return hints[i].Seq < hints[j].Seq
	})

	jobBySeq := make(map[int]seriesJob, len(jobs))
	for _, job := range jobs {
		jobBySeq[job.Seq] = job
	}

	assigned := make(map[string]struct{})
	var groups []seriesGroup

	for _, hint := range hints {
		if hint.Seq <= 0 {
			continue
		}
		if hint.IsRaw {
			continue
		}

		seqs := []int{hint.Seq - 3, hint.Seq - 2, hint.Seq - 1}
		j1, ok1 := jobBySeq[seqs[0]]
		j2, ok2 := jobBySeq[seqs[1]]
		j3, ok3 := jobBySeq[seqs[2]]
		if !(ok1 && ok2 && ok3) {
			continue
		}
		if j1.Seq+1 != j2.Seq || j2.Seq+1 != j3.Seq {
			continue
		}
		if _, used := assigned[j1.Path]; used {
			continue
		}
		if _, used := assigned[j2.Path]; used {
			continue
		}
		if _, used := assigned[j3.Path]; used {
			continue
		}

		if !validateHDRTiming(j1, j2, j3, hint) {
			continue
		}

		for _, p := range []string{j1.Path, j2.Path, j3.Path} {
			assigned[p] = struct{}{}
		}
		forced := ModeHDR
		groups = append(groups, seriesGroup{
			Jobs:       []seriesJob{j1, j2, j3},
			ForcedType: &forced,
		})
	}

	return groups, assigned
}

func validateHDRTiming(j1, j2, j3 seriesJob, hint hdrHint) bool {
	t1 := j1.Meta.CaptureTime
	t2 := j2.Meta.CaptureTime
	t3 := j3.Meta.CaptureTime
	th := hint.Meta.CaptureTime

	// HIF/JPEG time must match first frame within small tolerance.
	if !withinTolerance(t1, th, 200*time.Millisecond) {
		return false
	}

	// t2 ~= t1 + shutter1
	if !withinTolerance(t1.Add(durationFromExposure(j1.Meta.ExposureTime)), t2, sumTolerance(j1.Meta.ExposureTime)) {
		return false
	}
	// t3 ~= t2 + shutter2
	if !withinTolerance(t2.Add(durationFromExposure(j2.Meta.ExposureTime)), t3, sumTolerance(j2.Meta.ExposureTime)) {
		return false
	}

	return true
}

func validateHDRTimingRawAnchor(j1, j2, j3 seriesJob) bool {
	t1 := j1.Meta.CaptureTime
	t2 := j2.Meta.CaptureTime
	t3 := j3.Meta.CaptureTime

	if !withinTolerance(t1.Add(durationFromExposure(j1.Meta.ExposureTime)), t2, sumTolerance(j1.Meta.ExposureTime)) {
		return false
	}
	if !withinTolerance(t2.Add(durationFromExposure(j2.Meta.ExposureTime)), t3, sumTolerance(j2.Meta.ExposureTime)) {
		return false
	}
	return true
}

func increasingTime(a, b time.Time) bool {
	return !b.Before(a)
}

func withinTolerance(a, b time.Time, tol time.Duration) bool {
	gap := b.Sub(a)
	if gap < 0 {
		gap = -gap
	}
	return gap <= tol
}

func sumTolerance(exp float64) time.Duration {
	base := durationFromExposure(exp)
	// 50ms + 1% выдержки
	return 50*time.Millisecond + time.Duration(0.01*float64(base))
}

func durationFromExposure(exp float64) time.Duration {
	if exp <= 0 {
		return 0
	}
	return time.Duration(exp * float64(time.Second))
}

func isHDRMergedCandidate(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".hif":
		return true
	default:
		return false
	}
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

func shouldTagHDR(group []seriesJob, opts Options) bool {
	if len(group) == 0 {
		return false
	}
	if opts.Mode == ModeHDR {
		return true
	}

	for _, job := range group {
		if job.Meta.HDRHint {
			return true
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
		return false
	}
	sort.Float64s(evValues)
	rangeEv := evValues[len(evValues)-1] - evValues[0]
	return rangeEv >= evHDRThreshold
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
