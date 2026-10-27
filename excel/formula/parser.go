package formula

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is a parsed formula expression.
type Node interface{ node() }

type (
	numNode   struct{ v float64 }
	strNode   struct{ v string }
	boolNode  struct{ v bool }
	errNode   struct{ v Error }
	missNode  struct{} // omitted argument
	arrayNode struct {
		rows [][]Node // constants only
	}
	// refNode is a cell or range written in the formula. Absolute flags are
	// irrelevant to evaluation and dropped.
	refNode struct {
		sheet          string // "" = the formula's own sheet
		r1, c1, r2, c2 int    // -1 for whole column/row extents
		wholeCols      bool   // A:B
		wholeRows      bool   // 1:3
	}
	nameNode struct{ name, sheet string }
	callNode struct {
		name string // upper-case, prefixes stripped
		args []Node
	}
	// invokeNode calls the lambda an expression evaluates to: LAMBDA(x,x)(1).
	invokeNode struct {
		fn   Node
		args []Node
	}
	unaryNode struct {
		op string // "-", "+", "%", "@"
		x  Node
	}
	binaryNode struct {
		op   string
		l, r Node
	}
	spillNode struct{ x Node } // A1#
)

func (numNode) node()    {}
func (strNode) node()    {}
func (boolNode) node()   {}
func (errNode) node()    {}
func (missNode) node()   {}
func (arrayNode) node()  {}
func (refNode) node()    {}
func (nameNode) node()   {}
func (callNode) node()   {}
func (invokeNode) node() {}
func (unaryNode) node()  {}
func (binaryNode) node() {}
func (spillNode) node()  {}

// Parse parses formula text (with or without a leading '=').
func Parse(src string) (Node, error) {
	src = strings.TrimPrefix(strings.TrimSpace(src), "=")
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected token at %d", p.peek().pos)
	}
	return n, nil
}

type parser struct {
	toks []token
	i    int
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tEOF {
		p.i++
	}
	return t
}

// Binary operator precedence, low to high. Unary minus binds tighter than
// '^' in Excel (=-2^2 is 4); ':' binds tightest.
var binPrec = map[string]int{
	"=": 1, "<>": 1, "<": 1, "<=": 1, ">": 1, ">=": 1,
	"&": 2,
	"+": 3, "-": 3,
	"*": 4, "/": 4,
	"^": 5,
}

func (p *parser) expr(minPrec int) (Node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tOp {
			return left, nil
		}
		prec, ok := binPrec[t.text]
		if !ok || prec < minPrec {
			return left, nil
		}
		p.next()
		right, err := p.expr(prec + 1) // left-associative
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: t.text, l: left, r: right}
	}
}

func (p *parser) unary() (Node, error) {
	t := p.peek()
	if t.kind == tOp && (t.text == "-" || t.text == "+") {
		p.next()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: t.text, x: x}, nil
	}
	if t.kind == tAt {
		p.next()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: "@", x: x}, nil
	}
	return p.postfix()
}

func (p *parser) postfix() (Node, error) {
	x, err := p.rangeExpr()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		switch {
		case t.kind == tOp && t.text == "%":
			p.next()
			x = unaryNode{op: "%", x: x}
		case t.kind == tHash:
			p.next()
			x = spillNode{x: x}
		case t.kind == tLParen:
			// Calling the result of an expression: LAMBDA(x,x+1)(2).
			args, err := p.args()
			if err != nil {
				return nil, err
			}
			x = invokeNode{fn: x, args: args}
		default:
			return x, nil
		}
	}
}

// rangeExpr handles the ':' range operator between references.
func (p *parser) rangeExpr() (Node, error) {
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && p.peek().text == ":" {
		p.next()
		y, err := p.primary()
		if err != nil {
			return nil, err
		}
		// Sheet1!A1:B2 — the sheet applies to both ends.
		if a, ok := x.(refNode); ok {
			if b, ok := y.(refNode); ok && b.sheet == "" {
				b.sheet = a.sheet
				y = b
			}
		}
		x = binaryNode{op: ":", l: x, r: y}
	}
	return x, nil
}

func (p *parser) primary() (Node, error) {
	t := p.next()
	switch t.kind {
	case tNumber:
		return numNode{t.num}, nil
	case tString:
		return strNode{t.text}, nil
	case tBool:
		return boolNode{t.text == "TRUE"}, nil
	case tError:
		return errNode{Error(t.text)}, nil
	case tRef:
		return parseRef(t)
	case tName:
		return nameNode{name: strings.ToUpper(t.text), sheet: t.sheet}, nil
	case tFunc:
		args, err := p.args()
		if err != nil {
			return nil, err
		}
		return callNode{name: t.text, args: args}, nil
	case tLParen:
		x, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		if p.next().kind != tRParen {
			return nil, fmt.Errorf("missing )")
		}
		return x, nil
	case tLBrace:
		return p.arrayConst()
	}
	return nil, fmt.Errorf("unexpected token at %d", t.pos)
}

// args parses "(a, b, , c)"; empty arguments become missNode.
func (p *parser) args() ([]Node, error) {
	if p.next().kind != tLParen {
		return nil, fmt.Errorf("expected (")
	}
	var args []Node
	if p.peek().kind == tRParen {
		p.next()
		return args, nil
	}
	for {
		if k := p.peek().kind; k == tComma || k == tRParen {
			args = append(args, missNode{})
		} else {
			a, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			args = append(args, a)
		}
		switch p.next().kind {
		case tComma:
			continue
		case tRParen:
			return args, nil
		default:
			return nil, fmt.Errorf("expected , or )")
		}
	}
}

func (p *parser) arrayConst() (Node, error) {
	var rows [][]Node
	row := []Node{}
	for {
		var el Node
		t := p.next()
		neg := false
		if t.kind == tOp && (t.text == "-" || t.text == "+") {
			neg = t.text == "-"
			t = p.next()
		}
		switch t.kind {
		case tNumber:
			v := t.num
			if neg {
				v = -v
			}
			el = numNode{v}
		case tString:
			el = strNode{t.text}
		case tBool:
			el = boolNode{t.text == "TRUE"}
		case tError:
			el = errNode{Error(t.text)}
		default:
			return nil, fmt.Errorf("bad array constant")
		}
		row = append(row, el)
		switch p.next().kind {
		case tComma:
		case tSemi:
			rows = append(rows, row)
			row = []Node{}
		case tRBrace:
			rows = append(rows, row)
			for _, r := range rows {
				if len(r) != len(rows[0]) {
					return nil, fmt.Errorf("ragged array constant")
				}
			}
			return arrayNode{rows: rows}, nil
		default:
			return nil, fmt.Errorf("bad array constant")
		}
	}
}

func parseRef(t token) (Node, error) {
	s := strings.ReplaceAll(t.text, "$", "")
	r := refNode{sheet: t.sheet}
	a, b, isRange := strings.Cut(s, ":")
	if isRange {
		if c1, ok := colIndex(a); ok {
			c2, _ := colIndex(b)
			r.c1, r.c2, r.wholeCols = c1, c2, true
			return r, nil
		}
		r1, _ := strconv.Atoi(a)
		r2, _ := strconv.Atoi(b)
		r.r1, r.r2, r.wholeRows = r1-1, r2-1, true
		return r, nil
	}
	i := 0
	for i < len(s) && (s[i] < '0' || s[i] > '9') {
		i++
	}
	c, ok := colIndex(s[:i])
	row, err := strconv.Atoi(s[i:])
	if !ok || err != nil {
		return nil, fmt.Errorf("bad reference %q", t.text)
	}
	r.r1, r.c1, r.r2, r.c2 = row-1, c, row-1, c
	return r, nil
}

func colIndex(s string) (int, bool) {
	if s == "" || len(s) > 3 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if c < 'a' || c > 'z' {
			return 0, false
		}
		n = n*26 + int(c-'a'+1)
	}
	return n - 1, true
}
