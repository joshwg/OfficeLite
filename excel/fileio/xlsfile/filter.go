// The AutoFilter in BIFF8, read and written: its range is the sheet's
// built-in _FilterDatabase NAME; the sheet has AUTOFILTERINFO (the column
// count) and an AUTOFILTER record per filtered column with up to two
// conditions (DOPERs) or a Top 10, FILTERMODE while rows are filtered, and
// a drop-down drawing object per header cell (written with the notes'
// drawing; see annot.go). Value lists of more than two values are an
// Excel 2007 addition (AUTOFILTER12) .xls can't hold.

package xlsfile

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"

	"officelite/excel/formula"
	"officelite/excel/model"
)

const (
	recFilterMode     = 0x009B
	recAutoFilterInfo = 0x009D
	recAutoFilter     = 0x009E
	recAutoFilter12   = 0x087E

	builtinFilterDatabase = 0x0D

	// AUTOFILTER flags.
	afOr      = 0x0001
	afSimple1 = 0x0004
	afSimple2 = 0x0008
	afTopN    = 0x0010
	afTop     = 0x0020
	afPercent = 0x0040

	// DOPER value types.
	vtNone     = 0x00
	vtRK       = 0x02
	vtNumber   = 0x04
	vtString   = 0x06
	vtBoolErr  = 0x08
	vtBlanks   = 0x0C
	vtNonBlank = 0x0E
)

// signOf maps condition operators to DOPER comparison codes and back.
var signOf = map[model.FilterOp]byte{
	model.OpLess: 1, model.OpEqual: 2, model.OpLessEqual: 3,
	model.OpGreater: 4, model.OpNotEqual: 5, model.OpGreaterEqual: 6,
}

// filterReader collects a sheet's AutoFilter records while it is read.
type filterReader struct {
	info     bool
	cols     []model.FilterColumn // Col counts from the filter's first column
	hiddenHt map[int]float32      // stored heights of hidden rows
	future   bool                 // criteria only Excel 2007's records hold
}

// record handles a record that belongs to the AutoFilter and reports
// whether it did.
func (f *filterReader) record(r record) bool {
	switch r.id {
	case recAutoFilterInfo:
		f.info = true
	case recAutoFilter:
		if fc, ok := parseAutoFilter(r.body); ok {
			f.cols = append(f.cols, fc)
		} else {
			f.future = true
		}
	case recAutoFilter12:
		f.future = true
	case recFilterMode:
	default:
		return false
	}
	return true
}

// parseAutoFilter decodes an AUTOFILTER record; false when its criteria
// live in an AUTOFILTER12 record.
func parseAutoFilter(b []byte) (model.FilterColumn, bool) {
	if len(b) < 24 {
		return model.FilterColumn{}, false
	}
	fc := model.FilterColumn{Col: u16(b, 0)}
	flags := u16(b, 2)
	if flags&afTopN != 0 {
		fc.Kind = model.FilterTop10
		fc.Top10 = model.Top10{Top: flags&afTop != 0, Percent: flags&afPercent != 0, N: float64(flags >> 7)}
		if b[4] == vtNumber || b[4] == vtRK {
			fc.Top10.Val = doperNumber(b[4:14])
		}
		return fc, true
	}
	fc.Kind, fc.And = model.FilterCustom, flags&3 == 0
	off := 24
	for _, d := range [][]byte{b[4:14], b[14:24]} {
		vt, sign := d[0], d[1]
		var op model.FilterOp
		for o, s := range signOf {
			if s == sign {
				op = o
			}
		}
		cd := model.FilterCond{Op: op}
		switch vt {
		case vtNone:
			continue
		case vtRK, vtNumber:
			cd.Val = strconv.FormatFloat(doperNumber(d), 'g', -1, 64)
		case vtString:
			cch := int(d[6])
			if off >= len(b) {
				return fc, false
			}
			s, end := chars(b, off+1, cch, b[off]&1 != 0)
			cd.Val, off = s, end
		case vtBoolErr:
			if d[3] != 0 {
				cd.Val = errorTexts[d[2]]
			} else if d[2] != 0 {
				cd.Val = "TRUE"
			} else {
				cd.Val = "FALSE"
			}
		case vtBlanks:
			cd = model.FilterCond{Op: model.OpEqual}
		case vtNonBlank:
			cd = model.FilterCond{Op: model.OpNotEqual}
		}
		fc.Conds = append(fc.Conds, cd)
	}
	if len(fc.Conds) == 0 {
		return fc, false
	}
	// Equals conditions picked from the drop-down's list are a value
	// list.
	values := flags&3 == afOr && flags&afSimple1 != 0
	for _, cd := range fc.Conds {
		values = values && cd.Op == model.OpEqual && !formula.HasWildcards(cd.Val)
	}
	if values {
		fc.Kind, fc.And = model.FilterValues, false
		for _, cd := range fc.Conds {
			if cd.Val == "" {
				fc.Blanks = true
			} else {
				fc.Values = append(fc.Values, cd.Val)
			}
		}
		fc.Conds = nil
	}
	return fc, true
}

// doperNumber is the RK or IEEE number value of DOPER d.
func doperNumber(d []byte) float64 {
	if d[0] == vtRK {
		return rkValue(u32(d, 2))
	}
	return f64(d, 2)
}

// finishFilter sets up sh's AutoFilter from its records and its
// _FilterDatabase name (sheet itab).
func (p *parser) finishFilter(itab int, sh *model.Sheet, f *filterReader) {
	if !f.info {
		return
	}
	var rng model.Range
	found := false
	for _, n := range p.names {
		if n.builtin && n.name == "_FilterDatabase" && n.itab == itab {
			s, err := p.decodeFormula(n.rgce, n.extra, 0, 0, true)
			if err == nil {
				rng, found = parseAreaRef(s)
			}
		}
	}
	if !found {
		p.warnf("%s: the AutoFilter's range is missing; the filter was dropped", sh.Name)
		return
	}
	af := &model.AutoFilter{Range: rng}
	for _, fc := range f.cols {
		fc.Col += rng.C1
		if fc.Col <= rng.C2 {
			af.SetColumn(fc.Col, &fc)
		}
	}
	sh.AutoFilter = af
	if f.future {
		p.warnf("%s: AutoFilter criteria saved by Excel 2007 or later are not read; the rows they hide stay hidden", sh.Name)
	}
	if af.Active() || f.future {
		sh.AdoptFilteredRows(f.hiddenHt)
	}
}

// parseAreaRef reads the range of "Sheet1!$A$1:$C$9".
func parseAreaRef(s string) (model.Range, bool) {
	if i := strings.LastIndexByte(s, '!'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ReplaceAll(s, "$", "")
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		b = a
	}
	r1, c1, ok1 := parseCell(a)
	r2, c2, ok2 := parseCell(b)
	return model.Range{R1: r1, C1: c1, R2: r2, C2: c2}, ok1 && ok2
}

// parseCell parses an A1-style cell name without $ into 0-based row and
// column.
func parseCell(s string) (row, col int, ok bool) {
	i := strings.IndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if i <= 0 {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[i:])
	col = model.ParseColName(s[:i])
	return n - 1, col, err == nil && n > 0 && col >= 0
}

// --- writing ---

// filterFits reports whether the AutoFilter can be written to .xls.
func filterFits(sh *model.Sheet) bool {
	f := sh.AutoFilter
	return f != nil && f.Range.R2 < xlsMaxRows && f.Range.C2 < xlsMaxCols
}

// filterNames adds the hidden _FilterDatabase NAME of each sheet with an
// AutoFilter.
func (x *writer) filterNames() {
	for si, sh := range x.sheets {
		if sh.AutoFilter == nil {
			continue
		}
		if !filterFits(sh) {
			x.issue("%s: the AutoFilter lies beyond row 65,536 or column IV and will not be saved", sh.Name)
			continue
		}
		r := sh.AutoFilter.Range
		abs := func(row, col int) string { return "$" + model.ColName(col) + "$" + strconv.Itoa(row+1) }
		src := formula.QuoteSheet(x.names[si]) + "!" + abs(r.R1, r.C1) + ":" + abs(r.R2, r.C2)
		rgce, extra, err := compileFormula(x, src, si, true)
		if err != nil {
			continue
		}
		x.nameRecs = append(x.nameRecs, nameRec{name: string(rune(builtinFilterDatabase)), sheet: si,
			flags: nameHidden | nameBuiltin, rgce: rgce, extra: extra})
	}
}

// autoFilter writes FILTERMODE, AUTOFILTERINFO and the AUTOFILTER
// records of sh's filter.
func (x *writer) autoFilter(b []byte, sh *model.Sheet) []byte {
	if !filterFits(sh) {
		return b
	}
	f := sh.AutoFilter
	if len(sh.Filtered) > 0 {
		b = rec(b, recFilterMode, nil)
	}
	b = rec(b, recAutoFilterInfo, le(f.Range.C2-f.Range.C1+1))
	for _, fc := range f.Columns {
		if fc.Col < f.Range.C1 || fc.Col > f.Range.C2 {
			continue
		}
		if body, ok := x.autoFilterRecord(sh, fc, fc.Col-f.Range.C1); ok {
			b = rec(b, recAutoFilter, body)
		}
	}
	return b
}

// autoFilterRecord encodes fc as the AUTOFILTER body of filter column
// entry; false (with an issue reported where criteria are lost) when it
// has nothing .xls can hold.
func (x *writer) autoFilterRecord(sh *model.Sheet, fc model.FilterColumn, entry int) ([]byte, bool) {
	var (
		flags int
		conds []model.FilterCond
	)
	switch fc.Kind {
	case model.FilterTop10:
		t := fc.Top10
		n := min(max(int(t.N), 1), 500)
		flags = afTopN | n<<7
		sign := byte(3)
		if t.Top {
			flags |= afTop
			sign = 6
		}
		if t.Percent {
			flags |= afPercent
		}
		d := append([]byte{vtNumber, sign}, binary.LittleEndian.AppendUint64(nil, math.Float64bits(t.Val))...)
		return append(append(le(entry, flags), d...), make([]byte, 10)...), true
	case model.FilterValues:
		n := len(fc.Values)
		if fc.Blanks {
			n++
		}
		if n > 2 || len(fc.Dates) > 0 {
			x.issue("%s: the AutoFilter of column %s shows more values than .xls can hold; it will be saved without criteria (its rows stay hidden)",
				sh.Name, model.ColName(fc.Col))
			return nil, false
		}
		for _, v := range fc.Values {
			conds = append(conds, model.FilterCond{Op: model.OpEqual, Val: v})
		}
		if fc.Blanks {
			conds = append(conds, model.FilterCond{Op: model.OpEqual})
		}
		flags = afOr | afSimple1
		if len(conds) > 1 {
			flags |= afSimple2
		}
	case model.FilterCustom:
		conds = fc.Conds[:min(len(fc.Conds), 2)]
		if !fc.And {
			flags = afOr
		}
	case model.FilterDynamic:
		// Above and Below Average become the comparison they made.
		flags = afOr
		switch fc.Dynamic {
		case "aboveAverage":
			conds = []model.FilterCond{{Op: model.OpGreater, Val: strconv.FormatFloat(fc.DynamicVal, 'g', -1, 64)}}
		case "belowAverage":
			conds = []model.FilterCond{{Op: model.OpLess, Val: strconv.FormatFloat(fc.DynamicVal, 'g', -1, 64)}}
		default:
			x.issue("%s: the date filter of column %s can't be saved in .xls; it will be saved without criteria",
				sh.Name, model.ColName(fc.Col))
			return nil, false
		}
	}
	if len(conds) == 0 {
		return nil, false
	}
	body := le(entry, flags)
	var strs []byte
	for i := range 2 {
		if i >= len(conds) {
			body = append(body, make([]byte, 10)...)
			continue
		}
		d, s := doper(conds[i])
		body = append(body, d...)
		strs = append(strs, s...)
	}
	return append(body, strs...), true
}

// doper encodes a condition as a DOPER, with the string that follows the
// two DOPERs for a text value.
func doper(cd model.FilterCond) (d, str []byte) {
	sign := signOf[cd.Op]
	if cd.Val == "" {
		if cd.Op == model.OpNotEqual {
			return append([]byte{vtNonBlank, 5}, make([]byte, 8)...), nil
		}
		return append([]byte{vtBlanks, 2}, make([]byte, 8)...), nil
	}
	if v, err := strconv.ParseFloat(cd.Val, 64); err == nil {
		return append([]byte{vtNumber, sign}, binary.LittleEndian.AppendUint64(nil, math.Float64bits(v))...), nil
	}
	s := truncUTF16(cd.Val, 255)
	return []byte{vtString, sign, 0, 0, 0, 0, byte(strLen(s)), 0, 0, 0}, appendChars(nil, s)
}
