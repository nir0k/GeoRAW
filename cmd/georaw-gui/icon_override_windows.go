//go:build windows

package main

import _ "unsafe"

// appIconID overrides the wails internal AppIconID to match the default rsrc icon ID (1).
//
//go:linkname appIconID github.com/wailsapp/wails/v2/internal/frontend/desktop/windows/winc.AppIconID
var appIconID int

func init() {
	appIconID = 1
}
