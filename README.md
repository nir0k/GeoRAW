# GeoRAW

CLI tool that writes GPS coordinates to XMP sidecars for RAW photos using a GPX track. RAW files are never modified.

## Features
- Reads GPX and interpolates coordinates by capture time.
- Automatic camera clock offset detection (median of nearest GPX points, ±12h window) via `--auto-offset` (enabled by default).
- Manual time shift with `--time-offset` (e.g., `-30s`, `2m`).
- Updates existing XMP sidecars without wiping other tags—only GPS tags are replaced.
- Filters for common RAW extensions (Canon/Sony and others); logs skipped files and errors.

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

### Examples
- Simple run with auto offset:
  ```bash
  georaw -g track.gpx -i "/photos/*.ARW" -r
  ```
- Run with manual offset of -30 seconds:
  ```bash
  georaw -g track.gpx -i /photos -r --time-offset=-30s --auto-offset=false
  ```

### Notes
- Existing sidecars keep all other tags; only GPS-related tags are replaced (`GPSLatitude`, `GPSLongitude`, `GPSAltitude`, `GPSVersionID`, `GPSDateStamp`, `GPSTimeStamp`, and their refs).
- Logs are written to both file and console; successful operations are logged too.
- Supported RAW extensions include Canon/Sony and many others (`.cr2`, `.cr3`, `.arw`, `.nef`, `.raf`, `.dng`, etc. — see `internal/media/metadata.go`).
