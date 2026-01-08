package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nir0k/GeoRAW/internal/media"
)

const maxTreeEntries = 5000

// FileNode represents a directory or media file for the EXIF tab.
type FileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Children []FileNode `json:"children,omitempty"`
}

// FileTree is the root response for the EXIF browser.
type FileTree struct {
	Root      string     `json:"root"`
	Children  []FileNode `json:"children"`
	Truncated bool       `json:"truncated"`
}

// ListExifTree returns a recursive listing of directories/files under root, limited for safety.
func (b *Backend) ListExifTree(root string) (*FileTree, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("path is empty")
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}

	remaining := maxTreeEntries
	truncated := false

	children, err := buildFileTree(root, &remaining, &truncated)
	if err != nil {
		return nil, err
	}

	return &FileTree{
		Root:      root,
		Children:  children,
		Truncated: truncated,
	}, nil
}

func buildFileTree(root string, remaining *int, truncated *bool) ([]FileNode, error) {
	if *remaining <= 0 {
		*truncated = true
		return nil, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", root, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		iDir, jDir := entries[i].IsDir(), entries[j].IsDir()
		if iDir != jDir {
			return iDir // directories first
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	var nodes []FileNode

	for _, entry := range entries {
		if *remaining <= 0 {
			*truncated = true
			break
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue // avoid cycles
		}

		fullPath := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			*remaining--
			children, err := buildFileTree(fullPath, remaining, truncated)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, FileNode{
				Name:     entry.Name(),
				Path:     fullPath,
				IsDir:    true,
				Children: children,
			})
			continue
		}

		if !media.SupportedExif(fullPath) {
			continue
		}
		*remaining--
		nodes = append(nodes, FileNode{
			Name:  entry.Name(),
			Path:  fullPath,
			IsDir: false,
		})
	}

	return nodes, nil
}

// ReadExif returns flattened EXIF data for a single file.
func (b *Backend) ReadExif(path string, includeXmp bool) (*media.ExifDetails, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is empty")
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return media.ReadExifDetails(path, includeXmp)
}
