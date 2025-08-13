# App Icon Assets

Flat minimal icon set (cold, bright colors). No letters. Two variants:

- app_icon_flat.svg: symbol only, transparent background
- app_icon_square.svg: symbol on rounded-square background
- Android adaptive icon: `android/foreground.svg` + `android/background.svg`

## Recommended sizes per platform

- macOS (.icns)
  - 16, 32, 64, 128, 256, 512, 1024 px
- Windows (.ico)
  - 16, 24, 32, 48, 64, 128, 256 px
- Linux (PNG/SVG)
  - 16, 24, 32, 48, 64, 128, 256, 512, 1024 px
- Android (Adaptive Icon)
  - foreground/background SVGs; mipmap outputs mdpi~xxxhdpi (48, 72, 96, 144, 192, 256 px)

## Export script (optional)

Use `tools/export-icons.sh` to export all formats if you have these tools installed:

- Inkscape or rsvg-convert (PNG rendering)
- iconutil (macOS .icns)
- icoutils (icotool) or ImageMagick (Windows .ico)

## Integration hints

- macOS: place .icns in your app bundle and reference in Info.plist (CFBundleIconFile)
- Windows: reference .ico in your resource or manifest
- Linux: install PNG/SVG under appropriate icon theme directories or desktop file
- Android: put adaptive layers into `app/src/main/res/mipmap-anydpi-v26/ic_launcher.xml` and mipmap folders

