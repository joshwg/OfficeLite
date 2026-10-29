// datatools.go holds the Data Tools group: Text to Columns and Remove
// Duplicates (Data Validation is in validation.go).

package edit

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// ColFormat is the data format of one Text to Columns field.
type ColFormat int

const (
	ColGeneral ColFormat = iota // as typed: numbers, dates, text
	ColText                     // text, in a Text-formatted cell
	ColDateMDY                  // a date in the order given
	ColDateDMY
	ColDateYMD
	ColDateMYD
	ColDateDYM
	ColDateYDM
	ColSkip // Do not import column
)

// ColFormatNames name the formats as the Text to Columns dialog lists
// them.
var ColFormatNames = []string{"General", "Text", "Date: MDY", "Date: DMY", "Date: YMD", "Date: MYD", "Date: DYM", "Date: YDM", "Do not import (skip)"}

// TextToColsSpec is how Text to Columns splits each cell.
type TextToColsSpec struct {
	Fixed bool // fixed width at Breaks; else delimited
	// Delims are the delimiter characters (tab, semicolon, comma, space,
	// another); Consecutive treats a run of them as one.
	Delims      string
	Consecutive bool
	// Qualifier quotes fields that hold delimiters ('"', '\'' or 0).
	Qualifier rune
	Breaks    []int       // fixed width: the character positions fields start at
	Formats   []ColFormat // per field; fields past the end are General
}

// ErrTextToColsShape is Excel's refusal to split more than one column.
var ErrTextToColsShape = errors.New("Text to Columns can only convert one column at a time. The range can be many rows tall but no more than one column wide. Try again by selecting cells in one column only.")

// SplitFields splits one cell's text into fields.
func SplitFields(s string, spec TextToColsSpec) []string {
	if spec.Fixed {
		rs := []rune(s)
		breaks := slices.Clone(spec.Breaks)
		slices.Sort(breaks)
		breaks = slices.Compact(breaks)
		var out []string
		start := 0
		for _, b := range append(breaks, len(rs)) {
			if b <= start {
				continue
			}
			out = append(out, strings.TrimSpace(string(rs[min(start, len(rs)):min(b, len(rs))])))
			start = b
		}
		if len(out) == 0 {
			out = []string{strings.TrimSpace(s)}
		}
		return out
	}
	isDelim := func(r rune) bool { return strings.ContainsRune(spec.Delims, r) }
	rs := []rune(s)
	var out []string
	var f strings.Builder
	i := 0
	for {
		// One field: a qualified part (with doubled qualifiers inside),
		// then anything up to the next delimiter.
		f.Reset()
		if q := spec.Qualifier; q != 0 && i < len(rs) && rs[i] == q {
			i++
			for i < len(rs) {
				if rs[i] == q {
					if i+1 < len(rs) && rs[i+1] == q {
						f.WriteRune(q)
						i += 2
						continue
					}
					i++
					break
				}
				f.WriteRune(rs[i])
				i++
			}
		}
		for i < len(rs) && !isDelim(rs[i]) {
			f.WriteRune(rs[i])
			i++
		}
		out = append(out, f.String())
		if i >= len(rs) {
			return out
		}
		i++ // the delimiter
		if spec.Consecutive {
			for i < len(rs) && isDelim(rs[i]) {
				i++
			}
			if i >= len(rs) {
				return out
			}
		}
	}
}

// ParseDateOrder reads text such as 15/3/2024 or 2024-03-15 as a date with
// its day, month and year in the order of f (ColDateDMY, ...); 2-digit
// years are 1930-2029, as Excel takes them.
func ParseDateOrder(s string, f ColFormat, date1904 bool) (float64, bool) {
	parts := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || unicode.IsSpace(r)
	})
	if len(parts) != 3 {
		return 0, false
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return 0, false
		}
		n[i] = v
	}
	order := map[ColFormat][3]byte{
		ColDateMDY: {'m', 'd', 'y'}, ColDateDMY: {'d', 'm', 'y'}, ColDateYMD: {'y', 'm', 'd'},
		ColDateMYD: {'m', 'y', 'd'}, ColDateDYM: {'d', 'y', 'm'}, ColDateYDM: {'y', 'd', 'm'},
	}[f]
	var y, m, d int
	for i, k := range order {
		switch k {
		case 'y':
			y = n[i]
			if len(parts[i]) <= 2 {
				if y < 30 {
					y += 2000
				} else {
					y += 1900
				}
			}
		case 'm':
			m = n[i]
		case 'd':
			d = n[i]
		}
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return 0, false
	}
	v, ok := formula.DateSerial(y, m, d, date1904)
	if !ok {
		return 0, false
	}
	if y2, m2, d2 := formula.SerialToYMD(v, date1904); y2 != y || m2 != m || d2 != d {
		return 0, false // 31 April
	}
	return v, true
}

// textToColsCell is one field as a cell with the style of the cell it
// lands on.
func (b *Book) textToColsCell(style uint32, s string, f ColFormat) model.Cell {
	switch f {
	case ColText:
		st := b.style(style)
		st.NumFmtID, st.NumFmtCode = 49, "@"
		c := model.Cell{Style: b.addStyle(st)}
		if s != "" {
			c.Kind, c.Text = model.KindText, s
		}
		return c
	case ColGeneral:
		if c, err := b.inputCell(style, s); err == nil {
			return c
		}
	default:
		if v, ok := ParseDateOrder(s, f, b.WB.Date1904); ok {
			return b.makeCell(style, Input{Kind: model.KindNumber, Num: v, Format: "m/d/yyyy"})
		}
	}
	if s == "" {
		return model.Cell{Style: style}
	}
	return model.Cell{Style: style, Kind: model.KindText, Text: s}
}

// textToColsPlan is what Text to Columns writes: the cells by position.
func (b *Book) textToColsPlan(sheet int, src model.Range, row, col int, spec TextToColsSpec) map[[2]int]model.Cell {
	sh := b.WB.Sheets[sheet]
	out := map[[2]int]model.Cell{}
	last := min(src.R2, sh.UsedRows()-1)
	for r := src.R1; r <= last; r++ {
		c := sh.Get(r, src.C1)
		if c.IsBlank() {
			continue
		}
		dc := col
		for i, field := range SplitFields(c.Text, spec) {
			f := ColGeneral
			if i < len(spec.Formats) {
				f = spec.Formats[i]
			}
			if f == ColSkip {
				continue
			}
			if dc < model.MaxCols {
				rr := row + r - src.R1
				out[[2]int{rr, dc}] = b.textToColsCell(sh.Get(rr, dc).Style, field, f)
			}
			dc++
		}
	}
	return out
}

// TextToColumnsOverwrites reports whether Text to Columns would replace
// nonblank cells other than the ones being split, for Excel's "Do you
// want to replace the contents of the destination cells?".
func (b *Book) TextToColumnsOverwrites(sheet int, src model.Range, row, col int, spec TextToColsSpec) bool {
	sh := b.WB.Sheets[sheet]
	for k := range b.textToColsPlan(sheet, src, row, col, spec) {
		if (k[1] != src.C1 || !src.Contains(k[0], k[1])) && !sh.Get(k[0], k[1]).IsBlank() {
			return true
		}
	}
	return false
}

// TextToColumns splits the text of the one-column range src into fields
// written from row, col on (Data > Text to Columns). The source cells
// that are split and not written over keep their content, as in Excel.
// Undoable as one step.
func (b *Book) TextToColumns(sheet int, src model.Range, row, col int, spec TextToColsSpec) (calc.Result, error) {
	defer b.Op("Text to Columns")()
	if src.C1 != src.C2 {
		return calc.Result{}, ErrTextToColsShape
	}
	sh := b.WB.Sheets[sheet]
	child := spillChildren(sh)
	plan := b.textToColsPlan(sheet, src, row, col, spec)
	keys := make([][2]int, 0, len(plan))
	for k := range plan {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	var cells []cellChange
	bounds := model.Range{R1: row, C1: col, R2: row, C2: col}
	for _, k := range keys {
		before := snapshot(sh, child, k[0], k[1])
		after := plan[k]
		bounds.R2, bounds.C2 = max(bounds.R2, k[0]), max(bounds.C2, k[1])
		if before == after {
			continue
		}
		cells = append(cells, cellChange{sheet, k[0], k[1], before, after})
	}
	return b.apply(Where{sheet, bounds}, cells), nil
}

// RemoveDuplicates removes the rows of r whose values in the columns cols
// (sheet columns) repeat an earlier row's, comparing as Excel does: text
// ignoring case, numbers by value. The rows below move up within r only,
// and the rows freed at its bottom are emptied. With header r's first
// row stays out of it. It returns how many rows went and how many are
// left. Undoable as one step.
func (b *Book) RemoveDuplicates(sheet int, r model.Range, cols []int, header bool) (removed, remain int, res calc.Result) {
	defer b.Op("Remove Duplicates")()
	sh := b.WB.Sheets[sheet]
	r.R2 = min(r.R2, sh.UsedRows()-1)
	first := r.R1
	if header {
		first++
	}
	if first > r.R2 || len(cols) == 0 {
		return 0, 0, calc.Result{}
	}
	seen := map[string]bool{}
	var keep []int
	var k strings.Builder
	for row := first; row <= r.R2; row++ {
		k.Reset()
		for _, c := range cols {
			k.WriteString(dupKey(sh.Get(row, c)))
			k.WriteByte(0)
		}
		if seen[k.String()] {
			continue
		}
		seen[k.String()] = true
		keep = append(keep, row)
	}
	removed = r.R2 - first + 1 - len(keep)
	if removed == 0 {
		return 0, len(keep), calc.Result{}
	}
	child := spillChildren(sh)
	var cells []cellChange
	for i := first; i <= r.R2; i++ {
		for c := r.C1; c <= r.C2; c++ {
			before := snapshot(sh, child, i, c)
			var after model.Cell
			if j := i - first; j < len(keep) {
				after = snapshot(sh, child, keep[j], c)
				if after.Formula != "" {
					after.Formula = formula.Shift(after.Formula, i-keep[j], 0)
				}
			}
			if before != after {
				cells = append(cells, cellChange{sheet, i, c, before, after})
			}
		}
	}
	return removed, len(keep), b.apply(Where{sheet, r}, cells)
}

// dupKey is a cell's value for comparing rows.
func dupKey(c *model.Cell) string {
	switch c.Kind {
	case model.KindNumber:
		return "n" + strconv.FormatFloat(c.Num, 'g', -1, 64)
	case model.KindText:
		return "t" + strings.ToLower(c.Text)
	case model.KindBool:
		return "b" + c.Text
	case model.KindError:
		return "e" + c.Text
	}
	return ""
}
