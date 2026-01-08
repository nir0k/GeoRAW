# GeoRAW Release Notes

## Highlights
- **Progress & Results**
  - Progress bar now reflects real processing progress via emitted events.
  - Results view groups series, keeps skipped items collapsed, and shows clearer statuses.
  - HDR series handling hardened (safer maker-note reads, better metadata parsing) to reduce misclassification and crashes on malformed files.

- **EXIF Viewer**
  - New EXIF tab to browse folders (nested inline), double-click to drill into subfolders, and view collapsible grouped metadata.
  - Separate searches: file list filter on the left, metadata search (by field name or value) on the right with live filtering.
  - XMP/sidecar fields toggle (on by default) with XMP badges; keywords read from sidecars; supports RAW/JPEG/HEIF/HIF/AVIF/TIFF.
  - Responsive layout fills available width/height with consistent dark input styling.

- **EXIF Dependency**
  - `exiftool` must be in `PATH` for the EXIF viewer.
    - Linux/macOS: install via package manager (e.g., `apt install libimage-exiftool-perl`, `brew install exiftool`).
    - Windows: use the portable `exiftool.exe` (rename `exiftool(-k).exe`) next to the GUI exe or add to `%PATH%`; or `choco install exiftool`.
