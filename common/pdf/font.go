// Fonts: resolving a FontSpec to a chain of embeddable faces with
// fallbacks, and measuring text with it.

package pdf

import (
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
)

// FontSource finds the font files of a family, best first (a library may
// add metric-compatible substitutes after the family's own files).
type FontSource interface {
	FontFiles(family string) []string
}

// FontSpec names a font: a family and a style.
type FontSpec struct {
	Family       string
	Bold, Italic bool
}

// fileCache keeps parsed font files for the life of the process: the same
// files are embedded again on every export.
var fileCache = struct {
	sync.Mutex
	files map[string][]*face
}{files: map[string][]*face{}}

// loadFile returns the faces of the font file at path, parsed once and
// cached (nil when it can't be read or parsed).
func loadFile(path string) []*face {
	fileCache.Lock()
	defer fileCache.Unlock()
	if fs, ok := fileCache.files[path]; ok {
		return fs
	}
	var fs []*face
	if b, err := os.ReadFile(path); err == nil {
		fs, _ = parseFaces(b)
	}
	fileCache.files[path] = fs
	return fs
}

// goFaces is the built-in Go fonts by {bold, italic}, parsed once.
var goFaces = sync.OnceValue(func() map[[2]bool]*face {
	out := map[[2]bool]*face{}
	for k, b := range map[[2]bool][]byte{
		{false, false}: goregular.TTF, {true, false}: gobold.TTF,
		{false, true}: goitalic.TTF, {true, true}: gobolditalic.TTF,
	} {
		if fs, err := parseFaces(b); err == nil && len(fs) == 1 {
			out[k] = fs[0]
		}
	}
	return out
})

// member is one font of a Font's fallback chain, with the styling that
// must be synthesized because the face lacks it.
type member struct {
	face        *face
	fakeBold    bool
	fakeItalic  bool
	descendants *embedded
}

// Font is a font chain: the requested family's best face, then fallback
// families for characters it lacks, ending with the Go fonts (always
// available). Characters are drawn with the first member that has them.
type Font struct {
	doc   *Document
	Spec  FontSpec
	chain []*member
}

// Font returns the font for spec, resolving its files on first use.
func (d *Document) Font(spec FontSpec) *Font {
	if f, ok := d.fonts[spec]; ok {
		return f
	}
	f := &Font{doc: d, Spec: spec}
	seen := map[*face]bool{}
	add := func(fc *face) {
		if fc == nil || seen[fc] || !fc.embeddable() {
			return
		}
		seen[fc] = true
		f.chain = append(f.chain, &member{
			face:        fc,
			fakeBold:    spec.Bold && !fc.bold(),
			fakeItalic:  spec.Italic && !fc.italic,
			descendants: d.embed(fc),
		})
	}
	families := []string{spec.Family}
	families = append(families, d.Fallbacks...)
	for _, fam := range families {
		if fam == "" {
			continue
		}
		add(d.findFace(fam, spec.Bold, spec.Italic))
	}
	add(goFaces()[[2]bool{spec.Bold, spec.Italic}])
	d.fonts[spec] = f
	return f
}

// findFace picks the face of family closest to the wanted style, nil if
// no file has the family or none may be embedded.
func (d *Document) findFace(family string, bold, italic bool) *face {
	if d.Fonts == nil {
		return nil
	}
	key := strings.ToLower(family)
	var cands []*face
	if cs, ok := d.famCache[key]; ok {
		cands = cs
	} else {
		var named, other []*face
		for _, p := range d.Fonts.FontFiles(family) {
			for _, fc := range loadFile(p) {
				if !fc.embeddable() {
					continue
				}
				if fc.hasFamily(family) {
					named = append(named, fc)
				} else {
					other = append(other, fc) // a substitute family
				}
			}
		}
		cands = named
		if len(cands) == 0 {
			// Use the first substitute family only, all its styles.
			for _, fc := range other {
				if len(cands) == 0 || sameFamily(cands[0], fc) {
					cands = append(cands, fc)
				}
			}
		}
		d.famCache[key] = cands
	}
	var best *face
	bestScore := -1
	for _, fc := range cands {
		s := 0
		if fc.bold() == bold {
			s += 4
		}
		if fc.italic == italic {
			s += 2
		}
		// Prefer the regular weight (400) over light or semibold ones.
		if w := fc.weight; (bold && w == 700) || (!bold && w == 400) {
			s++
		}
		if s > bestScore {
			best, bestScore = fc, s
		}
	}
	return best
}

// sameFamily reports whether b has any of a's family names.
func sameFamily(a, b *face) bool {
	for _, n := range a.family {
		if b.hasFamily(n) {
			return true
		}
	}
	return false
}

// pick returns the chain member that draws r (the first one with a glyph;
// the first member when none has it) and the glyph.
func (f *Font) pick(r rune) (*member, uint16) {
	for _, m := range f.chain {
		if g := m.face.glyph(r); g != 0 {
			return m, g
		}
	}
	m := f.chain[0]
	return m, m.face.glyph(r)
}

// Advance is the width of r at size (points).
func (f *Font) Advance(r rune, size float64) float64 {
	m, g := f.pick(r)
	return m.face.advance(g) * size / 1000
}

// Measure is the width of s at size (points), without kerning.
func (f *Font) Measure(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		w += f.Advance(r, size)
	}
	return w
}

// Ascent is the primary face's ascent at size, in points.
func (f *Font) Ascent(size float64) float64 {
	fc := f.chain[0].face
	return fc.ascent / fc.unitsPerEm * size
}

// Descent is the primary face's descent at size, in points, positive
// (below the baseline).
func (f *Font) Descent(size float64) float64 {
	fc := f.chain[0].face
	return -fc.descent / fc.unitsPerEm * size
}

// Embedded reports the family actually embedded for the font's own
// characters (after substitution and license fallbacks).
func (f *Font) Embedded() string {
	if fs := f.chain[0].face.family; len(fs) > 0 {
		return fs[0]
	}
	return f.chain[0].face.psName
}
