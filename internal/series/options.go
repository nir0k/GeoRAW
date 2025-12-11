package series

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// Mode represents detection mode.
type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeHDR   Mode = "hdr"
	ModeFocus Mode = "focus"
)

// Options represents user-provided parameters for series tagging.
type Options struct {
	InputPath    string
	Recursive    bool
	LogLevel     string
	LogFile      string
	Overwrite    bool
	Mode         Mode
	Prefix       string
	StartIndex   int
	HDRTag       string
	FocusTag     string
	PrintSummary bool
}

// Validate performs basic validation and assigns defaults where needed.
func (o *Options) Validate() error {
	o.InputPath = strings.TrimSpace(o.InputPath)
	o.LogLevel = strings.TrimSpace(o.LogLevel)
	o.LogFile = strings.TrimSpace(o.LogFile)
	o.Prefix = strings.TrimSpace(o.Prefix)
	o.HDRTag = strings.TrimSpace(o.HDRTag)
	o.FocusTag = strings.TrimSpace(o.FocusTag)

	if o.InputPath == "" {
		return fmt.Errorf("input path is required")
	}
	if o.LogLevel == "" {
		o.LogLevel = "info"
	}
	if o.LogFile == "" {
		defaultPath, err := defaultLogPath()
		if err != nil {
			return err
		}
		o.LogFile = defaultPath
	}
	o.Mode = Mode(strings.ToLower(string(o.Mode)))
	if o.Mode == "" {
		o.Mode = ModeAuto
	}
	switch o.Mode {
	case ModeAuto, ModeHDR, ModeFocus:
	default:
		return fmt.Errorf("invalid mode %q (expected auto, hdr, or focus)", o.Mode)
	}

	if o.Prefix == "" {
		pfx, err := randomPrefix(6)
		if err != nil {
			return fmt.Errorf("generate prefix: %w", err)
		}
		o.Prefix = pfx
	}
	if len(o.Prefix) < 3 {
		return fmt.Errorf("prefix must be at least 3 characters")
	}
	if o.StartIndex < 1 {
		o.StartIndex = 1
	}
	if o.HDRTag == "" {
		o.HDRTag = "hdr_mode"
	}
	if o.FocusTag == "" {
		o.FocusTag = "focus_br"
	}

	return nil
}

func defaultLogPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	dir := filepath.Dir(exe)
	// When running via `go run`, executable resides in temp; prefer current working dir then.
	if strings.HasPrefix(dir, os.TempDir()) {
		cwd, err := os.Getwd()
		if err == nil {
			dir = cwd
		}
	}
	return filepath.Join(dir, "georaw.log"), nil
}

func randomPrefix(n int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b strings.Builder
	for i := 0; i < n; i++ {
		val, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[val.Int64()])
	}
	return b.String(), nil
}
