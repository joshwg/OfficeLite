package xlsxfile

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"
	"time"

	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/pivot"
)

// PivotTables: a worksheet's relationships point at its
// pivotTableDefinition parts, each of which points at a
// pivotCacheDefinition (the source and its fields' items). The reader
// takes the definition; the report's cells are the worksheet's. The
// writer (pivotwrite.go) writes the cache with its records again.

type xPivotCache struct {
	Source struct {
		Type string `xml:"type,attr"`
		WS   struct {
			Ref   string `xml:"ref,attr"`
			Sheet string `xml:"sheet,attr"`
			Name  string `xml:"name,attr"`
		} `xml:"worksheetSource"`
	} `xml:"cacheSource"`
	Fields []struct {
		Name  string `xml:"name,attr"`
		Items struct {
			List []xSharedItem `xml:",any"`
		} `xml:"sharedItems"`
	} `xml:"cacheFields>cacheField"`
}

type xSharedItem struct {
	XMLName xml.Name
	V       string `xml:"v,attr"`
}

type xPivotTable struct {
	Name           string `xml:"name,attr"`
	Compact        string `xml:"compact,attr"`
	Outline        string `xml:"outline,attr"`
	RowGrandTotals string `xml:"rowGrandTotals,attr"`
	ColGrandTotals string `xml:"colGrandTotals,attr"`
	Location       struct {
		Ref            string `xml:"ref,attr"`
		FirstHeaderRow int    `xml:"firstHeaderRow,attr"`
		FirstDataRow   int    `xml:"firstDataRow,attr"`
		FirstDataCol   int    `xml:"firstDataCol,attr"`
		RowPageCount   int    `xml:"rowPageCount,attr"`
	} `xml:"location"`
	Fields []struct {
		Axis            string `xml:"axis,attr"`
		Name            string `xml:"name,attr"`
		Compact         string `xml:"compact,attr"`
		Outline         string `xml:"outline,attr"`
		SortType        string `xml:"sortType,attr"`
		DefaultSubtotal string `xml:"defaultSubtotal,attr"`
		SubtotalTop     string `xml:"subtotalTop,attr"`
		InsertBlankRow  string `xml:"insertBlankRow,attr"`
		Items           []struct {
			X string `xml:"x,attr"`
			T string `xml:"t,attr"`
			H string `xml:"h,attr"`
		} `xml:"items>item"`
		Ext []struct {
			Field struct {
				FillDownLabels string `xml:"fillDownLabels,attr"`
			} `xml:"pivotField"`
		} `xml:"extLst>ext"`
	} `xml:"pivotFields>pivotField"`
	RowFields []struct {
		X int `xml:"x,attr"`
	} `xml:"rowFields>field"`
	ColFields []struct {
		X int `xml:"x,attr"`
	} `xml:"colFields>field"`
	PageFields []struct {
		Fld  int     `xml:"fld,attr"`
		Item *string `xml:"item,attr"`
	} `xml:"pageFields>pageField"`
	DataFields []struct {
		Name       string `xml:"name,attr"`
		Fld        int    `xml:"fld,attr"`
		Subtotal   string `xml:"subtotal,attr"`
		ShowDataAs string `xml:"showDataAs,attr"`
		NumFmtID   int    `xml:"numFmtId,attr"`
	} `xml:"dataFields>dataField"`
	Style struct {
		Name string `xml:"name,attr"`
	} `xml:"pivotTableStyleInfo"`
}

// pivotFuncs are the dataField subtotal names, by model.PivotFunc.
var pivotFuncs = []string{"sum", "count", "average", "max", "min", "product", "countNums", "stdDev", "stdDevp", "var", "varp"}

// showAsNames are the showDataAs names, by model.PivotShowAs.
var showAsNames = []string{"normal", "percentOfTotal", "percentOfCol", "percentOfRow"}

// readPivots reads the PivotTables of worksheet part (sheet sh, whose
// index is sheet) listed in its relationships.
func (r *reader) readPivots(part string, sh *model.Sheet) {
	rels, _ := r.rels(part)
	for _, rel := range rels {
		if !strings.HasSuffix(rel.Type, "/pivotTable") {
			continue
		}
		pt, err := r.readPivot(rel.Target, sh)
		if err != nil {
			r.wb.Warnings = append(r.wb.Warnings, sh.Name+": a PivotTable couldn't be read ("+err.Error()+"); its cells are kept")
			continue
		}
		sh.Pivots = append(sh.Pivots, pt)
	}
}

func (r *reader) readPivot(part string, sh *model.Sheet) (*model.PivotTable, error) {
	var x xPivotTable
	if err := r.unmarshal(part, &x); err != nil {
		return nil, err
	}
	var cache xPivotCache
	rels, _ := r.rels(part)
	found := false
	for _, rel := range rels {
		if strings.HasSuffix(rel.Type, "/pivotCacheDefinition") {
			if err := r.unmarshal(rel.Target, &cache); err != nil {
				return nil, err
			}
			found = true
		}
	}
	if !found {
		return nil, errNoCache
	}
	body, ok := pivot.ParseRange(x.Location.Ref)
	if !ok {
		return nil, errNoCache
	}
	pt := &model.PivotTable{Name: x.Name, Row: body.R1, Col: body.C1, Style: x.Style.Name}
	ws := cache.Source.WS
	switch {
	case ws.Name != "":
		pt.Source = ws.Name
	case ws.Ref != "":
		rng, ok := pivot.ParseRange(ws.Ref)
		if !ok {
			return nil, errNoCache
		}
		sheet := ws.Sheet
		if sheet == "" {
			sheet = sh.Name
		}
		pt.Source = formula.QuoteSheet(sheet) + "!" + absRange(rng)
	default:
		return nil, errExternal
	}
	// Items by shared item index, as keys.
	keys := make([][]string, len(cache.Fields))
	for i, cf := range cache.Fields {
		pt.Fields = append(pt.Fields, model.PivotField{Name: cf.Name})
		for _, it := range cf.Items.List {
			keys[i] = append(keys[i], r.sharedKey(it))
		}
	}
	compact := x.Compact != "0"
	outline := x.Outline == "1"
	for i, f := range x.Fields {
		if i >= len(pt.Fields) {
			break
		}
		pf := &pt.Fields[i]
		pf.Caption = f.Name
		pf.NoSubtotal = f.DefaultSubtotal == "0"
		switch f.SortType {
		case "ascending":
			pf.Sort = model.SortAscending
		case "descending":
			pf.Sort = model.SortDescending
		default:
			pf.Sort = model.SortManual
		}
		for _, it := range f.Items {
			if it.T != "" && it.T != "data" {
				continue // subtotal items
			}
			xi, err := strconv.Atoi(it.X)
			if err != nil || xi < 0 || xi >= len(keys[i]) {
				continue
			}
			k := keys[i][xi]
			if pf.Sort == model.SortManual {
				pf.Order = append(pf.Order, k)
			}
			if it.H == "1" {
				pf.Hidden = append(pf.Hidden, k)
			}
		}
		if f.Axis == "axisRow" {
			if f.Compact == "0" {
				compact = false
			}
			if f.Outline == "0" {
				outline = false
			} else if f.Outline == "1" {
				outline = true
			}
			if f.SubtotalTop == "0" {
				pt.SubtotalsBottom = true
			}
			if f.InsertBlankRow == "1" {
				pt.BlankRows = true
			}
			for _, e := range f.Ext {
				if e.Field.FillDownLabels == "1" {
					pt.RepeatLabels = true
				}
			}
		}
	}
	switch {
	case compact && outline:
		pt.Layout = model.LayoutCompact
	case outline:
		pt.Layout = model.LayoutOutline
	default:
		pt.Layout = model.LayoutTabular
	}
	field := func(x int) (int, bool) {
		if x == model.DataField || x == -2 {
			return model.DataField, true
		}
		return x, x >= 0 && x < len(pt.Fields)
	}
	for _, f := range x.RowFields {
		if v, ok := field(f.X); ok {
			pt.Rows = append(pt.Rows, v)
		}
	}
	for _, f := range x.ColFields {
		if v, ok := field(f.X); ok {
			pt.Cols = append(pt.Cols, v)
		}
	}
	for _, pf := range x.PageFields {
		if pf.Fld < 0 || pf.Fld >= len(pt.Fields) {
			continue
		}
		pt.Pages = append(pt.Pages, pf.Fld)
		if pf.Item == nil || pf.Fld >= len(x.Fields) {
			continue
		}
		// One item chosen: every other item is hidden.
		n, err := strconv.Atoi(*pf.Item)
		items := x.Fields[pf.Fld].Items
		if err != nil || n < 0 || n >= len(items) {
			continue
		}
		var hidden []string
		for i, it := range items {
			xi, err := strconv.Atoi(it.X)
			if i == n || err != nil || xi < 0 || xi >= len(keys[pf.Fld]) || it.T != "" && it.T != "data" {
				continue
			}
			hidden = append(hidden, keys[pf.Fld][xi])
		}
		pt.Fields[pf.Fld].Hidden = hidden
	}
	for _, df := range x.DataFields {
		if df.Fld < 0 || df.Fld >= len(pt.Fields) {
			continue
		}
		v := model.PivotValue{Field: df.Fld, Name: df.Name}
		for i, n := range pivotFuncs {
			if df.Subtotal == n {
				v.Func = model.PivotFunc(i)
			}
		}
		for i, n := range showAsNames {
			if df.ShowDataAs == n {
				v.ShowAs = model.PivotShowAs(i)
			}
		}
		v.NumFmtID = df.NumFmtID
		if code, ok := r.numFmtCodes[df.NumFmtID]; ok {
			v.NumFmtCode = code
		}
		if v.Name == "" {
			v.Name = pivot.ValueName(v.Func, pt.Fields[df.Fld].Name)
		}
		pt.Values = append(pt.Values, v)
	}
	pt.NoRowGrand = x.RowGrandTotals == "0"
	pt.NoColGrand = x.ColGrandTotals == "0"
	o := &pt.Out
	o.Body = body
	o.FirstHeaderRow, o.FirstDataRow, o.FirstDataCol = x.Location.FirstHeaderRow, x.Location.FirstDataRow, x.Location.FirstDataCol
	o.Area = body
	if n := len(pt.Pages); n > 0 {
		o.Area.R1 = max(body.R1-n-1, 0)
		o.Area.C2 = max(o.Area.C2, o.Area.C1+1)
	}
	return pt, nil
}

type pivotError string

func (e pivotError) Error() string { return string(e) }

const (
	errNoCache  pivotError = "no pivot cache"
	errExternal pivotError = "its source isn't a range of the workbook"
)

// sharedKey is a shared item's key (see pivot.ItemKey).
func (r *reader) sharedKey(it xSharedItem) string {
	switch it.XMLName.Local {
	case "s":
		return pivot.TextKey(it.V)
	case "n":
		v, _ := strconv.ParseFloat(it.V, 64)
		return pivot.NumKey(v)
	case "d":
		if t, err := time.Parse("2006-01-02T15:04:05", it.V); err == nil {
			return pivot.NumKey(dateSerial(t, r.wb.Date1904))
		}
	case "b":
		if it.V == "1" || strings.EqualFold(it.V, "true") {
			return "b:TRUE"
		}
		return "b:FALSE"
	case "e":
		return "e:" + it.V
	}
	return "" // m: missing (blank)
}

// dateSerial is t as an Excel date serial number.
func dateSerial(t time.Time, date1904 bool) float64 {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	d := t.Sub(base).Hours() / 24
	return math.Round(d*86400) / 86400
}

// serialDate is the inverse of dateSerial.
func serialDate(v float64, date1904 bool) time.Time {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return base.Add(time.Duration(math.Round(v*86400)) * time.Second)
}

// absRange is $A$1:$D$6.
func absRange(r model.Range) string {
	return "$" + model.ColName(r.C1) + "$" + strconv.Itoa(r.R1+1) + ":$" + model.ColName(r.C2) + "$" + strconv.Itoa(r.R2+1)
}
