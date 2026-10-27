package gui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/font/gofont"
)

// The Go fonts are embedded in gofont as raw TTF bytes we can write out.
func TestReadFamilies(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(goRegularPath(t))
	if err != nil {
		t.Skip("goregular.ttf not found in module cache:", err)
	}
	p := filepath.Join(dir, "goregular.ttf")
	if err := os.WriteFile(p, src, 0o644); err != nil {
		t.Fatal(err)
	}
	fams := readFamilies(p)
	if !slices.Contains(fams, "Go") {
		t.Fatalf("families = %q, want Go", fams)
	}

	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	lib := NewFontLibrary(dir)
	if faces := lib.Faces("Go"); len(faces) == 0 {
		t.Fatal("no faces loaded for Go")
	}
	if faces := lib.Faces("Nonexistent"); len(faces) != 0 {
		t.Fatalf("got %d faces for an unknown family", len(faces))
	}
}

func goRegularPath(t *testing.T) string {
	_ = gofont.Collection
	matches, _ := filepath.Glob(filepath.Join(os.Getenv("HOME"), "go/pkg/mod/golang.org/x/image@*/font/gofont/ttfs/Go-Regular.ttf"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func TestPrepareFontHidesBitmaps(t *testing.T) {
	// Minimal sfnt header with two table records: EBDT and glyf.
	b := make([]byte, 12+2*16)
	b[5] = 2
	copy(b[12:], "EBDT")
	copy(b[28:], "glyf")
	out := prepareFont(b)
	if string(out[12:16]) != "xBDT" || string(out[28:32]) != "glyf" {
		t.Fatalf("tags = %q %q", out[12:16], out[28:32])
	}
	if string(b[12:16]) != "EBDT" {
		t.Fatal("input was modified")
	}
}
