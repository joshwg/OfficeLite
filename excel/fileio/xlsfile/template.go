// template.go decodes a shared formula (SHRFMLA) once for all the cells
// that share it. The record's references are offsets from each cell, so
// the decoder writes those as placeholders, and each cell's text is the
// literal runs with its own references put back: the same text decoding
// the record at that cell gives, without decoding it again (100K cells
// sharing an 8 KB record took minutes).

package xlsfile

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// refHoleLen is the length of a reference placeholder: that of the
// longest reference text that depends on the formula's cell
// ("$XFD$65536:$XFD$65536"), so a decode with placeholders hits
// excel.XlsMaxFormulaText no later than a decode at any cell would.
const refHoleLen = 21

// refHole is a reference whose text depends on the formula's cell: the
// fields of its tRefN/tAreaN/tRef3d/tArea3d token.
type refHole struct {
	area                 bool
	rw1, rw2, col1, col2 int
}

// hole lists h and returns its placeholder: NUL, its index in 19 digits,
// SOH.
func (d *decoder) hole(h refHole) string {
	*d.holes = append(*d.holes, h)
	return fmt.Sprintf("\x00%019d\x01", len(*d.holes)-1)
}

// fmlaTemplate is a shared formula decoded once: literal runs between the
// references that depend on the cell.
type fmlaTemplate struct {
	rgce, extra []byte
	slow        bool  // couldn't be split: decode at every cell
	err         error // the decode's error, the same at every cell
	lits        []string
	refs        []refHole // len(lits)-1 of them
	size        int       // the literal runs' total length
}

// formulaTemplate decodes a shared formula's rgce once, at the cell
// row, col (whose warnings it gives, as the first cell's decode did).
func (p *parser) formulaTemplate(rgce, extra []byte, row, col int) *fmlaTemplate {
	t := &fmlaTemplate{rgce: rgce, extra: extra}
	// A plain decode first: its errors don't depend on the cell (except
	// for the length cap), and a NUL in it (from a string in the file)
	// would pass for a placeholder.
	plain, err := p.decodeFormula(rgce, extra, row, col, true)
	switch {
	case errors.Is(err, errFormulaTooLong):
		t.slow = true
		return t
	case err != nil:
		t.err = err
		return t
	case strings.IndexByte(plain, 0) >= 0:
		t.slow = true
		return t
	}
	var holes []refHole
	d := decoder{p: p, row: row, col: col, rel: true, warn: p.warnf, holes: &holes}
	s, err := d.decode(rgce, extra)
	if err != nil {
		t.slow = true
		return t
	}
	last, next := 0, 0
	for {
		j := strings.IndexByte(s[last:], 0)
		if j < 0 {
			break
		}
		j += last
		end := j + refHoleLen
		if end > len(s) || s[end-1] != 1 {
			t.slow = true
			return t
		}
		k, err := strconv.Atoi(s[j+1 : end-1])
		if err != nil || k < next || k >= len(holes) {
			t.slow = true
			return t
		}
		t.lits = append(t.lits, s[last:j])
		t.refs = append(t.refs, holes[k])
		last, next = end, k+1
	}
	t.lits = append(t.lits, s[last:])
	for _, l := range t.lits {
		t.size += len(l)
	}
	return t
}

// at returns the formula's text at the cell row, col: what decodeFormula
// gives there.
func (t *fmlaTemplate) at(p *parser, row, col int) (string, error) {
	switch {
	case t.slow:
		return p.decodeFormula(t.rgce, t.extra, row, col, true)
	case t.err != nil:
		return "", t.err
	}
	d := decoder{p: p, row: row, col: col, rel: true, warn: p.warnf}
	var b strings.Builder
	b.Grow(t.size + 12*len(t.refs))
	for i, h := range t.refs {
		b.WriteString(t.lits[i])
		if h.area {
			b.WriteString(d.areaRef(h.rw1, h.rw2, h.col1, h.col2, true))
		} else {
			b.WriteString(d.cellRef(h.rw1, h.col1, true))
		}
	}
	b.WriteString(t.lits[len(t.refs)])
	return b.String(), nil
}
