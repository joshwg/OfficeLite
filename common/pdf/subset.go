package pdf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
)

// subsetTrueType returns a TrueType font holding only the glyphs in keep
// (plus .notdef and the components of composite glyphs). Glyph IDs are
// unchanged, so a CIDFontType2 can use the identity CID-to-GID mapping;
// dropped glyphs simply become empty. Only the tables a PDF viewer needs
// are kept.
func subsetTrueType(f *face, keep map[uint16]bool, chars map[rune]uint16) ([]byte, error) {
	head, loca, glyf := f.tables["head"], f.tables["loca"], f.tables["glyf"]
	if len(head) < 54 || loca == nil || glyf == nil || len(f.tables["hhea"]) < 36 || len(f.tables["maxp"]) < 6 || f.tables["hmtx"] == nil {
		return nil, errors.New("pdf: not a TrueType-outline font")
	}
	long := be.Uint16(head[50:]) != 0
	at := func(i int) int {
		if long {
			if 4*i+4 > len(loca) {
				return -1
			}
			return int(be.Uint32(loca[4*i:]))
		}
		if 2*i+2 > len(loca) {
			return -1
		}
		return 2 * int(be.Uint16(loca[2*i:]))
	}
	// A glyph ends where the next glyph's data starts. That is usually
	// loca[g+1], but fonts of a collection that share glyph data (the
	// Korean .ttc fonts) store glyphs out of order, so use the next higher
	// offset of any glyph.
	starts := make([]int, 0, f.numGlyphs+1)
	for i := 0; i <= f.numGlyphs; i++ {
		if a := at(i); a >= 0 {
			starts = append(starts, a)
		}
	}
	starts = append(starts, len(glyf))
	slices.Sort(starts)
	starts = slices.Compact(starts)
	offset := func(g int) (int, int, bool) {
		a, next := at(g), at(g+1)
		if a < 0 || next < 0 || a > len(glyf) {
			return 0, 0, false
		}
		if next == a {
			return a, a, true // empty glyph
		}
		i, _ := slices.BinarySearch(starts, a+1)
		if i >= len(starts) {
			return 0, 0, false
		}
		return a, min(starts[i], len(glyf)), true
	}

	// Close over composite glyph components.
	want := map[uint16]bool{0: true}
	queue := []uint16{0}
	for g := range keep {
		if !want[g] && int(g) < f.numGlyphs {
			want[g] = true
			queue = append(queue, g)
		}
	}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		a, b, ok := offset(int(g))
		if !ok || b-a < 10 || int16(be.Uint16(glyf[a:])) >= 0 {
			continue // empty or simple glyph
		}
		p := a + 10
		for p+4 <= b {
			flags := be.Uint16(glyf[p:])
			comp := be.Uint16(glyf[p+2:])
			if int(comp) < f.numGlyphs && !want[comp] {
				want[comp] = true
				queue = append(queue, comp)
			}
			p += 4
			if flags&0x0001 != 0 { // ARG_1_AND_2_ARE_WORDS
				p += 4
			} else {
				p += 2
			}
			switch {
			case flags&0x0008 != 0: // WE_HAVE_A_SCALE
				p += 2
			case flags&0x0040 != 0: // X_AND_Y_SCALE
				p += 4
			case flags&0x0080 != 0: // TWO_BY_TWO
				p += 8
			}
			if flags&0x0020 == 0 { // MORE_COMPONENTS
				break
			}
		}
	}

	// The font ends at the highest glyph kept: glyph IDs stay the same,
	// and loca and hmtx get shorter.
	n := 1
	for g := range want {
		n = max(n, int(g)+1)
	}

	// New glyf and (long) loca.
	var newGlyf bytes.Buffer
	newLoca := make([]byte, 4*(n+1))
	for g := 0; g < n; g++ {
		binary.BigEndian.PutUint32(newLoca[4*g:], uint32(newGlyf.Len()))
		if !want[uint16(g)] {
			continue
		}
		if a, b, ok := offset(g); ok && b > a {
			newGlyf.Write(glyf[a:b])
			for newGlyf.Len()%4 != 0 {
				newGlyf.WriteByte(0)
			}
		}
	}
	binary.BigEndian.PutUint32(newLoca[4*n:], uint32(newGlyf.Len()))

	newHead := slices.Clone(head)
	binary.BigEndian.PutUint16(newHead[50:], 1) // long loca
	binary.BigEndian.PutUint32(newHead[8:], 0)  // checkSumAdjustment, set below

	// hmtx: the first nh glyphs' advances and side bearings, then side
	// bearings only.
	hhea, hmtx := slices.Clone(f.tables["hhea"]), f.tables["hmtx"]
	nh := min(int(be.Uint16(hhea[34:])), n)
	binary.BigEndian.PutUint16(hhea[34:], uint16(nh))
	newHmtx := make([]byte, 4*nh+2*(n-nh))
	copy(newHmtx, hmtx[:min(len(hmtx), 4*nh)])
	oldNh := int(be.Uint16(f.tables["hhea"][34:]))
	for g := nh; g < n; g++ {
		// Side bearing of glyph g in the old table.
		var src int
		if g < oldNh {
			src = 4*g + 2
		} else {
			src = 4*oldNh + 2*(g-oldNh)
		}
		if src+2 <= len(hmtx) {
			copy(newHmtx[4*nh+2*(g-nh):], hmtx[src:src+2])
		}
	}
	maxp := slices.Clone(f.tables["maxp"])
	binary.BigEndian.PutUint16(maxp[4:], uint16(n))

	tables := map[string][]byte{
		"head": newHead,
		"hhea": hhea,
		"hmtx": newHmtx,
		"maxp": maxp,
		"loca": newLoca,
		"glyf": newGlyf.Bytes(),
		"cmap": cmap4(chars),
	}
	if post := f.tables["post"]; len(post) >= 32 {
		// Format 3: no glyph names.
		p := slices.Clone(post[:32])
		binary.BigEndian.PutUint32(p, 0x00030000)
		tables["post"] = p
	}
	for _, tag := range []string{"cvt ", "fpgm", "prep", "OS/2"} {
		if t, ok := f.tables[tag]; ok {
			tables[tag] = t
		}
	}
	return writeSfnt(tables, 0x00010000), nil
}

// writeSfnt assembles a TrueType file from tables, with correct checksums.
func writeSfnt(tables map[string][]byte, version uint32) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	slices.Sort(tags)
	n := len(tags)
	entrySel := 0
	for 1<<(entrySel+1) <= n {
		entrySel++
	}
	searchRange := (1 << entrySel) * 16
	var out bytes.Buffer
	hdr := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(hdr[0:], version)
	binary.BigEndian.PutUint16(hdr[4:], uint16(n))
	binary.BigEndian.PutUint16(hdr[6:], uint16(searchRange))
	binary.BigEndian.PutUint16(hdr[8:], uint16(entrySel))
	binary.BigEndian.PutUint16(hdr[10:], uint16(n*16-searchRange))
	off := len(hdr)
	headOff := -1
	for i, tag := range tags {
		t := tables[tag]
		rec := hdr[12+16*i:]
		copy(rec, tag)
		binary.BigEndian.PutUint32(rec[4:], checksum(t))
		binary.BigEndian.PutUint32(rec[8:], uint32(off))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(t)))
		if tag == "head" {
			headOff = off
		}
		off += (len(t) + 3) &^ 3
	}
	out.Write(hdr)
	for _, tag := range tags {
		t := tables[tag]
		out.Write(t)
		for range (4 - len(t)%4) % 4 {
			out.WriteByte(0)
		}
	}
	b := out.Bytes()
	if headOff >= 0 {
		binary.BigEndian.PutUint32(b[headOff+8:], 0xB1B0AFBA-checksum(b))
	}
	return b
}

func checksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:])
		sum += binary.BigEndian.Uint32(w[:])
	}
	return sum
}

// sfntFile returns a standalone font file for f (a face of a collection
// becomes a single font), keeping every table.
func sfntFile(f *face) []byte {
	if f.cff {
		return writeSfnt(f.tables, 0x4F54544F) // "OTTO"
	}
	return writeSfnt(f.tables, 0x00010000)
}

// cmap4 builds a (3,1) format 4 cmap for the BMP characters in chars, one
// segment per character. Viewers draw by glyph ID, but some font parsers
// insist on a cmap.
func cmap4(chars map[rune]uint16) []byte {
	var rs []rune
	for r := range chars {
		if r > 0 && r < 0xFFFF && len(rs) < 8000 {
			rs = append(rs, r)
		}
	}
	slices.Sort(rs)
	seg := len(rs) + 1
	sub := make([]byte, 16+8*seg)
	put := func(off int, v uint16) { binary.BigEndian.PutUint16(sub[off:], v) }
	put(0, 4)
	put(2, uint16(len(sub)))
	put(6, uint16(2*seg))
	es := 0
	for 1<<(es+1) <= seg {
		es++
	}
	put(8, uint16(2<<es))
	put(10, uint16(es))
	put(12, uint16(2*seg-2<<es))
	ends, starts := 14, 16+2*seg
	deltas, ranges := starts+2*seg, starts+4*seg
	for i, r := range rs {
		put(ends+2*i, uint16(r))
		put(starts+2*i, uint16(r))
		put(deltas+2*i, chars[r]-uint16(r))
	}
	put(ends+2*len(rs), 0xFFFF)
	put(starts+2*len(rs), 0xFFFF)
	put(deltas+2*len(rs), 1)
	_ = ranges // idRangeOffsets stay 0
	out := make([]byte, 12, 12+len(sub))
	binary.BigEndian.PutUint16(out[2:], 1)  // one subtable
	binary.BigEndian.PutUint16(out[4:], 3)  // Windows
	binary.BigEndian.PutUint16(out[6:], 1)  // Unicode BMP
	binary.BigEndian.PutUint32(out[8:], 12) // offset
	return append(out, sub...)
}
