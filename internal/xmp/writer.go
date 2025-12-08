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
	"strings"
	"time"

	"github.com/nir0k/GeoRAW/internal/gpx"
)

// ErrGPSAlreadyPresent is returned when GPS tags already exist and overwriting is disabled.
var ErrGPSAlreadyPresent = errors.New("gps already present in sidecar")

// BuildSidecar returns XMP payload with GPS information.
func BuildSidecar(coord gpx.Coordinate, ts time.Time) []byte {
	latRef := "N"
	if coord.Latitude < 0 {
		latRef = "S"
	}

	lonRef := "E"
	if coord.Longitude < 0 {
		lonRef = "W"
	}

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
	builder.WriteString("    <rdf:Description rdf:about=\"\" xmlns:exif=\"http://ns.adobe.com/exif/1.0/\">\n")
	builder.WriteString(fmt.Sprintf("      <exif:GPSLatitude>%.8f</exif:GPSLatitude>\n", coord.Latitude))
	builder.WriteString(fmt.Sprintf("      <exif:GPSLatitudeRef>%s</exif:GPSLatitudeRef>\n", latRef))
	builder.WriteString(fmt.Sprintf("      <exif:GPSLongitude>%.8f</exif:GPSLongitude>\n", coord.Longitude))
	builder.WriteString(fmt.Sprintf("      <exif:GPSLongitudeRef>%s</exif:GPSLongitudeRef>\n", lonRef))
	if hasAlt {
		builder.WriteString(fmt.Sprintf("      <exif:GPSAltitude>%0.2f</exif:GPSAltitude>\n", altVal))
		builder.WriteString(fmt.Sprintf("      <exif:GPSAltitudeRef>%d</exif:GPSAltitudeRef>\n", altRef))
	}
	builder.WriteString("      <exif:GPSVersionID>2.3.0.0</exif:GPSVersionID>\n")
	builder.WriteString(fmt.Sprintf("      <exif:GPSDateStamp>%s</exif:GPSDateStamp>\n", gpsDate))
	builder.WriteString(fmt.Sprintf("      <exif:GPSTimeStamp>%s</exif:GPSTimeStamp>\n", gpsTime))
	builder.WriteString("    </rdf:Description>\n")
	builder.WriteString("  </rdf:RDF>\n")
	builder.WriteString("</x:xmpmeta>\n")
	builder.WriteString("<?xpacket end=\"w\"?>")

	return []byte(builder.String())
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

	doc, err := parseXMP(existing)
	if err != nil {
		return nil, fmt.Errorf("parse existing xmp: %w", err)
	}

	descIdx := selectDescription(doc.RDF.Descriptions)
	if descIdx == -1 {
		descIdx = 0
		doc.RDF.Descriptions = append(doc.RDF.Descriptions, rdfDescription{})
	}

	desc := doc.RDF.Descriptions[descIdx]
	desc.Attrs = ensureExifNamespace(desc.Attrs)
	desc.Inner = mergeGPSInner(desc.Inner, coord, ts)
	doc.RDF.Descriptions[descIdx] = desc

	out, err := marshalXMP(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal merged xmp: %w", err)
	}
	return out, nil
}

func mergeGPSInner(inner string, coord gpx.Coordinate, ts time.Time) string {
	inner = stripGPSTags(inner)
	gpsBlock := buildGPSBlock(coord, ts)

	inner = strings.TrimSpace(inner)
	if inner == "" {
		return gpsBlock
	}
	return inner + "\n" + gpsBlock
}

func buildGPSBlock(coord gpx.Coordinate, ts time.Time) string {
	latRef := "N"
	if coord.Latitude < 0 {
		latRef = "S"
	}
	lonRef := "E"
	if coord.Longitude < 0 {
		lonRef = "W"
	}

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

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<exif:GPSLatitude>%.8f</exif:GPSLatitude>\n", coord.Latitude))
	b.WriteString(fmt.Sprintf("<exif:GPSLatitudeRef>%s</exif:GPSLatitudeRef>\n", latRef))
	b.WriteString(fmt.Sprintf("<exif:GPSLongitude>%.8f</exif:GPSLongitude>\n", coord.Longitude))
	b.WriteString(fmt.Sprintf("<exif:GPSLongitudeRef>%s</exif:GPSLongitudeRef>\n", lonRef))
	if hasAlt {
		b.WriteString(fmt.Sprintf("<exif:GPSAltitude>%0.2f</exif:GPSAltitude>\n", altVal))
		b.WriteString(fmt.Sprintf("<exif:GPSAltitudeRef>%d</exif:GPSAltitudeRef>\n", altRef))
	}
	b.WriteString("<exif:GPSVersionID>2.3.0.0</exif:GPSVersionID>\n")
	b.WriteString(fmt.Sprintf("<exif:GPSDateStamp>%s</exif:GPSDateStamp>\n", gpsDate))
	b.WriteString(fmt.Sprintf("<exif:GPSTimeStamp>%s</exif:GPSTimeStamp>", gpsTime))

	return b.String()
}

func stripGPSTags(inner string) string {
	for _, re := range gpsTagRegexes {
		inner = re.ReplaceAllString(inner, "")
	}
	return strings.TrimSpace(inner)
}

func hasGPSData(data []byte) bool {
	text := strings.ToLower(string(data))
	for _, tag := range []string{
		"<exif:gpslatitude",
		"<exif:gpslongitude",
		"<exif:gpsaltitude",
		"<exif:gpstimestamp",
		"<exif:gpsdatestamp",
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
				(attr.Name.Local == "exif" && strings.Contains(attr.Value, "ns.adobe.com/exif/1.0")) {
				return i
			}
		}
	}
	return 0
}

func ensureExifNamespace(attrs []xml.Attr) []xml.Attr {
	for _, attr := range attrs {
		if attr.Name.Local == "exif" && strings.Contains(attr.Value, "ns.adobe.com/exif/1.0") {
			return attrs
		}
		if attr.Name.Local == "xmlns:exif" || (attr.Name.Space == "xmlns" && attr.Name.Local == "exif") {
			return attrs
		}
	}
	return append(attrs, xml.Attr{
		Name:  xml.Name{Space: "xmlns", Local: "exif"},
		Value: "http://ns.adobe.com/exif/1.0/",
	})
}
