package media

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/exif2"
	"github.com/evanoberholster/imagemeta/exif2/ifds"
	"github.com/evanoberholster/imagemeta/exif2/ifds/exififd"
	"github.com/evanoberholster/imagemeta/exif2/ifds/mknote/canon"
	"github.com/evanoberholster/imagemeta/exif2/tag"
	"github.com/evanoberholster/imagemeta/imagetype"
	"github.com/evanoberholster/imagemeta/isobmff"
	"github.com/evanoberholster/imagemeta/jpeg"
	"github.com/evanoberholster/imagemeta/tiff"
)

// Metadata represents a subset of photo metadata required for geotagging.
type Metadata struct {
	CaptureTime time.Time
	CameraMake  string
	CameraModel string
}

// SeriesMetadata represents richer metadata needed for series detection/tagging.
type SeriesMetadata struct {
	CaptureTime  time.Time
	CameraMake   string
	CameraModel  string
	ExposureTime float64 // seconds
	FNumber      float64 // aperture value (f/x)
	ISO          uint32
	HDRHint      bool // true when maker note indicates HDR=On (for JPEG/HIF merged output)
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

// ReadSeriesMetadata extracts detailed fields for series detection (Canon only).
// It uses a custom EXIF parser to capture maker note flags and exposure data.
func ReadSeriesMetadata(path string) (SeriesMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return SeriesMetadata{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	meta, err := decodeSeriesExifSafe(file, path)
	if err != nil {
		return SeriesMetadata{}, fmt.Errorf("decode metadata: %w", err)
	}

	ts := meta.captureTime
	if ts.IsZero() {
		ts = meta.createDate
	}
	if ts.IsZero() {
		ts = meta.modifyDate
	}
	if ts.IsZero() {
		return SeriesMetadata{}, fmt.Errorf("capture time not found in metadata")
	}

	return SeriesMetadata{
		CaptureTime:  ts,
		CameraMake:   strings.TrimSpace(meta.cameraMake),
		CameraModel:  strings.TrimSpace(meta.cameraModel),
		ExposureTime: meta.exposureTime,
		FNumber:      meta.fNumber,
		ISO:          meta.iso,
		HDRHint:      meta.hdr,
	}, nil
}

type seriesExif struct {
	cameraMake   string
	cameraModel  string
	captureTime  time.Time
	createDate   time.Time
	modifyDate   time.Time
	subsec       uint16
	exposureTime float64
	fNumber      float64
	iso          uint32
	hdr          bool
}

func decodeSeriesExifSafe(r io.ReadSeeker, path string) (se seriesExif, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic while decoding %s: %v", path, rec)
		}
	}()
	se, err = decodeSeriesExif(r)
	return se, err
}

func decodeSeriesExif(r io.ReadSeeker) (seriesExif, error) {
	reader := bufio.NewReaderSize(nil, 4*1024)
	reader.Reset(r)

	ir := exif2.NewIfdReader(exif2.Logger)
	defer ir.Close()

	state := seriesExif{}
	ir.SetCustomTagParser(makeSeriesTagParser(&state))

	imgType, err := imagetype.ScanBuf(reader)
	if err != nil {
		return seriesExif{}, err
	}

	switch imgType {
	case imagetype.ImageJPEG:
		if err := jpeg.ScanJPEG(reader, ir.DecodeJPEGIfd, nil); err != nil {
			return seriesExif{}, err
		}
	case imagetype.ImageCR2, imagetype.ImageTiff, imagetype.ImagePanaRAW, imagetype.ImageDNG:
		header, err := tiff.ScanTiffHeader(reader, imgType)
		if err != nil {
			return seriesExif{}, err
		}
		if err := ir.DecodeTiff(reader, header); err != nil {
			return seriesExif{}, err
		}
	case imagetype.ImageCR3, imagetype.ImageAVIF:
		boxReader := isobmff.NewReader(reader)
		defer boxReader.Close()
		boxReader.ExifReader = ir.DecodeIfd
		if err := boxReader.ReadFTYP(); err != nil {
			return seriesExif{}, err
		}
		if err := boxReader.ReadMetadata(); err != nil {
			return seriesExif{}, err
		}
	case imagetype.ImageHEIF:
		header, err := tiff.ScanTiffHeader(reader, imgType)
		if err != nil {
			return seriesExif{}, err
		}
		if err := ir.DecodeTiff(reader, header); err != nil {
			return seriesExif{}, err
		}
	default:
		return seriesExif{}, fmt.Errorf("metadata reading not supported for this format")
	}

	if state.subsec > 0 {
		ms := time.Duration(state.subsec) * time.Millisecond
		if !state.captureTime.IsZero() {
			state.captureTime = state.captureTime.Add(ms)
		}
		if !state.createDate.IsZero() {
			state.createDate = state.createDate.Add(ms)
		}
		if !state.modifyDate.IsZero() {
			state.modifyDate = state.modifyDate.Add(ms)
		}
	}
	return state, nil
}

func makeSeriesTagParser(dst *seriesExif) exif2.TagParserFn {
	return func(p exif2.TagParser, t exif2.Tag) error {
		switch ifds.IfdType(t.Ifd) {
		case ifds.IFD0:
			switch t.ID {
			case ifds.Make:
				_, makeStr := p.ParseCameraMake(t)
				dst.cameraMake = strings.TrimSpace(makeStr)
			case ifds.Model:
				_, modelStr := p.ParseCameraModel(t)
				dst.cameraModel = strings.TrimSpace(modelStr)
			case ifds.DateTime:
				if dst.modifyDate.IsZero() {
					dst.modifyDate = p.ParseDate(t)
				}
			}
		case ifds.ExifIFD:
			switch t.ID {
			case ifds.DateTimeOriginal:
				dst.captureTime = p.ParseDate(t)
			case ifds.DateTimeDigitized:
				if dst.createDate.IsZero() {
					dst.createDate = p.ParseDate(t)
				}
			case exififd.SubSecTimeOriginal:
				if dst.subsec == 0 {
					dst.subsec = p.ParseSubSecTime(t)
				}
			case exififd.ExposureTime:
				val := p.ParseRationalU(t)
				if val[1] != 0 {
					dst.exposureTime = float64(val[0]) / float64(val[1])
				}
			case exififd.FNumber:
				val := p.ParseRationalU(t)
				if val[1] != 0 {
					dst.fNumber = float64(val[0]) / float64(val[1])
				}
			case exififd.ISOSpeedRatings:
				dst.iso = p.ParseUint32(t)
			}
		case ifds.MknoteIFD, ifds.MkNoteCanonIFD:
			if t.ID == tag.ID(canon.CanonHDRInfo) {
				val16 := p.ParseUint16(t)
				val32 := p.ParseUint32(t)
				if val16 != 0 || val32 != 0 {
					dst.hdr = true
				}
			}
		default:
			// Some Canon HDR flags in JPEG live in a dedicated CanonHdr IFD (id 0x0001).
			if t.ID == tag.ID(0x0001) {
				if p.ParseUint32(t) != 0 {
					dst.hdr = true
				}
			}
		}
		return nil
	}
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
