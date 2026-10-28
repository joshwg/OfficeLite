package pdf

import (
	"encoding/binary"
	"errors"
	"strings"
	"sync"
	"unicode/utf16"
)

// face is one font of an OpenType/TrueType file (or collection), parsed
// just enough to embed it: the table directory, glyph advances, the
// character map and the metrics a font descriptor needs.
type face struct {
	tables     map[string][]byte
	cff        bool // PostScript (CFF) outlines rather than TrueType
	unitsPerEm float64
	numGlyphs  int
	advances   []uint16 // per glyph, font units
	cmap       []byte   // the chosen cmap subtable
	cmapFmt    uint16
	symbol     bool // (3,0) symbol cmap: characters live at U+F000+

	// Metrics in font units.
	ascent, descent, capHeight float64
	bbox                       [4]float64
	italicAngle                float64
	fixedPitch                 bool

	weight int  // OS/2 usWeightClass
	italic bool // fsSelection / macStyle
	fsType uint16
	family []string // name IDs 1 and 16
	psName string   // name ID 6

	mu       sync.Mutex // faces are shared between documents
	glyphMap map[rune]uint16
}

var be = binary.BigEndian

// parseFaces parses every font in a file (one, or several in a .ttc).
func parseFaces(b []byte) ([]*face, error) {
	if len(b) < 12 {
		return nil, errors.New("pdf: font file too short")
	}
	offsets := []int{0}
	if string(b[:4]) == "ttcf" {
		n := int(be.Uint32(b[8:]))
		if n <= 0 || n > 256 || 12+4*n > len(b) {
			return nil, errors.New("pdf: bad font collection")
		}
		offsets = offsets[:0]
		for i := range n {
			offsets = append(offsets, int(be.Uint32(b[12+4*i:])))
		}
	}
	var out []*face
	for _, off := range offsets {
		f, err := parseFace(b, off)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func parseFace(b []byte, off int) (*face, error) {
	if off+12 > len(b) {
		return nil, errors.New("pdf: bad font offset")
	}
	f := &face{tables: map[string][]byte{}, glyphMap: map[rune]uint16{}}
	switch tag := string(b[off : off+4]); tag {
	case "\x00\x01\x00\x00", "true":
	case "OTTO":
		f.cff = true
	default:
		return nil, errors.New("pdf: unsupported font format")
	}
	n := int(be.Uint16(b[off+4:]))
	for i := range n {
		rec := off + 12 + 16*i
		if rec+16 > len(b) {
			return nil, errors.New("pdf: truncated table directory")
		}
		tOff, tLen := int(be.Uint32(b[rec+8:])), int(be.Uint32(b[rec+12:]))
		if tOff < 0 || tLen < 0 || tOff+tLen > len(b) {
			continue
		}
		f.tables[string(b[rec:rec+4])] = b[tOff : tOff+tLen]
	}
	head, hhea, maxp := f.tables["head"], f.tables["hhea"], f.tables["maxp"]
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 {
		return nil, errors.New("pdf: font lacks head/hhea/maxp")
	}
	f.unitsPerEm = float64(be.Uint16(head[18:]))
	if f.unitsPerEm == 0 {
		f.unitsPerEm = 1000
	}
	for i := range 4 {
		f.bbox[i] = float64(int16(be.Uint16(head[36+2*i:])))
	}
	f.italic = be.Uint16(head[44:])&2 != 0
	f.weight = 400
	if be.Uint16(head[44:])&1 != 0 {
		f.weight = 700
	}
	f.numGlyphs = int(be.Uint16(maxp[4:]))
	f.ascent = float64(int16(be.Uint16(hhea[4:])))
	f.descent = float64(int16(be.Uint16(hhea[6:])))
	f.capHeight = f.ascent * 0.7

	// Advances: numberOfHMetrics full records, then the last advance
	// repeats.
	nh := int(be.Uint16(hhea[34:]))
	hmtx := f.tables["hmtx"]
	f.advances = make([]uint16, f.numGlyphs)
	var last uint16
	for g := range f.numGlyphs {
		if g < nh && 4*g+2 <= len(hmtx) {
			last = be.Uint16(hmtx[4*g:])
		}
		f.advances[g] = last
	}

	if os2 := f.tables["OS/2"]; len(os2) >= 78 {
		f.weight = int(be.Uint16(os2[4:]))
		f.fsType = be.Uint16(os2[8:])
		sel := be.Uint16(os2[62:])
		f.italic = sel&1 != 0
		if sel&(1<<7) != 0 { // USE_TYPO_METRICS
			f.ascent = float64(int16(be.Uint16(os2[68:])))
			f.descent = float64(int16(be.Uint16(os2[70:])))
		}
		if be.Uint16(os2[0:]) >= 2 && len(os2) >= 90 {
			if ch := float64(int16(be.Uint16(os2[88:]))); ch > 0 {
				f.capHeight = ch
			}
		}
	}
	if post := f.tables["post"]; len(post) >= 16 {
		f.italicAngle = float64(int32(be.Uint32(post[4:]))) / 65536
		f.fixedPitch = be.Uint32(post[12:]) != 0
	}
	f.readNames()
	f.chooseCmap()
	if f.cmap == nil {
		return nil, errors.New("pdf: font has no Unicode cmap")
	}
	return f, nil
}

func (f *face) readNames() {
	t := f.tables["name"]
	if len(t) < 6 {
		return
	}
	count := int(be.Uint16(t[2:]))
	strOff := int(be.Uint16(t[4:]))
	for i := range count {
		if 6+12*i+12 > len(t) {
			break
		}
		r := t[6+12*i:]
		platform, lang, id := be.Uint16(r[0:]), be.Uint16(r[4:]), be.Uint16(r[6:])
		n, o := int(be.Uint16(r[8:])), strOff+int(be.Uint16(r[10:]))
		if (id != 1 && id != 16 && id != 6) || o+n > len(t) {
			continue
		}
		var s string
		switch {
		case platform == 3 && (lang == 0x409 || lang == 0):
			u := make([]uint16, n/2)
			for k := range u {
				u[k] = be.Uint16(t[o+2*k:])
			}
			s = string(utf16.Decode(u))
		case platform == 1 && lang == 0:
			s = string(t[o : o+n])
		default:
			continue
		}
		if id == 6 {
			if f.psName == "" {
				f.psName = s
			}
		} else {
			f.family = append(f.family, s)
		}
	}
}

// hasFamily reports whether name is one of the face's family names.
func (f *face) hasFamily(name string) bool {
	for _, fam := range f.family {
		if strings.EqualFold(fam, name) {
			return true
		}
	}
	return false
}

// chooseCmap picks the best Unicode cmap subtable: full repertoire
// (3,10)/(0,4+) format 12, then BMP (3,1)/(0,x) format 4, then symbol.
func (f *face) chooseCmap() {
	t := f.tables["cmap"]
	if len(t) < 4 {
		return
	}
	n := int(be.Uint16(t[2:]))
	best := -1
	for i := range n {
		if 4+8*i+8 > len(t) {
			break
		}
		r := t[4+8*i:]
		pid, eid, off := be.Uint16(r[0:]), be.Uint16(r[2:]), int(be.Uint32(r[4:]))
		if off+4 > len(t) {
			continue
		}
		format := be.Uint16(t[off:])
		if format != 4 && format != 12 {
			continue
		}
		score := 0
		switch {
		case pid == 3 && eid == 10 && format == 12, pid == 0 && format == 12:
			score = 4
		case pid == 3 && eid == 1 && format == 4, pid == 0 && format == 4:
			score = 3
		case pid == 3 && eid == 0 && format == 4:
			score = 1
		default:
			continue
		}
		if score > best {
			best = score
			f.cmap, f.cmapFmt = t[off:], format
			f.symbol = score == 1
		}
	}
}

// glyph returns the glyph of r, 0 if the font lacks it.
func (f *face) glyph(r rune) uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.glyphMap[r]; ok {
		return g
	}
	g := f.lookup(r)
	if g == 0 && f.symbol && r < 0x100 {
		g = f.lookup(0xF000 + r)
	}
	if int(g) >= f.numGlyphs {
		g = 0
	}
	f.glyphMap[r] = g
	return g
}

func (f *face) lookup(r rune) uint16 {
	t := f.cmap
	switch f.cmapFmt {
	case 4:
		if r > 0xFFFF || len(t) < 14 {
			return 0
		}
		segX2 := int(be.Uint16(t[6:]))
		ends, starts := 14, 16+segX2
		deltas, ranges := starts+segX2, starts+2*segX2
		if ranges+segX2 > len(t) {
			return 0
		}
		c := uint16(r)
		// Binary search the end codes.
		lo, hi := 0, segX2/2
		for lo < hi {
			m := (lo + hi) / 2
			if be.Uint16(t[ends+2*m:]) < c {
				lo = m + 1
			} else {
				hi = m
			}
		}
		if lo >= segX2/2 {
			return 0
		}
		i := lo
		start := be.Uint16(t[starts+2*i:])
		if c < start {
			return 0
		}
		delta := be.Uint16(t[deltas+2*i:])
		ro := int(be.Uint16(t[ranges+2*i:]))
		if ro == 0 {
			return c + delta
		}
		p := ranges + 2*i + ro + 2*int(c-start)
		if p+2 > len(t) {
			return 0
		}
		if g := be.Uint16(t[p:]); g != 0 {
			return g + delta
		}
		return 0
	case 12:
		if len(t) < 16 {
			return 0
		}
		n := int(be.Uint32(t[12:]))
		lo, hi := 0, n
		c := uint32(r)
		for lo < hi {
			m := (lo + hi) / 2
			p := 16 + 12*m
			if p+12 > len(t) {
				return 0
			}
			start, end := be.Uint32(t[p:]), be.Uint32(t[p+4:])
			switch {
			case c < start:
				hi = m
			case c > end:
				lo = m + 1
			default:
				return uint16(be.Uint32(t[p+8:]) + c - start)
			}
		}
	}
	return 0
}

// advance is glyph g's advance width in 1/1000 em.
func (f *face) advance(g uint16) float64 {
	if int(g) >= len(f.advances) {
		return 0
	}
	return float64(f.advances[g]) * 1000 / f.unitsPerEm
}

// embeddable reports whether the font's license allows embedding it in a
// document (OS/2 fsType: not "restricted license" and not "bitmap only").
func (f *face) embeddable() bool {
	return f.fsType&0x000F != 0x0002 && f.fsType&0x0200 == 0
}

// subsettable reports whether the license allows embedding a subset.
func (f *face) subsettable() bool { return f.fsType&0x0100 == 0 }

func (f *face) bold() bool { return f.weight >= 600 }
