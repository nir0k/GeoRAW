package frontend

import "embed"

// Assets contains the embedded frontend files.
//
//go:embed *
var Assets embed.FS
