package gpx

import (
	"errors"
	"fmt"
	"sort"
	"time"

	gogpx "github.com/tkrajina/gpxgo/gpx"
)

// ErrTimestampOutOfBounds signals that the requested time is outside GPX coverage.
var ErrTimestampOutOfBounds = errors.New("timestamp outside GPX track bounds")

// Coordinate represents interpolated location data.
type Coordinate struct {
	Latitude  float64
	Longitude float64
	Altitude  *float64
}

// TrackIndex keeps GPX points sorted by timestamp for quick lookups.
type TrackIndex struct {
	points []trackPoint
}

type trackPoint struct {
	coord Coordinate
	time  time.Time
}

// LoadTrack parses a GPX file and prepares the lookup index.
func LoadTrack(path string) (*TrackIndex, error) {
	parsed, err := gogpx.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("parse gpx: %w", err)
	}

	collected := collectPoints(parsed)
	if len(collected) == 0 {
		return nil, fmt.Errorf("gpx file contains no track points")
	}

	sort.Slice(collected, func(i, j int) bool {
		return collected[i].time.Before(collected[j].time)
	})

	return &TrackIndex{points: collected}, nil
}

// CoordinateAt returns an interpolated coordinate for the provided timestamp.
func (ti *TrackIndex) CoordinateAt(ts time.Time) (Coordinate, error) {
	if len(ti.points) == 0 {
		return Coordinate{}, fmt.Errorf("no track points loaded")
	}
	target := ts.UTC()

	if target.Before(ti.points[0].time) || target.After(ti.points[len(ti.points)-1].time) {
		return Coordinate{}, fmt.Errorf("%w: %s", ErrTimestampOutOfBounds, target.Format(time.RFC3339))
	}

	idx := sort.Search(len(ti.points), func(i int) bool {
		return !ti.points[i].time.Before(target)
	})

	if idx == len(ti.points) {
		return ti.points[len(ti.points)-1].coord, nil
	}
	if ti.points[idx].time.Equal(target) {
		return ti.points[idx].coord, nil
	}
	if idx == 0 {
		return ti.points[0].coord, nil
	}

	prev := ti.points[idx-1]
	next := ti.points[idx]

	total := next.time.Sub(prev.time).Seconds()
	if total <= 0 {
		return prev.coord, nil
	}

	progress := target.Sub(prev.time).Seconds() / total
	lat := prev.coord.Latitude + progress*(next.coord.Latitude-prev.coord.Latitude)
	lon := prev.coord.Longitude + progress*(next.coord.Longitude-prev.coord.Longitude)

	var alt *float64
	if prev.coord.Altitude != nil && next.coord.Altitude != nil {
		v := *prev.coord.Altitude + progress*(*next.coord.Altitude-*prev.coord.Altitude)
		alt = &v
	} else if prev.coord.Altitude != nil {
		altVal := *prev.coord.Altitude
		alt = &altVal
	} else if next.coord.Altitude != nil {
		altVal := *next.coord.Altitude
		alt = &altVal
	}

	return Coordinate{
		Latitude:  lat,
		Longitude: lon,
		Altitude:  alt,
	}, nil
}

// Nearest returns the nearest track point and its timestamp for a given time.
func (ti *TrackIndex) Nearest(ts time.Time) (Coordinate, time.Time, error) {
	if len(ti.points) == 0 {
		return Coordinate{}, time.Time{}, fmt.Errorf("no track points loaded")
	}
	target := ts.UTC()

	if target.Before(ti.points[0].time) {
		return ti.points[0].coord, ti.points[0].time, nil
	}
	if target.After(ti.points[len(ti.points)-1].time) {
		last := ti.points[len(ti.points)-1]
		return last.coord, last.time, nil
	}

	idx := sort.Search(len(ti.points), func(i int) bool {
		return !ti.points[i].time.Before(target)
	})
	if idx == len(ti.points) {
		last := ti.points[len(ti.points)-1]
		return last.coord, last.time, nil
	}
	if ti.points[idx].time.Equal(target) || idx == 0 {
		return ti.points[idx].coord, ti.points[idx].time, nil
	}

	prev := ti.points[idx-1]
	next := ti.points[idx]
	if target.Sub(prev.time) <= next.time.Sub(target) {
		return prev.coord, prev.time, nil
	}
	return next.coord, next.time, nil
}

// Bounds returns the first and last timestamps in the track.
func (ti *TrackIndex) Bounds() (time.Time, time.Time) {
	if len(ti.points) == 0 {
		return time.Time{}, time.Time{}
	}
	return ti.points[0].time, ti.points[len(ti.points)-1].time
}

// PointCount returns number of GPX points indexed.
func (ti *TrackIndex) PointCount() int {
	return len(ti.points)
}

func collectPoints(doc *gogpx.GPX) []trackPoint {
	points := make([]trackPoint, 0)

	for _, track := range doc.Tracks {
		for _, segment := range track.Segments {
			for _, pt := range segment.Points {
				coord := Coordinate{
					Latitude:  pt.GetLatitude(),
					Longitude: pt.GetLongitude(),
				}
				if ele := pt.GetElevation(); ele.NotNull() {
					val := ele.Value()
					coord.Altitude = &val
				}
				points = append(points, trackPoint{
					coord: coord,
					time:  pt.Timestamp.UTC(),
				})
			}
		}
	}

	return points
}
