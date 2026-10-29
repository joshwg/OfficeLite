// Package numbersfile imports Apple Numbers spreadsheets (.numbers, 2013
// and later) read-only: each table becomes a sheet with its values,
// formats, merged cells, sizes and basic cell styles. Formulas come in as
// their last computed values; charts, pictures and shapes are dropped
// with a warning.
package numbersfile

import (
	"cmp"
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"

	"officelite/common/iwork"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Object types of a Numbers document.
const (
	typeSheet          = 2    // TN.SheetArchive
	typeChart          = 5021 // TSCH.ChartDrawableArchive
	typeImage          = 3005 // TSD.ImageArchive
	typeMovie          = 3007 // TSD.MovieArchive
	typeGroup          = 3008 // TSD.GroupArchive
	typeConnectionLine = 3009 // TSD.ConnectionLineArchive
)

// Numbers' default text: Helvetica Neue 10pt.
const (
	defaultFont = "Helvetica Neue"
	defaultSize = 10
)

type reader struct {
	doc       *iwork.Document
	wb        *model.Workbook
	nf        *numfmt.Formatter
	styles    map[model.Style]uint32
	names     map[string]bool
	formulas  int // cells whose formula came in as its value
	tr        *translator
	tableWarn map[string][]string // a table warning -> the sheets it applies to
	warnOrder []string
}

// Read imports a .numbers file.
func Read(data []byte) (_ *model.Workbook, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the file is damaged or not supported (internal error: %v)", r)
		}
	}()
	doc, err := iwork.Open(data)
	if err != nil {
		return nil, err
	}
	if doc.App != iwork.Numbers {
		return nil, fmt.Errorf("this is a %s document, not a Numbers spreadsheet", doc.App)
	}
	r := &reader{
		doc:       doc,
		wb:        &model.Workbook{Styles: []model.Style{model.DefaultStyle()}},
		nf:        numfmt.New(false),
		styles:    map[model.Style]uint32{},
		names:     map[string]bool{},
		tableWarn: map[string][]string{},
		tr:        newTranslator(),
	}
	defer r.nf.Close()
	r.styles[r.wb.Styles[0]] = 0
	dropped, seen := map[string]int{}, map[uint64]bool{}
	type named struct {
		t    *iwork.Table
		name string
	}
	var sheets []named
	for _, sid := range doc.Msg(1).Refs(1) {
		if doc.Type(sid) != typeSheet {
			continue
		}
		sheet := doc.Msg(sid)
		var tables []*iwork.Table
		for _, did := range sheet.Refs(2) {
			switch doc.Type(did) {
			case iwork.TypeTableInfo:
				t, err := doc.TableModel(did)
				if err != nil {
					r.wb.Warnings = append(r.wb.Warnings, fmt.Sprintf("%s: a table could not be read: %v", sheet.Str(1), err))
					continue
				}
				tables = append(tables, t)
			default:
				r.countDropped(did, dropped, seen)
			}
		}
		for _, t := range tables {
			name := t.Name
			if len(tables) > 1 {
				name = sheet.Str(1) + " - " + t.Name
			}
			name = r.sheetName(name)
			r.tr.add(t, name, t.UUID)
			sheets = append(sheets, named{t, name})
		}
	}
	// Formulas can refer to any table, so name every sheet first.
	for _, s := range sheets {
		r.addTable(s.t, s.name)
	}
	r.formulas += verify(r.wb, r.tr.reg)
	for _, w := range r.warnOrder {
		sheets := r.tableWarn[w]
		list := strings.Join(sheets[:min(len(sheets), 3)], ", ")
		if len(sheets) > 3 {
			list += fmt.Sprintf(" and %d more", len(sheets)-3)
		}
		r.wb.Warnings = append(r.wb.Warnings, fmt.Sprintf("%s: %s", list, w))
	}
	for _, what := range []string{"charts", "pictures", "shapes and text boxes", "movies and other objects"} {
		if n := dropped[what]; n > 0 {
			r.wb.Warnings = append(r.wb.Warnings, fmt.Sprintf("%d %s were not imported (Numbers objects outside tables)", n, what))
		}
	}
	if r.formulas > 0 {
		r.wb.Warnings = append(r.wb.Warnings, fmt.Sprintf("%d formulas were imported as their values (Numbers-only functions or references, or results Excel would compute differently)", r.formulas))
	}
	if len(r.wb.Sheets) == 0 {
		r.wb.Warnings = append(r.wb.Warnings, "the document has no tables")
	}
	return r.wb, nil
}

// countDropped tallies a sheet object that isn't imported. seen guards
// against groups that (in a damaged file) contain themselves.
func (r *reader) countDropped(id uint64, dropped map[string]int, seen map[uint64]bool) {
	if seen[id] {
		return
	}
	seen[id] = true
	switch r.doc.Type(id) {
	case typeChart:
		dropped["charts"]++
	case typeImage:
		dropped["pictures"]++
	case iwork.TypeShapeInfo, typeConnectionLine:
		dropped["shapes and text boxes"]++
	case typeGroup:
		for _, c := range r.doc.Msg(id).Refs(2) {
			r.countDropped(c, dropped, seen)
		}
	default:
		dropped["movies and other objects"]++
	}
}

// sheetName makes name a valid, unique Excel sheet name: at most 31
// characters, none of : \ / ? * [ ].
func (r *reader) sheetName(name string) string {
	name = strings.Map(func(c rune) rune {
		if strings.ContainsRune(`:\/?*[]`, c) {
			return '_'
		}
		return c
	}, strings.Trim(name, "'"))
	if strings.TrimSpace(name) == "" {
		name = "Sheet"
	}
	base := []rune(name)
	if len(base) > 31 {
		base = base[:31]
	}
	name = string(base)
	for i := 2; r.names[strings.ToLower(name)]; i++ {
		suffix := fmt.Sprintf(" (%d)", i)
		name = string(base[:min(len(base), 31-len(suffix))]) + suffix
	}
	r.names[strings.ToLower(name)] = true
	return name
}

func (r *reader) addTable(t *iwork.Table, name string) {
	sh := model.NewSheet(name)
	r.wb.Sheets = append(r.wb.Sheets, sh)
	for _, w := range t.Warnings {
		if r.tableWarn[w] == nil {
			r.warnOrder = append(r.warnOrder, w)
		}
		r.tableWarn[w] = append(r.tableWarn[w], name)
	}
	rows, cols := min(t.Rows, model.MaxRows), min(t.Cols, model.MaxCols)
	// Every row and column of the table gets its size; the sheet around
	// it keeps Excel's defaults.
	for row := range rows {
		h := t.RowInfo[row]
		switch {
		case h.Hidden:
			sh.RowHeight[row] = 0
		case h.Size > 0:
			sh.RowHeight[row] = model.PointsToPx(h.Size)
		case t.DefaultRowHeight > 0:
			sh.RowHeight[row] = model.PointsToPx(t.DefaultRowHeight)
		}
	}
	for col := range cols {
		h := t.ColInfo[col]
		switch {
		case h.Hidden:
			sh.ColWidth[col] = 0
		case h.Size > 0:
			sh.ColWidth[col] = model.PxToColWidth(h.Size*96/72, model.DefaultDigitWidth)
		case t.DefaultColWidth > 0:
			sh.ColWidth[col] = model.PxToColWidth(t.DefaultColWidth*96/72, model.DefaultDigitWidth)
		}
	}
	for _, m := range t.Merges {
		if m.R1 < rows && m.C1 < cols {
			sh.Merges = append(sh.Merges, model.Range{R1: m.R1, C1: m.C1, R2: min(m.R2, rows-1), C2: min(m.C2, cols-1)})
		}
	}
	if t.HeaderRowsFrozen {
		sh.FreezeRows = t.HeaderRows
	}
	if t.HeaderColsFrozen {
		sh.FreezeCols = t.HeaderCols
	}
	dropped := 0
	each := func(f func(row, col int)) {
		if rows*cols <= 1<<20 {
			for row := range rows {
				for col := range cols {
					f(row, col)
				}
			}
			return
		}
		// A big table: only its stored cells (empty ones keep no style).
		keys := make([][2]int, 0, len(t.Cells))
		for k := range t.Cells {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b [2]int) int { return cmp.Or(a[0]-b[0], a[1]-b[1]) })
		for _, k := range keys {
			f(k[0], k[1])
		}
	}
	each(func(row, col int) {
		switch {
		case row >= t.Rows || col >= t.Cols:
			return // storage left over outside the table
		case row >= rows || col >= cols:
			dropped++
			return
		}
		c := t.Cell(row, col)
		mc := r.cell(t, row, col, c)
		if c != nil && c.Formula != nil {
			if f, err := r.tr.translate(c.Formula, t, row, col); err == nil {
				mc.Formula = f
			} else {
				r.formulas++
			}
		}
		if mc.Kind == model.KindEmpty && mc.Style == 0 && mc.Formula == "" {
			return
		}
		if !sh.Set(row, col, mc) {
			dropped++
		}
	})
	r.wb.WarnDropped(name, dropped)
}

// cell converts one table cell (c may be nil: an empty cell that still
// gets its row, column and area style).
func (r *reader) cell(t *iwork.Table, row, col int, c *iwork.Cell) model.Cell {
	st, wrap := r.style(t, row, col, c)
	var mc model.Cell
	if c != nil {
		st.NumFmtCode = r.doc.ExcelFormat(c.Format())
		switch c.Type {
		case iwork.CellNumber, iwork.CellCurrency:
			mc = model.Cell{Kind: model.KindNumber, Num: c.Num}
		case iwork.CellDate:
			mc = model.Cell{Kind: model.KindNumber, Num: c.Seconds/86400 + iwork.ExcelEpochSerial}
			if st.NumFmtCode == "" {
				st.NumFmtCode = "m/d/yyyy"
			}
		case iwork.CellDuration:
			mc = model.Cell{Kind: model.KindNumber, Num: c.Num / 86400}
			if st.NumFmtCode == "" {
				st.NumFmtCode = "[h]:mm:ss"
			}
		case iwork.CellBool:
			mc = model.Cell{Kind: model.KindBool, Text: "FALSE"}
			if c.Num != 0 {
				mc.Num, mc.Text = 1, "TRUE"
			}
			st.NumFmtCode = ""
		case iwork.CellText, iwork.CellRichText:
			mc = model.Cell{Kind: model.KindText, Text: c.Text}
			st.NumFmtCode = ""
			if strings.ContainsRune(c.Text, '\n') {
				wrap = true
			}
		case iwork.CellError:
			mc = model.Cell{Kind: model.KindError, Text: "#VALUE!"}
			st.NumFmtCode = ""
		default:
			if !strings.ContainsAny(st.NumFmtCode, "0#?") {
				st.NumFmtCode = "" // a text format on an empty cell
			}
		}
		if mc.Kind == model.KindNumber {
			mc.Text = r.nf.Format(mc.Num, 0, st.NumFmtCode)
		}
	}
	st.Wrap = wrap
	mc.Style = r.wb.AddStyle(st, r.styles)
	return mc
}

// style resolves a cell's look: the table area's styles, then the
// column's and row's, then the cell's own. It reports text wrapping
// separately, so plain empty cells keep the default style.
func (r *reader) style(t *iwork.Table, row, col int, c *iwork.Cell) (model.Style, bool) {
	area := t.Area(row, col)
	textIDs := []uint64{t.AreaTextStyle[area], t.ColInfo[col].TextStyle, t.RowInfo[row].TextStyle}
	cellIDs := []uint64{t.AreaCellStyle[area], t.ColInfo[col].CellStyle, t.RowInfo[row].CellStyle}
	if c != nil {
		textIDs = append(textIDs, c.TextStyle)
		cellIDs = append(cellIDs, c.CellStyle)
	}
	text := iwork.TextProps{Font: "HelveticaNeue", Size: defaultSize}
	var para iwork.ParaProps
	for _, id := range textIDs {
		if s := r.doc.Style(id); s != nil {
			text = text.Merge(s.Text)
			para = para.Merge(s.Para)
		}
	}
	var cp iwork.CellProps
	for _, id := range cellIDs {
		if s := r.doc.Style(id); s != nil {
			if s.Cell.Fill.Set {
				cp.Fill = s.Cell.Fill // may be "no fill"
			}
			if s.Cell.Wrap != iwork.Unset {
				cp.Wrap = s.Cell.Wrap
			}
			if s.Cell.VAlign != 0 {
				cp.VAlign = s.Cell.VAlign
			}
		}
	}
	if (!cp.Fill.Set || cp.Fill.A == 0) && t.Banded(row) {
		cp.Fill = t.BandFill
	}
	family, bold, italic := iwork.FontFamily(text.Font)
	if family == "" {
		family = defaultFont
	}
	st := model.Style{Font: model.Font{
		Family:    family,
		SizePt:    float32(math.Round(text.Size*2) / 2),
		Bold:      bold || text.Bold == iwork.On,
		Italic:    italic || text.Italic == iwork.On,
		Underline: text.Underline == iwork.On,
		Strike:    text.Strike == iwork.On,
		Color:     color.NRGBA{A: 255},
	}}
	if text.Color.Set {
		st.Font.Color = text.Color.NRGBA
		st.Font.Color.A = 255
	}
	if cp.Fill.Set && cp.Fill.A > 0 {
		st.Fill = cp.Fill.NRGBA
		st.Fill.A = 255
	}
	if para.Align != nil {
		switch *para.Align {
		case iwork.AlignLeft:
			st.HAlign = model.HAlignLeft
		case iwork.AlignRight:
			st.HAlign = model.HAlignRight
		case iwork.AlignCenter:
			st.HAlign = model.HAlignCenter
		case iwork.AlignJustify:
			st.HAlign = model.HAlignJustify
		}
	}
	switch cp.VAlign {
	case 1:
		st.VAlign = model.VAlignTop
	case 2:
		st.VAlign = model.VAlignCenter
	}
	return st, cp.Wrap == iwork.On
}
