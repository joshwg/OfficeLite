package numbersfile

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"officelite/common/iwork"
	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

// Formulas are stored as their abstract syntax tree in postfix order
// (TSCE.ASTNodeArrayArchive); these are the node types translated.
const (
	nodeAdd          = 1
	nodeSub          = 2
	nodeMul          = 3
	nodeDiv          = 4
	nodePow          = 5
	nodeConcat       = 6
	nodeGT           = 7
	nodeGE           = 8
	nodeLT           = 9
	nodeLE           = 10
	nodeEQ           = 11
	nodeNE           = 12
	nodeNegate       = 13
	nodePlus         = 14
	nodePercent      = 15
	nodeFunction     = 16
	nodeNumber       = 17
	nodeBool         = 18
	nodeString       = 19
	nodeDate         = 20
	nodeEmptyArg     = 22
	nodeToken        = 23
	nodeList         = 25
	nodeColon        = 29
	nodeAppendSpace  = 32
	nodePrependSpace = 33
	nodeBeginThunk   = 34
	nodeEndThunk     = 35
	nodeCellRef      = 36
	nodeColonTract   = 67
)

var binaryOps = map[uint64]string{
	nodeAdd: "+", nodeSub: "-", nodeMul: "*", nodeDiv: "/", nodePow: "^", nodeConcat: "&",
	nodeGT: ">", nodeGE: ">=", nodeLT: "<", nodeLE: "<=", nodeEQ: "=", nodeNE: "<>",
}

var errUntranslatable = errors.New("formula can't be translated")

// operand is a translated subexpression; ref marks a single-cell
// reference (which a colon can join into a range).
type operand struct {
	text string
	ref  bool
}

// translator turns Numbers formulas into Excel formulas for the tables
// of one document. Each table is a sheet with the table at A1, so a
// table's own cell coordinates are the sheet's.
type translator struct {
	sheets map[string]string // table UUID (hex, no dashes) -> sheet name
	tables map[string]*iwork.Table
	reg    *formula.Registry
}

func newTranslator() *translator {
	return &translator{sheets: map[string]string{}, tables: map[string]*iwork.Table{}, reg: functions.Default()}
}

// add registers a table under its sheet name, for cross-table references.
func (tr *translator) add(t *iwork.Table, sheet string, uuid string) {
	key := strings.ToUpper(strings.ReplaceAll(uuid, "-", ""))
	tr.sheets[key], tr.tables[key] = sheet, t
}

// uuidKey turns a TSP.CFUUIDArchive into the form table IDs are kept in.
func uuidKey(m iwork.Message) string {
	if b := m.Bytes(1); len(b) == 16 {
		return fmt.Sprintf("%X", b)
	}
	var s strings.Builder
	for _, n := range []int{2, 3, 4, 5} {
		w := uint32(m.Uint(n))
		fmt.Fprintf(&s, "%02X%02X%02X%02X", byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
	}
	return s.String()
}

// translate converts the formula of the cell at row, col of table t; it
// fails for anything Excel (or ExcelLite) can't express the same way:
// Numbers-only functions, durations, references to whole columns of
// other tables...
func (tr *translator) translate(f iwork.Message, t *iwork.Table, row, col int) (string, error) {
	var stack []operand
	pop := func() (operand, error) {
		if len(stack) == 0 {
			return operand{}, errUntranslatable
		}
		o := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return o, nil
	}
	popn := func(n int) ([]string, error) {
		if n > len(stack) {
			return nil, errUntranslatable
		}
		args := make([]string, n)
		for i, o := range stack[len(stack)-n:] {
			args[i] = o.text
		}
		stack = stack[:len(stack)-n]
		return args, nil
	}
	for _, n := range f.Msg(1).Msgs(1) {
		typ := n.Uint(1)
		if op, ok := binaryOps[typ]; ok {
			b, err := pop()
			if err != nil {
				return "", err
			}
			a, err := pop()
			if err != nil {
				return "", err
			}
			stack = append(stack, operand{text: a.text + op + b.text})
			continue
		}
		switch typ {
		case nodeNegate, nodePercent:
			a, err := pop()
			if err != nil {
				return "", err
			}
			if typ == nodeNegate {
				stack = append(stack, operand{text: "-" + a.text})
			} else {
				stack = append(stack, operand{text: a.text + "%"})
			}
		case nodePlus, nodeAppendSpace, nodePrependSpace, nodeBeginThunk, nodeEndThunk:
		case nodeFunction:
			name, ok := numbersFunctions[n.Uint(2)]
			if !ok || tr.reg.Lookup(name) == nil {
				return "", errUntranslatable
			}
			args, err := popn(int(n.Uint(3)))
			if err != nil {
				return "", err
			}
			stack = append(stack, operand{text: name + "(" + strings.Join(args, ",") + ")"})
		case nodeNumber:
			text := strconv.FormatFloat(n.Double(4), 'g', -1, 64)
			if n.Uint(43) == 0x3040000000000000 {
				text = strconv.FormatUint(n.Uint(42), 10) // an integer decimal
			}
			stack = append(stack, operand{text: text})
		case nodeBool, nodeToken:
			v := n.Bool(5)
			if typ == nodeToken {
				v = n.Bool(10)
			}
			stack = append(stack, operand{text: strings.ToUpper(strconv.FormatBool(v))})
		case nodeString:
			stack = append(stack, operand{text: `"` + strings.ReplaceAll(n.Str(6), `"`, `""`) + `"`})
		case nodeDate:
			d := iwork.Epoch.Add(time.Duration(n.Double(7) * float64(time.Second)))
			stack = append(stack, operand{text: fmt.Sprintf("DATE(%d,%d,%d)", d.Year(), d.Month(), d.Day())})
		case nodeEmptyArg:
			stack = append(stack, operand{})
		case nodeList:
			args, err := popn(int(n.Uint(13)))
			if err != nil {
				return "", err
			}
			stack = append(stack, operand{text: "(" + strings.Join(args, ",") + ")"})
		case nodeColon:
			b, err := pop()
			if err != nil {
				return "", err
			}
			a, err := pop()
			if err != nil {
				return "", err
			}
			if !a.ref || !b.ref {
				return "", errUntranslatable
			}
			bt := b.text
			if i := strings.LastIndexByte(bt, '!'); i >= 0 {
				bt = bt[i+1:]
			}
			stack = append(stack, operand{text: a.text + ":" + bt})
		case nodeCellRef, nodeColonTract:
			o, err := tr.reference(n, t, row, col)
			if err != nil {
				return "", err
			}
			stack = append(stack, o)
		default:
			return "", errUntranslatable
		}
	}
	if len(stack) != 1 || stack[0].text == "" {
		return "", errUntranslatable
	}
	text := stack[0].text
	if _, err := formula.Parse(text); err != nil {
		return "", errUntranslatable
	}
	return text, nil
}

// zigzag decodes a sint32 field.
func zigzag(v uint64) int { return int(int64(v>>1) ^ -int64(v&1)) }

// reference translates a cell reference or colon tract node.
func (tr *translator) reference(n iwork.Message, t *iwork.Table, row, col int) (operand, error) {
	target, prefix := t, ""
	if x := n.Msg(28); x != nil { // another table
		key := uuidKey(x.Msg(1))
		target = tr.tables[key]
		if target == nil {
			return operand{}, errUntranslatable
		}
		prefix = formula.QuoteSheet(tr.sheets[key]) + "!"
	}
	cell := func(r, c int, rAbs, cAbs bool) string {
		s := ""
		if cAbs {
			s += "$"
		}
		s += model.ColName(c)
		if rAbs {
			s += "$"
		}
		return s + strconv.Itoa(r+1)
	}
	if typ := n.Uint(1); typ == nodeColonTract {
		tract, sticky := n.Msg(40), n.Msg(33)
		span := func(abs bool, absList, relList []iwork.Message, at int) (int, int, bool) {
			switch {
			case abs && len(absList) > 0:
				b := int(absList[0].Uint(1))
				e := b
				if v, ok := absList[0].UintOK(2); ok {
					e = int(v)
				}
				return b, e, true
			case len(relList) > 0:
				b := at + int(relList[0].Int(1))
				e := b
				if _, ok := relList[0].UintOK(2); ok {
					e = at + int(relList[0].Int(2))
				}
				return b, e, true
			}
			return 0, 0, false
		}
		r1, r2, okR := span(sticky.Bool(1), tract.Msgs(4), tract.Msgs(2), row)
		c1, c2, okC := span(sticky.Bool(2), tract.Msgs(3), tract.Msgs(1), col)
		if !okR || !okC || r1 < 0 || c1 < 0 || r2 >= 0x7fff || c2 >= 0x7fff {
			return operand{}, errUntranslatable // whole rows or columns
		}
		ref := cell(r1, c1, sticky.Bool(1), sticky.Bool(2))
		if r2 != r1 || c2 != c1 {
			ref += ":" + cell(r2, c2, sticky.Bool(3), sticky.Bool(4))
		}
		return operand{text: prefix + ref, ref: r1 == r2 && c1 == c2}, nil
	}
	cm, rm := n.Msg(26), n.Msg(27)
	r, c := row, col
	if cm != nil {
		c = zigzag(cm.Uint(1))
		if !cm.Bool(2) {
			c += col
		}
	}
	if rm != nil {
		r = zigzag(rm.Uint(1))
		if !rm.Bool(2) {
			r += row
		}
	}
	switch {
	case r < 0 || c < 0:
		return operand{}, errUntranslatable
	case cm != nil && rm != nil:
		return operand{text: prefix + cell(r, c, rm.Bool(2), cm.Bool(2)), ref: true}, nil
	case cm != nil: // a column: Numbers means its body cells
		r1, r2 := target.HeaderRows, target.Rows-target.FooterRows-1
		if r2 < r1 {
			return operand{}, errUntranslatable
		}
		return operand{text: prefix + cell(r1, c, true, cm.Bool(2)) + ":" + cell(r2, c, true, cm.Bool(2))}, nil
	case rm != nil: // a row: its cells right of the header columns
		c1, c2 := target.HeaderCols, target.Cols-1
		if c2 < c1 {
			return operand{}, errUntranslatable
		}
		return operand{text: prefix + cell(r, c1, rm.Bool(2), true) + ":" + cell(r, c2, rm.Bool(2), true)}, nil
	}
	return operand{}, errUntranslatable
}

// verify recalculates the translated formulas and keeps only those that
// give the value Numbers saved: a function that behaves differently in
// Excel (or reads Numbers' header rows differently) must not change the
// sheet when it is recalculated. It returns how many formulas it
// dropped.
func verify(wb *model.Workbook, reg *formula.Registry) int {
	type cellKey struct{ sheet, row, col int }
	saved := map[cellKey]model.Cell{}
	for si, sh := range wb.Sheets {
		for r := range sh.UsedRows() {
			for c := range sh.RowLen(r) {
				if cell := sh.Get(r, c); cell.Formula != "" {
					saved[cellKey{si, r, c}] = *cell
				}
			}
		}
	}
	dropped := 0
	for range 4 { // each dropped formula can change what its dependents compute
		calc.Recalculate(wb, reg, calc.Full, time.Now())
		changed := false
		for k, want := range saved {
			sh := wb.Sheets[k.sheet]
			got := sh.Get(k.row, k.col)
			if got.Formula == "" || sameValue(got, &want) {
				continue
			}
			want.Formula = ""
			sh.Set(k.row, k.col, want)
			dropped++
			changed = true
		}
		if !changed {
			break
		}
	}
	// Whatever was recalculated keeps Numbers' value and display.
	for k, want := range saved {
		sh := wb.Sheets[k.sheet]
		f := sh.Get(k.row, k.col).Formula
		want.Formula = f
		sh.Set(k.row, k.col, want)
	}
	return dropped
}

func sameValue(a, b *model.Cell) bool {
	if a.Kind != b.Kind {
		return a.Kind == model.KindEmpty && b.Kind == model.KindText && b.Text == "" ||
			b.Kind == model.KindEmpty && a.Kind == model.KindText && a.Text == ""
	}
	switch a.Kind {
	case model.KindNumber, model.KindBool:
		return math.Abs(a.Num-b.Num) <= 1e-9*math.Max(1, math.Abs(b.Num))
	case model.KindText:
		return a.Text == b.Text
	}
	return true
}
