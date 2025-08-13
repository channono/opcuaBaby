#!/usr/bin/env bash
set -euo pipefail

# Export icon assets for macOS, Windows, Linux, Android
# Requirements:
# - inkscape (or rsvg-convert)
# - iconutil (macOS) for .icns
# - icotool (icoutils) or ImageMagick 'convert' for .ico

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SRC_DIR="$ROOT_DIR/assets/icons"
OUT_DIR="$ROOT_DIR/dist/icons"

mkdir -p "$OUT_DIR/png" "$OUT_DIR/macos" "$OUT_DIR/windows" "$OUT_DIR/linux" "$OUT_DIR/android"

render_png() {
  local svg="$1"; shift
  local size="$1"; shift
  local out="$OUT_DIR/png/$(basename "${svg%.svg}")_${size}.png"
  if command -v inkscape >/dev/null 2>&1; then
    inkscape "$svg" --export-type=png -w "$size" -h "$size" -o "$out"
  elif command -v rsvg-convert >/dev/null 2>&1; then
    rsvg-convert -w "$size" -h "$size" -o "$out" "$svg"
  else
    echo "Neither inkscape nor rsvg-convert found. Install one to render PNGs." >&2
    return 1
  fi
}

# Common PNG sizes
SIZES=(16 24 32 48 64 72 96 128 144 192 256 384 512 1024)

# Render PNGs for both variants
for svg in "$SRC_DIR/app_icon_flat.svg" "$SRC_DIR/app_icon_square.svg"; do
  for s in "${SIZES[@]}"; do
    render_png "$svg" "$s"
  done
done

# macOS .icns
ICONSET_DIR="$OUT_DIR/macos/App.iconset"
mkdir -p "$ICONSET_DIR"
for s in 16 32 128 256 512; do
  cp "$OUT_DIR/png/app_icon_square_${s}.png" "$ICONSET_DIR/icon_${s}x${s}.png"
  cp "$OUT_DIR/png/app_icon_square_$((s*2)).png" "$ICONSET_DIR/icon_${s}x${s}@2x.png"
done
if command -v iconutil >/dev/null 2>&1; then
  iconutil -c icns "$ICONSET_DIR" -o "$OUT_DIR/macos/App.icns"
else
  echo "iconutil not found; skipping .icns generation." >&2
fi

# Windows .ico (256->ico)
if command -v icotool >/dev/null 2>&1; then
  icotool -c -o "$OUT_DIR/windows/App.ico" \
    "$OUT_DIR/png/app_icon_square_16.png" \
    "$OUT_DIR/png/app_icon_square_24.png" \
    "$OUT_DIR/png/app_icon_square_32.png" \
    "$OUT_DIR/png/app_icon_square_48.png" \
    "$OUT_DIR/png/app_icon_square_64.png" \
    "$OUT_DIR/png/app_icon_square_128.png" \
    "$OUT_DIR/png/app_icon_square_256.png"
elif command -v convert >/dev/null 2>&1; then
  convert \
    "$OUT_DIR/png/app_icon_square_16.png" \
    "$OUT_DIR/png/app_icon_square_24.png" \
    "$OUT_DIR/png/app_icon_square_32.png" \
    "$OUT_DIR/png/app_icon_square_48.png" \
    "$OUT_DIR/png/app_icon_square_64.png" \
    "$OUT_DIR/png/app_icon_square_128.png" \
    "$OUT_DIR/png/app_icon_square_256.png" \
    "$OUT_DIR/windows/App.ico"
else
  echo "icotool or ImageMagick convert not found; skipping .ico generation." >&2
fi

# Linux: copy PNGs and SVGs
cp "$SRC_DIR/app_icon_flat.svg" "$OUT_DIR/linux/"
cp "$SRC_DIR/app_icon_square.svg" "$OUT_DIR/linux/"
for s in 16 24 32 48 64 128 256 512 1024; do
  cp "$OUT_DIR/png/app_icon_square_${s}.png" "$OUT_DIR/linux/" || true
  cp "$OUT_DIR/png/app_icon_flat_${s}.png" "$OUT_DIR/linux/" || true
done

# Android adaptive icon
cat > "$OUT_DIR/android/ic_launcher.xml" <<'XML'
<?xml version="1.0" encoding="utf-8"?>
<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
    <background android:drawable="@drawable/ic_launcher_background"/>
    <foreground android:drawable="@drawable/ic_launcher_foreground"/>
</adaptive-icon>
XML

# Export foreground/background as PNGs for preview/reference
for s in 108 162 216 324; do # mdpi..xxxhdpi logical edge
  render_png "$SRC_DIR/android/background.svg" "$s" || true
  mv "$OUT_DIR/png/background_${s}.png" "$OUT_DIR/android/background_${s}.png" 2>/dev/null || true
  render_png "$SRC_DIR/android/foreground.svg" "$s" || true
  mv "$OUT_DIR/png/foreground_${s}.png" "$OUT_DIR/android/foreground_${s}.png" 2>/dev/null || true
done

echo "Done. Outputs in $OUT_DIR"
