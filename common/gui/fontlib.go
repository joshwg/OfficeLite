package gui

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf16"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
)

// FontLibrary indexes font folders (the OS font folders plus extras) by
// family name and loads only the families a document actually uses. The
// index reads just each file's name table and is cached between runs.
//
// The library replaces Gio's own system-font loading so every face passes
// through prepareFont, which hides embedded grayscale bitmap strikes:
// Windows fonts such as Calibri carry them for small sizes, and Gio cannot
// draw them, so text would vanish.
type FontLibrary struct {
	mu      sync.Mutex
	index   map[string][]string // lower-case family -> font files
	loaded  map[string][]font.FontFace
	files   map[font.Face]FaceFile // where each loaded face came from
	scanned bool
	dirs    []string
}

// DefaultFontDirs returns the font folders to index, in priority order: a
// "Fonts" folder next to the executable or in the working directory, any
// folders listed in OFFICELITE_FONTS (OS path-list separated), then the
// operating system's font folders.
func DefaultFontDirs() []string {
	var dirs []string
	add := func(d string) {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			if abs, err := filepath.Abs(d); err == nil && !slices.Contains(dirs, abs) {
				dirs = append(dirs, abs)
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		add(filepath.Join(filepath.Dir(exe), "Fonts"))
		add(filepath.Join(filepath.Dir(exe), "..", "Fonts")) // bin/ under the project root
	}
	add("Fonts")
	for _, d := range filepath.SplitList(os.Getenv("OFFICELITE_FONTS")) {
		add(d)
	}
	for _, d := range systemFontDirs() {
		add(d)
	}
	return dirs
}

func systemFontDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("WINDIR"), "Fonts"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"),
			// Office "cloud fonts" (Aptos, the Office 2023+ default, ...).
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "FontCache", "4", "CloudFonts"),
		}
	case "darwin":
		return []string{"/System/Library/Fonts", "/Library/Fonts", filepath.Join(home, "Library", "Fonts")}
	default:
		return []string{"/usr/share/fonts", "/usr/local/share/fonts",
			filepath.Join(home, ".local", "share", "fonts"), filepath.Join(home, ".fonts")}
	}
}

// FallbackFamilies are always loaded so that text renders even when a
// document's font is unavailable.
var FallbackFamilies = []string{"Segoe UI", "Arial", "Liberation Sans", "DejaVu Sans", "Segoe UI Symbol"}

// Shaper returns a text shaper holding the given families, the fallbacks
// and the Go fonts (always present), without Gio's own system-font scan.
func (l *FontLibrary) Shaper(families ...string) *text.Shaper {
	faces := l.Faces(append(slices.Clone(families), FallbackFamilies...)...)
	faces = append(faces, gofontCollection()...)
	sh := text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	l.registerShaper(sh, faces)
	return sh
}

// FaceFile is the font file (and face within a collection) a loaded face
// was read from.
type FaceFile struct {
	Path  string
	Index int
}

// registerShaper records, for each face index the shaper gives its
// glyphs, the file the face came from, so that its glyphs can be drawn by
// the system rasterizer (see PaintGlyphs). Faces the shaper skips as
// duplicates take no index, as in Gio's shaper.
func (l *FontLibrary) registerShaper(sh *text.Shaper, faces []font.FontFace) {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[any]bool{}
	var files []FaceFile
	for _, f := range faces {
		k := any(f.Face.Face().Font)
		if seen[k] {
			continue
		}
		seen[k] = true
		files = append(files, l.files[f.Face]) // zero for the Go fonts
	}
	shaperFaces.Store(sh, files)
}

// shaperFaces maps each *text.Shaper made by a FontLibrary to its faces'
// files, by face index.
var shaperFaces sync.Map

func faceFiles(sh *text.Shaper) []FaceFile {
	v, _ := shaperFaces.Load(sh)
	files, _ := v.([]FaceFile)
	return files
}

func NewFontLibrary(dirs ...string) *FontLibrary {
	return &FontLibrary{dirs: dirs, index: map[string][]string{}, loaded: map[string][]font.FontFace{}, files: map[font.Face]FaceFile{}}
}

// Faces returns the faces for the given family names and their
// metric-compatible substitutes, loading files on first use.
func (l *FontLibrary) Faces(families ...string) []font.FontFace {
	if l == nil || len(l.dirs) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.scanned {
		l.scan()
		l.scanned = true
	}
	var want []string // in priority order
	for _, f := range families {
		f = strings.ToLower(f)
		want = append(want, f)
		for _, sub := range strings.Split(metricCompatible[f], ",") {
			if sub = strings.ToLower(strings.TrimSpace(sub)); sub != "" {
				want = append(want, sub)
			}
		}
	}
	var out []font.FontFace
	seen := map[string]bool{}
	for _, fam := range want {
		if seen[fam] {
			continue
		}
		seen[fam] = true
		faces, ok := l.loaded[fam]
		if !ok {
			files := map[string]bool{}
			for _, p := range l.index[fam] {
				// The first folder with the family wins (./Fonts overrides
				// the system), but take every file (styles) from it.
				if len(files) > 0 && !files[filepath.Dir(p)] {
					continue
				}
				files[filepath.Dir(p)] = true
				if b, err := os.ReadFile(p); err == nil {
					if fs, err := opentype.ParseCollection(prepareFont(b)); err == nil {
						for i, f := range fs {
							l.files[f.Face] = FaceFile{p, i}
						}
						faces = append(faces, fs...)
					}
				}
			}
			l.loaded[fam] = faces
		}
		out = append(out, faces...)
	}
	return out
}

// prepareFont hides grayscale bitmap strike tables (EBLC/EBDT/EBSC and
// Apple's bloc/bdat) by renaming their tags, so the outlines are used.
// Color bitmaps (CBDT, emoji) are kept. The table directory is a lookup map
// in the parser, so renamed tags simply go unused.
func prepareFont(b []byte) []byte {
	be := binary.BigEndian
	offsets := []int{0}
	if len(b) >= 12 && string(b[:4]) == "ttcf" {
		n := int(be.Uint32(b[8:]))
		offsets = offsets[:0]
		for i := 0; i < n && 12+4*i+4 <= len(b); i++ {
			offsets = append(offsets, int(be.Uint32(b[12+4*i:])))
		}
	}
	copied := false
	for _, off := range offsets {
		if off+12 > len(b) {
			continue
		}
		n := int(be.Uint16(b[off+4:]))
		for i := 0; i < n; i++ {
			rec := off + 12 + 16*i
			if rec+4 > len(b) {
				break
			}
			switch string(b[rec : rec+4]) {
			case "EBLC", "EBDT", "EBSC", "bloc", "bdat":
				if !copied {
					b, copied = slices.Clone(b), true
				}
				b[rec] = 'x'
			}
		}
	}
	return b
}

type fontCacheEntry struct {
	Size     int64    `json:"size"`
	ModTime  int64    `json:"mtime"`
	Families []string `json:"families"`
}

func fontCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "officelite", "fontindex.json")
}

func (l *FontLibrary) scan() {
	cache := map[string]fontCacheEntry{}
	cachePath := fontCachePath()
	if b, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	changed := false
	for _, dir := range l.dirs {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(p)) {
			case ".ttf", ".otf", ".ttc", ".otc":
			default:
				return nil // .fon bitmap fonts and other files
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			e, ok := cache[p]
			if !ok || e.Size != info.Size() || e.ModTime != info.ModTime().Unix() {
				e = fontCacheEntry{Size: info.Size(), ModTime: info.ModTime().Unix(), Families: readFamilies(p)}
				cache[p] = e
				changed = true
			}
			for _, fam := range e.Families {
				k := strings.ToLower(fam)
				l.index[k] = append(l.index[k], p)
			}
			return nil
		})
	}
	if changed && cachePath != "" {
		if b, err := json.Marshal(cache); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
			_ = os.WriteFile(cachePath, b, 0o644)
		}
	}
}

// readFamilies returns the family names (name IDs 1 and 16) of every font in
// an OpenType/TrueType file or collection, reading only the headers.
func readFamilies(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	head := make([]byte, 12)
	if _, err := f.ReadAt(head, 0); err != nil {
		return nil
	}
	offsets := []int64{0}
	if string(head[:4]) == "ttcf" {
		n := int64(binary.BigEndian.Uint32(head[8:]))
		if n <= 0 || n > 256 {
			return nil
		}
		b := make([]byte, 4*n)
		if _, err := f.ReadAt(b, 12); err != nil {
			return nil
		}
		offsets = offsets[:0]
		for i := range n {
			offsets = append(offsets, int64(binary.BigEndian.Uint32(b[4*i:])))
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, off := range offsets {
		for _, fam := range fontFamilies(f, off) {
			if !seen[fam] {
				seen[fam] = true
				out = append(out, fam)
			}
		}
	}
	return out
}

func fontFamilies(r io.ReaderAt, off int64) []string {
	hdr := make([]byte, 12)
	if _, err := r.ReadAt(hdr, off); err != nil {
		return nil
	}
	numTables := int(binary.BigEndian.Uint16(hdr[4:]))
	dir := make([]byte, 16*numTables)
	if _, err := r.ReadAt(dir, off+12); err != nil {
		return nil
	}
	for i := range numTables {
		rec := dir[16*i:]
		if string(rec[:4]) != "name" {
			continue
		}
		tOff := int64(binary.BigEndian.Uint32(rec[8:]))
		tLen := binary.BigEndian.Uint32(rec[12:])
		if tLen > 1<<20 {
			return nil
		}
		tbl := make([]byte, tLen)
		if _, err := r.ReadAt(tbl, tOff); err != nil && err != io.EOF {
			return nil
		}
		return parseNameTable(tbl)
	}
	return nil
}

func parseNameTable(t []byte) []string {
	if len(t) < 6 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(t[2:]))
	strOff := int(binary.BigEndian.Uint16(t[4:]))
	var out []string
	for i := range count {
		r := t[6+12*i:]
		if len(r) < 12 {
			break
		}
		platform := binary.BigEndian.Uint16(r[0:])
		lang := binary.BigEndian.Uint16(r[4:])
		nameID := binary.BigEndian.Uint16(r[6:])
		n := int(binary.BigEndian.Uint16(r[8:]))
		o := strOff + int(binary.BigEndian.Uint16(r[10:]))
		if (nameID != 1 && nameID != 16) || o+n > len(t) {
			continue
		}
		s := t[o : o+n]
		switch {
		case platform == 3 && (lang == 0x409 || lang == 0): // Windows, UTF-16BE
			u := make([]uint16, len(s)/2)
			for k := range u {
				u[k] = binary.BigEndian.Uint16(s[2*k:])
			}
			out = append(out, string(utf16.Decode(u)))
		case platform == 1 && lang == 0: // Mac Roman, English
			out = append(out, string(s))
		}
	}
	return out
}
