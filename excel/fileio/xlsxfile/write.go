package xlsxfile

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/flate"
	"fmt"
	"image/color"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"officelite/common/colors"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// Write saves wb as an .xlsx package.
//
// Cell data goes through excelize's StreamWriter, which keeps memory flat at
// 256K rows. The StreamWriter cannot express a few things the model holds:
// error values (t="e"), dynamic-array formulas (t="array" plus the cm
// attribute and xl/metadata.xml that make Excel spill them instead of
// showing "@"), formulas without a cached value and control characters in
// text. Such cells are written as empty placeholders and replaced in a
// second pass over the package. That pass also writes column widths (with
// excelize's own <cols>, every cell written costs a scan of all columns
// with a width) and the Normal style's font size (excelize only sets its
// name). It copies untouched parts without recompressing them.
func Write(wb *model.Workbook, w io.Writer) error {
	f := excelize.NewFile()
	defer f.Close()
	x := &writer{f: f, wb: wb, styles: map[uint32]int{}, sheets: map[string]*sheetPatch{}}
	if err := x.build(); err != nil {
		return err
	}
	if len(x.sheets) == 0 && x.normalFont == nil && len(x.pivots) == 0 && len(x.charts) == 0 && (wb.Theme == nil || len(wb.Theme.XML) == 0) {
		return f.Write(w)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return err
	}
	return x.patch(buf.Bytes(), w)
}

type writer struct {
	f      *excelize.File
	wb     *model.Workbook
	styles map[uint32]int // model style index -> excelize style id

	sheets     map[string]*sheetPatch // worksheet part -> what patch changes
	dynamic    bool                   // some cell is a dynamic array (needs metadata.xml)
	normalFont *model.Font            // replaces the Normal style's font, if not Calibri 11
	// filterNames are the _xlnm._FilterDatabase names of sheets with an
	// AutoFilter, added to workbook.xml (see filter.go).
	filterNames []string
	pivots      []pendingPivot  // PivotTables (pivotwrite.go)
	charts      []pendingCharts // charts and chart sheets (chartwrite.go)
	// dropParts are parts excelize wrote that patch leaves out (chart
	// sheets' placeholder worksheets).
	dropParts   []string
	activeSheet int
}

// sheetPatch is what the second pass changes in one worksheet: the <cols>
// element, the cells written as placeholders, in document order, and the
// <autoFilter> element.
type sheetPatch struct {
	cols       string
	cells      []rawCell
	autoFilter string
	// formatPr is attributes added to <sheetFormatPr> (the outline's
	// depth).
	formatPr string
	// drawing is the <drawing> element of a sheet with charts.
	drawing string
}

func (x *writer) sheetPatch(part string) *sheetPatch {
	p := x.sheets[part]
	if p == nil {
		p = &sheetPatch{}
		x.sheets[part] = p
	}
	return p
}

type rawCell struct {
	ref, xml string
	// row, when set, is a row number whose <row> start tag gets xml
	// as extra attributes instead (collapsed="1").
	row int
}

func (x *writer) build() error {
	f, wb := x.f, x.wb
	sheets := wb.Sheets
	if len(sheets) == 0 {
		sheets = []*model.Sheet{model.NewSheet("Sheet1")}
	}
	if len(wb.Styles) > 0 {
		if def := wb.Styles[0].Font; def.Family != "Calibri" || def.SizePt != 11 {
			x.normalFont = &def
		}
	}

	// Workbook-level settings first: SetSheetVisible reads every worksheet,
	// which no longer works once a StreamWriter has flushed one.
	names := sheetNames(sheets)
	if err := f.SetSheetName("Sheet1", names[0]); err != nil {
		return err
	}
	for _, n := range names[1:] {
		if _, err := f.NewSheet(n); err != nil {
			return err
		}
	}
	if wb.Date1904 {
		t := true
		if err := f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &t}); err != nil {
			return err
		}
	}
	// Excel needs one visible sheet, and the active one must be visible.
	active := slices.IndexFunc(sheets, func(sh *model.Sheet) bool { return !sh.Hidden })
	if active < 0 {
		active = 0
	}
	f.SetActiveSheet(active)
	x.activeSheet = active
	for i, sh := range sheets {
		if sh.Hidden && i != active {
			if err := f.SetSheetVisible(names[i], false); err != nil {
				return err
			}
		}
	}
	for _, dn := range wb.Names {
		d := &excelize.DefinedName{Name: dn.Name, RefersTo: formula.AddFuturePrefixes(dn.Formula)}
		if dn.Sheet >= 0 {
			if dn.Sheet >= len(names) {
				continue
			}
			d.Scope = names[dn.Sheet]
		}
		// A name excelize rejects (invalid or duplicate) is dropped rather
		// than failing the whole save.
		_ = f.SetDefinedName(d)
	}

	for i, sh := range sheets {
		part := fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		x.collectCharts(i, sh, part)
		if sh.IsChartSheet() {
			continue // a placeholder, replaced by patch (chartwrite.go)
		}
		if err := x.writeSheet(sh, names[i], part); err != nil {
			return fmt.Errorf("sheet %q: %w", sh.Name, err)
		}
		x.collectPivots(i, sh, part)
		if af := sh.AutoFilter; af != nil {
			x.sheetPatch(part).autoFilter = autoFilterXML(af)
			x.filterNames = append(x.filterNames, filterDatabase(i, names[i], af.Range))
		}
	}
	return nil
}

// sheetNames makes the sheet names valid for Excel: at most 31 characters,
// none of : \ / ? * [ ], no leading or trailing apostrophe, and unique
// ignoring case. CSV sheets are named after the file and break these rules.
func sheetNames(sheets []*model.Sheet) []string {
	out := make([]string, len(sheets))
	used := map[string]bool{}
	for i, sh := range sheets {
		n := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`:\/?*[]`, r) || r < ' ' {
				return '_'
			}
			return r
		}, sh.Name)
		n = truncRunes(strings.Trim(n, "'"), 31)
		if n == "" {
			n = "Sheet" + strconv.Itoa(i+1)
		}
		for k, base := 2, n; used[strings.ToLower(n)]; k++ {
			suffix := " (" + strconv.Itoa(k) + ")"
			n = strings.TrimRight(truncRunes(base, 31-len(suffix)), " ") + suffix
		}
		used[strings.ToLower(n)] = true
		out[i] = n
	}
	return out
}

func truncRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// pxToPt converts 96-DPI pixels (the model's row heights) to points.
func pxToPt(px float32) float64 { return float64(px) * 72 / 96 }

func (x *writer) writeSheet(sh *model.Sheet, name, part string) error {
	f := x.f

	// Sheet format, view and panes live in the worksheet the StreamWriter
	// starts from, so they are set before it is created.
	props := &excelize.SheetPropsOptions{}
	if sh.DefaultColWidth > 0 {
		v := float64(sh.DefaultColWidth)
		props.DefaultColWidth = &v
	}
	if sh.BaseColWidth > 0 && sh.BaseColWidth != model.DefaultBaseColWidth {
		v := uint8(min(sh.BaseColWidth, 255))
		props.BaseColWidth = &v
	}
	if sh.DefaultRowHeight > 0 {
		ht, custom := pxToPt(sh.DefaultRowHeight), sh.DefaultRowHeight != model.DefaultRowHeightPx
		props.DefaultRowHeight, props.CustomHeight = &ht, &custom
	}
	if sh.Outline.SummaryAbove || sh.Outline.SummaryLeft {
		below, right := !sh.Outline.SummaryAbove, !sh.Outline.SummaryLeft
		props.OutlineSummaryBelow, props.OutlineSummaryRight = &below, &right
	}
	if err := f.SetSheetProps(name, props); err != nil {
		return err
	}
	view := &excelize.ViewOptions{ShowGridLines: &sh.ShowGridLines}
	if sh.TopRow > 0 || sh.LeftCol > 0 {
		tl := model.CellName(sh.TopRow, sh.LeftCol)
		view.TopLeftCell = &tl
	}
	if err := f.SetSheetView(name, 0, view); err != nil {
		return err
	}
	activeRef := model.CellName(sh.ActiveRow, sh.ActiveCol)
	panes := &excelize.Panes{}
	if sh.FreezeRows > 0 || sh.FreezeCols > 0 {
		panes.Freeze = true
		panes.XSplit, panes.YSplit = sh.FreezeCols, sh.FreezeRows
		panes.TopLeftCell = model.CellName(sh.FreezeRows, sh.FreezeCols)
		switch {
		case sh.FreezeRows > 0 && sh.FreezeCols > 0:
			panes.ActivePane = "bottomRight"
		case sh.FreezeRows > 0:
			panes.ActivePane = "bottomLeft"
		default:
			panes.ActivePane = "topRight"
		}
	}
	if panes.Freeze || activeRef != "A1" {
		panes.Selection = []excelize.Selection{{SQRef: activeRef, ActiveCell: activeRef, Pane: panes.ActivePane}}
		if err := f.SetPanes(name, panes); err != nil {
			return err
		}
	}

	if err := x.writeAnnotations(sh, name); err != nil {
		return err
	}
	if err := x.writeValidations(sh, name); err != nil {
		return err
	}
	sw, err := f.NewStreamWriter(name)
	if err != nil {
		return err
	}
	if cols := colsXML(sh); cols != "" {
		x.sheetPatch(part).cols = cols
	}
	if lr, lc := sh.MaxLevel(true), sh.MaxLevel(false); lr > 0 || lc > 0 {
		var a strings.Builder
		if lr > 0 {
			fmt.Fprintf(&a, ` outlineLevelRow="%d"`, lr)
		}
		if lc > 0 {
			fmt.Fprintf(&a, ` outlineLevelCol="%d"`, lc)
		}
		x.sheetPatch(part).formatPr = a.String()
	}

	// Rows: every row with cells or its own height, in order.
	nRows := sh.UsedRows()
	for r := range sh.RowHeight {
		if r < model.MaxRows {
			nRows = max(nRows, r+1)
		}
	}
	for r := range sh.Filtered {
		if r < model.MaxRows {
			nRows = max(nRows, r+1)
		}
	}
	for _, m := range []map[int]uint8{sh.Outline.RowLevel} {
		for r := range m {
			nRows = max(nRows, min(r+1, model.MaxRows))
		}
	}
	for r := range sh.Outline.RowCollapsed {
		nRows = max(nRows, min(r+1, model.MaxRows))
	}
	var vals []any
	for r := 0; r < nRows; r++ {
		vals = vals[:0]
		mark := -1 // where this row's cell patches start
		if p := x.sheets[part]; p != nil {
			mark = len(p.cells)
		}
		for c := 0; c < sh.RowLen(r); c++ {
			if len(sh.Pivots) > 0 && placeholder(sh, r, c) {
				vals = append(vals, nil)
				continue
			}
			vals = append(vals, x.cellValue(sh, part, r, c))
		}
		for len(vals) > 0 && vals[len(vals)-1] == nil {
			vals = vals[:len(vals)-1]
		}
		var opts excelize.RowOpts
		h, hasHeight := sh.RowHeight[r]
		if hasHeight {
			if h == 0 {
				opts.Hidden = true
			} else {
				opts.Height = min(pxToPt(h), excelize.MaxRowHeight)
			}
		}
		if sh.Filtered[r] {
			opts.Hidden = true // rows the AutoFilter hides are hidden rows in the file
		}
		if h := sh.Outline.RowShowSize[r]; opts.Hidden && h > 0 {
			opts.Height = min(pxToPt(h), excelize.MaxRowHeight) // its height when shown
		}
		opts.OutlineLevel = sh.Level(true, r)
		collapsed := sh.IsCollapsed(true, r)
		if len(vals) == 0 && !hasHeight && !opts.Hidden && opts.OutlineLevel == 0 && !collapsed {
			continue
		}
		if collapsed {
			// excelize can't write collapsed="1": patch adds it.
			p := x.sheetPatch(part)
			if mark < 0 {
				mark = 0
			}
			p.cells = slices.Insert(p.cells, mark, rawCell{xml: ` collapsed="1"`, row: r + 1})
		}
		if err := sw.SetRow(model.CellName(r, 0), vals, opts); err != nil {
			return err
		}
	}

	for _, m := range sh.Merges {
		if m.R1 == m.R2 && m.C1 == m.C2 {
			continue
		}
		if err := sw.MergeCell(model.CellName(m.R1, m.C1), model.CellName(m.R2, m.C2)); err != nil {
			return err
		}
	}
	return sw.Flush()
}

// colsXML returns the <cols> element for the column widths, joining runs
// of equal width.
func colsXML(sh *model.Sheet) string {
	set := map[int]bool{}
	for _, m := range []map[int]float32{sh.ColWidth} {
		for c := range m {
			set[c] = true
		}
	}
	for c := range sh.Outline.ColLevel {
		set[c] = true
	}
	for c := range sh.Outline.ColCollapsed {
		set[c] = true
	}
	cols := make([]int, 0, len(set))
	for c := range set {
		if c >= 0 && c < model.MaxCols {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return ""
	}
	slices.Sort(cols)
	// A column's attributes: its width (-1 for the default, written as no
	// width at all), the width
	// it gets back when shown, its outline level and collapsed flag.
	type colAttrs struct {
		w, show float32
		level   int
		coll    bool
	}
	attrsOf := func(c int) colAttrs {
		w, ok := sh.ColWidth[c]
		if !ok {
			w = -1
		}
		return colAttrs{w, sh.Outline.ColShowSize[c], sh.Level(false, c), sh.IsCollapsed(false, c)}
	}
	defWidth := sh.DefaultColWidth
	if defWidth <= 0 {
		defWidth = model.PxToColWidth(float64(model.BaseColWidthToPx(float64(sh.BaseColWidth), model.DefaultDigitWidth)), model.DefaultDigitWidth)
	}
	num := func(v float32) string {
		return strconv.FormatFloat(min(float64(v), excelize.MaxColumnWidth), 'g', -1, 32)
	}
	var b strings.Builder
	b.WriteString("<cols>")
	for i := 0; i < len(cols); {
		lo, a := cols[i], attrsOf(cols[i])
		j := i + 1
		for j < len(cols) && cols[j] == cols[j-1]+1 && attrsOf(cols[j]) == a {
			j++
		}
		fmt.Fprintf(&b, `<col min="%d" max="%d"`, lo+1, cols[j-1]+1)
		switch {
		case a.w == 0 && a.show > 0:
			fmt.Fprintf(&b, ` width="%s" hidden="1" customWidth="1"`, num(a.show))
		case a.w == 0 && a.level > 0:
			// Shown again by the outline's +, it gets the standard width.
			fmt.Fprintf(&b, ` width="%s" hidden="1"`, num(defWidth))
		case a.w == 0:
			b.WriteString(` width="0" hidden="1" customWidth="1"`)
		case a.w > 0:
			fmt.Fprintf(&b, ` width="%s" customWidth="1"`, num(a.w))
		}
		if a.level > 0 {
			fmt.Fprintf(&b, ` outlineLevel="%d"`, a.level)
		}
		if a.coll {
			b.WriteString(` collapsed="1"`)
		}
		b.WriteString(`/>`)
		i = j
	}
	b.WriteString("</cols>")
	return b.String()
}

// cellValue returns what the StreamWriter writes for one cell: nil for
// nothing, otherwise an excelize.Cell. Cells it cannot express are queued
// for patch and written as a styled placeholder.
func (x *writer) cellValue(sh *model.Sheet, part string, r, c int) any {
	cell := sh.Get(r, c)
	if cell.IsBlank() && cell.Style == 0 {
		return nil
	}
	sid := x.styleID(cell.Style)
	if cell.Formula != "" {
		// Excel shows #NAME? for functions newer than 2007 written without
		// their _xlfn. prefix.
		c := *cell
		c.Formula = formula.AddFuturePrefixes(c.Formula)
		cell = &c
	}
	out := excelize.Cell{StyleID: sid, Formula: cell.Formula}

	spill := ""
	if cell.Formula != "" && cell.Dynamic {
		rng, ok := sh.Spills[[2]int{r, c}]
		if !ok {
			rng = model.Range{R1: r, C1: c, R2: r, C2: c}
		}
		spill = model.CellName(rng.R1, rng.C1)
		if rng.R2 != rng.R1 || rng.C2 != rng.C1 {
			spill += ":" + model.CellName(rng.R2, rng.C2)
		}
	}
	if spill != "" || cell.Kind == model.KindError || (cell.Formula != "" && cell.Kind == model.KindEmpty) ||
		cell.Kind == model.KindText && hasControlChars(cell.Text) ||
		cell.Kind == model.KindNumber && needsExponent(cell.Num) {
		ref := model.CellName(r, c)
		p := x.sheetPatch(part)
		p.cells = append(p.cells, rawCell{ref: ref, xml: rawCellXML(ref, sid, cell, spill)})
		x.dynamic = x.dynamic || spill != ""
		return excelize.Cell{StyleID: sid}
	}

	switch cell.Kind {
	case model.KindNumber:
		out.Value = cell.Num
	case model.KindText:
		out.Value = cell.Text // excelize escapes a literal "_xHHHH_"
	case model.KindBool:
		out.Value = cell.Num != 0
	}
	return out
}

// rawCellXML writes a <c> element by hand. spill, when set, is the range
// of a dynamic-array formula; cm="1" points at the dynamic-array cell
// metadata written by patch.
func rawCellXML(ref string, sid int, c *model.Cell, spill string) string {
	var b strings.Builder
	b.WriteString(`<c r="`)
	b.WriteString(ref)
	b.WriteByte('"')
	if sid != 0 {
		b.WriteString(` s="`)
		b.WriteString(strconv.Itoa(sid))
		b.WriteByte('"')
	}
	constText := c.Kind == model.KindText && c.Formula == ""
	switch {
	case constText:
		b.WriteString(` t="inlineStr"`)
	case c.Kind == model.KindText:
		b.WriteString(` t="str"`)
	case c.Kind == model.KindBool:
		b.WriteString(` t="b"`)
	case c.Kind == model.KindError:
		b.WriteString(` t="e"`)
	}
	if spill != "" {
		b.WriteString(` cm="1"`)
	}
	b.WriteByte('>')
	if c.Formula != "" {
		if spill != "" {
			b.WriteString(`<f t="array" ref="`)
			b.WriteString(spill)
			b.WriteString(`">`)
		} else {
			b.WriteString(`<f>`)
		}
		b.WriteString(xmlEscape(c.Formula))
		b.WriteString(`</f>`)
	}
	var v string
	switch c.Kind {
	case model.KindNumber:
		v = strconv.FormatFloat(c.Num, 'g', -1, 64)
	case model.KindText:
		v = xmlEscape(escapeOOXML(c.Text))
	case model.KindBool:
		v = "0"
		if c.Num != 0 {
			v = "1"
		}
	case model.KindError:
		v = xmlEscape(c.Text)
	}
	if constText {
		b.WriteString(`<is><t xml:space="preserve">`)
		b.WriteString(v)
		b.WriteString(`</t></is>`)
	} else if c.Kind != model.KindEmpty {
		b.WriteString(`<v>`)
		b.WriteString(v)
		b.WriteString(`</v>`)
	}
	b.WriteString(`</c>`)
	return b.String()
}

// needsExponent reports whether v needs exponent notation. excelize writes
// numbers in plain decimal, so 1e300 would become 301 digits, which Excel
// refuses to open.
func needsExponent(v float64) bool {
	return strings.ContainsRune(strconv.FormatFloat(v, 'g', -1, 64), 'e')
}

func xmlEscape(s string) string {
	if !strings.ContainsAny(s, `&<>"'`+"\r\t\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\r':
			b.WriteString("&#xD;")
		case '\t':
			b.WriteString("&#x9;")
		case '\n':
			b.WriteString("&#xA;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasControlChars reports whether s holds characters XML cannot carry,
// which excelize would turn into U+FFFD.
func hasControlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < ' ' && c != '\t' && c != '\n' && c != '\r' {
			return true
		}
	}
	return false
}

// escapeOOXML is the inverse of unescapeOOXML: control characters XML
// cannot hold become _xHHHH_, and a literal "_xHHHH_" gets its underscore
// escaped so it survives the round trip.
func escapeOOXML(s string) string {
	needs := false
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < ' ' && c != '\t' && c != '\n' && c != '\r' || c == '_' && isOOXMLEscape(s[i:]) {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c < ' ' && c != '\t' && c != '\n' && c != '\r':
			fmt.Fprintf(&b, "_x%04X_", c)
		case c == '_' && isOOXMLEscape(s[i:]):
			b.WriteString("_x005F_")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isOOXMLEscape(s string) bool {
	if len(s) < 7 || s[0] != '_' || s[1] != 'x' || s[6] != '_' {
		return false
	}
	_, err := strconv.ParseUint(s[2:6], 16, 16)
	return err == nil
}

// --- styles ---

// styleID returns the excelize style for a model style index, creating it
// on first use. The workbook default maps to excelize's style 0 when it is
// a plain font (patch makes that the Normal style's font).
func (x *writer) styleID(i uint32) int {
	if id, ok := x.styles[i]; ok {
		return id
	}
	id := 0
	if int(i) < len(x.wb.Styles) {
		st := x.wb.Styles[i]
		if i != 0 || st != (model.Style{Font: st.Font}) {
			id = x.newStyle(st)
		}
	}
	x.styles[i] = id
	return id
}

func (x *writer) newStyle(st model.Style) int {
	es := excelStyle(st)
	if id, err := x.f.NewStyle(es); err == nil {
		return id
	}
	// Something excelize rejects (an overlong font name, an odd number
	// format): keep what it accepts.
	es.CustomNumFmt, es.NumFmt = nil, 0
	if id, err := x.f.NewStyle(es); err == nil {
		return id
	}
	es.Font = nil
	if id, err := x.f.NewStyle(es); err == nil {
		return id
	}
	return 0
}

// localeNumFmts are the built-in currency formats excelize will not write
// by id; they are written as their en-US codes instead.
var localeNumFmts = map[int]string{
	5: `"$"#,##0_);\("$"#,##0\)`,
	6: `"$"#,##0_);[Red]\("$"#,##0\)`,
	7: `"$"#,##0.00_);\("$"#,##0.00\)`,
	8: `"$"#,##0.00_);[Red]\("$"#,##0.00\)`,
}

var (
	hAlignNames = map[model.HAlign]string{
		model.HAlignLeft: "left", model.HAlignCenter: "center", model.HAlignRight: "right",
		model.HAlignFill: "fill", model.HAlignJustify: "justify",
		model.HAlignCenterContinuous: "centerContinuous", model.HAlignDistributed: "distributed",
	}
	vAlignNames = map[model.VAlign]string{
		model.VAlignTop: "top", model.VAlignCenter: "center",
		model.VAlignJustify: "justify", model.VAlignDistributed: "distributed",
	}
	sideNames = [4]string{model.Left: "left", model.Top: "top", model.Right: "right", model.Bottom: "bottom"}
)

func hexColor(c color.NRGBA) string {
	if c.A == 0 {
		return ""
	}
	return colors.Hex(c)
}

// excelStyle converts a model style to excelize's form.
func excelStyle(st model.Style) *excelize.Style {
	es := &excelize.Style{Font: &excelize.Font{
		Family: st.Font.Family,
		Size:   float64(st.Font.SizePt),
		Bold:   st.Font.Bold,
		Italic: st.Font.Italic,
		Strike: st.Font.Strike,
		Color:  hexColor(st.Font.Color),
	}}
	if st.Font.Underline {
		es.Font.Underline = "single"
	}
	if st.Fill.A != 0 {
		es.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{hexColor(st.Fill)}}
	}
	for side, e := range st.Border {
		if e.Style != model.BorderNone {
			es.Border = append(es.Border, excelize.Border{Type: sideNames[side], Style: int(e.Style), Color: hexColor(e.Color)})
		}
	}
	if st.HAlign != model.HAlignGeneral || st.VAlign != model.VAlignBottom || st.Wrap || st.Indent != 0 || st.Rotation != 0 {
		es.Alignment = &excelize.Alignment{
			Horizontal:   hAlignNames[st.HAlign],
			Vertical:     vAlignNames[st.VAlign],
			WrapText:     st.Wrap,
			Indent:       st.Indent,
			TextRotation: st.Rotation,
		}
	}
	switch code, ok := localeNumFmts[st.NumFmtID]; {
	case st.NumFmtCode != "":
		code := st.NumFmtCode
		es.CustomNumFmt = &code
	case ok:
		es.CustomNumFmt = &code
	default:
		es.NumFmt = st.NumFmtID
	}
	return es
}

// --- package patching ---

const (
	metadataPart = "xl/metadata.xml"

	metadataXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<metadata xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:xda="http://schemas.microsoft.com/office/spreadsheetml/2017/dynamicarray">` +
		`<metadataTypes count="1"><metadataType name="XLDAPR" minSupportedVersion="120000" copy="1" pasteAll="1" pasteValues="1" merge="1" splitFirst="1" rowColShift="1" clearFormats="1" clearComments="1" assign="1" coerce="1" cellMeta="1"/></metadataTypes>` +
		`<futureMetadata name="XLDAPR" count="1"><bk><extLst><ext uri="{bdbb8cdc-fa1e-496e-a857-3c3f30c029c3}"><xda:dynamicArrayProperties fDynamic="1" fCollapsed="0"/></ext></extLst></bk></futureMetadata>` +
		`<cellMetadata count="1"><bk><rc t="1" v="0"/></bk></cellMetadata></metadata>`

	metadataOverride = `<Override PartName="/xl/metadata.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheetMetadata+xml"/>`
	metadataRelType  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/sheetMetadata"
)

// patch copies the package excelize wrote to w, splicing in the raw cells,
// the dynamic-array metadata and the Normal font.
func (x *writer) patch(data []byte, w io.Writer) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	// Patched worksheets are compressed a second time; the fastest level
	// keeps that from doubling the save time, for files ~10% larger.
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	type editFn = func([]byte) ([]byte, error)
	edits := map[string][]editFn{}
	if x.dynamic {
		edits["[Content_Types].xml"] = append(edits["[Content_Types].xml"], func(b []byte) ([]byte, error) { return insertBefore(b, "</Types>", metadataOverride) })
		edits["xl/_rels/workbook.xml.rels"] = append(edits["xl/_rels/workbook.xml.rels"], addMetadataRel)
	}
	if x.normalFont != nil {
		edits["xl/styles.xml"] = append(edits["xl/styles.xml"], x.setNormalFont)
	}
	if len(x.filterNames) > 0 {
		edits["xl/workbook.xml"] = append(edits["xl/workbook.xml"], x.addFilterNames)
	}
	newParts, err := x.pivotParts(zr, edits)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, zf := range zr.File {
		names[zf.Name] = true
	}
	for _, p := range newParts {
		names[p.name] = true
	}
	newParts = append(newParts, x.chartParts(names, edits)...)
	done := 0
	for _, zf := range zr.File {
		if slices.Contains(x.dropParts, zf.Name) {
			delete(edits, zf.Name)
			continue
		}
		sp, isSheet := x.sheets[zf.Name]
		fns := edits[zf.Name]
		delete(edits, zf.Name)
		switch {
		case isSheet:
			done++
		case len(fns) == 0:
			if err := copyRaw(zw, zf); err != nil {
				return err
			}
			continue
		}

		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := zw.CreateHeader(&zip.FileHeader{Name: zf.Name, Method: zip.Deflate, Modified: zf.Modified})
		if err != nil {
			rc.Close()
			return err
		}
		if isSheet {
			err = patchSheet(out, rc, sp)
		} else {
			var b []byte
			if b, err = io.ReadAll(rc); err == nil {
				for _, fn := range fns {
					if b, err = fn(b); err != nil {
						break
					}
				}
				if err == nil {
					_, err = out.Write(b)
				}
			}
		}
		rc.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", zf.Name, err)
		}
	}
	if done != len(x.sheets) {
		return fmt.Errorf("xlsx writer: %d of %d worksheet parts found", done, len(x.sheets))
	}
	// Parts that didn't exist: edits of a missing part start from nothing
	// (a worksheet's relationships), then the new parts.
	for _, name := range slices.Sorted(maps.Keys(edits)) {
		var b []byte
		for _, fn := range edits[name] {
			if b, err = fn(b); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		newParts = append(newParts, part{name, b})
	}
	for _, p := range newParts {
		out, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		if _, err := out.Write(p.data); err != nil {
			return err
		}
	}
	if x.dynamic {
		out, err := zw.CreateHeader(&zip.FileHeader{Name: metadataPart, Method: zip.Deflate})
		if err != nil {
			return err
		}
		if _, err := io.WriteString(out, metadataXML); err != nil {
			return err
		}
	}
	return zw.Close()
}

func copyRaw(zw *zip.Writer, zf *zip.File) error {
	rc, err := zf.OpenRaw()
	if err != nil {
		return err
	}
	h := zf.FileHeader
	out, err := zw.CreateRaw(&h)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, rc)
	return err
}

func insertBefore(b []byte, marker, s string) ([]byte, error) {
	i := bytes.LastIndex(b, []byte(marker))
	if i < 0 {
		return nil, fmt.Errorf("no %s", marker)
	}
	return slices.Concat(b[:i], []byte(s), b[i:]), nil
}

func addMetadataRel(b []byte) ([]byte, error) {
	n := 1
	for bytes.Contains(b, []byte(`Id="rId`+strconv.Itoa(n)+`"`)) {
		n++
	}
	rel := `<Relationship Id="rId` + strconv.Itoa(n) + `" Type="` + metadataRelType + `" Target="metadata.xml"/>`
	return insertBefore(b, "</Relationships>", rel)
}

// setNormalFont replaces font 0, the Normal style's font, which sets the
// unit of column widths and the look of cells without a style.
func (x *writer) setNormalFont(b []byte) ([]byte, error) {
	fonts := bytes.Index(b, []byte("<fonts"))
	if fonts < 0 {
		return nil, fmt.Errorf("no fonts")
	}
	start := bytes.Index(b[fonts+1:], []byte("<font>"))
	if start < 0 {
		return nil, fmt.Errorf("no font")
	}
	start += fonts + 1
	end := bytes.Index(b[start:], []byte("</font>"))
	if end < 0 {
		return nil, fmt.Errorf("unterminated font")
	}
	end += start + len("</font>")
	fn := x.normalFont
	var s strings.Builder
	s.WriteString("<font>")
	if fn.Bold {
		s.WriteString(`<b/>`)
	}
	if fn.Italic {
		s.WriteString(`<i/>`)
	}
	if fn.Strike {
		s.WriteString(`<strike/>`)
	}
	if fn.Underline {
		s.WriteString(`<u/>`)
	}
	size := fn.SizePt
	if size <= 0 {
		size = 11
	}
	fmt.Fprintf(&s, `<sz val="%s"/>`, strconv.FormatFloat(float64(size), 'g', -1, 32))
	if c := hexColor(fn.Color); c != "" {
		fmt.Fprintf(&s, `<color rgb="FF%s"/>`, c)
	} else {
		s.WriteString(`<color theme="1"/>`)
	}
	family := fn.Family
	if family == "" {
		family = "Calibri"
	}
	fmt.Fprintf(&s, `<name val="%s"/><family val="2"/></font>`, xmlEscape(family))
	return slices.Concat(b[:start], []byte(s.String()), b[end:]), nil
}

// patchSheet streams a worksheet from src to dst, inserting <cols> before
// <sheetData> and replacing each placeholder <c r="REF" ...></c> with its
// raw XML.
func patchSheet(dst io.Writer, src io.Reader, p *sheetPatch) error {
	br := bufio.NewReaderSize(src, 64<<10)
	bw := bufio.NewWriterSize(dst, 64<<10)
	if p.formatPr != "" {
		if err := copyUntil(bw, br, "sheetFormatPr"); err != nil {
			return fmt.Errorf("no sheetFormatPr")
		}
		bw.WriteString("<sheetFormatPr")
		bw.WriteString(p.formatPr)
	}
	if p.cols != "" {
		if err := copyUntil(bw, br, "sheetData>"); err != nil {
			return fmt.Errorf("no sheetData")
		}
		bw.WriteString(p.cols)
		bw.WriteString("<sheetData>")
	}
	for _, rc := range p.cells {
		if rc.row > 0 {
			tag := `row r="` + strconv.Itoa(rc.row) + `"`
			if err := copyUntil(bw, br, tag); err != nil {
				return fmt.Errorf("row %d not found", rc.row)
			}
			bw.WriteString("<" + tag + rc.xml)
			continue
		}
		if err := copyUntil(bw, br, `c r="`+rc.ref+`"`); err != nil {
			return fmt.Errorf("cell %s not found", rc.ref)
		}
		// The placeholder is a start tag and </c>.
		if _, err := br.ReadSlice('>'); err != nil {
			return err
		}
		if _, err := br.ReadSlice('>'); err != nil {
			return err
		}
		bw.WriteString(rc.xml)
	}
	if p.autoFilter != "" {
		// <autoFilter> follows <sheetData> (and comes before <mergeCells>).
		if err := copyUntil(bw, br, "/sheetData>"); err != nil {
			return fmt.Errorf("no sheetData end")
		}
		bw.WriteString("</sheetData>")
		bw.WriteString(p.autoFilter)
	}
	if p.drawing != "" {
		if p.autoFilter == "" {
			if err := copyUntil(bw, br, "/sheetData>"); err != nil {
				return fmt.Errorf("no sheetData end")
			}
			bw.WriteString("</sheetData>")
		}
		// <drawing> comes after the page setup and before the legacy
		// drawing (notes), table parts and extLst: the tail after
		// <sheetData> is small.
		tail, err := io.ReadAll(br)
		if err != nil {
			return err
		}
		at := len(tail)
		for _, tag := range []string{"<legacyDrawing", "<legacyDrawingHF", "<drawingHF", "<picture", "<oleObjects", "<controls", "<webPublishItems", "<tableParts", "<extLst", "</worksheet>"} {
			if i := bytes.Index(tail, []byte(tag)); i >= 0 && i < at {
				at = i
			}
		}
		bw.Write(tail[:at])
		bw.WriteString(p.drawing)
		bw.Write(tail[at:])
		return bw.Flush()
	}
	if _, err := io.Copy(bw, br); err != nil {
		return err
	}
	return bw.Flush()
}

// copyUntil copies br to bw up to the next tag that starts "<"+tag, and
// consumes that tag's first len(tag)+1 bytes without writing them.
func copyUntil(bw *bufio.Writer, br *bufio.Reader, tag string) error {
	for {
		chunk, err := br.ReadSlice('<')
		if err == bufio.ErrBufferFull {
			bw.Write(chunk)
			continue
		}
		if err != nil {
			return err
		}
		bw.Write(chunk[:len(chunk)-1])
		if p, _ := br.Peek(len(tag)); string(p) == tag {
			br.Discard(len(tag))
			return nil
		}
		bw.WriteByte('<')
	}
}
