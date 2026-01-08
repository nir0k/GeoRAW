package media

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evanoberholster/imagemeta/exif2"
	"github.com/nir0k/GeoRAW/internal/xmp"
)

// ExifField is a single label/value pair for EXIF display.
type ExifField struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Group string `json:"group,omitempty"`
}

// ExifDetails holds flattened EXIF data for UI consumption.
type ExifDetails struct {
	Path   string      `json:"path"`
	Fields []ExifField `json:"fields"`
}

var exifExt = func() map[string]bool {
	exts := make(map[string]bool, len(rawExt)+10)
	for ext := range rawExt {
		exts[ext] = true
	}
	for _, ext := range []string{
		".jpg", ".jpeg", ".jpe",
		".tif", ".tiff",
		".heic", ".heif", ".hif",
		".avif",
	} {
		exts[ext] = true
	}
	return exts
}()

// SupportedExif reports whether the provided path likely contains EXIF data.
func SupportedExif(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return exifExt[ext]
}

// ReadExifDetails reads EXIF tags and formats a user-friendly subset.
func ReadExifDetails(path string, includeXmp bool) (*ExifDetails, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}
	if !SupportedExif(path) {
		return nil, fmt.Errorf("file type is not supported for EXIF viewing")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	exif, err := decodeExifSafe(file, path)
	if err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}

	out := &ExifDetails{Path: path}

	add := func(group, label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		out.Fields = append(out.Fields, ExifField{
			Label: label,
			Value: value,
			Group: group,
		})
	}

	add("File", "File name", filepath.Base(path))
	add("File", "Directory", filepath.Dir(path))
	add("File", "Size", humanSize(info.Size()))
	add("File", "Modified", info.ModTime().Local().Format(time.RFC3339))

	capture := exif.DateTimeOriginal()
	createDate := exif.CreateDate()
	modifyDate := exif.ModifyDate()

	if !capture.IsZero() {
		add("Capture", "Captured", formatTS(capture))
	}
	if !createDate.IsZero() && !createDate.Equal(capture) {
		add("Capture", "Digitized", formatTS(createDate))
	}
	if !modifyDate.IsZero() && !modifyDate.Equal(capture) {
		add("Capture", "Modified (EXIF)", formatTS(modifyDate))
	}

	add("Camera", "Make", exif.Make)
	add("Camera", "Model", exif.Model)
	add("Camera", "Serial", exif.CameraSerial)
	add("Camera", "Owner", exif.OwnerName)
	add("Camera", "Artist", exif.Artist)
	add("Camera", "Copyright", exif.Copyright)
	add("Camera", "Software", exif.Software)
	add("Camera", "Processing software", exif.ProcessingSoftware)
	add("Camera", "Document", exif.DocumentName)
	add("Camera", "Image ID", exif.ImageUniqueID)
	if exif.ImageNumber != 0 {
		add("Camera", "Image number", fmt.Sprintf("%d", exif.ImageNumber))
	}
	if exif.Rating != 0 {
		add("Camera", "Rating", fmt.Sprintf("%d", exif.Rating))
	}
	if exif.ImageType.String() != "" {
		add("Camera", "Image type", exif.ImageType.String())
	}

	add("Lens", "Lens", exif.LensModel)
	add("Lens", "Lens make", exif.LensMake)
	add("Lens", "Lens serial", exif.LensSerial)
	if lens := lensInfoLabel(exif.LensInfo); lens != "" {
		add("Lens", "Lens info", lens)
	}

	if includeXmp {
		if kw := readKeywords(path); len(kw) > 0 {
			add("Keywords", "Keywords xmp", strings.Join(kw, ", "))
		}
	}

	if exif.ExposureTime != 0 {
		add("Exposure", "Shutter", exif.ExposureTime.String()+" s")
	}
	if exif.FNumber != 0 {
		add("Exposure", "Aperture", fmt.Sprintf("f/%.1f", exif.FNumber))
	}
	if exif.ISO != 0 {
		add("Exposure", "ISO", fmt.Sprintf("%d", exif.ISO))
	} else if exif.ISOSpeed != 0 {
		add("Exposure", "ISO", fmt.Sprintf("%d", exif.ISOSpeed))
	}
	if exif.ExposureBias != 0 {
		add("Exposure", "Exposure bias", exif.ExposureBias.String())
	}
	if exif.ExposureProgram != 0 {
		add("Exposure", "Program", exif.ExposureProgram.String())
	}
	if exif.ExposureMode != 0 {
		add("Exposure", "Mode", exif.ExposureMode.String())
	}
	if exif.MeteringMode != 0 {
		add("Exposure", "Metering", exif.MeteringMode.String())
	}
	if exif.Flash != 0 {
		add("Exposure", "Flash", exif.Flash.String())
	}

	if exif.FocalLength != 0 {
		add("Exposure", "Focal length", exif.FocalLength.String())
	}
	if exif.FocalLengthIn35mmFormat != 0 {
		add("Exposure", "35mm equivalent", exif.FocalLengthIn35mmFormat.String())
	}
	if exif.SubjectDistance > 0 {
		add("Exposure", "Subject distance", fmt.Sprintf("%.2fm", exif.SubjectDistance))
	}

	if exif.ImageWidth != 0 && exif.ImageHeight != 0 {
		add("Image", "Resolution", fmt.Sprintf("%dx%d", exif.ImageWidth, exif.ImageHeight))
	}
	if exif.XResolution != 0 || exif.YResolution != 0 {
		add("Image", "DPI", resolutionLabel(exif.XResolution, exif.YResolution, exif.ResolutionUnit))
	}
	if exif.Orientation != 0 {
		add("Image", "Orientation", exif.Orientation.String())
	}
	if exif.ColorSpace != 0 {
		add("Image", "Color space", colorSpaceLabel(exif.ColorSpace))
	}

	lat := exif.GPS.Latitude()
	lon := exif.GPS.Longitude()
	alt := exif.GPS.Altitude()
	if lat != 0 || lon != 0 {
		add("GPS", "Latitude", fmt.Sprintf("%.6f", lat))
		add("GPS", "Longitude", fmt.Sprintf("%.6f", lon))
		if alt != 0 {
			add("GPS", "Altitude", fmt.Sprintf("%.2fm", alt))
		}
		gpsDate := exif.GPS.Date()
		if !gpsDate.IsZero() {
			add("GPS", "GPS time", formatTS(gpsDate))
		}
	}

	toolFields, err := readExifToolFields(path, includeXmp)
	if err != nil {
		return nil, err
	}
	out.Fields = append(out.Fields, toolFields...)

	return out, nil
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05 -0700")
}

func colorSpaceLabel(cs exif2.ColorSpace) string {
	switch uint16(cs) {
	case 1:
		return "sRGB (1)"
	case 2:
		return "Adobe RGB (2)"
	case 0xFFFF:
		return "Uncalibrated (65535)"
	default:
		return fmt.Sprintf("%d", cs)
	}
}

func lensInfoLabel(info exif2.LensInfo) string {
	if info == (exif2.LensInfo{}) {
		return ""
	}
	get := func(i int) float64 {
		num, den := info[i], info[i+1]
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den)
	}
	minF, maxF := get(0), get(2)
	minA, maxA := get(4), get(6)

	focal := ""
	switch {
	case minF == 0 && maxF == 0:
	case maxF == 0 || minF == maxF:
		focal = fmt.Sprintf("%.1fmm", minF)
	default:
		focal = fmt.Sprintf("%.1f-%.1fmm", minF, maxF)
	}

	aperture := ""
	switch {
	case minA == 0 && maxA == 0:
	case maxA == 0 || minA == maxA:
		aperture = fmt.Sprintf("f/%.1f", minA)
	default:
		aperture = fmt.Sprintf("f/%.1f-%.1f", minA, maxA)
	}

	if focal != "" && aperture != "" {
		return fmt.Sprintf("%s %s", focal, aperture)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", focal, aperture))
}

func resolutionLabel(x, y uint32, unit uint16) string {
	unitLabel := ""
	switch unit {
	case 2:
		unitLabel = "dpi"
	case 3:
		unitLabel = "dpcm"
	default:
		unitLabel = "units"
	}

	if x == 0 && y == 0 {
		return ""
	}
	if x == y || y == 0 {
		return fmt.Sprintf("%d %s", x, unitLabel)
	}
	return fmt.Sprintf("%d x %d %s", x, y, unitLabel)
}

func readKeywords(rawPath string) []string {
	sidecar := xmp.SidecarPath(rawPath)
	data, err := os.ReadFile(sidecar)
	if err != nil || len(data) == 0 {
		return nil
	}
	return extractKeywords(data)
}

var (
	subjectRe = regexp.MustCompile(`(?is)<dc:subject[^>]*>.*?</dc:subject>`)
	liRe      = regexp.MustCompile(`(?is)<rdf:li[^>]*>(.*?)</rdf:li>`)
)

func extractKeywords(data []byte) []string {
	var out []string
	for _, block := range subjectRe.FindAllString(string(data), -1) {
		matches := liRe.FindAllStringSubmatch(block, -1)
		for _, m := range matches {
			val := strings.TrimSpace(htmlUnescape(m[1]))
			if val != "" {
				out = append(out, val)
			}
		}
	}
	return out
}

func htmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
	)
	return replacer.Replace(s)
}

func readExifToolFields(path string, includeXmp bool) ([]ExifField, error) {
	exe, err := exec.LookPath("exiftool")
	if err != nil {
		return nil, fmt.Errorf("exiftool not found in PATH; install it and retry")
	}

	args := []string{"-json", "-G", "-n", "-sort"}
	if !includeXmp {
		args = append(args, "-api", "IgnoreSidecar=1")
	}
	args = append(args, path)

	cmd := exec.Command(exe, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exiftool error: %w", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("exiftool parse error: %w", err)
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("exiftool returned no data")
	}

	fields := flattenExifTool(parsed[0], includeXmp)
	return fields, nil
}

func flattenExifTool(m map[string]any, includeXmp bool) []ExifField {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "SourceFile" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type entry struct {
		group string
		label string
		value string
		isXmp bool
	}
	var entries []entry
	for _, k := range keys {
		val := formatExifToolValue(m[k])
		if val == "" {
			continue
		}
		group, label := splitExifToolKey(k)
		if label == "ExifToolVersion" {
			continue
		}
		isXmp := strings.HasPrefix(group, "XMP")
		if isXmp && !includeXmp {
			continue
		}
		entries = append(entries, entry{
			group: group,
			label: label,
			value: val,
			isXmp: isXmp,
		})
	}

	out := make([]ExifField, 0, len(entries))
	seen := make(map[string]int)
	for _, e := range entries {
		base := e.label
		if idx, ok := seen[base]; ok {
			if e.isXmp {
				out[idx] = ExifField{
					Label: fmt.Sprintf("%s xmp", e.label),
					Value: e.value,
					Group: e.group,
				}
			}
			continue
		}
		lbl := e.label
		if e.isXmp {
			lbl = fmt.Sprintf("%s xmp", e.label)
		}
		seen[base] = len(out)
		out = append(out, ExifField{
			Label: lbl,
			Value: e.value,
			Group: e.group,
		})
	}
	return out
}

func splitExifToolKey(k string) (string, string) {
	if idx := strings.Index(k, ":"); idx > 0 {
		return k[:idx], k[idx+1:]
	}
	return "EXIFTool", k
}

func formatExifToolValue(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case []any:
		var parts []string
		for _, item := range t {
			s := formatExifToolValue(item)
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		if b, err := json.Marshal(t); err == nil {
			return string(b)
		}
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}
