package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nir0k/GeoRAW/internal/series"
	"github.com/spf13/pflag"
)

func main() {
	var opts series.Options

	pflag.StringVarP(&opts.InputPath, "input", "i", "", "Path to a photo file, directory, or glob pattern")
	pflag.BoolVarP(&opts.Recursive, "recursive", "r", false, "Scan subdirectories when the input is a folder")
	pflag.StringVar(&opts.LogLevel, "log-level", "info", "Logging level for both file and console outputs")
	pflag.StringVar(&opts.LogFile, "log-file", "", "Optional log file path (defaults to a file next to the binary)")
	pflag.BoolVar(&opts.Overwrite, "overwrite-series", false, "Overwrite existing series tags in XMP sidecars")
	pflag.StringVar(&opts.Prefix, "prefix", "", "Prefix for generated series IDs (random 6 characters by default)")
	pflag.IntVar(&opts.StartIndex, "start-index", 1, "Starting index for generated series IDs")
	pflag.StringVar((*string)(&opts.Mode), "mode", "auto", "Detection mode: auto, hdr, or focus")
	pflag.StringVar(&opts.HDRTag, "hdr-tag", "hdr_mode", "Keyword to tag HDR series")
	pflag.StringVar(&opts.FocusTag, "focus-tag", "focus_br", "Keyword to tag focus bracketing series")

	pflag.Parse()
	opts.PrintSummary = true

	ctx := context.Background()
	if _, err := series.Run(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "georaw-series failed: %v\n", err)
		os.Exit(1)
	}
}
