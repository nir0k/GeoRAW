package media

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CollectFiles resolves the input path into a list of files to process.
// It supports direct file paths, directories, and glob patterns.
func CollectFiles(input string, recursive bool) ([]string, error) {
	inputs := splitInputs(input)
	if len(inputs) == 0 {
		return nil, fmt.Errorf("input path is empty")
	}

	unique := make(map[string]struct{})
	var results []string

	addFile := func(path string) {
		if _, exists := unique[path]; !exists {
			unique[path] = struct{}{}
			results = append(results, path)
		}
	}

	for _, in := range inputs {
		matches, err := expandInput(in)
		if err != nil {
			return nil, err
		}

		for _, candidate := range matches {
			info, err := os.Stat(candidate)
			if err != nil {
				return nil, fmt.Errorf("stat %s: %w", candidate, err)
			}
			if info.IsDir() {
				err = walkDir(candidate, recursive, addFile)
				if err != nil {
					return nil, err
				}
				continue
			}
			addFile(candidate)
		}
	}

	return results, nil
}

func splitInputs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\r'
	})
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expandInput(input string) ([]string, error) {
	if containsGlob(input) {
		matches, err := filepath.Glob(input)
		if err != nil {
			return nil, fmt.Errorf("expand glob: %w", err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no files matched pattern %q", input)
		}
		return matches, nil
	}
	return []string{input}, nil
}

func containsGlob(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func walkDir(root string, recursive bool, add func(string)) error {
	if recursive {
		return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() {
				add(path)
			}
			return nil
		})
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", root, err)
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			add(filepath.Join(root, entry.Name()))
		}
	}
	return nil
}
