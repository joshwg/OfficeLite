package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/font"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// The face index in each glyph ID must lead to the file the shaper took
// the glyph from, or DirectWrite draws it from another face (bold text
// for regular, say).
func TestFaceFilesMatchShaper(t *testing.T) {
	lib := NewFontLibrary(DefaultFontDirs()...)
	if len(lib.Faces("Arial")) < 2 {
		t.Skip("Arial (regular and bold) not installed")
	}
	sh := lib.Shaper("Arial")
	files := faceFiles(sh)
	for _, c := range []struct {
		w    font.Weight
		file string
	}{{font.Normal, "arial.ttf"}, {font.Bold, "arialbd.ttf"}} {
		sh.LayoutString(text.Parameters{Font: font.Font{Typeface: Typeface("Arial"), Weight: c.w}, PxPerEm: fixed.I(15), MaxWidth: 1 << 20}, "A")
		g, _ := sh.NextGlyph()
		_, fi, _ := splitGlyphID(g.ID)
		if fi >= len(files) || !strings.EqualFold(filepath.Base(files[fi].Path), c.file) {
			t.Errorf("weight %v: face %d is %v, want %s", c.w, fi, files, c.file)
		}
	}
}
