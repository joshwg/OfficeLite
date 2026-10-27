package docfile

import (
	"strings"
	"unicode/utf16"

	"officelite/word/model"
)

// Number formats (nfc).
var nfcNames = map[byte]string{
	0: "decimal", 1: "upperRoman", 2: "lowerRoman", 3: "upperLetter", 4: "lowerLetter",
	5: "ordinal", 22: "decimalZero", 23: "bullet", 255: "none",
}

func nfcFor(format string) byte {
	for k, v := range nfcNames {
		if v == format {
			return k
		}
	}
	return 0
}

// lstf is a parsed list definition.
type lstf struct {
	lsid   int32
	simple bool
	levels []model.ListLevel
}

// readLists parses PlfLst (with the LVLs after it) and PlfLfo into
// doc.Lists, keyed by the 1-based LFO index used by sprmPIlfo.
func (r *reader) readLists() {
	if !r.fib.has(fcPlfLst) || !r.fib.has(fcPlfLfo) {
		return
	}
	fc := int(r.fib.fcLcb[fcPlfLst][0])
	t := r.table
	if fc+2 > len(t) {
		return
	}
	n := int(i16(t[fc:]))
	p := fc + 2
	defs := make([]*lstf, 0, n)
	for i := 0; i < n && p+28 <= len(t); i++ {
		defs = append(defs, &lstf{lsid: int32(u32(t[p:])), simple: t[p+26]&1 != 0})
		p += 28
	}
	for _, d := range defs {
		cnt := 9
		if d.simple {
			cnt = 1
		}
		for l := 0; l < cnt; l++ {
			lvl, next, ok := r.parseLvl(p)
			if !ok {
				break
			}
			d.levels = append(d.levels, lvl)
			p = next
		}
	}
	byID := map[int32]*lstf{}
	for _, d := range defs {
		byID[d.lsid] = d
	}

	lfo := r.fib.blob(t, fcPlfLfo)
	if len(lfo) < 4 {
		return
	}
	m := int(u32(lfo))
	q := 4
	type lfoEntry struct {
		lsid int32
		clvl int
	}
	var lfos []lfoEntry
	for i := 0; i < m && q+16 <= len(lfo); i++ {
		lfos = append(lfos, lfoEntry{int32(u32(lfo[q:])), int(lfo[q+12])})
		q += 16
	}
	// LFOData with level overrides (start-at only).
	q = int(r.fib.fcLcb[fcPlfLfo][0]) + 4 + 16*m
	for i, e := range lfos {
		d := byID[e.lsid]
		list := &model.List{}
		if d != nil {
			list.Levels = append(list.Levels, d.levels...)
		}
		if q+4 <= len(t) {
			q += 4 // cp
			for k := 0; k < e.clvl && q+8 <= len(t); k++ {
				start := int(int32(u32(t[q:])))
				fl := u32(t[q+4:])
				q += 8
				ilvl := int(fl & 0xF)
				if fl&0x20 != 0 { // fFormatting: a full LVL follows
					lvl, next, ok := r.parseLvl(q)
					if !ok {
						break
					}
					q = next
					if ilvl < len(list.Levels) {
						list.Levels[ilvl] = lvl
					}
				} else if fl&0x10 != 0 && ilvl < len(list.Levels) { // fStartAt
					list.Levels[ilvl].Start = start
				}
			}
		}
		r.doc.Lists[i+1] = list
	}
}

// parseLvl decodes the LVL at offset p of the table stream.
func (r *reader) parseLvl(p int) (model.ListLevel, int, bool) {
	t := r.table
	if p+28 > len(t) {
		return model.ListLevel{}, p, false
	}
	lvl := model.ListLevel{Start: int(int32(u32(t[p:])))}
	nfc := t[p+4]
	lvl.Format = nfcNames[nfc]
	if lvl.Format == "" {
		lvl.Format = "decimal"
	}
	var nums [9]byte
	copy(nums[:], t[p+6:p+15])
	cbChpx, cbPapx := int(t[p+24]), int(t[p+25])
	q := p + 28
	if q+cbPapx+cbChpx+2 > len(t) {
		return lvl, p, false
	}
	var pi papInfo
	r.decodePap(&pi, t[q:q+cbPapx])
	if pi.props.IndentLeft != nil {
		lvl.Indent = *pi.props.IndentLeft
	}
	if pi.props.IndentFirst != nil {
		lvl.Hang = -*pi.props.IndentFirst
	}
	q += cbPapx
	c := &chpInfo{istd: -1}
	r.decodeChp(c, t[q:q+cbChpx])
	lvl.Font = c.props.Font
	q += cbChpx
	cch := int(u16(t[q:]))
	q += 2
	if q+2*cch > len(t) {
		return lvl, p, false
	}
	xst := make([]uint16, cch)
	for i := range xst {
		xst[i] = u16(t[q+2*i:])
	}
	q += 2 * cch
	lvl.Text = lvlText(xst, nums, nfc == 23)
	return lvl, q, true
}

// lvlText turns an LVL xst into "%1." form: the characters at the
// rgbxchNums positions are level placeholders.
func lvlText(xst []uint16, nums [9]byte, bullet bool) string {
	if bullet {
		return string(utf16.Decode(xst))
	}
	place := map[int]bool{}
	for _, n := range nums {
		if n == 0 {
			break
		}
		place[int(n)-1] = true
	}
	var b strings.Builder
	var run []uint16
	flush := func() {
		b.WriteString(string(utf16.Decode(run)))
		run = run[:0]
	}
	for i, c := range xst {
		if place[i] && c < 9 {
			flush()
			b.WriteByte('%')
			b.WriteByte(byte('1' + c))
			continue
		}
		run = append(run, c)
	}
	flush()
	return b.String()
}
