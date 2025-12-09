package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nir0k/GeoRAW/internal/app"
	"github.com/spf13/pflag"
)

func main() {
	var opts app.Options

	pflag.StringVarP(&opts.GPXPath, "gpx", "g", "", "Path to GPX track file")
	pflag.StringVarP(&opts.InputPath, "input", "i", "", "Path to a photo file, directory, or glob pattern")
	pflag.BoolVarP(&opts.Recursive, "recursive", "r", false, "Scan subdirectories when the input is a folder")
	pflag.StringVarP(&opts.LogLevel, "log-level", "l", "info", "Logging level for both file and console outputs")
	pflag.StringVar(&opts.LogFile, "log-file", "", "Optional log file path (defaults to a file next to the binary)")
	pflag.DurationVar(&opts.TimeOffset, "time-offset", 0, "Offset added to photo capture time (e.g. -30s or 2m)")
	pflag.BoolVar(&opts.AutoOffset, "auto-offset", true, "Automatically estimate time offset between camera clock and GPX track when time-offset is zero")
	pflag.BoolVarP(&opts.Overwrite, "overwrite-gps", "w", false, "Overwrite existing GPS data in XMP sidecars")

	pflag.Parse()

	opts.PrintSummary = true

	ctx := context.Background()
	if _, err := app.Run(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "georaw failed: %v\n", err)
		os.Exit(1)
	}
}
