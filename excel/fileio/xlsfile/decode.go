package xlsfile

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// Parsed-expression tokens (ptgs) of BIFF8 formulas. Operand tokens from
// 0x20 up come in three classes: reference (0x20..0x3F), value (+0x20) and
// array (+0x40); the constants below are the reference-class ids.
const (
	ptgExp       = 0x01
	ptgTbl       = 0x02
	ptgAdd       = 0x03
	ptgSub       = 0x04
	ptgMul       = 0x05
	ptgDiv       = 0x06
	ptgPower     = 0x07
	ptgConcat    = 0x08
	ptgLT        = 0x09
	ptgLE        = 0x0A
	ptgEQ        = 0x0B
	ptgGE        = 0x0C
	ptgGT        = 0x0D
	ptgNE        = 0x0E
	ptgIsect     = 0x0F
	ptgUnion     = 0x10
	ptgRange     = 0x11
	ptgUplus     = 0x12
	ptgUminus    = 0x13
	ptgPercent   = 0x14
	ptgParen     = 0x15
	ptgMissArg   = 0x16
	ptgStr       = 0x17
	ptgAttr      = 0x19
	ptgErr       = 0x1C
	ptgBool      = 0x1D
	ptgInt       = 0x1E
	ptgNum       = 0x1F
	ptgArray     = 0x20
	ptgFunc      = 0x21
	ptgFuncVar   = 0x22
	ptgName      = 0x23
	ptgRef       = 0x24
	ptgArea      = 0x25
	ptgMemArea   = 0x26
	ptgMemErr    = 0x27
	ptgMemNoMem  = 0x28
	ptgMemFunc   = 0x29
	ptgRefErr    = 0x2A
	ptgAreaErr   = 0x2B
	ptgRefN      = 0x2C
	ptgAreaN     = 0x2D
	ptgMemAreaN  = 0x2E
	ptgMemNoMemN = 0x2F
	ptgNameX     = 0x39
	ptgRef3d     = 0x3A
	ptgArea3d    = 0x3B
	ptgRefErr3d  = 0x3C
	ptgAreaErr3d = 0x3D

	// tAttr option bits
	attrVolatile = 0x01
	attrIf       = 0x02
	attrChoose   = 0x04
	attrSkip     = 0x08
	attrSum      = 0x10
	attrSpace    = 0x40
)

// binary operators by ptg, with their precedence (higher binds tighter).
var binOps = map[byte]struct {
	op   string
	prec int
}{
	ptgAdd: {"+", 3}, ptgSub: {"-", 3}, ptgMul: {"*", 4}, ptgDiv: {"/", 4},
	ptgPower: {"^", 5}, ptgConcat: {"&", 2},
	ptgLT: {"<", 1}, ptgLE: {"<=", 1}, ptgEQ: {"=", 1}, ptgGE: {">=", 1}, ptgGT: {">", 1}, ptgNE: {"<>", 1},
	ptgIsect: {" ", 9}, ptgUnion: {",", 8}, ptgRange: {":", 10},
}

const (
	precUnary = 6
	precAtom  = 100
	precUnion = 8
)

// errShared is returned for a formula that is a tExp pointing at the shared
// or array formula anchored at row, col.
type errShared struct{ row, col int }

func (e errShared) Error() string { return "shared formula" }

var errTable = errors.New("data table formula (TABLE)")

// supbook is a SUPBOOK record: this workbook, an add-in, or another file.
type supbook struct {
	kind   int // supSelf, supAddin, supExternal
	path   string
	sheets []string
	names  []string // EXTERNNAME records that follow it
	ext    int      // 1-based number among external books, as Excel shows [n]
}

const (
	supSelf = iota
	supAddin
	supExternal
)

// xti is an EXTERNSHEET entry: a supporting book and a sheet range in it.
type xti struct{ book, first, last int }

// definedName is a NAME record.
type definedName struct {
	name    string // display name (built-in names spelled out)
	itab    int    // 1-based BOUNDSHEET index, 0 = workbook scope
	flags   int
	builtin bool
	rgce    []byte
	extra   []byte
}

const (
	nameHidden  = 0x0001
	nameFunc    = 0x0002
	nameVB      = 0x0004
	nameProc    = 0x0008
	nameBuiltin = 0x0020
)

var builtinNames = []string{
	"Consolidate_Area", "Auto_Open", "Auto_Close", "Extract", "Database", "Criteria",
	"Print_Area", "Print_Titles", "Recorder", "Data_Form", "Auto_Activate",
	"Auto_Deactivate", "Sheet_Title", "_FilterDatabase",
}

type item struct {
	s    string
	prec int
}

// decoder turns a parsed expression back into formula text.
type decoder struct {
	p        *parser
	row, col int  // the cell the formula belongs to (base of tRefN/tAreaN)
	rel      bool // relative parts of 3-D refs are offsets (names, shared formulas)
	warn     func(format string, args ...any)
}

// decodeFormula returns the text (without '=') of rgce, whose trailing
// data (array constants) is extra.
func (p *parser) decodeFormula(rgce, extra []byte, row, col int, rel bool) (string, error) {
	d := decoder{p: p, row: row, col: col, rel: rel, warn: p.warnf}
	return d.decode(rgce, extra)
}

func (d *decoder) decode(b, extra []byte) (string, error) {
	var st []item
	// Spaces from tAttrSpace go before the next token's text, or before
	// the parentheses of a tParen.
	var sp, spOpen, spClose string
	take := func() string {
		s := sp
		sp = ""
		return s
	}
	push := func(s string, prec int) { st = append(st, item{take() + s, prec}) }
	pop := func() item {
		if len(st) == 0 {
			return item{"", precAtom}
		}
		it := st[len(st)-1]
		st = st[:len(st)-1]
		return it
	}
	need := func(n int) error {
		if len(st) < n {
			return errors.New("formula stack underflow")
		}
		return nil
	}
	i := 0
	have := func(n int) error {
		if i+n > len(b) {
			return errors.New("truncated formula")
		}
		return nil
	}
	for i < len(b) {
		id := b[i]
		i++
		if id == ptgExp || id == ptgTbl {
			if err := have(4); err != nil {
				return "", err
			}
			if id == ptgTbl {
				return "", errTable
			}
			return "", errShared{u16(b, i), u16(b, i+2)}
		}
		if op, ok := binOps[id]; ok {
			if err := need(2); err != nil {
				return "", err
			}
			r, l := pop(), pop()
			ls, rs := l.s, r.s
			if l.prec < op.prec {
				ls = "(" + ls + ")"
			}
			if r.prec <= op.prec {
				rs = "(" + rs + ")"
			}
			st = append(st, item{ls + take() + op.op + rs, op.prec})
			continue
		}
		switch id {
		case ptgUplus, ptgUminus:
			if err := need(1); err != nil {
				return "", err
			}
			x := pop()
			s := x.s
			if x.prec < precUnary {
				s = "(" + s + ")"
			}
			push(map[byte]string{ptgUplus: "+", ptgUminus: "-"}[id]+s, precUnary)
			continue
		case ptgPercent:
			if err := need(1); err != nil {
				return "", err
			}
			x := pop()
			s := x.s
			if x.prec < precUnary {
				s = "(" + s + ")"
			}
			st = append(st, item{s + take() + "%", precUnary})
			continue
		case ptgParen:
			if err := need(1); err != nil {
				return "", err
			}
			x := pop().s
			push(spOpen+"("+x+spClose+")", precAtom)
			spOpen, spClose = "", ""
			continue
		case ptgMissArg:
			push("", precAtom)
			continue
		case ptgStr:
			if err := have(2); err != nil {
				return "", err
			}
			s, end := shortString(b, i)
			i = end
			push(quoteString(s), precAtom)
			continue
		case ptgAttr:
			if err := have(3); err != nil {
				return "", err
			}
			grbit, data := b[i], u16(b, i+1)
			i += 3
			switch {
			case grbit&attrSpace != 0:
				ws := strings.Repeat(" ", data>>8)
				if data&1 != 0 {
					ws = strings.Repeat("\n", data>>8)
				}
				switch data & 0xFF {
				case 0, 1:
					sp += ws
				case 2, 3:
					spOpen += ws
				case 4, 5:
					spClose += ws
				}
			case grbit&attrChoose != 0:
				i += 2 * (data + 1)
			case grbit&attrSum != 0:
				if err := need(1); err != nil {
					return "", err
				}
				push("SUM("+pop().s+")", precAtom)
			}
			continue
		case ptgErr:
			if err := have(1); err != nil {
				return "", err
			}
			push(errorText(b[i]), precAtom)
			i++
			continue
		case ptgBool:
			if err := have(1); err != nil {
				return "", err
			}
			push(map[bool]string{true: "TRUE", false: "FALSE"}[b[i] != 0], precAtom)
			i++
			continue
		case ptgInt:
			if err := have(2); err != nil {
				return "", err
			}
			push(strconv.Itoa(u16(b, i)), precAtom)
			i += 2
			continue
		case ptgNum:
			if err := have(8); err != nil {
				return "", err
			}
			push(numberText(f64(b, i)), precAtom)
			i += 8
			continue
		}
		if id < 0x20 {
			return "", fmt.Errorf("unsupported token %#02x", id)
		}
		base := id&0x1F | 0x20
		switch base {
		case ptgArray:
			if err := have(7); err != nil {
				return "", err
			}
			i += 7
			s, n, err := arrayConstant(extra)
			if err != nil {
				return "", err
			}
			extra = extra[n:]
			push(s, precAtom)
		case ptgFunc, ptgFuncVar:
			var argc, iftab int
			if base == ptgFunc {
				if err := have(2); err != nil {
					return "", err
				}
				iftab = u16(b, i)
				i += 2
			} else {
				if err := have(3); err != nil {
					return "", err
				}
				argc, iftab = int(b[i]&0x7F), u16(b, i+1)
				i += 3
				if iftab&0x8000 != 0 {
					return "", fmt.Errorf("macro command %d in formula", iftab&0x7FFF)
				}
			}
			var name string
			if iftab != funcUser {
				f := funcByID[iftab]
				if f == nil {
					return "", fmt.Errorf("unknown function id %d", iftab)
				}
				if base == ptgFunc {
					argc = f.min
				}
				name = f.name
			}
			if err := need(argc); err != nil {
				return "", err
			}
			args := make([]string, argc)
			for k := argc - 1; k >= 0; k-- {
				it := pop()
				args[k] = it.s
				if it.prec == precUnion {
					args[k] = "(" + it.s + ")"
				}
			}
			if iftab == funcUser {
				if argc == 0 {
					return "", errors.New("add-in call without a name")
				}
				name, args = funcName(args[0]), args[1:]
			}
			push(name+"("+strings.Join(args, ",")+")", precAtom)
		case ptgName:
			if err := have(4); err != nil {
				return "", err
			}
			n := u16(b, i)
			i += 4
			if n < 1 || n > len(d.p.names) {
				return "", fmt.Errorf("bad name index %d", n)
			}
			push(d.p.names[n-1].name, precAtom)
		case ptgNameX:
			if err := have(6); err != nil {
				return "", err
			}
			s, err := d.externName(u16(b, i), u16(b, i+2))
			if err != nil {
				return "", err
			}
			i += 6
			push(s, precAtom)
		case ptgRef, ptgRefN:
			if err := have(4); err != nil {
				return "", err
			}
			push(d.cellRef(u16(b, i), u16(b, i+2), base == ptgRefN), precAtom)
			i += 4
		case ptgArea, ptgAreaN:
			if err := have(8); err != nil {
				return "", err
			}
			push(d.areaRef(u16(b, i), u16(b, i+2), u16(b, i+4), u16(b, i+6), base == ptgAreaN), precAtom)
			i += 8
		case ptgRef3d, ptgArea3d:
			n := 6
			if base == ptgArea3d {
				n = 10
			}
			if err := have(n); err != nil {
				return "", err
			}
			sheet, err := d.sheetPrefix(u16(b, i))
			if err != nil {
				return "", err
			}
			var ref string
			if base == ptgRef3d {
				ref = d.cellRef(u16(b, i+2), u16(b, i+4), d.rel)
			} else {
				ref = d.areaRef(u16(b, i+2), u16(b, i+4), u16(b, i+6), u16(b, i+8), d.rel)
			}
			i += n
			if sheet == "" {
				ref = "#REF!"
			}
			push(sheet+ref, precAtom)
		case ptgRefErr, ptgAreaErr, ptgRefErr3d, ptgAreaErr3d:
			n := map[byte]int{ptgRefErr: 4, ptgAreaErr: 8, ptgRefErr3d: 6, ptgAreaErr3d: 10}[base]
			if err := have(n); err != nil {
				return "", err
			}
			s := "#REF!"
			if base == ptgRefErr3d || base == ptgAreaErr3d {
				if sheet, err := d.sheetPrefix(u16(b, i)); err == nil {
					s = sheet + s
				}
			}
			i += n
			push(s, precAtom)
		case ptgMemArea:
			// The sub-expression follows as ordinary tokens; the cached
			// rectangles are in the trailing data.
			if err := have(6); err != nil {
				return "", err
			}
			i += 6
			if len(extra) >= 2 {
				extra = extra[min(len(extra), 2+8*u16(extra, 0)):]
			}
		case ptgMemErr, ptgMemNoMem:
			if err := have(6); err != nil {
				return "", err
			}
			i += 6
		case ptgMemFunc, ptgMemAreaN, ptgMemNoMemN:
			if err := have(2); err != nil {
				return "", err
			}
			i += 2
		default:
			return "", fmt.Errorf("unsupported token %#02x", id)
		}
	}
	if len(st) != 1 {
		return "", fmt.Errorf("formula left %d values", len(st))
	}
	return st[0].s, nil
}

// funcName normalizes the name of an add-in or future function: Excel
// writes _xlfn.IFERROR in .xls, but only functions newer than the .xlsx
// format keep the prefix there.
func funcName(s string) string {
	up := formula.FormulaName(s)
	if strings.HasPrefix(strings.ToUpper(s), "_XLFN.") || formula.FuturePrefix(up) != "" {
		return formula.FuturePrefix(up) + up
	}
	if addinFuncs[up] {
		return up
	}
	return s
}

func (d *decoder) externName(ixti, n int) (string, error) {
	if ixti >= len(d.p.xti) {
		return "", fmt.Errorf("bad EXTERNSHEET index %d", ixti)
	}
	x := d.p.xti[ixti]
	if x.book >= len(d.p.supbooks) {
		return "", fmt.Errorf("bad SUPBOOK index %d", x.book)
	}
	sb := d.p.supbooks[x.book]
	switch sb.kind {
	case supSelf:
		if n < 1 || n > len(d.p.names) {
			return "", fmt.Errorf("bad name index %d", n)
		}
		return d.p.names[n-1].name, nil
	case supAddin:
		if n < 1 || n > len(sb.names) {
			return "", fmt.Errorf("bad add-in name index %d", n)
		}
		return sb.names[n-1], nil
	}
	if n < 1 || n > len(sb.names) {
		return "", fmt.Errorf("bad external name index %d", n)
	}
	d.warn("formulas refer to other workbooks; those references are not calculated")
	return "[" + strconv.Itoa(sb.ext) + "]!" + sb.names[n-1], nil
}

// sheetPrefix returns "Sheet1!" (quoted when needed) for an EXTERNSHEET
// entry, or "" if the sheet was deleted.
func (d *decoder) sheetPrefix(ixti int) (string, error) {
	if ixti >= len(d.p.xti) {
		return "", fmt.Errorf("bad EXTERNSHEET index %d", ixti)
	}
	x := d.p.xti[ixti]
	if x.book >= len(d.p.supbooks) {
		return "", fmt.Errorf("bad SUPBOOK index %d", x.book)
	}
	sb := d.p.supbooks[x.book]
	var names []string
	switch sb.kind {
	case supSelf:
		for _, bs := range d.p.sheets {
			names = append(names, bs.name)
		}
	case supExternal:
		names = sb.sheets
		d.warn("formulas refer to other workbooks; those references are not calculated")
	default:
		return "", errors.New("3-D reference into an add-in")
	}
	if x.first < 0 || x.first >= len(names) || x.last < x.first || x.last >= len(names) {
		return "", nil
	}
	s := names[x.first]
	quote := quoteSheet(s) != s
	if x.last != x.first {
		s += ":" + names[x.last]
		quote = quote || quoteSheet(names[x.last]) != names[x.last]
	}
	if sb.kind == supExternal {
		s = "[" + strconv.Itoa(sb.ext) + "]" + s
	}
	if quote {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'!", nil
	}
	return s + "!", nil
}

// cellRef formats a row and a column field (with relative flags in bits
// 14 and 15). In tRefN tokens (rel), relative parts are offsets from the
// formula's cell.
func (d *decoder) cellRef(rw, col int, rel bool) string {
	r, c, rr, cr := d.coords(rw, col, rel)
	return colText(c, cr) + rowText(r, rr)
}

func (d *decoder) areaRef(rw1, rw2, col1, col2 int, rel bool) string {
	r1, c1, rr1, cr1 := d.coords(rw1, col1, rel)
	r2, c2, rr2, cr2 := d.coords(rw2, col2, rel)
	switch {
	case r1 == 0 && r2 == 0xFFFF:
		return colText(c1, cr1) + ":" + colText(c2, cr2)
	case c1 == 0 && c2 == 0xFF:
		return rowText(r1, rr1) + ":" + rowText(r2, rr2)
	}
	return colText(c1, cr1) + rowText(r1, rr1) + ":" + colText(c2, cr2) + rowText(r2, rr2)
}

func (d *decoder) coords(rw, col int, rel bool) (r, c int, rowRel, colRel bool) {
	rowRel, colRel = col&0x8000 != 0, col&0x4000 != 0
	r, c = rw, col&0x3FFF
	if rel {
		if rowRel {
			r = (d.row + int(int16(rw))) & 0xFFFF
		}
		if colRel {
			c = (d.col + int(int8(col))) & 0xFF
		}
	}
	return r, c, rowRel, colRel
}

func colText(c int, rel bool) string {
	if rel {
		return model.ColName(c)
	}
	return "$" + model.ColName(c)
}

func rowText(r int, rel bool) string {
	if rel {
		return strconv.Itoa(r + 1)
	}
	return "$" + strconv.Itoa(r+1)
}

// arrayConstant decodes the data of a tArray token: {1,2;"a",TRUE}. It
// returns the number of bytes used.
func arrayConstant(b []byte) (string, int, error) {
	if len(b) < 3 {
		return "", 0, errors.New("truncated array constant")
	}
	cols, rows := int(b[0])+1, u16(b, 1)+1
	i := 3
	var sb strings.Builder
	sb.WriteByte('{')
	for r := range rows {
		if r > 0 {
			sb.WriteByte(';')
		}
		for c := range cols {
			if c > 0 {
				sb.WriteByte(',')
			}
			if i >= len(b) {
				return "", 0, errors.New("truncated array constant")
			}
			t := b[i]
			i++
			switch t {
			case 0x00: // empty
				i += 8
			case 0x01:
				sb.WriteString(numberText(f64(b, i)))
				i += 8
			case 0x02:
				cch := u16(b, i)
				if i+2 >= len(b) {
					return "", 0, errors.New("truncated array constant")
				}
				s, end := chars(b, i+3, cch, b[i+2]&0x01 != 0)
				sb.WriteString(quoteString(s))
				i = end
			case 0x04:
				sb.WriteString(map[bool]string{true: "TRUE", false: "FALSE"}[i < len(b) && b[i] != 0])
				i += 8
			case 0x10:
				if i < len(b) {
					sb.WriteString(errorText(b[i]))
				}
				i += 8
			default:
				return "", 0, fmt.Errorf("bad array element type %d", t)
			}
		}
	}
	sb.WriteByte('}')
	return sb.String(), min(i, len(b)), nil
}

// shortString reads a ShortXLUnicodeString (8-bit length) at off.
func shortString(b []byte, off int) (string, int) {
	if off+2 > len(b) {
		return "", len(b)
	}
	return chars(b, off+2, int(b[off]), b[off+1]&0x01 != 0)
}

func errorText(code byte) string {
	if s, ok := errorTexts[code]; ok {
		return s
	}
	return "#N/A"
}

func quoteString(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// numberText formats a number constant the way Excel shows it in a
// formula: 0.5, 1E+20, 1E-07.
func numberText(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'G', -1, 64)
}

// quoteSheet quotes a sheet name for use in a formula when it is not a
// plain identifier: 'My Sheet', '2024', 'A1'.
func quoteSheet(name string) string {
	plain := name != ""
	for i, c := range name {
		switch {
		case c == '_' || c == '.' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80:
		case c >= '0' && c <= '9':
			if i == 0 {
				plain = false
			}
		default:
			plain = false
		}
	}
	if plain {
		up := strings.ToUpper(name)
		if up == "TRUE" || up == "FALSE" || looksLikeCell(up) || looksLikeR1C1(up) {
			plain = false
		}
	}
	if plain {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

// looksLikeCell reports whether s is a cell reference such as A1 or XFD9.
func looksLikeCell(s string) bool {
	i := 0
	for i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		i++
	}
	if i == 0 || i > 3 || i == len(s) {
		return false
	}
	for _, c := range s[i:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// looksLikeR1C1 reports whether s is an R1C1-style reference (R, C, R1C2).
func looksLikeR1C1(s string) bool {
	if s == "R" || s == "C" {
		return true
	}
	if s == "" || s[0] != 'R' && s[0] != 'C' {
		return false
	}
	for _, c := range s[1:] {
		if (c < '0' || c > '9') && c != 'R' && c != 'C' {
			return false
		}
	}
	return true
}
