// Writing embedded fonts: the Type0/CIDFont objects, font descriptor,
// font program and ToUnicode map for each face a document uses.

package pdf

import (
	"crypto/md5"
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"
)

// font writes an embedded face as a Type0 font with Identity-H encoding:
// the Type0 dictionary, its CIDFont with glyph widths, the font
// descriptor, the (subsetted) font program and the ToUnicode map.
func (o *objWriter) font(e *embedded) error {
	f := e.face
	cid, desc, file, tounicode := o.alloc(), o.alloc(), o.alloc(), o.alloc()
	gids := make([]uint16, 0, len(e.used))
	for g := range e.used {
		gids = append(gids, g)
	}
	slices.Sort(gids)

	// The font program: a TrueType subset when allowed, else the whole
	// font (CFF outlines are embedded whole as OpenType).
	var prog []byte
	subset := false
	if !f.cff && f.subsettable() {
		keep := make(map[uint16]bool, len(gids))
		chars := map[rune]uint16{}
		for _, g := range gids {
			keep[g] = true
			if rs := []rune(e.used[g]); len(rs) == 1 {
				chars[rs[0]] = g
			}
		}
		if b, err := subsetTrueType(f, keep, chars); err == nil {
			prog, subset = b, true
		}
	}
	if prog == nil {
		prog = sfntFile(f)
	}

	base := psName(f)
	if subset {
		base = subsetTag(gids, base) + "+" + base
	}
	o.object(e.obj, fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H /DescendantFonts [%s] /ToUnicode %s >>",
		base, ref(cid), ref(tounicode)))

	// Widths, grouped into runs of consecutive glyphs.
	var w strings.Builder
	for i := 0; i < len(gids); {
		j := i
		for j+1 < len(gids) && gids[j+1] == gids[j]+1 {
			j++
		}
		fmt.Fprintf(&w, "%d [", gids[i])
		for k := i; k <= j; k++ {
			if k > i {
				w.WriteByte(' ')
			}
			w.WriteString(num(f.advance(gids[k])))
		}
		w.WriteString("] ")
		i = j + 1
	}
	subtype, extra := "CIDFontType2", " /CIDToGIDMap /Identity"
	if f.cff {
		subtype, extra = "CIDFontType0", ""
	}
	o.object(cid, fmt.Sprintf("<< /Type /Font /Subtype /%s /BaseFont /%s /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor %s /DW %s /W [%s]%s >>",
		subtype, base, ref(desc), num(f.advance(0)), strings.TrimSpace(w.String()), extra))

	scale := func(v float64) string { return num(v * 1000 / f.unitsPerEm) }
	flags := 4 // symbolic: the glyphs aren't the standard Latin set
	if f.fixedPitch {
		flags |= 1
	}
	if f.italic || f.italicAngle != 0 {
		flags |= 64
	}
	stemV := 80
	if f.bold() {
		stemV = 140
	}
	fileKey := "/FontFile2"
	if f.cff {
		fileKey = "/FontFile3"
	}
	o.object(desc, fmt.Sprintf("<< /Type /FontDescriptor /FontName /%s /Flags %d /FontBBox [%s %s %s %s] /ItalicAngle %s /Ascent %s /Descent %s /CapHeight %s /StemV %d %s %s >>",
		base, flags, scale(f.bbox[0]), scale(f.bbox[1]), scale(f.bbox[2]), scale(f.bbox[3]),
		num(f.italicAngle), scale(f.ascent), scale(f.descent), scale(f.capHeight), stemV, fileKey, ref(file)))
	if f.cff {
		o.stream(file, "/Subtype /OpenType ", prog, true)
	} else {
		o.stream(file, fmt.Sprintf("/Length1 %d ", len(prog)), prog, true)
	}
	o.stream(tounicode, "", toUnicodeCMap(gids, e.used), true)
	return o.err
}

// psName is the face's PostScript name, made safe for a PDF name.
func psName(f *face) string {
	n := f.psName
	if n == "" && len(f.family) > 0 {
		n = f.family[0]
	}
	var b strings.Builder
	for _, r := range n {
		if r > 0x20 && r < 0x7F && !strings.ContainsRune("()<>[]{}/%#", r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "Font"
	}
	return b.String()
}

// subsetTag is the six-letter prefix naming a font subset, derived from
// the glyphs it holds.
func subsetTag(gids []uint16, name string) string {
	h := md5.New()
	h.Write([]byte(name))
	for _, g := range gids {
		h.Write([]byte{byte(g >> 8), byte(g)})
	}
	sum := h.Sum(nil)
	tag := make([]byte, 6)
	for i := range tag {
		tag[i] = 'A' + sum[i]%26
	}
	return string(tag)
}

// toUnicodeCMap maps each glyph to the text it was drawn for, so the PDF's
// text can be copied and searched.
func toUnicodeCMap(gids []uint16, text map[uint16]string) []byte {
	var b strings.Builder
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	var entries []string
	for _, g := range gids {
		s := text[g]
		if s == "" {
			continue
		}
		var u strings.Builder
		for _, c := range utf16.Encode([]rune(s)) {
			fmt.Fprintf(&u, "%04X", c)
		}
		entries = append(entries, fmt.Sprintf("<%04X> <%s>", g, u.String()))
	}
	for len(entries) > 0 {
		n := min(len(entries), 100)
		fmt.Fprintf(&b, "%d beginbfchar\n%s\nendbfchar\n", n, strings.Join(entries[:n], "\n"))
		entries = entries[n:]
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return []byte(b.String())
}
