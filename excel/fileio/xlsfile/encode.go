package xlsfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// BIFF8 sheet limits.
const (
	xlsMaxRows = 65536
	xlsMaxCols = 256
	// maxFormulaBytes is the longest parsed expression Excel accepts.
	maxFormulaBytes = 1800
)

// Formula AST used by the encoder. Unlike excel/formula's evaluator AST it
// keeps what the file needs: absolute flags, parentheses and spaces.
type xnode struct {
	kind int
	ws   string // spaces before the node's own token
	ws2  string // spaces before ')' of a parenthesis
	num  float64
	str  string // string, error, name or function name; operator
	ref  xref
	args []*xnode // operands, arguments
	rows [][]*xnode
}

const (
	nNum = iota
	nStr
	nBool
	nErr
	nMiss
	nArray
	nRef
	nName
	nCall
	nUnary
	nBinary
	nParen
)

// xref is a cell or area reference. sheet1 is "" for the formula's own
// sheet; sheet2 is set for a 3-D range (Sheet1:Sheet3!A1).
type xref struct {
	sheet1, sheet2 string
	r1, c1, r2, c2 int
	rel            [4]bool // r1, c1, r2, c2 relative
	area           bool
}

// parseXFormula parses formula text (without '=') for encoding. union
// allows a union without parentheses (A1,B2), as defined names have.
func parseXFormula(src string, union bool) (*xnode, error) {
	toks, err := formula.Tokens(src)
	if err != nil {
		return nil, err
	}
	p := &xparser{src: src, toks: toks}
	n, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	for union && p.peek().Kind == formula.TokComma {
		ws := p.ws(p.i)
		p.i++
		y, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		n = &xnode{kind: nBinary, str: ",", ws: ws, args: []*xnode{n, y}}
	}
	if p.i < len(toks) {
		return nil, fmt.Errorf("unexpected %q", p.text(p.i))
	}
	return n, nil
}

type xparser struct {
	src  string
	toks []formula.Token
	i    int
}

func (p *xparser) peek() *formula.Token {
	if p.i >= len(p.toks) {
		return &formula.Token{}
	}
	return &p.toks[p.i]
}

func (p *xparser) text(i int) string { return p.src[p.toks[i].Pos:p.toks[i].End] }

// ws is the whitespace before token i.
func (p *xparser) ws(i int) string {
	if i >= len(p.toks) {
		return ""
	}
	start := 0
	if i > 0 {
		start = p.toks[i-1].End
	}
	return p.src[start:p.toks[i].Pos]
}

func (p *xparser) isOp(op string) bool {
	t := p.peek()
	return t.Kind == formula.TokOp && t.Text == op
}

var xbinPrec = map[string]int{
	"=": 1, "<>": 1, "<": 1, "<=": 1, ">": 1, ">=": 1,
	"&": 2, "+": 3, "-": 3, "*": 4, "/": 4, "^": 5,
}

func (p *xparser) expr(minPrec int) (*xnode, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		prec, ok := xbinPrec[t.Text]
		if t.Kind != formula.TokOp || !ok || prec < minPrec {
			return left, nil
		}
		ws := p.ws(p.i)
		p.i++
		right, err := p.expr(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &xnode{kind: nBinary, str: t.Text, ws: ws, args: []*xnode{left, right}}
	}
}

func (p *xparser) unary() (*xnode, error) {
	t := p.peek()
	ws := p.ws(p.i)
	if t.Kind == formula.TokOp && (t.Text == "-" || t.Text == "+") || t.Kind == formula.TokAt {
		p.i++
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		if t.Kind == formula.TokAt {
			// @ is the SINGLE function in files.
			return &xnode{kind: nCall, str: "SINGLE", ws: ws, args: []*xnode{x}}, nil
		}
		return &xnode{kind: nUnary, str: t.Text, ws: ws, args: []*xnode{x}}, nil
	}
	return p.postfix()
}

func (p *xparser) postfix() (*xnode, error) {
	x, err := p.rangeExpr()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		switch {
		case t.Kind == formula.TokOp && t.Text == "%":
			x = &xnode{kind: nUnary, str: "%", ws: p.ws(p.i), args: []*xnode{x}}
			p.i++
		case t.Kind == formula.TokHash:
			// A1# is ANCHORARRAY(A1) in files.
			x = &xnode{kind: nCall, str: "ANCHORARRAY", args: []*xnode{x}}
			p.i++
		case t.Kind == formula.TokLParen:
			return nil, errors.New("calling the result of an expression (LAMBDA(...)(...)) cannot be saved")
		default:
			return x, nil
		}
	}
}

func (p *xparser) rangeExpr() (*xnode, error) {
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.isOp(":") {
		ws := p.ws(p.i)
		p.i++
		y, err := p.primary()
		if err != nil {
			return nil, err
		}
		// Sheet1:Sheet3!A1 lexes as a name, ':' and a sheet reference.
		if x.kind == nName && x.ref.sheet1 == "" && y.kind == nRef && y.ref.sheet1 != "" && y.ref.sheet2 == "" {
			y.ref.sheet1, y.ref.sheet2 = x.str, y.ref.sheet1
			y.ws = x.ws
			x = y
			continue
		}
		// A1:B2 and Sheet1!A1:B2 are one area.
		if x.kind == nRef && y.kind == nRef && !x.ref.area && !y.ref.area && ws == "" && y.ws == "" &&
			(y.ref.sheet1 == "" || strings.EqualFold(y.ref.sheet1, x.ref.sheet1) && y.ref.sheet2 == x.ref.sheet2) {
			a := x.ref
			a.r2, a.c2, a.rel[2], a.rel[3] = y.ref.r1, y.ref.c1, y.ref.rel[0], y.ref.rel[1]
			a.area = true
			if a.r1 > a.r2 {
				a.r1, a.r2, a.rel[0], a.rel[2] = a.r2, a.r1, a.rel[2], a.rel[0]
			}
			if a.c1 > a.c2 {
				a.c1, a.c2, a.rel[1], a.rel[3] = a.c2, a.c1, a.rel[3], a.rel[1]
			}
			x = &xnode{kind: nRef, ws: x.ws, ref: a}
			continue
		}
		x = &xnode{kind: nBinary, str: ":", ws: ws, args: []*xnode{x, y}}
	}
	return x, nil
}

func (p *xparser) primary() (*xnode, error) {
	if p.i >= len(p.toks) {
		return nil, errors.New("unexpected end of formula")
	}
	t := p.toks[p.i]
	ws := p.ws(p.i)
	p.i++
	switch t.Kind {
	case formula.TokNumber:
		return &xnode{kind: nNum, num: t.Num, ws: ws}, nil
	case formula.TokString:
		return &xnode{kind: nStr, str: t.Text, ws: ws}, nil
	case formula.TokBool:
		n := &xnode{kind: nBool, ws: ws}
		if t.Text == "TRUE" {
			n.num = 1
		}
		return n, nil
	case formula.TokError:
		return &xnode{kind: nErr, str: t.Text, ws: ws}, nil
	case formula.TokRef:
		r, err := parseXRef(t.Text)
		if err != nil {
			return nil, err
		}
		r.sheet1 = t.Sheet
		if a, b, ok := strings.Cut(t.Sheet, ":"); ok && !strings.HasPrefix(t.Sheet, "[") {
			r.sheet1, r.sheet2 = a, b
		}
		return &xnode{kind: nRef, ref: r, ws: ws}, nil
	case formula.TokName:
		return &xnode{kind: nName, str: t.Text, ref: xref{sheet1: t.Sheet}, ws: ws}, nil
	case formula.TokFunc:
		args, err := p.args()
		if err != nil {
			return nil, err
		}
		return &xnode{kind: nCall, str: t.Text, ws: ws, args: args}, nil
	case formula.TokLParen:
		// Parentheses, possibly around a union: (A1,B2).
		x, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		for p.peek().Kind == formula.TokComma {
			cws := p.ws(p.i)
			p.i++
			y, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			x = &xnode{kind: nBinary, str: ",", ws: cws, args: []*xnode{x, y}}
		}
		if p.peek().Kind != formula.TokRParen {
			return nil, errors.New("missing )")
		}
		ws2 := p.ws(p.i)
		p.i++
		return &xnode{kind: nParen, ws: ws, ws2: ws2, args: []*xnode{x}}, nil
	case formula.TokLBrace:
		return p.arrayConst(ws)
	}
	return nil, fmt.Errorf("unexpected %q", p.text(p.i-1))
}

func (p *xparser) args() ([]*xnode, error) {
	if p.peek().Kind != formula.TokLParen {
		return nil, errors.New("expected (")
	}
	p.i++
	var args []*xnode
	if p.peek().Kind == formula.TokRParen {
		p.i++
		return args, nil
	}
	for {
		if k := p.peek().Kind; k == formula.TokComma || k == formula.TokRParen {
			args = append(args, &xnode{kind: nMiss, ws: p.ws(p.i)})
		} else {
			a, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			args = append(args, a)
		}
		switch p.peek().Kind {
		case formula.TokComma:
			p.i++
		case formula.TokRParen:
			p.i++
			return args, nil
		default:
			return nil, errors.New("expected , or )")
		}
	}
}

func (p *xparser) arrayConst(ws string) (*xnode, error) {
	n := &xnode{kind: nArray, ws: ws}
	row := []*xnode{}
	for {
		if p.i >= len(p.toks) {
			return nil, errors.New("bad array constant")
		}
		t := p.toks[p.i]
		p.i++
		neg := false
		if t.Kind == formula.TokOp && (t.Text == "-" || t.Text == "+") && p.i < len(p.toks) {
			neg = t.Text == "-"
			t = p.toks[p.i]
			p.i++
		}
		var el *xnode
		switch t.Kind {
		case formula.TokNumber:
			el = &xnode{kind: nNum, num: t.Num}
			if neg {
				el.num = -el.num
			}
		case formula.TokString:
			el = &xnode{kind: nStr, str: t.Text}
		case formula.TokBool:
			el = &xnode{kind: nBool}
			if t.Text == "TRUE" {
				el.num = 1
			}
		case formula.TokError:
			el = &xnode{kind: nErr, str: t.Text}
		default:
			return nil, errors.New("bad array constant")
		}
		row = append(row, el)
		switch p.peek().Kind {
		case formula.TokComma:
		case formula.TokSemi:
			n.rows = append(n.rows, row)
			row = []*xnode{}
		case formula.TokRBrace:
			n.rows = append(n.rows, row)
			p.i++
			for _, r := range n.rows {
				if len(r) != len(n.rows[0]) {
					return nil, errors.New("ragged array constant")
				}
			}
			return n, nil
		default:
			return nil, errors.New("bad array constant")
		}
		p.i++
	}
}

// parseXRef parses A1, $A$1, A:B or 1:3 (sheet prefix already removed).
func parseXRef(s string) (xref, error) {
	var r xref
	if a, b, ok := strings.Cut(s, ":"); ok {
		r.area = true
		c1, cr1, n1 := parseCol(a)
		c2, cr2, n2 := parseCol(b)
		if n1 == len(a) && n2 == len(b) && n1 > 0 { // A:B
			r.c1, r.c2, r.rel[1], r.rel[3] = c1, c2, cr1, cr2
			r.r1, r.r2 = 0, xlsMaxRows-1
			return r, nil
		}
		r1, rr1, ok1 := parseRow(a)
		r2, rr2, ok2 := parseRow(b)
		if !ok1 || !ok2 {
			return r, fmt.Errorf("bad reference %q", s)
		}
		r.r1, r.r2, r.rel[0], r.rel[2] = r1, r2, rr1, rr2
		r.c1, r.c2 = 0, xlsMaxCols-1
		return r, nil
	}
	c, cr, n := parseCol(s)
	row, rr, ok := parseRow(s[n:])
	if n == 0 || !ok {
		return r, fmt.Errorf("bad reference %q", s)
	}
	r.r1, r.c1, r.r2, r.c2 = row, c, row, c
	r.rel = [4]bool{rr, cr, rr, cr}
	return r, nil
}

func parseCol(s string) (col int, rel bool, n int) {
	rel = true
	if strings.HasPrefix(s, "$") {
		rel, n = false, 1
	}
	start := n
	for n < len(s) && (s[n]|0x20 >= 'a' && s[n]|0x20 <= 'z') {
		n++
	}
	if n == start {
		return 0, rel, 0
	}
	return model.ParseColName(s[start:n]), rel, n
}

func parseRow(s string) (row int, rel, ok bool) {
	rel = true
	if strings.HasPrefix(s, "$") {
		rel, s = false, s[1:]
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return 0, rel, false
	}
	return v - 1, rel, true
}

// fmlaContext is what the encoder needs from the workbook being written.
type fmlaContext interface {
	sheetIndex(name string) int // -1 if there is no such sheet
	xtiIndex(first, last int) int
	nameIndex(name string, sheet int) int // 1-based NAME index, 0 if undefined
	funcNameRef(name string) (ptg byte, data []byte)
}

// encoder compiles one formula.
type encoder struct {
	ctx      fmlaContext
	sheet    int  // the formula's sheet
	name     bool // a NAME formula: every reference is 3-D
	out      []byte
	extra    []byte
	volatile bool
}

// compile encodes formula text (without '=') as a parsed expression with
// its trailing data. root is the class of the result: 'V' for cells, 'R'
// for defined names.
func compileFormula(ctx fmlaContext, src string, sheet int, name bool) (rgce, extra []byte, err error) {
	n, err := parseXFormula(src, name)
	if err != nil {
		return nil, nil, err
	}
	e := &encoder{ctx: ctx, sheet: sheet, name: name}
	if usesLambda(n) {
		// Excel does not evaluate LET and LAMBDA variables read from .xls.
		return nil, nil, errors.New("LET and LAMBDA cannot be saved in .xls")
	}
	root := byte('V')
	if name {
		root = 'R'
	}
	if err := e.emit(n, root); err != nil {
		return nil, nil, err
	}
	if e.volatile {
		e.out = append([]byte{ptgAttr, attrVolatile, 0, 0}, e.out...)
	}
	if len(e.out) > maxFormulaBytes {
		return nil, nil, errors.New("formula is too long for .xls")
	}
	return e.out, e.extra, nil
}

// tok returns the operand token id with base id (reference class) in the
// class of ctx: reference 0x20.., value 0x40.., array 0x60...
func tok(base, ctx byte) byte {
	switch ctx {
	case 'V':
		return base&0x1F | 0x40
	case 'A':
		return base&0x1F | 0x60
	}
	return base&0x1F | 0x20
}

func (e *encoder) u8(b byte) { e.out = append(e.out, b) }
func (e *encoder) u16(v int) { e.out = binary.LittleEndian.AppendUint16(e.out, uint16(v)) }
func (e *encoder) f64(v float64) {
	e.out = binary.LittleEndian.AppendUint64(e.out, math.Float64bits(v))
}

// space writes tAttrSpace tokens for the whitespace before a token: typ
// is 0 for spaces before an operand or operator, 2 before '(' and 4
// before ')' (line breaks use typ+1).
func (e *encoder) space(ws string, typ byte) {
	sp, cr := strings.Count(ws, " ")+strings.Count(ws, "\t"), strings.Count(ws, "\n")
	if cr > 0 {
		e.out = append(e.out, ptgAttr, attrSpace, typ+1, byte(min(cr, 255)))
	}
	if sp > 0 {
		e.out = append(e.out, ptgAttr, attrSpace, typ, byte(min(sp, 255)))
	}
}

var errorCodes = map[string]byte{
	"#NULL!": 0x00, "#DIV/0!": 0x07, "#VALUE!": 0x0F, "#REF!": 0x17,
	"#NAME?": 0x1D, "#NUM!": 0x24, "#N/A": 0x2A,
}

var binPtgs = map[string]byte{
	"+": ptgAdd, "-": ptgSub, "*": ptgMul, "/": ptgDiv, "^": ptgPower, "&": ptgConcat,
	"<": ptgLT, "<=": ptgLE, "=": ptgEQ, ">=": ptgGE, ">": ptgGT, "<>": ptgNE,
	",": ptgUnion, ":": ptgRange,
}

func (e *encoder) emit(n *xnode, ctx byte) error {
	// Operators inside arguments that take ranges or arrays work on whole
	// arrays, as in Excel 365: SUM(A1:A3*2), FILTER(A1:A9,A1:A9>2).
	opCtx := byte('V')
	if ctx != 'V' {
		opCtx = 'A'
	}
	switch n.kind {
	case nNum:
		e.space(n.ws, 0)
		if n.num >= 0 && n.num <= 0xFFFF && n.num == math.Trunc(n.num) {
			e.u8(ptgInt)
			e.u16(int(n.num))
		} else {
			e.u8(ptgNum)
			e.f64(n.num)
		}
	case nStr:
		u := utf16.Encode([]rune(n.str))
		if len(u) > 255 {
			return errors.New("text constant longer than 255 characters")
		}
		e.space(n.ws, 0)
		e.u8(ptgStr)
		e.out = appendShortString(e.out, n.str)
	case nBool:
		e.space(n.ws, 0)
		e.u8(ptgBool)
		e.u8(byte(n.num))
	case nErr:
		code, ok := errorCodes[strings.ToUpper(n.str)]
		if !ok {
			return fmt.Errorf("%s cannot be saved in .xls", n.str)
		}
		e.space(n.ws, 0)
		e.u8(ptgErr)
		e.u8(code)
	case nMiss:
		e.space(n.ws, 0)
		e.u8(ptgMissArg)
	case nArray:
		ex, err := arrayData(n)
		if err != nil {
			return err
		}
		e.space(n.ws, 0)
		if ctx != 'V' {
			ctx = 'A'
		}
		e.u8(tok(ptgArray, ctx))
		e.out = append(e.out, make([]byte, 7)...)
		e.extra = append(e.extra, ex...)
	case nRef:
		return e.emitRef(n, ctx)
	case nName:
		if n.ref.sheet1 != "" {
			if e.ctx.sheetIndex(n.ref.sheet1) < 0 {
				return fmt.Errorf("unknown sheet %q", n.ref.sheet1)
			}
		}
		sheet := e.sheet
		if n.ref.sheet1 != "" {
			sheet = e.ctx.sheetIndex(n.ref.sheet1)
		}
		idx := e.ctx.nameIndex(n.str, sheet)
		if idx == 0 {
			return fmt.Errorf("undefined name %s", n.str)
		}
		e.space(n.ws, 0)
		e.u8(tok(ptgName, ctx))
		e.u16(idx)
		e.u16(0)
	case nParen:
		if err := e.emit(n.args[0], ctx); err != nil {
			return err
		}
		e.space(n.ws, 2)
		e.space(n.ws2, 4)
		e.u8(ptgParen)
	case nUnary:
		if err := e.emit(n.args[0], opCtx); err != nil {
			return err
		}
		e.space(n.ws, 0)
		e.u8(map[string]byte{"-": ptgUminus, "+": ptgUplus, "%": ptgPercent}[n.str])
	case nBinary:
		c := opCtx
		if n.str == ":" || n.str == "," {
			c = 'R'
		}
		for _, a := range n.args {
			if err := e.emit(a, c); err != nil {
				return err
			}
		}
		e.space(n.ws, 0)
		e.u8(binPtgs[n.str])
	case nCall:
		return e.emitCall(n, ctx)
	default:
		return errors.New("unsupported expression")
	}
	return nil
}

func (e *encoder) emitRef(n *xnode, ctx byte) error {
	r := n.ref
	if r.r1 >= xlsMaxRows || r.r2 >= xlsMaxRows || r.c1 >= xlsMaxCols || r.c2 >= xlsMaxCols {
		return errors.New("reference beyond row 65,536 or column IV")
	}
	threeD := e.name || r.sheet1 != ""
	ixti := 0
	if threeD {
		first, last := e.sheet, e.sheet
		if r.sheet1 != "" {
			if strings.HasPrefix(r.sheet1, "[") {
				return errors.New("references to other workbooks cannot be saved in .xls")
			}
			first = e.ctx.sheetIndex(r.sheet1)
			last = first
			if r.sheet2 != "" {
				last = e.ctx.sheetIndex(r.sheet2)
			}
			if first < 0 || last < 0 {
				return fmt.Errorf("unknown sheet %q", r.sheet1)
			}
			if first > last {
				first, last = last, first
			}
		}
		ixti = e.ctx.xtiIndex(first, last)
	}
	e.space(n.ws, 0)
	col := func(c int, rowRel, colRel bool) int {
		v := c
		if rowRel {
			v |= 0x8000
		}
		if colRel {
			v |= 0x4000
		}
		return v
	}
	switch {
	case !r.area && threeD:
		e.u8(tok(ptgRef3d, ctx))
		e.u16(ixti)
	case !r.area:
		e.u8(tok(ptgRef, ctx))
	case threeD:
		e.u8(tok(ptgArea3d, ctx))
		e.u16(ixti)
	default:
		e.u8(tok(ptgArea, ctx))
	}
	if !r.area {
		e.u16(r.r1)
		e.u16(col(r.c1, r.rel[0], r.rel[1]))
		return nil
	}
	e.u16(r.r1)
	e.u16(r.r2)
	e.u16(col(r.c1, r.rel[0], r.rel[1]))
	e.u16(col(r.c2, r.rel[2], r.rel[3]))
	return nil
}

func (e *encoder) emitCall(n *xnode, ctx byte) error {
	name := formula.FormulaName(n.str)
	if volatileFuncs[name] {
		e.volatile = true
	}
	f := funcByName[name]
	if f == nil {
		// Add-in and Excel 2007+ functions: the name, then the arguments,
		// then function 255.
		if len(n.args)+1 > maxArgs {
			return fmt.Errorf("%s has more than %d arguments", name, maxArgs-1)
		}
		ptg, data := e.ctx.funcNameRef(name)
		e.u8(ptg)
		e.out = append(e.out, data...)
		for _, a := range n.args {
			if err := e.emit(a, 'R'); err != nil {
				return err
			}
		}
		e.space(n.ws, 0)
		e.u8(tok(ptgFuncVar, funcClass('V', ctx)))
		e.u8(byte(len(n.args) + 1))
		e.u16(funcUser)
		return nil
	}
	argc := len(n.args)
	if f.fixed() && argc != f.min || argc > f.max {
		return fmt.Errorf("%s with %d arguments cannot be saved in .xls", name, argc)
	}
	for i, a := range n.args {
		c := f.argClass(i)
		if c == 'V' && ctx == 'A' {
			c = 'A'
		}
		if err := e.emit(a, c); err != nil {
			return err
		}
	}
	e.space(n.ws, 0)
	cls := funcClass(f.ret, ctx)
	if f.fixed() {
		e.u8(tok(ptgFunc, cls))
		e.u16(f.id)
	} else {
		e.u8(tok(ptgFuncVar, cls))
		e.u8(byte(argc))
		e.u16(f.id)
	}
	return nil
}

// funcClass is the token class of a function returning ret where ctx is
// expected.
func funcClass(ret, ctx byte) byte {
	switch {
	case ret == 'R':
		return ctx
	case ctx == 'A' || ret == 'A' && ctx == 'R':
		return 'A'
	}
	return 'V'
}

// arrayData encodes the values of an array constant (the data that
// follows the formula).
func arrayData(n *xnode) ([]byte, error) {
	rows, cols := len(n.rows), len(n.rows[0])
	if cols > xlsMaxCols || rows > xlsMaxRows {
		return nil, errors.New("array constant too large")
	}
	b := []byte{byte(cols - 1)}
	b = binary.LittleEndian.AppendUint16(b, uint16(rows-1))
	for _, row := range n.rows {
		for _, el := range row {
			switch el.kind {
			case nNum:
				b = append(b, 0x01)
				b = binary.LittleEndian.AppendUint64(b, math.Float64bits(el.num))
			case nStr:
				b = append(b, 0x02)
				b = appendString(b, el.str)
			case nBool:
				b = append(b, 0x04, byte(el.num), 0, 0, 0, 0, 0, 0, 0)
			case nErr:
				code, ok := errorCodes[strings.ToUpper(el.str)]
				if !ok {
					return nil, fmt.Errorf("%s cannot be saved in .xls", el.str)
				}
				b = append(b, 0x10, code, 0, 0, 0, 0, 0, 0, 0)
			}
		}
	}
	return b, nil
}

// isLatin1 reports whether s can be stored with 8-bit characters.
func isLatin1(s string) bool {
	for _, c := range s {
		if c > 0xFF {
			return false
		}
	}
	return true
}

// appendChars appends the option byte and characters of s.
func appendChars(b []byte, s string) []byte {
	if isLatin1(s) {
		b = append(b, 0)
		for _, c := range s {
			b = append(b, byte(c))
		}
		return b
	}
	b = append(b, 1)
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

func strLen(s string) int { return len(utf16.Encode([]rune(s))) }

// appendString appends an XLUnicodeString (16-bit length).
func appendString(b []byte, s string) []byte {
	b = binary.LittleEndian.AppendUint16(b, uint16(strLen(s)))
	return appendChars(b, s)
}

// appendShortString appends a ShortXLUnicodeString (8-bit length).
func appendShortString(b []byte, s string) []byte {
	b = append(b, byte(strLen(s)))
	return appendChars(b, s)
}

// usesLambda reports whether the formula calls LET or LAMBDA.
func usesLambda(n *xnode) bool {
	if n.kind == nCall {
		if name := formula.FormulaName(n.str); name == "LET" || name == "LAMBDA" {
			return true
		}
	}
	for _, a := range n.args {
		if usesLambda(a) {
			return true
		}
	}
	return false
}
