package media

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/exif2"
)

// Metadata represents a subset of photo metadata required for geotagging.
type Metadata struct {
	CaptureTime time.Time
	CameraMake  string
	CameraModel string
}

// SupportedRaw reports whether the provided path has a supported RAW extension.
func SupportedRaw(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return rawExt[ext]
}

// ReadMetadata extracts capture time and camera details from a RAW file.
func ReadMetadata(path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	exif, err := decodeExifSafe(file, path)
	if err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}

	ts := exif.DateTimeOriginal()
	if ts.IsZero() {
		ts = exif.CreateDate()
	}
	if ts.IsZero() {
		ts = exif.ModifyDate()
	}
	if ts.IsZero() {
		return Metadata{}, fmt.Errorf("capture time not found in metadata")
	}

	return Metadata{
		CaptureTime: ts,
		CameraMake:  strings.TrimSpace(exif.Make),
		CameraModel: strings.TrimSpace(exif.Model),
	}, nil
}

// decodeExifSafe protects against panics from the decoder on malformed files.
func decodeExifSafe(r io.ReadSeeker, path string) (ex exif2.Exif, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic while decoding %s: %v", path, rec)
		}
	}()

	ex, err = imagemeta.Decode(r)
	return ex, err
}

var rawExt = map[string]bool{
	".3fr": true, // Hasselblad
	".arw": true, // Sony
	".cr2": true, // Canon
	".cr3": true, // Canon
	".dng": true, // Adobe DNG
	".erf": true, // Epson
	".kdc": true, // Kodak
	".mrw": true, // Minolta
	".nef": true, // Nikon
	".nrw": true, // Nikon
	".orf": true, // Olympus
	".pef": true, // Pentax
	".raf": true, // Fujifilm
	".raw": true, // Panasonic/Leica generic
	".rw2": true, // Panasonic
	".rwl": true, // Leica
	".sr2": true, // Sony
	".srf": true, // Sony
	".srw": true, // Samsung
	".x3f": true, // Sigma
}
