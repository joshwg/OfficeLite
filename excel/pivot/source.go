// Package pivot computes PivotTable reports: it reads the source data
// into fields and items, groups and summarizes the records, and lays the
// report out cell by cell (compact, outline or tabular form, subtotals,
// grand totals, report filters) with the look of Excel's default
// PivotStyleLight16. excel/edit writes the cells into the sheet.
package pivot

import (
	"cmp"
	"errors"
	"strconv"
	"strings"

	"officelite/excel/model"
)

// ErrFieldName is Excel's refusal of source data without a header for
// every column.
var ErrFieldName = errors.New("The PivotTable field name is not valid. To create a PivotTable report, you must use data that is organized as a list with labeled columns. If you are changing the name of a PivotTable field, you must type a new name for the field.")

// Item is a distinct value of a field. Items are told apart as Excel
// tells them apart: numbers (and dates) by value, text ignoring case.
type Item struct {
	Key   string // see ItemKey
	Label string // as shown: the first cell's displayed text, "(blank)"
	Kind  model.Kind
	Num   float64
	Style uint32 // the first cell's style (its number format labels a number)
}

// Blank is the label of the blank item.
const Blank = "(blank)"

// Field is a source column.
type Field struct {
	Name  string
	Items []Item
	Rec   []int32 // the item of each record
	index map[string]int32
	// Numeric is set when every record holds a number: a field that
	// becomes a value field is summed (otherwise counted), and checking
	// it in the field list puts it in Values.
	Numeric bool
	Date    bool // some record holds a date
}

// Source is the data a PivotTable summarizes: the columns of a range as
// fields, its rows below the header row as records.
type Source struct {
	Sheet  int
	Range  model.Range
	Fields []*Field
	N      int // records
}

// ItemKey identifies a cell's value as an item: "" for a blank, "n:"
// and the number, "s:" and the lower-cased text, "b:" or "e:" and the
// value's text.
func ItemKey(c *model.Cell) string {
	switch c.Kind {
	case model.KindNumber:
		return NumKey(c.Num)
	case model.KindText:
		if c.Text == "" {
			return ""
		}
		return "s:" + strings.ToLower(c.Text)
	case model.KindBool:
		return "b:" + strings.ToUpper(c.Text)
	case model.KindError:
		return "e:" + c.Text
	}
	return ""
}

// NumKey is the key of a number item.
func NumKey(v float64) string { return "n:" + strconv.FormatFloat(v, 'g', -1, 64) }

// TextKey is the key of a text item.
func TextKey(s string) string {
	if s == "" {
		return ""
	}
	return "s:" + strings.ToLower(s)
}

// ReadSource reads range r of sheet sh (index sheet) as source data. Its
// first row names the fields; a blank name is ErrFieldName and repeated
// names get a number (Qty, Qty2). isDate reports whether a style's number
// format shows dates.
func ReadSource(sh *model.Sheet, sheet int, r model.Range, isDate func(style uint32) bool) (*Source, error) {
	r.R2 = min(r.R2, max(sh.UsedRows()-1, r.R1))
	src := &Source{Sheet: sheet, Range: r, N: r.R2 - r.R1}
	used := map[string]bool{}
	for c := r.C1; c <= r.C2; c++ {
		name := strings.TrimSpace(sh.Get(r.R1, c).Text)
		if name == "" {
			return nil, ErrFieldName
		}
		base := name
		for k := 2; used[strings.ToLower(name)]; k++ {
			name = base + strconv.Itoa(k)
		}
		used[strings.ToLower(name)] = true
		f := &Field{Name: name, Rec: make([]int32, src.N), index: map[string]int32{}, Numeric: src.N > 0}
		for i := range src.N {
			cell := sh.Get(r.R1+1+i, c)
			k := ItemKey(cell)
			if cell.Kind != model.KindNumber {
				f.Numeric = false
			} else if !f.Date && isDate != nil && isDate(cell.Style) {
				f.Date = true
			}
			idx, ok := f.index[k]
			if !ok {
				it := Item{Key: k, Label: cell.Text, Kind: cell.Kind, Num: cell.Num, Style: cell.Style}
				if k == "" {
					it = Item{Label: Blank}
				}
				idx = int32(len(f.Items))
				f.Items = append(f.Items, it)
				f.index[k] = idx
			}
			f.Rec[i] = idx
		}
		src.Fields = append(src.Fields, f)
	}
	return src, nil
}

// Item returns the index of the item with key k, or -1.
func (f *Field) Item(k string) int {
	if i, ok := f.index[k]; ok {
		return int(i)
	}
	return -1
}

// Names lists the field names.
func (s *Source) Names() []string {
	out := make([]string, len(s.Fields))
	for i, f := range s.Fields {
		out[i] = f.Name
	}
	return out
}

// kindRank orders item kinds as Excel sorts them: numbers, text,
// logical values, errors, blanks last.
func kindRank(it *Item) int {
	switch {
	case it.Key == "":
		return 4
	case it.Kind == model.KindNumber:
		return 0
	case it.Kind == model.KindText:
		return 1
	case it.Kind == model.KindBool:
		return 2
	}
	return 3
}

// compareItems orders two items ascending.
func compareItems(a, b *Item) int {
	if c := cmp.Compare(kindRank(a), kindRank(b)); c != 0 {
		return c
	}
	if a.Kind == model.KindNumber {
		return cmp.Compare(a.Num, b.Num)
	}
	return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
}
