package ui

import (
    _ "embed"
    "fyne.io/fyne/v2"
    "os"
    "path/filepath"
)

// Embed the bundled subset font that always exists inside this package.
//go:embed fonts/cjk-subset.ttf
var cjkSubsetData []byte

// CJKSubsetFont is the fyne Resource for the CJK font used by the app.
// At runtime we prefer NotoSansSC from assets/fonts if found; otherwise we
// fall back to the embedded subset to guarantee glyph availability in builds.
var CJKSubsetFont fyne.Resource

func init() {
    // Try common locations for the provided font.
    candidates := []string{
        // Running from project root
        filepath.Join("assets", "fonts", "NotoSansSC-Regular-Instance.ttf"),
    }
    if exe, err := os.Executable(); err == nil {
        exeDir := filepath.Dir(exe)
        candidates = append(candidates,
            // Next to packaged executable (desktop, mobile bundles may vary)
            filepath.Join(exeDir, "assets", "fonts", "NotoSansSC-Regular-Instance.ttf"),
        )
    }
    for _, p := range candidates {
        if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
            CJKSubsetFont = fyne.NewStaticResource("NotoSansSC-Regular-Instance.ttf", b)
            return
        }
    }
    // Fallback to embedded subset
    CJKSubsetFont = fyne.NewStaticResource("cjk-subset.ttf", cjkSubsetData)
}
