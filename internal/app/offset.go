package app

import (
	"fmt"
	"sort"
	"time"

	"github.com/nir0k/GeoRAW/internal/gpx"
	"github.com/nir0k/GeoRAW/internal/media"
)

const (
	maxAutoOffset = 12 * time.Hour
)

type photoJob struct {
	Path string
	Meta media.Metadata
}

// detectOffset tries to find a consistent offset between camera time and GPX points.
func detectOffset(track *gpx.TrackIndex, photos []photoJob) (time.Duration, int, error) {
	var diffs []time.Duration

	for _, job := range photos {
		_, nearestTime, err := track.Nearest(job.Meta.CaptureTime)
		if err != nil {
			continue
		}
		diff := nearestTime.Sub(job.Meta.CaptureTime.UTC())
		if absDuration(diff) > maxAutoOffset {
			continue
		}
		diffs = append(diffs, diff)
	}

	if len(diffs) == 0 {
		return 0, 0, fmt.Errorf("unable to detect offset: no usable samples within %s window", maxAutoOffset)
	}

	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i] < diffs[j]
	})

	var median time.Duration
	mid := len(diffs) / 2
	if len(diffs)%2 == 0 {
		median = (diffs[mid-1] + diffs[mid]) / 2
	} else {
		median = diffs[mid]
	}

	return median, len(diffs), nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
