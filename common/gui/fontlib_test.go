// Tests for FontLibrary: family indexing, bitmap hiding and weak shaper
// registration.

package gui

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
	"weak"

	"gioui.org/font/gofont"
	"golang.org/x/image/font/gofont/goregular"
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

// TestFacesFallsBackPastBadFile checks that a family whose file in the
// first folder is damaged or unreadable is loaded from the next folder,
// and that a good first folder still wins over the later ones.
func TestFacesFallsBackPastBadFile(t *testing.T) {
	good := goregular.TTF
	first, second := t.TempDir(), t.TempDir()
	damaged := filepath.Join(first, "Go-Regular.ttf")
	fallback := filepath.Join(second, "Go-Regular.ttf")
	for _, c := range []struct {
		name  string
		first []byte // the first folder's file; nil: missing
		from  string // the folder the face must come from
	}{
		{"damaged", good[:100], fallback},
		{"empty", []byte{}, fallback},
		{"unreadable", nil, fallback},
		{"good", good, damaged},
	} {
		os.Remove(damaged)
		if c.first != nil {
			os.WriteFile(damaged, c.first, 0o644)
		}
		os.WriteFile(fallback, good, 0o644)
		l := NewFontLibrary(first, second)
		l.scanned = true
		l.index["go"] = []string{damaged, fallback} // as the scan would list them
		faces := l.Faces("Go")
		if len(faces) == 0 {
			t.Errorf("%s: no faces", c.name)
			continue
		}
		if got := l.files[faces[0].Face].Path; got != c.from {
			t.Errorf("%s: face from %s, want %s", c.name, got, c.from)
		}
	}
}

// TestWinMetricsCollectionIndex checks that readWinMetrics reads a face of
// a collection by its index and refuses an index past the collection's
// font count, whatever the offset table's next bytes hold.
func TestWinMetricsCollectionIndex(t *testing.T) {
	ttf := goregular.TTF
	src := filepath.Join(t.TempDir(), "Go-Regular.ttf")
	if err := os.WriteFile(src, ttf, 0o644); err != nil {
		t.Fatal(err)
	}
	// A collection of one font that leaves room for a second offset (which
	// points at the same font), then the font with its table offsets moved.
	const head = 20
	be := binary.BigEndian
	ttc := make([]byte, head, head+len(ttf))
	copy(ttc, "ttcf")
	be.PutUint32(ttc[4:], 0x00010000)
	be.PutUint32(ttc[8:], 1) // one font
	be.PutUint32(ttc[12:], head)
	be.PutUint32(ttc[16:], head)
	ttc = append(ttc, ttf...)
	for k := range int(be.Uint16(ttf[4:])) {
		rec := head + 12 + 16*k + 8
		be.PutUint32(ttc[rec:], be.Uint32(ttc[rec:])+head)
	}
	path := filepath.Join(t.TempDir(), "Go.ttc")
	if err := os.WriteFile(path, ttc, 0o644); err != nil {
		t.Fatal(err)
	}
	want := readWinMetrics(FaceFile{Path: src})
	if !want.ok {
		t.Fatal("no metrics for the plain font")
	}
	if got := readWinMetrics(FaceFile{Path: path, Index: 0}); got != want {
		t.Errorf("index 0 = %+v, want %+v", got, want)
	}
	for _, i := range []int{1, 2, -1} {
		if got := readWinMetrics(FaceFile{Path: path, Index: i}); got.ok {
			t.Errorf("index %d past the collection's one font gave %+v", i, got)
		}
	}
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
