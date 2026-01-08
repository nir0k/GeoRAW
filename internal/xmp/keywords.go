package xmp

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ErrKeywordsAlreadyPresent is returned when requested tags are already present and overwriting is disabled.
var ErrKeywordsAlreadyPresent = errors.New("series tags already present")

// MergeKeywords updates or creates an XMP sidecar with the provided keyword list.
// It preserves other tags and merges with existing keywords unless overwrite is true.
func MergeKeywords(path string, tags []string, overwrite bool) (bool, error) {
	tags = normalizeTags(tags)
	if len(tags) == 0 {
		return false, fmt.Errorf("no tags provided")
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read existing sidecar: %w", err)
	}

	payload, changed, err := mergeKeywordPayload(existing, tags, overwrite)
	if errors.Is(err, ErrKeywordsAlreadyPresent) {
		return false, err
	}
	if err != nil {
		return false, err
	}
	if !changed {
		return false, ErrKeywordsAlreadyPresent
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create sidecar dir: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func mergeKeywordPayload(existing []byte, tags []string, overwrite bool) ([]byte, bool, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return buildKeywordsSidecar(tags), true, nil
	}

	doc, err := parseXMP(existing)
	if err != nil {
		return nil, false, fmt.Errorf("parse existing xmp: %w", err)
	}

	descIdx := selectDescription(doc.RDF.Descriptions)
	if descIdx == -1 {
		descIdx = 0
		doc.RDF.Descriptions = append(doc.RDF.Descriptions, rdfDescription{})
	}

	desc := doc.RDF.Descriptions[descIdx]
	desc.Attrs = ensureDCNamespace(desc.Attrs)

	inner, changed, err := mergeKeywordsInner(desc.Inner, tags, overwrite)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return nil, false, ErrKeywordsAlreadyPresent
	}
	desc.Inner = inner
	doc.RDF.Descriptions[descIdx] = desc

	out, err := marshalXMP(doc)
	if err != nil {
		return nil, false, fmt.Errorf("marshal merged xmp: %w", err)
	}
	return out, true, nil
}

func mergeKeywordsInner(inner string, tags []string, overwrite bool) (string, bool, error) {
	existing := extractKeywords(inner)
	// If overwrite is disabled and all tags exist, nothing to do.
	if !overwrite && containsAll(existing, tags) {
		return inner, false, nil
	}

	merged := make([]string, 0, len(existing)+len(tags))
	seen := make(map[string]struct{})

	tagSet := make(map[string]struct{})
	for _, t := range tags {
		tagSet[strings.ToLower(t)] = struct{}{}
	}

	for _, kw := range existing {
		lower := strings.ToLower(kw)
		if overwrite {
			if _, toReplace := tagSet[lower]; toReplace {
				continue // drop old copy of our tags
			}
		}
		if _, ok := seen[lower]; !ok {
			seen[lower] = struct{}{}
			merged = append(merged, kw)
		}
	}

	for _, kw := range tags {
		lower := strings.ToLower(kw)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		merged = append(merged, kw)
	}

	sort.Strings(merged)

	trimmed := strings.TrimSpace(stripSubject(inner))
	subjectBlock := buildSubjectBlock(merged)
	if trimmed == "" {
		return subjectBlock, true, nil
	}
	return trimmed + "\n" + subjectBlock, true, nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		l := strings.ToLower(t)
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, t)
	}
	return out
}

func containsAll(existing, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(existing))
	for _, kw := range existing {
		set[strings.ToLower(kw)] = struct{}{}
	}
	for _, kw := range required {
		if _, ok := set[strings.ToLower(kw)]; !ok {
			return false
		}
	}
	return true
}

func extractKeywords(inner string) []string {
	subjectRe := regexp.MustCompile(`(?is)<dc:subject[^>]*>.*?</dc:subject>`)
	liRe := regexp.MustCompile(`(?is)<rdf:li[^>]*>(.*?)</rdf:li>`)

	var out []string
	for _, block := range subjectRe.FindAllString(inner, -1) {
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

func stripSubject(inner string) string {
	subjectRe := regexp.MustCompile(`(?is)<dc:subject[^>]*>.*?</dc:subject>`)
	return strings.TrimSpace(subjectRe.ReplaceAllString(inner, ""))
}

func buildSubjectBlock(keywords []string) string {
	var b strings.Builder
	b.WriteString("<dc:subject>\n")
	b.WriteString("  <rdf:Bag>\n")
	for _, kw := range keywords {
		b.WriteString(fmt.Sprintf("    <rdf:li>%s</rdf:li>\n", xmlEscape(kw)))
	}
	b.WriteString("  </rdf:Bag>\n")
	b.WriteString("</dc:subject>")
	return b.String()
}

func ensureDCNamespace(attrs []xml.Attr) []xml.Attr {
	for _, attr := range attrs {
		if attr.Name.Local == "xmlns:dc" || (attr.Name.Space == "xmlns" && attr.Name.Local == "dc") {
			return attrs
		}
	}
	return append(attrs, xml.Attr{
		Name:  xml.Name{Space: "xmlns", Local: "dc"},
		Value: "http://purl.org/dc/elements/1.1/",
	})
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

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

func buildKeywordsSidecar(keywords []string) []byte {
	var b strings.Builder
	b.WriteString(`<?xpacket begin=" " id="W5M0MpCehiHzreSzNTczkc9d"?>`)
	b.WriteString("\n<x:xmpmeta xmlns:x=\"adobe:ns:meta/\" x:xmptk=\"GeoRAW\">\n")
	b.WriteString("  <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n")
	b.WriteString("    <rdf:Description rdf:about=\"\" xmlns:dc=\"http://purl.org/dc/elements/1.1/\">\n")
	b.WriteString(indentBlock(buildSubjectBlock(keywords), "      "))
	b.WriteString("\n    </rdf:Description>\n")
	b.WriteString("  </rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")
	b.WriteString("<?xpacket end=\"w\"?>")
	return []byte(b.String())
}

func indentBlock(block, prefix string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
