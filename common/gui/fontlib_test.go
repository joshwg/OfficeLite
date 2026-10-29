// Tests for FontLibrary: family indexing, bitmap hiding and weak shaper
// registration.

package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
	"weak"

	"gioui.org/font/gofont"
)

// TestParseNameTableTruncated checks that a name table claiming more
// records than it holds yields the ones present instead of panicking.
func TestParseNameTableTruncated(t *testing.T) {
	// Header: format 0, count 100, strings at 18; one Mac Roman family
	// record (platform 1, encoding 0, lang 0, ID 1, length 3, offset 0).
	tbl := []byte{0, 0, 0, 100, 0, 18,
		0, 1, 0, 0, 0, 0, 0, 1, 0, 3, 0, 0,
		'A', 'b', 'c'}
	if got := parseNameTable(tbl); !slices.Equal(got, []string{"Abc"}) {
		t.Fatalf("parseNameTable = %q, want [Abc]", got)
	}
}

// TestReadFamilies reads the family of a copy of Go-Regular.ttf from the
// module cache and loads its faces by that name; it skips if the file is
// not there.
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

// goRegularPath returns the path of Go-Regular.ttf in the Go module cache
// under $HOME/go, or "" if none is found.
func goRegularPath(t *testing.T) string {
	_ = gofont.Collection
	matches, _ := filepath.Glob(filepath.Join(os.Getenv("HOME"), "go/pkg/mod/golang.org/x/image@*/font/gofont/ttfs/Go-Regular.ttf"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// TestPrepareFontHidesBitmaps checks that prepareFont renames a bitmap
// table tag on a copy, leaving other tables and its input untouched.
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

// TestShaperFacesWeak checks that registering a shaper's faces doesn't
// keep the shaper alive.
func TestShaperFacesWeak(t *testing.T) {
	sh := NewFontLibrary().Shaper()
	if len(faceFiles(sh)) == 0 {
		t.Fatal("no faces registered for the shaper")
	}
	k := weak.Make(sh)
	held := func() bool {
		shaperFacesMu.RLock()
		defer shaperFacesMu.RUnlock()
		_, ok := shaperFaces[k]
		return ok
	}
	sh = nil
	for deadline := time.Now().Add(5 * time.Second); held(); {
		if time.Now().After(deadline) {
			t.Fatal("shaper's faces kept after it was dropped")
		}
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
}
