package xlsxfile

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// The AutoFilter: <autoFilter ref> with a <filterColumn> per filtered
// column (value lists, custom, Top 10 and dynamic filters), and the
// hidden _xlnm._FilterDatabase name Excel keeps for it. Color and icon
// filters are dropped with a warning.

var filterOps = []string{
	model.OpEqual:        "equal",
	model.OpNotEqual:     "notEqual",
	model.OpGreater:      "greaterThan",
	model.OpGreaterEqual: "greaterThanOrEqual",
	model.OpLess:         "lessThan",
	model.OpLessEqual:    "lessThanOrEqual",
}

// autoFilterReader builds a sheet's AutoFilter from its elements as the
// worksheet is streamed.
type autoFilterReader struct {
	f       *model.AutoFilter
	col     *model.FilterColumn
	skip    bool // the column holds a filter that isn't modeled
	dropped bool
}

func (a *autoFilterReader) start(t xml.StartElement, sh *model.Sheet) {
	if t.Name.Local == "autoFilter" {
		if rng, ok := parseRange(attr(t, "ref")); ok {
			a.f = &model.AutoFilter{Range: rng}
		} else if r, c, ok := parseCellRef(attr(t, "ref")); ok {
			a.f = &model.AutoFilter{Range: model.Range{R1: r, C1: c, R2: r, C2: c}}
		}
		sh.AutoFilter = a.f
		return
	}
	if a.f == nil {
		return
	}
	if t.Name.Local == "filterColumn" {
		id, err := strconv.Atoi(attr(t, "colId"))
		if err != nil {
			return
		}
		a.col = &model.FilterColumn{Col: a.f.Range.C1 + id,
			HideButton: xmlBool(attr(t, "hiddenButton")) || attr(t, "showButton") == "0" || attr(t, "showButton") == "false"}
		a.skip = false
		return
	}
	fc := a.col
	if fc == nil {
		return
	}
	switch t.Name.Local {
	case "filters":
		fc.Kind = model.FilterValues
		fc.Blanks = xmlBool(attr(t, "blank"))
	case "filter":
		fc.Values = append(fc.Values, attr(t, "val"))
	case "dateGroupItem":
		n := func(name string) int { v, _ := strconv.Atoi(attr(t, name)); return v }
		fc.Dates = append(fc.Dates, model.DateGroup{Year: n("year"), Month: n("month"), Day: n("day"),
			Hour: n("hour"), Minute: n("minute"), Second: n("second"), Grouping: attr(t, "dateTimeGrouping")})
	case "customFilters":
		fc.Kind = model.FilterCustom
		fc.And = xmlBool(attr(t, "and"))
	case "customFilter":
		op := model.OpEqual
		for i, name := range filterOps {
			if name == attr(t, "operator") {
				op = model.FilterOp(i)
			}
		}
		fc.Conds = append(fc.Conds, model.FilterCond{Op: op, Val: attr(t, "val")})
	case "top10":
		fc.Kind = model.FilterTop10
		top := attr(t, "top")
		fc.Top10 = model.Top10{Top: top == "" || xmlBool(top), Percent: xmlBool(attr(t, "percent")),
			N: parseFloat(attr(t, "val")), Val: parseFloat(attr(t, "filterVal"))}
	case "dynamicFilter":
		fc.Kind = model.FilterDynamic
		fc.Dynamic = attr(t, "type")
		fc.DynamicVal = parseFloat(attr(t, "val"))
	case "colorFilter", "iconFilter":
		a.skip, a.dropped = true, true
	}
}

func (a *autoFilterReader) endColumn() {
	if a.col != nil && a.f != nil && !a.skip {
		a.f.SetColumn(a.col.Col, a.col)
	}
	a.col = nil
}

// autoFilterXML is the <autoFilter> element for f.
func autoFilterXML(f *model.AutoFilter) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<autoFilter ref="%s">`, rangeRef(f.Range))
	num := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	for _, fc := range f.Columns {
		if fc.Col < f.Range.C1 || fc.Col > f.Range.C2 {
			continue
		}
		fmt.Fprintf(&b, `<filterColumn colId="%d"`, fc.Col-f.Range.C1)
		if fc.HideButton {
			b.WriteString(` hiddenButton="1"`)
		}
		b.WriteByte('>')
		switch fc.Kind {
		case model.FilterValues:
			b.WriteString(`<filters`)
			if fc.Blanks {
				b.WriteString(` blank="1"`)
			}
			b.WriteByte('>')
			for _, v := range fc.Values {
				fmt.Fprintf(&b, `<filter val="%s"/>`, xmlEscape(v))
			}
			for _, g := range fc.Dates {
				fmt.Fprintf(&b, `<dateGroupItem year="%d"`, g.Year)
				for _, p := range []struct {
					name string
					v    int
				}{{"month", g.Month}, {"day", g.Day}, {"hour", g.Hour}, {"minute", g.Minute}, {"second", g.Second}} {
					if p.v != 0 {
						fmt.Fprintf(&b, ` %s="%d"`, p.name, p.v)
					}
				}
				grouping := g.Grouping
				if grouping == "" {
					grouping = "day"
				}
				fmt.Fprintf(&b, ` dateTimeGrouping="%s"/>`, grouping)
			}
			b.WriteString(`</filters>`)
		case model.FilterCustom:
			b.WriteString(`<customFilters`)
			if fc.And {
				b.WriteString(` and="1"`)
			}
			b.WriteByte('>')
			for _, cd := range fc.Conds[:min(len(fc.Conds), 2)] {
				b.WriteString(`<customFilter`)
				if cd.Op != model.OpEqual && int(cd.Op) < len(filterOps) {
					fmt.Fprintf(&b, ` operator="%s"`, filterOps[cd.Op])
				}
				fmt.Fprintf(&b, ` val="%s"/>`, xmlEscape(cd.Val))
			}
			b.WriteString(`</customFilters>`)
		case model.FilterTop10:
			t := fc.Top10
			b.WriteString(`<top10`)
			if !t.Top {
				b.WriteString(` top="0"`)
			}
			if t.Percent {
				b.WriteString(` percent="1"`)
			}
			fmt.Fprintf(&b, ` val="%s" filterVal="%s"/>`, num(t.N), num(t.Val))
		case model.FilterDynamic:
			fmt.Fprintf(&b, `<dynamicFilter type="%s"`, xmlEscape(fc.Dynamic))
			if fc.DynamicVal != 0 {
				fmt.Fprintf(&b, ` val="%s"`, num(fc.DynamicVal))
			}
			b.WriteString(`/>`)
		}
		b.WriteString(`</filterColumn>`)
	}
	b.WriteString(`</autoFilter>`)
	return b.String()
}

func rangeRef(r model.Range) string {
	s := model.CellName(r.R1, r.C1)
	if r.R2 != r.R1 || r.C2 != r.C1 {
		s += ":" + model.CellName(r.R2, r.C2)
	}
	return s
}

// filterDatabase is the hidden defined name Excel writes for a sheet's
// AutoFilter, sheet being its index in the workbook.
func filterDatabase(sheet int, name string, r model.Range) string {
	abs := func(row, col int) string { return "$" + model.ColName(col) + "$" + strconv.Itoa(row+1) }
	return fmt.Sprintf(`<definedName name="_xlnm._FilterDatabase" localSheetId="%d" hidden="1">%s!%s:%s</definedName>`,
		sheet, xmlEscape(formula.QuoteSheet(name)), abs(r.R1, r.C1), abs(r.R2, r.C2))
}

// addFilterNames adds the _FilterDatabase names to workbook.xml, which
// excelize won't write itself.
func (x *writer) addFilterNames(b []byte) ([]byte, error) {
	names := strings.Join(x.filterNames, "")
	if i := bytes.Index(b, []byte("<definedNames>")); i >= 0 {
		i += len("<definedNames>")
		return append(b[:i:i], append([]byte(names), b[i:]...)...), nil
	}
	i := bytes.Index(b, []byte("</sheets>"))
	if i < 0 {
		return nil, fmt.Errorf("no sheets")
	}
	i += len("</sheets>")
	return append(b[:i:i], append([]byte("<definedNames>"+names+"</definedNames>"), b[i:]...)...), nil
}
