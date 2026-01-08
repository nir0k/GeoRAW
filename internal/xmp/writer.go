package xmp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nir0k/GeoRAW/internal/gpx"
)

// ErrGPSAlreadyPresent is returned when GPS tags already exist and overwriting is disabled.
var ErrGPSAlreadyPresent = errors.New("gps already present in sidecar")

const exifNamespace = "http://ns.adobe.com/exif/1.0/"

// BuildSidecar returns XMP payload with GPS information.
func BuildSidecar(coord gpx.Coordinate, ts time.Time) []byte {
	latVal, latRef := formatGPSCoordinate(coord.Latitude, "N", "S")
	lonVal, lonRef := formatGPSCoordinate(coord.Longitude, "E", "W")

	var (
		altRef int
		altVal float64
		hasAlt bool
	)
	if coord.Altitude != nil {
		altVal = *coord.Altitude
		if altVal < 0 {
			altRef = 1
			altVal = math.Abs(altVal)
		}
		hasAlt = true
	}

	gpsDate := ts.UTC().Format("2006:01:02")
	gpsTime := ts.UTC().Format("15:04:05")

	var builder strings.Builder
	builder.WriteString(`<?xpacket begin=" " id="W5M0MpCehiHzreSzNTczkc9d"?>`)
	builder.WriteString("\n<x:xmpmeta xmlns:x=\"adobe:ns:meta/\" x:xmptk=\"GeoRAW\">\n")
	builder.WriteString("  <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n")
	builder.WriteString(fmt.Sprintf("    <rdf:Description rdf:about=\"\" xmlns:exif=\"%s\"", exifNamespace))
	builder.WriteString(fmt.Sprintf(" exif:GPSLatitude=\"%s\"", latVal))
	builder.WriteString(fmt.Sprintf(" exif:GPSLatitudeRef=\"%s\"", latRef))
	builder.WriteString(fmt.Sprintf(" exif:GPSLongitude=\"%s\"", lonVal))
	builder.WriteString(fmt.Sprintf(" exif:GPSLongitudeRef=\"%s\"", lonRef))
	if hasAlt {
		builder.WriteString(fmt.Sprintf(" exif:GPSAltitude=\"%0.2f\"", altVal))
		builder.WriteString(fmt.Sprintf(" exif:GPSAltitudeRef=\"%d\"", altRef))
	}
	builder.WriteString(" exif:GPSVersionID=\"2.3.0.0\"")
	builder.WriteString(fmt.Sprintf(" exif:GPSDateStamp=\"%s\"", gpsDate))
	builder.WriteString(fmt.Sprintf(" exif:GPSTimeStamp=\"%s\"", gpsTime))
	builder.WriteString(">\n")
	builder.WriteString("    </rdf:Description>\n")
	builder.WriteString("  </rdf:RDF>\n")
	builder.WriteString("</x:xmpmeta>\n")
	builder.WriteString("<?xpacket end=\"w\"?>")

	return []byte(builder.String())
}

func formatGPSCoordinate(value float64, positiveRef, negativeRef string) (string, string) {
	ref := positiveRef
	if value < 0 {
		ref = negativeRef
	}

	abs := math.Abs(value)
	deg := math.Floor(abs)
	minutes := (abs - deg) * 60

	minutes = math.Round(minutes*1e10) / 1e10
	if minutes >= 60 {
		deg++
		minutes = 0
	}

	degStr := strconv.FormatFloat(deg, 'f', 0, 64)
	minStr := strconv.FormatFloat(minutes, 'f', 10, 64)
	minStr = strings.TrimRight(minStr, "0")
	minStr = strings.TrimRight(minStr, ".")
	if minStr == "" {
		minStr = "0"
	}

	return fmt.Sprintf("%s,%s%s", degStr, minStr, ref), ref
}

// SidecarPath returns the expected XMP filename for a RAW file.
func SidecarPath(rawPath string) string {
	// If path already ends with .xmp (or .XMP), strip it first, then drop the previous extension.
	path := rawPath
	if strings.EqualFold(filepath.Ext(path), ".xmp") {
		path = strings.TrimSuffix(path, filepath.Ext(path))
	}

	ext := filepath.Ext(path)
	if ext == "" {
		return path + ".xmp"
	}
	return strings.TrimSuffix(path, ext) + ".xmp"
}

// MergeAndWrite updates or creates an XMP sidecar with GPS tags, preserving other tags.
// It returns true if data was written, false if skipped due to existing GPS when overwrite is false.
func MergeAndWrite(path string, coord gpx.Coordinate, ts time.Time, overwrite bool) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read existing sidecar: %w", err)
	}

	if !overwrite && len(existing) > 0 && hasGPSData(existing) {
		return false, ErrGPSAlreadyPresent
	}

	payload, err := mergeSidecar(existing, coord, ts)
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create sidecar dir: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func mergeSidecar(existing []byte, coord gpx.Coordinate, ts time.Time) ([]byte, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return BuildSidecar(coord, ts), nil
	}
	merged, err := mergeGPSInPlace(existing, coord, ts)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

var descriptionTagRegex = regexp.MustCompile(`(?is)<rdf:Description\b[^>]*>`)
var gpsAttrRegex = regexp.MustCompile(`(?is)\s+exif:GPS(?:Latitude|LatitudeRef|Longitude|LongitudeRef|Altitude|AltitudeRef|VersionID|DateStamp|TimeStamp)\s*=\s*("[^"]*"|'[^']*')`)
var exifNamespaceRegex = regexp.MustCompile(`(?is)\bxmlns:exif\s*=\s*("[^"]*"|'[^']*')`)

func mergeGPSInPlace(existing []byte, coord gpx.Coordinate, ts time.Time) ([]byte, error) {
	text := string(existing)
	loc := descriptionTagRegex.FindStringIndex(text)
	if loc == nil {
		return nil, fmt.Errorf("rdf:Description tag not found")
	}

	tag := text[loc[0]:loc[1]]
	updatedTag, err := updateDescriptionTag(tag, coord, ts)
	if err != nil {
		return nil, err
	}

	updated := text[:loc[0]] + updatedTag + text[loc[1]:]
	updated = stripGPSTagsFromXMP(updated)

	return []byte(updated), nil
}

func updateDescriptionTag(tag string, coord gpx.Coordinate, ts time.Time) (string, error) {
	latVal, latRef := formatGPSCoordinate(coord.Latitude, "N", "S")
	lonVal, lonRef := formatGPSCoordinate(coord.Longitude, "E", "W")

	var (
		altRef int
		altVal float64
		hasAlt bool
	)
	if coord.Altitude != nil {
		altVal = *coord.Altitude
		if altVal < 0 {
			altRef = 1
			altVal = math.Abs(altVal)
		}
		hasAlt = true
	}

	gpsDate := ts.UTC().Format("2006:01:02")
	gpsTime := ts.UTC().Format("15:04:05")

	clean := gpsAttrRegex.ReplaceAllString(tag, "")

	attrs := make([]string, 0, 10)
	if !exifNamespaceRegex.MatchString(clean) {
		attrs = append(attrs, fmt.Sprintf(`xmlns:exif="%s"`, exifNamespace))
	}
	attrs = append(attrs,
		fmt.Sprintf(`exif:GPSLatitude="%s"`, latVal),
		fmt.Sprintf(`exif:GPSLatitudeRef="%s"`, latRef),
		fmt.Sprintf(`exif:GPSLongitude="%s"`, lonVal),
		fmt.Sprintf(`exif:GPSLongitudeRef="%s"`, lonRef),
		`exif:GPSVersionID="2.3.0.0"`,
		fmt.Sprintf(`exif:GPSDateStamp="%s"`, gpsDate),
		fmt.Sprintf(`exif:GPSTimeStamp="%s"`, gpsTime),
	)
	if hasAlt {
		attrs = append(attrs,
			fmt.Sprintf(`exif:GPSAltitude="%0.2f"`, altVal),
			fmt.Sprintf(`exif:GPSAltitudeRef="%d"`, altRef),
		)
	}

	updated, err := insertTagAttributes(clean, attrs)
	if err != nil {
		return "", err
	}
	return updated, nil
}

func insertTagAttributes(tag string, attrs []string) (string, error) {
	if len(attrs) == 0 {
		return tag, nil
	}

	closeIdx := strings.LastIndex(tag, ">")
	if closeIdx == -1 {
		return "", fmt.Errorf("invalid rdf:Description tag")
	}

	prefix := tag[:closeIdx]
	suffix := tag[closeIdx:]
	if strings.HasSuffix(prefix, "/") {
		prefix = strings.TrimSuffix(prefix, "/")
		suffix = "/>"
	}

	if strings.Contains(prefix, "\n") {
		indent := guessAttributeIndent(prefix)
		var b strings.Builder
		b.WriteString(prefix)
		for _, attr := range attrs {
			b.WriteString("\n")
			b.WriteString(indent)
			b.WriteString(attr)
		}
		b.WriteString(suffix)
		return b.String(), nil
	}

	return prefix + " " + strings.Join(attrs, " ") + suffix, nil
}

func guessAttributeIndent(prefix string) string {
	lines := strings.Split(prefix, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		for i, r := range line {
			if r != ' ' && r != '\t' {
				return line[:i]
			}
		}
		return line
	}
	return "  "
}

func stripGPSTagsFromXMP(text string) string {
	for _, re := range gpsTagRegexes {
		text = re.ReplaceAllString(text, "")
	}
	return text
}

func hasGPSData(data []byte) bool {
	text := strings.ToLower(string(data))
	for _, tag := range []string{
		"<exif:gpslatitude",
		"exif:gpslatitude=",
		"<exif:gpslongitude",
		"exif:gpslongitude=",
		"<exif:gpsaltitude",
		"exif:gpsaltitude=",
		"<exif:gpstimestamp",
		"exif:gpstimestamp=",
		"<exif:gpsdatestamp",
		"exif:gpsdatestamp=",
	} {
		if strings.Contains(text, tag) {
			return true
		}
	}
	for _, re := range gpsTagRegexes {
		if re.Match(data) {
			return true
		}
	}
	return false
}

var gpsTagRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<exif:GPSLatitudeRef[^>]*>.*?</exif:GPSLatitudeRef>`),
	regexp.MustCompile(`(?is)<exif:GPSLatitude[^>]*>.*?</exif:GPSLatitude>`),
	regexp.MustCompile(`(?is)<exif:GPSLongitudeRef[^>]*>.*?</exif:GPSLongitudeRef>`),
	regexp.MustCompile(`(?is)<exif:GPSLongitude[^>]*>.*?</exif:GPSLongitude>`),
	regexp.MustCompile(`(?is)<exif:GPSAltitudeRef[^>]*>.*?</exif:GPSAltitudeRef>`),
	regexp.MustCompile(`(?is)<exif:GPSAltitude[^>]*>.*?</exif:GPSAltitude>`),
	regexp.MustCompile(`(?is)<exif:GPSVersionID[^>]*>.*?</exif:GPSVersionID>`),
	regexp.MustCompile(`(?is)<exif:GPSDateStamp[^>]*>.*?</exif:GPSDateStamp>`),
	regexp.MustCompile(`(?is)<exif:GPSTimeStamp[^>]*>.*?</exif:GPSTimeStamp>`),
}

type xmpPacket struct {
	XMLName xml.Name     `xml:"xmpmeta"`
	Attrs   []xml.Attr   `xml:",any,attr"`
	RDF     rdfContainer `xml:"RDF"`
}

type rdfContainer struct {
	XMLName      xml.Name         `xml:"RDF"`
	Attrs        []xml.Attr       `xml:",any,attr"`
	Descriptions []rdfDescription `xml:"Description"`
}

type rdfDescription struct {
	XMLName xml.Name   `xml:"Description"`
	Attrs   []xml.Attr `xml:",any,attr"`
	Inner   string     `xml:",innerxml"`
}

func parseXMP(data []byte) (xmpPacket, error) {
	var pkt xmpPacket
	if err := xml.Unmarshal(data, &pkt); err == nil && len(pkt.RDF.Descriptions) > 0 {
		return pkt, nil
	}

	// Try fallback when root is rdf:RDF without xmpmeta wrapper.
	var rdfOnly rdfContainer
	if err := xml.Unmarshal(data, &rdfOnly); err == nil && len(rdfOnly.Descriptions) > 0 {
		return xmpPacket{
			XMLName: xml.Name{Local: "xmpmeta"},
			RDF:     rdfOnly,
		}, nil
	}

	return xmpPacket{}, fmt.Errorf("unsupported XMP structure")
}

func marshalXMP(doc xmpPacket) ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteString(`<?xpacket begin=" " id="W5M0MpCehiHzreSzNTczkc9d"?>`)
	buf.WriteString("\n")

	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}

	buf.WriteString("\n<?xpacket end=\"w\"?>")
	return buf.Bytes(), nil
}

func selectDescription(descriptions []rdfDescription) int {
	if len(descriptions) == 0 {
		return -1
	}
	for i, d := range descriptions {
		for _, attr := range d.Attrs {
			if (attr.Name.Space == "xmlns" && attr.Name.Local == "exif") ||
				attr.Name.Local == "xmlns:exif" ||
				(attr.Name.Local == "exif" && strings.Contains(attr.Value, exifNamespace)) {
				return i
			}
		}
	}
	return 0
}

func ensureExifNamespace(attrs []xml.Attr) []xml.Attr {
	for _, attr := range attrs {
		if attr.Name.Local == "exif" && strings.Contains(attr.Value, exifNamespace) {
			return attrs
		}
		if attr.Name.Local == "xmlns:exif" || (attr.Name.Space == "xmlns" && attr.Name.Local == "exif") {
			return attrs
		}
	}
	return append(attrs, xml.Attr{
		Name:  xml.Name{Space: "xmlns", Local: "exif"},
		Value: exifNamespace,
	})
}
