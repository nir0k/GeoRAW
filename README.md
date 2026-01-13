# GeoRAW

CLI tool that writes GPS coordinates to XMP sidecars for RAW photos using a GPX track. RAW files are never modified.

## Features
- Reads GPX and interpolates coordinates by capture time.
- Automatic camera clock offset detection (median of nearest GPX points, ±12h window) via `--auto-offset` (enabled by default).
- Manual time shift with `--time-offset` (e.g., `-30s`, `2m`).
- Updates existing XMP sidecars without wiping other tags—only GPS tags are replaced.
- Filters for common RAW extensions (Canon/Sony and others); logs skipped files and errors.
- Canon HDR series detection with series keywords written to XMP sidecars (no RAW changes).

## Requirements
- Go 1.25+

## Install
```bash
go install github.com/nir0k/GeoRAW/cmd/georaw@latest
```

## Usage
```bash
georaw --gpx /path/track.gpx --input "/photos/*.CR3" --recursive \
       --time-offset=0s --auto-offset=true --log-level=info --log-file=/path/georaw.log
```

### Flags
- `--gpx, -g` — path to GPX file.
- `--input, -i` — file, directory, or glob pattern (e.g., `"*.CR3"`).
- `--recursive, -r` — recurse into subdirectories when input is a folder.
- `--time-offset` — manual time shift; if `0` and `--auto-offset=true`, auto-detection is applied.
- `--auto-offset` — enable/disable auto clock offset detection.
- `--overwrite-gps, -w` — replace GPS tags even if the XMP sidecar already contains GPS.
- `--log-level` — log level (`trace|debug|info|warning|error|fatal`).
- `--log-file` — log file path (defaults to `georaw.log` next to the binary).

## HDR series tagging (Canon RAW)
Detects HDR series (Canon only), groups shots by time/order, and writes two keywords to XMP sidecars: type (`hdr_mode` by default) and unique ID (`PREFIX_00001`, etc.). RAW files are never modified; non-Canon RAWs are skipped. Run it from the GUI Series tab (auto detection or forced HDR), with prefix/start index, extra tags (comma-separated), recursion, and overwrite controls; results include per-file statuses and series IDs with logs available via the modal.

### GUI
The GUI has three tabs:
- **GPS tagging** — existing GPX workflow.
- **Series tagging** — select photos (file/folder/glob), mode (auto or force HDR), prefix/start index, extra tags (comma-separated), recursion, overwrite toggle, and run. Results show per-file status plus series type/ID tags; logs available via the modal.
- **EXIF viewer** — browse a folder (nested directories inline), filter files in the tree, and separately search metadata (by field name or value) for supported photo formats (RAW/JPEG/HEIF/HIF/AVIF/TIFF). XMP/sidecar fields can be toggled and are included by default. Requires `exiftool` in `PATH` (see below).

## GUI (Wails)
A simple Wails UI is available to run the same workflow. Launch:
```bash
go run -tags dev ./cmd/georaw-gui
```
The window lets you pick GPX, photo path (file/folder/glob), toggle recursion, auto-offset, overwrite GPS, a human-friendly time offset (`+1h30m`, `-00:00:30`, `90s`), and log level. Logs are written to `georaw.log`; a completion summary plus per-file results are shown in the UI.  
Notes:
- Linux: install WebKitGTK/GTK dev libs (e.g. Debian/Ubuntu: `libwebkit2gtk-4.0-dev libgtk-3-dev`; Fedora: `webkit2gtk3-devel gtk3-devel cairo-devel pango-devel gdk-pixbuf2-devel libsoup3-devel`).
- Dev builds may print `Overriding existing handler for signal 10...` from WebKitGTK; this is a harmless message about GC signals.
- For packaged GUI binaries use `make gui-linux` / `make gui-windows` (frontend embedded, Windows build hides the console). Rebuild with make if you previously ran with an external `frontend` folder.

### Examples
- Simple run with auto offset:
  ```bash
  georaw -g track.gpx -i "/photos/*.ARW" -r
  ```
- Run with manual offset of -30 seconds:
  ```bash
  georaw -g track.gpx -i /photos -r --time-offset=-30s --auto-offset=false
  ```

### EXIF viewer dependency
To view full EXIF data in the GUI, `exiftool` must be in your `PATH`:
- Linux/macOS: install via your package manager (e.g., `apt install libimage-exiftool-perl`, `brew install exiftool`).
- Windows: download the portable `exiftool(-k).exe` from exiftool.org, rename to `exiftool.exe`, and place it next to the GeoRAW GUI exe or in `%PATH%` (Chocolatey: `choco install exiftool`).

## Build via Makefile
- CLI Linux: `make cli-linux` → `bin/georaw.linux-amd64`
- CLI Windows: `make cli-windows` → `bin/georaw.exe`
- GUI Linux (production embed): `make gui-linux` → `bin/georaw-gui.linux-amd64`
- GUI Windows (production embed, no console window): `make gui-windows` → `bin/georaw-gui.exe` (requires CGO/Windows toolchain, WebView2 SDK)
  
Version injection: `VERSION=v1.2.3 make gui-linux` (default is derived from `git describe --tags --always --dirty` or `dev`).

After a make build, run the produced binary from `bin/` (e.g., `./bin/georaw.linux-amd64` or `./bin/georaw-gui.exe`).

## Build on Windows (PowerShell)
Prereqs: Go, CGO toolchain (MSVC Build Tools or MinGW-w64), WebView2 Runtime.

- CLI:
  ```powershell
  go build -o bin/georaw.exe ./cmd/georaw
  ```
- GUI dev (runs from disk, shows console for logs):
  ```powershell
  $env:assetdir="frontend"; $env:CGO_ENABLED=1; go run -tags dev ./cmd/georaw-gui
  ```
- GUI production build (frontend embedded, no console window):
  ```powershell
  $env:CGO_ENABLED=1; go build -tags production -ldflags="-H windowsgui" -o bin/georaw-gui.exe ./cmd/georaw-gui
  ```

### Windows build with custom icon
Use an `.ico` with multiple sizes (at least 16, 32, 48, 64, 128, 256).

1) Place your icon as `cmd/georaw-gui/icon.ico`.
2) Generate the resource (once per icon change):
   ```powershell
   cd cmd/georaw-gui
   rsrc -ico icon.ico -o icon.syso
   cd ..
   ```
3) Build the GUI:
   ```powershell
   set CGO_ENABLED=1
   go build -tags production -ldflags="-H windowsgui" -o bin/georaw-gui.exe ./cmd/georaw-gui
   ```
Dev (`go run -tags dev`) не показывает иконку; она видна только в собранном exe.

## App icon (Windows)
- Формат: `.ico` с несколькими размерами внутри (рекомендуется минимум 16×16, 32×32, 48×48, 64×64, 128×128, 256×256).
- Для production exe на Windows иконка подхватывается из `cmd/georaw-gui/icon.syso`, собранного из `icon.ico` (см. шаги выше). In dev-run иконка окна не отображается — это ограничение Wails.

### Notes
- Existing sidecars keep all other tags; only GPS-related tags are replaced (`GPSLatitude`, `GPSLongitude`, `GPSAltitude`, `GPSVersionID`, `GPSDateStamp`, `GPSTimeStamp`, and their refs).
- Logs are written to a file (console output is disabled for GUI; CLI prints a final summary).
- Supported RAW extensions include Canon/Sony and many others (`.cr2`, `.cr3`, `.arw`, `.nef`, `.raf`, `.dng`, etc. — see `internal/media/metadata.go`).
