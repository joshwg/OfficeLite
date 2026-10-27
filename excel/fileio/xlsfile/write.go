package xlsfile

import (
	"encoding/binary"
	"fmt"
	"image/color"
	"io"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"officelite/common/colors"
	"officelite/common/ole"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// Write encodes wb as an Excel 97-2003 (BIFF8) workbook.
//
// What BIFF8 cannot hold is dropped or approximated, as Fits lists: cells
// beyond 65,536 rows × 256 columns, cell styles beyond 4,000, colors
// beyond the 56-entry palette (nearest color), text beyond 32,767
// characters, and formulas the format cannot express (those keep their
// value). Dynamic-array formulas are written as ordinary formulas with
// their value; the cells they spilled into become plain values. Every
// formula is marked to be recalculated when Excel opens the file.
func Write(wb *model.Workbook, w io.Writer) error {
	x := newWriter(wb)
	x.build()
	return ole.Write(w, []ole.Stream{{Name: "Workbook", Data: x.stream()}})
}

// Fits lists what of wb a .xls file cannot hold; it is empty when the
// whole workbook fits.
func Fits(wb *model.Workbook) []string {
	x := newWriter(wb)
	x.build()
	return x.issues
}

// Record identifiers used only by the writer.
const (
	recCalcCount     = 0x000C
	recCalcMode      = 0x000D
	recPrecision     = 0x000E
	recRefMode       = 0x000F
	recDelta         = 0x0010
	recIteration     = 0x0011
	recProtect       = 0x0012
	recPassword      = 0x0013
	recWindowProtect = 0x0019
	recPrintHeaders  = 0x002A
	recPrintGrid     = 0x002B
	recWindow1       = 0x003D
	recBackup        = 0x0040
	recCodePage      = 0x0042
	recSaveRecalc    = 0x005F
	recGuts          = 0x0080
	recWsBool        = 0x0081
	recGridSet       = 0x0082
	recCountry       = 0x008C
	recHideObj       = 0x008D
	recInterfaceHdr  = 0x00E1
	recInterfaceEnd  = 0x00E2
	recMMS           = 0x00C1
	recBookBool      = 0x00DA
	recExtSST        = 0x00FF
	recTabID         = 0x013D
	recDSF           = 0x0161
	recProt4Rev      = 0x01AF
	recProt4RevPass  = 0x01BC
	recRefreshAll    = 0x01B7
	recDimensions    = 0x0200
	recStyle         = 0x0293
	recWriteAccess   = 0x005C

	maxRecord     = 8224 // largest record body; longer data goes in CONTINUE records
	maxCellStyles = 4000
	maxText       = 32767
	defaultXF     = 15 // the cell XF of unformatted cells
	firstCellXF   = 16
	icvAutoFont   = 0x7FFF
)

type writer struct {
	wb     *model.Workbook
	sheets []*model.Sheet
	names  []string // sheet names as written
	active int      // the sheet shown when the file opens
	issues []string

	// Formats: model style -> XF index, the styles in XF order, fonts and
	// number formats.
	xfOf    map[uint32]int
	styles  []model.Style
	fonts   []model.Font // FONT records after the four default ones
	fontOf  map[model.Font]int
	formats []string // custom number formats, ids 164...
	fmtOf   map[string]int
	pal     *palette

	sst    []string
	sstOf  map[string]int
	sstUse int

	// Formula context.
	xtis     []xti
	xtiOf    map[xti]int
	addins   []string
	addinOf  map[string]int
	nameRecs []nameRec
	fnNameOf map[string]int // hidden function NAME -> 1-based index

	sheetData [][]byte
}

// nameRec is a NAME record to write.
type nameRec struct {
	name        string
	sheet       int // model sheet, -1 = workbook
	flags       int
	rgce, extra []byte
}

const (
	bookSelf  = 0
	bookAddin = 1
)

func newWriter(wb *model.Workbook) *writer {
	x := &writer{
		wb: wb, xfOf: map[uint32]int{}, fontOf: map[model.Font]int{}, fmtOf: map[string]int{},
		sstOf: map[string]int{}, xtiOf: map[xti]int{}, addinOf: map[string]int{}, fnNameOf: map[string]int{},
	}
	x.sheets = wb.Sheets
	if len(x.sheets) == 0 {
		x.sheets = []*model.Sheet{model.NewSheet("Sheet1")}
	}
	if len(x.sheets) > 0xFFFF {
		x.sheets = x.sheets[:0xFFFF]
	}
	x.names = x.sheetNames()
	x.active = slices.IndexFunc(x.sheets, func(sh *model.Sheet) bool { return !sh.Hidden })
	if x.active < 0 {
		x.active = 0
		x.issue("every sheet is hidden; the first sheet will be shown")
	}
	return x
}

func (x *writer) issue(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if !slices.Contains(x.issues, s) {
		x.issues = append(x.issues, s)
	}
}

// sheetNames makes the sheet names valid for Excel: 1 to 31 characters,
// none of []:*?/\, unique ignoring case.
func (x *writer) sheetNames() []string {
	out := make([]string, len(x.sheets))
	used := map[string]bool{}
	for i, sh := range x.sheets {
		n := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`[]:*?/\`, r) {
				return '_'
			}
			return r
		}, sh.Name)
		n = strings.Trim(n, "'")
		if utf8.RuneCountInString(n) > 31 {
			n = string([]rune(n)[:31])
		}
		if n == "" {
			n = fmt.Sprintf("Sheet%d", i+1)
		}
		base := n
		for k := 2; used[strings.ToUpper(n)]; k++ {
			suffix := fmt.Sprintf(" (%d)", k)
			r := []rune(base)
			n = string(r[:min(len(r), 31-len(suffix))]) + suffix
		}
		used[strings.ToUpper(n)] = true
		if n != sh.Name {
			x.issue("sheet %q will be named %q", sh.Name, n)
		}
		out[i] = n
	}
	return out
}

// build converts the workbook: names, then every sheet's records (which
// assigns styles, shared strings and formula names), then the formats.
func (x *writer) build() {
	if len(x.wb.Styles) > 0 {
		x.styles = []model.Style{x.wb.Styles[0]}
	} else {
		x.styles = []model.Style{model.DefaultStyle()}
	}
	x.xfOf[0] = defaultXF
	x.definedNames()
	for i, sh := range x.sheets {
		x.sheetData = append(x.sheetData, x.sheetRecords(i, sh))
	}
	x.buildFormats()
}

// --- formula context ---

func (x *writer) sheetIndex(name string) int {
	for i, sh := range x.sheets {
		if strings.EqualFold(sh.Name, name) || strings.EqualFold(x.names[i], name) {
			return i
		}
	}
	return -1
}

func (x *writer) xtiIndex(first, last int) int {
	return x.xtiFor(xti{bookSelf, first, last})
}

func (x *writer) xtiFor(t xti) int {
	if i, ok := x.xtiOf[t]; ok {
		return i
	}
	x.xtis = append(x.xtis, t)
	x.xtiOf[t] = len(x.xtis) - 1
	return len(x.xtis) - 1
}

// nameIndex finds a user name: the sheet's own name first, then the
// workbook's. User names are the first NAME records.
func (x *writer) nameIndex(name string, sheet int) int {
	found := 0
	for i, n := range x.wb.Names {
		if i >= len(x.nameRecs) || !strings.EqualFold(n.Name, name) {
			continue
		}
		if n.Sheet == sheet {
			return i + 1
		}
		if n.Sheet < 0 {
			found = i + 1
		}
	}
	return found
}

// excel2007Funcs are functions newer than BIFF8 that need no prefix in
// .xlsx; .xls files still name them _xlfn.
var excel2007Funcs = map[string]bool{
	"IFERROR": true, "SUMIFS": true, "COUNTIFS": true, "AVERAGEIF": true, "AVERAGEIFS": true,
	"CUBEKPIMEMBER": true, "CUBEMEMBER": true, "CUBEMEMBERPROPERTY": true, "CUBERANKEDMEMBER": true,
	"CUBESET": true, "CUBESETCOUNT": true, "CUBEVALUE": true,
}

// funcNameRef returns the token naming a function that has no BIFF8 id:
// an add-in name (tNameX) for Analysis ToolPak functions, otherwise a
// hidden function NAME (tName), "_xlfn.XLOOKUP" for Excel 2007+
// functions, as Excel writes them.
func (x *writer) funcNameRef(name string) (byte, []byte) {
	if addinFuncs[name] {
		i, ok := x.addinOf[name]
		if !ok {
			x.addins = append(x.addins, name)
			i = len(x.addins)
			x.addinOf[name] = i
		}
		ixti := x.xtiFor(xti{bookAddin, 0xFFFE, 0xFFFE})
		return ptgNameX, []byte{byte(ixti), byte(ixti >> 8), byte(i), byte(i >> 8), 0, 0}
	}
	full := name
	if p := formula.FuturePrefix(name); p != "" {
		full = p + name
	} else if excel2007Funcs[name] {
		full = "_xlfn." + name
	}
	i, ok := x.fnNameOf[full]
	if !ok {
		x.nameRecs = append(x.nameRecs, nameRec{name: full, sheet: -1, flags: nameHidden | nameFunc | nameProc})
		i = len(x.nameRecs)
		x.fnNameOf[full] = i
	}
	return ptgName, []byte{byte(i), byte(i >> 8), 0, 0}
}

// definedNames compiles the workbook's defined names into the first NAME
// records. A name whose formula cannot be saved is written as #REF!.
func (x *writer) definedNames() {
	x.nameRecs = make([]nameRec, len(x.wb.Names))
	for i, n := range x.wb.Names {
		sheet := n.Sheet
		if sheet >= len(x.sheets) {
			sheet = -1
		}
		x.nameRecs[i] = nameRec{name: n.Name, sheet: sheet}
	}
	for i, n := range x.wb.Names {
		base := max(x.nameRecs[i].sheet, 0)
		rgce, extra, err := compileFormula(x, strings.TrimPrefix(n.Formula, "="), base, true)
		if err != nil {
			x.issue("defined name %s cannot be saved in .xls (%v); it will refer to #REF!", n.Name, err)
			rgce, extra = []byte{ptgErr, 0x17}, nil
		}
		x.nameRecs[i].rgce, x.nameRecs[i].extra = rgce, extra
	}
}

// --- sheets ---

// sheetRecords encodes one worksheet substream.
func (x *writer) sheetRecords(si int, sh *model.Sheet) []byte {
	var b []byte
	b = rec(b, recBOF, le(biff8Version, bofTypeWorksheet, 0x0DBB, 0x07CC, 0, 0, 6, 0))
	b = rec(b, recCalcMode, le(1))
	b = rec(b, recCalcCount, le(100))
	b = rec(b, recRefMode, le(1))
	b = rec(b, recIteration, le(0))
	b = rec(b, recDelta, binary.LittleEndian.AppendUint64(nil, math.Float64bits(0.001)))
	b = rec(b, recSaveRecalc, le(1))
	b = rec(b, recPrintHeaders, le(0))
	b = rec(b, recPrintGrid, le(0))
	b = rec(b, recGridSet, le(1))
	b = rec(b, recGuts, le(0, 0, 0, 0))
	defTw := pxToTwips(sh.DefaultRowHeight)
	if defTw <= 0 {
		defTw = pxToTwips(model.DefaultRowHeightPx)
	}
	unsynced := 0
	if sh.DefaultRowHeight > 0 && sh.DefaultRowHeight != model.DefaultRowHeightPx {
		unsynced = 1
	}
	b = rec(b, recDefaultRowHt, le(unsynced, defTw))
	b = rec(b, recWsBool, le(0x04C1))
	base := int(math.Round(float64(sh.BaseColWidth)))
	if base <= 0 {
		base = model.DefaultBaseColWidth
	}
	b = rec(b, recDefColWidth, le(min(base, 255)))
	if sh.DefaultColWidth > 0 {
		b = rec(b, recStandardWidth, le(min(int(math.Round(float64(sh.DefaultColWidth)*256)), 0xFFFF)))
	}
	b = x.colInfo(b, sh)

	// Extent of what is written.
	rows := min(sh.UsedRows(), xlsMaxRows)
	dropped := 0
	for r := xlsMaxRows; r < sh.UsedRows(); r++ {
		for c := 0; c < sh.RowLen(r); c++ {
			if !isBlank(sh.Get(r, c)) {
				dropped++
			}
		}
	}
	r1, r2, c1, c2 := -1, -1, -1, -1
	for r := 0; r < rows; r++ {
		for c := 0; c < sh.RowLen(r); c++ {
			if isBlank(sh.Get(r, c)) && sh.Get(r, c).Style == 0 {
				continue
			}
			if c >= xlsMaxCols {
				if !isBlank(sh.Get(r, c)) {
					dropped++
				}
				continue
			}
			if r1 < 0 {
				r1 = r
			}
			r2 = r
			if c1 < 0 || c < c1 {
				c1 = c
			}
			c2 = max(c2, c)
		}
	}
	if dropped > 0 {
		x.issue("%s: %d cells beyond row 65,536 or column IV will not be saved", sh.Name, dropped)
	}
	if r1 < 0 {
		b = rec(b, recDimensions, append(le32(0, 0), le(0, 0, 0)...))
	} else {
		b = rec(b, recDimensions, append(le32(uint32(r1), uint32(r2+1)), le(c1, c2+1, 0)...))
	}

	// Rows in blocks of 32: the ROW records, then the block's cells.
	lastRow := rows
	for r := range sh.RowHeight {
		if r < xlsMaxRows {
			lastRow = max(lastRow, r+1)
		}
	}
	var fails []string
	nFails, nDynamic := 0, 0
	for start := 0; start < lastRow; start += 32 {
		end := min(start+32, lastRow)
		var cells []byte
		for r := start; r < end; r++ {
			first, last := -1, -1
			for c := 0; c < min(sh.RowLen(r), xlsMaxCols); c++ {
				cell := sh.Get(r, c)
				if isBlank(cell) && cell.Style == 0 {
					continue
				}
				if first < 0 {
					first = c
				}
				last = c
				var err error
				cells, err = x.cell(cells, si, sh, r, c, cell)
				if err != nil {
					if nFails++; len(fails) < 5 {
						fails = append(fails, fmt.Sprintf("%s (=%s: %v)", model.CellName(r, c), cell.Formula, err))
					}
				}
				if cell.Formula != "" && cell.Dynamic {
					if rng, ok := sh.Spills[[2]int{r, c}]; ok && (rng.R1 != rng.R2 || rng.C1 != rng.C2) {
						nDynamic++
					}
				}
			}
			h, custom := sh.RowHeight[r]
			if first < 0 && !custom {
				continue
			}
			flags := 0x0100 | defaultXF<<16
			height := defTw
			switch {
			case custom && h == 0:
				flags |= 0x20 | 0x40
			case custom:
				flags |= 0x40
				height = min(pxToTwips(h), 8192)
			}
			colMic, colMac := 0, 0
			if first >= 0 {
				colMic, colMac = first, last+1
			}
			b = rec(b, recRow, append(le(r, colMic, colMac, height, 0, 0), le32(uint32(flags))...))
		}
		b = append(b, cells...)
	}
	if nFails > 0 {
		more := ""
		if nFails > len(fails) {
			more = fmt.Sprintf(" and %d more", nFails-len(fails))
		}
		x.issue("%s: formulas that .xls cannot hold will be saved as values: %s%s", sh.Name, strings.Join(fails, ", "), more)
	}
	if nDynamic > 0 {
		x.issue("%s: %d dynamic-array formulas will be saved as ordinary formulas; the cells they spill into keep their current values", sh.Name, nDynamic)
	}

	b = x.window(b, si, sh)
	b = x.merges(b, sh)
	return rec(b, recEOF, nil)
}

// isBlank reports whether a cell has neither a value nor a formula.
func isBlank(c *model.Cell) bool { return c.Kind == model.KindEmpty && c.Formula == "" }

func pxToTwips(px float32) int { return int(math.Round(float64(px) * 15)) }

// colInfo writes COLINFO records for runs of columns with equal widths.
func (x *writer) colInfo(b []byte, sh *model.Sheet) []byte {
	var cols []int
	for c := range sh.ColWidth {
		if c >= 0 && c < xlsMaxCols {
			cols = append(cols, c)
		}
	}
	slices.Sort(cols)
	hiddenWidth := sh.DefaultColWidth
	if hiddenWidth <= 0 {
		hiddenWidth = sh.BaseColWidth + 0.71
	}
	for i := 0; i < len(cols); {
		lo, w := cols[i], sh.ColWidth[cols[i]]
		j := i + 1
		for j < len(cols) && cols[j] == cols[j-1]+1 && sh.ColWidth[cols[j]] == w {
			j++
		}
		flags := 0
		if w == 0 {
			flags, w = 1, hiddenWidth
		}
		width := min(int(math.Round(float64(w)*256)), 0xFFFF)
		b = rec(b, recColInfo, le(lo, cols[j-1], width, defaultXF, flags, 0))
		i = j
	}
	return b
}

// cell appends the records of one cell. A formula that cannot be encoded
// is written as its value and reported.
func (x *writer) cell(b []byte, si int, sh *model.Sheet, r, c int, cell *model.Cell) ([]byte, error) {
	xf := x.xf(cell.Style)
	head := le(r, c, xf)
	var ferr error
	if cell.Formula != "" {
		rgce, extra, err := compileFormula(x, cell.Formula, si, false)
		if err == nil && 22+len(rgce)+len(extra) > maxRecord {
			err = fmt.Errorf("formula is too long for .xls")
		}
		if err == nil {
			return x.formula(b, head, cell, rgce, extra), nil
		}
		ferr = err
		if cell.Kind == model.KindEmpty {
			return rec(b, recBlank, head), ferr
		}
	}
	switch cell.Kind {
	case model.KindNumber:
		if rk, ok := rkEncode(cell.Num); ok {
			return rec(b, recRK, append(head, le32(rk)...)), ferr
		}
		return rec(b, recNumber, binary.LittleEndian.AppendUint64(head, math.Float64bits(cell.Num))), ferr
	case model.KindText:
		return rec(b, recLabelSST, append(head, le32(uint32(x.sstIndex(x.text(sh, cell.Text))))...)), ferr
	case model.KindBool:
		return rec(b, recBoolErr, append(head, byte(cell.Num), 0)), ferr
	case model.KindError:
		return rec(b, recBoolErr, append(head, errorCode(cell.Text), 1)), ferr
	}
	return rec(b, recBlank, head), ferr
}

// text truncates s to the longest text a cell can hold.
func (x *writer) text(sh *model.Sheet, s string) string {
	if strLen(s) <= maxText {
		return s
	}
	x.issue("%s: text longer than 32,767 characters will be cut", sh.Name)
	r := []rune(s)
	for strLen(string(r)) > maxText {
		r = r[:len(r)-max(1, (strLen(string(r))-maxText)/2)]
	}
	return string(r)
}

func errorCode(s string) byte {
	if c, ok := errorCodes[strings.ToUpper(s)]; ok {
		return c
	}
	return errorCodes["#VALUE!"]
}

// formula appends a FORMULA record, and a STRING record for a text result.
func (x *writer) formula(b, head []byte, cell *model.Cell, rgce, extra []byte) []byte {
	var val []byte
	str := false
	switch cell.Kind {
	case model.KindNumber:
		val = binary.LittleEndian.AppendUint64(nil, math.Float64bits(cell.Num))
	case model.KindText:
		if cell.Text == "" {
			val = []byte{3, 0, 0, 0, 0, 0, 0xFF, 0xFF}
		} else {
			val, str = []byte{0, 0, 0, 0, 0, 0, 0xFF, 0xFF}, true
		}
	case model.KindBool:
		val = []byte{1, 0, byte(cell.Num), 0, 0, 0, 0xFF, 0xFF}
	case model.KindError:
		val = []byte{2, 0, errorCode(cell.Text), 0, 0, 0, 0xFF, 0xFF}
	default:
		val = []byte{3, 0, 0, 0, 0, 0, 0xFF, 0xFF}
	}
	body := append(head, val...)
	body = append(body, le(0x0002, 0, 0, len(rgce))...) // calculate on load; chn
	body = append(body, rgce...)
	body = append(body, extra...)
	b = rec(b, recFormula, body)
	if str {
		s := cell.Text
		if strLen(s) > maxText {
			s = string([]rune(s)[:maxText/2])
		}
		b = stringRecords(b, recString, nil, []string{s}, nil)
	}
	return b
}

// window writes WINDOW2, PANE and SELECTION.
func (x *writer) window(b []byte, si int, sh *model.Sheet) []byte {
	grbit := 0x04 | 0x10 | 0x20 | 0x80 // headers, zeros, default header color, outline symbols
	if sh.ShowGridLines {
		grbit |= 0x02
	}
	frows, fcols := min(sh.FreezeRows, xlsMaxRows-1), min(sh.FreezeCols, xlsMaxCols-1)
	frozen := frows > 0 || fcols > 0
	if frozen {
		grbit |= 0x08 | 0x100
	}
	if si == x.active {
		grbit |= 0x200 | 0x400
	}
	top, left := min(sh.TopRow, xlsMaxRows-1), min(sh.LeftCol, xlsMaxCols-1)
	b = rec(b, recWindow2, append(le(grbit, top, left), append(le32(64), le(0, 0, 0, 0)...)...))
	pane := 3
	if frozen {
		switch {
		case frows > 0 && fcols > 0:
			pane = 0
		case frows > 0:
			pane = 2
		default:
			pane = 1
		}
		b = rec(b, recPane, append(le(fcols, frows, max(frows, top), max(fcols, left)), byte(pane), 0))
	}
	ar, ac := min(max(sh.ActiveRow, 0), xlsMaxRows-1), min(max(sh.ActiveCol, 0), xlsMaxCols-1)
	sel := append([]byte{byte(pane)}, le(ar, ac, 0, 1, ar, ar)...)
	sel = append(sel, byte(ac), byte(ac))
	return rec(b, recSelection, sel)
}

// merges writes MERGEDCELLS records, at most 1026 ranges each.
func (x *writer) merges(b []byte, sh *model.Sheet) []byte {
	var body []byte
	n := 0
	flush := func() {
		if n > 0 {
			b = rec(b, recMergedCells, append(le(n), body...))
		}
		body, n = nil, 0
	}
	for _, m := range sh.Merges {
		if m.R1 >= xlsMaxRows || m.C1 >= xlsMaxCols {
			continue
		}
		r2, c2 := min(m.R2, xlsMaxRows-1), min(m.C2, xlsMaxCols-1)
		if m.R1 == r2 && m.C1 == c2 {
			continue
		}
		body = append(body, le(m.R1, r2, m.C1, c2)...)
		if n++; n == 1026 {
			flush()
		}
	}
	flush()
	return b
}

func (x *writer) sstIndex(s string) int {
	x.sstUse++
	if i, ok := x.sstOf[s]; ok {
		return i
	}
	x.sst = append(x.sst, s)
	x.sstOf[s] = len(x.sst) - 1
	return len(x.sst) - 1
}

// --- formats ---

// xf returns the XF index of a model style, assigning one on first use.
func (x *writer) xf(style uint32) int {
	if i, ok := x.xfOf[style]; ok {
		return i
	}
	if int(style) >= len(x.wb.Styles) {
		return defaultXF
	}
	if len(x.styles) > maxCellStyles {
		x.issue("more than %d cell styles: the rest will use the default style", maxCellStyles)
		x.xfOf[style] = defaultXF
		return defaultXF
	}
	x.styles = append(x.styles, x.wb.Styles[style])
	i := firstCellXF + len(x.styles) - 2
	x.xfOf[style] = i
	return i
}

// buildFormats builds the palette, fonts and number formats of the styles
// in use.
func (x *writer) buildFormats() {
	var used []color.NRGBA
	add := func(c color.NRGBA) {
		if c.A != 0 && !slices.Contains(used, c) {
			used = append(used, c)
		}
	}
	for _, st := range x.styles {
		if st.Font.Color != colors.Black {
			add(st.Font.Color)
		}
		add(st.Fill)
		for _, e := range st.Border {
			if e.Style != model.BorderNone {
				add(e.Color)
			}
		}
	}
	x.pal = newPalette(used)
	if x.pal.approximated > 0 {
		x.issue("%d colors will be replaced by the nearest of the 56 colors an .xls file can hold", x.pal.approximated)
	}
	for _, st := range x.styles {
		x.fontIndex(st.Font)
		x.numFormat(st)
	}
}

// fontIndex returns the XF font index of f. FONT records 0-3 are the
// default font; index 4 is never used, so record k (k >= 4) is index k+1.
func (x *writer) fontIndex(f model.Font) int {
	if f == x.styles[0].Font {
		return 0
	}
	if i, ok := x.fontOf[f]; ok {
		return i
	}
	x.fonts = append(x.fonts, f)
	i := 4 + len(x.fonts)
	x.fontOf[f] = i
	return i
}

func (x *writer) numFormat(st model.Style) int {
	code := st.NumFmtCode
	if code == "" {
		if st.NumFmtID > 0 && st.NumFmtID < 164 {
			return st.NumFmtID
		}
		return 0
	}
	if strings.EqualFold(code, "General") {
		return 0
	}
	if strLen(code) > 255 {
		x.issue("a number format longer than 255 characters will be replaced by General")
		return 0
	}
	if i, ok := x.fmtOf[code]; ok {
		return i
	}
	x.formats = append(x.formats, code)
	i := 163 + len(x.formats)
	x.fmtOf[code] = i
	return i
}

func (x *writer) fontRecord(f model.Font) []byte {
	grbit := 0
	if f.Italic {
		grbit |= 0x02
	}
	if f.Strike {
		grbit |= 0x08
	}
	icv := icvAutoFont
	if f.Color.A != 0 && f.Color != colors.Black {
		icv = x.pal.index(f.Color)
	}
	bls := 400
	if f.Bold {
		bls = 700
	}
	size := f.SizePt
	if size <= 0 {
		size = 11
	}
	uls := byte(0)
	if f.Underline {
		uls = 1
	}
	name := f.Family
	if name == "" {
		name = "Calibri"
	}
	b := le(min(int(math.Round(float64(size)*20)), 0x7FFF), grbit, icv, bls, 0)
	b = append(b, uls, 0, 0, 0)
	return appendShortString(b, truncUTF16(name, 255))
}

func truncUTF16(s string, n int) string {
	r := []rune(s)
	for strLen(string(r)) > n {
		r = r[:len(r)-1]
	}
	return string(r)
}

// xfRecord encodes a cell XF.
func (x *writer) xfRecord(st model.Style) []byte {
	valign := map[model.VAlign]int{
		model.VAlignTop: 0, model.VAlignCenter: 1, model.VAlignBottom: 2,
		model.VAlignJustify: 3, model.VAlignDistributed: 4,
	}[st.VAlign]
	align := int(st.HAlign)&0x07 | valign<<4
	if st.Wrap {
		align |= 0x08
	}
	rot := st.Rotation
	if rot < 0 || rot > 180 && rot != 255 {
		rot = 0
	}
	var dg [4]int
	var icv [4]int
	for side, e := range st.Border {
		if e.Style != model.BorderNone {
			dg[side] = int(e.Style) & 0x0F
			icv[side] = x.pal.index(e.Color)
		}
	}
	border1 := uint32(dg[model.Left]) | uint32(dg[model.Right])<<4 | uint32(dg[model.Top])<<8 |
		uint32(dg[model.Bottom])<<12 | uint32(icv[model.Left])<<16 | uint32(icv[model.Right])<<23
	border2 := uint32(icv[model.Top]) | uint32(icv[model.Bottom])<<7
	fore, back := 64, 65
	if st.Fill.A != 0 {
		border2 |= 1 << 26
		fore = x.pal.index(st.Fill)
	}
	b := le(x.fontIndex(st.Font), x.numFormat(st), 0x0001) // locked, parent XF 0
	b = append(b, byte(align), byte(rot), byte(min(max(st.Indent, 0), 15)), 0xFC)
	b = append(b, le32(border1, border2)...)
	return append(b, le(fore|back<<7)...)
}

// --- palette ---

// palette is the 56-color table (indexes 8..63) that every BIFF8 color
// refers to.
type palette struct {
	colors       [56]color.NRGBA
	custom       bool
	approximated int
}

// newPalette starts from Excel's default palette and replaces entries
// with the colors used, from the last entry backwards; colors that do not
// fit map to the nearest entry.
func newPalette(used []color.NRGBA) *palette {
	p := &palette{}
	for i := range p.colors {
		p.colors[i] = colors.HexOr(excelize.IndexedColorMapping[8+i], colors.Black)
	}
	taken := map[int]bool{}
	var missing []color.NRGBA
	for _, c := range used {
		c.A = 255
		if i := slices.Index(p.colors[:], c); i >= 0 {
			taken[i] = true
		} else {
			missing = append(missing, c)
		}
	}
	free := 55
	for _, c := range missing {
		for free >= 2 && taken[free] { // keep black and white
			free--
		}
		if free < 2 {
			p.approximated++
			continue
		}
		p.colors[free] = c
		taken[free] = true
		p.custom = true
	}
	return p
}

// index returns the palette index (8..63) of the entry nearest to c.
func (p *palette) index(c color.NRGBA) int {
	best, bestD := 0, math.MaxInt
	for i, e := range p.colors {
		dr, dg, db := int(e.R)-int(c.R), int(e.G)-int(c.G), int(e.B)-int(c.B)
		if d := dr*dr + dg*dg + db*db; d < bestD {
			best, bestD = i, d
		}
	}
	return 8 + best
}

// --- workbook stream ---

// stream assembles the workbook globals and the sheets.
func (x *writer) stream() []byte {
	var b []byte
	b = rec(b, recBOF, le(biff8Version, 0x0005, 0x0DBB, 0x07CC, 0, 0, 6, 0))
	b = rec(b, recInterfaceHdr, le(1200))
	b = rec(b, recMMS, le(0))
	b = rec(b, recInterfaceEnd, nil)
	wa := appendString(nil, "OfficeLite")
	for len(wa) < 112 {
		wa = append(wa, ' ')
	}
	b = rec(b, recWriteAccess, wa)
	b = rec(b, recCodePage, le(1200))
	b = rec(b, recDSF, le(0))
	var tabs []byte
	for i := range x.sheets {
		tabs = append(tabs, le(i+1)...)
	}
	b = rec(b, recTabID, tabs)
	b = rec(b, recWindowProtect, le(0))
	b = rec(b, recProtect, le(0))
	b = rec(b, recPassword, le(0))
	b = rec(b, recProt4Rev, le(0))
	b = rec(b, recProt4RevPass, le(0))
	b = rec(b, recWindow1, le(0x01E0, 0x005A, 0x3FCF, 0x2A4E, 0x0038, x.active, 0, 1, 0x0258))
	b = rec(b, recBackup, le(0))
	b = rec(b, recHideObj, le(0))
	date1904 := 0
	if x.wb.Date1904 {
		date1904 = 1
	}
	b = rec(b, recDateMode, le(date1904))
	b = rec(b, recPrecision, le(1))
	b = rec(b, recRefreshAll, le(0))
	b = rec(b, recBookBool, le(0))

	def := x.fontRecord(x.styles[0].Font)
	for range 4 {
		b = rec(b, recFont, def)
	}
	for _, f := range x.fonts {
		b = rec(b, recFont, x.fontRecord(f))
	}
	for i, code := range x.formats {
		b = rec(b, recFormat, appendString(le(164+i), code))
	}
	// Style XFs 0-14 (Normal and the built-in outline styles), then the
	// cell XFs.
	for i := range 15 {
		font, used := 0, 0xF4
		switch {
		case i == 0:
			used = 0
		case i <= 2:
			font = 1
		case i <= 4:
			font = 2
		}
		b = rec(b, recXF, append(le(font, 0, 0xFFF5), append([]byte{0x20, 0, 0, byte(used)}, append(le32(0, 0), le(0x20C0)...)...)...))
	}
	for _, st := range x.styles {
		b = rec(b, recXF, x.xfRecord(st))
	}
	b = rec(b, recStyle, append(le(0x8000), 0, 0xFF))
	if x.pal.custom {
		pb := le(56)
		for _, c := range x.pal.colors {
			pb = append(pb, c.R, c.G, c.B, 0)
		}
		b = rec(b, recPalette, pb)
	}
	sheetPos := make([]int, len(x.sheets))
	for i, sh := range x.sheets {
		sheetPos[i] = len(b) + 4
		hidden := byte(0)
		if sh.Hidden && i != x.active {
			hidden = 1
		}
		b = rec(b, recBoundSheet, appendShortString(append(le32(0), hidden, 0), x.names[i]))
	}
	b = rec(b, recCountry, le(1, 1))
	b = x.links(b)
	b = x.sstRecords(b)
	b = rec(b, recEOF, nil)
	for i, data := range x.sheetData {
		binary.LittleEndian.PutUint32(b[sheetPos[i]:], uint32(len(b)))
		b = append(b, data...)
	}
	return b
}

// links writes SUPBOOK, EXTERNNAME, EXTERNSHEET and NAME records.
func (x *writer) links(b []byte) []byte {
	if len(x.nameRecs) == 0 && len(x.xtis) == 0 {
		return b
	}
	if len(x.xtis) == 0 {
		x.xtiIndex(0, 0)
	}
	b = rec(b, recSupBook, le(len(x.sheets), 0x0401))
	if len(x.addins) > 0 {
		b = rec(b, recSupBook, le(1, 0x3A01))
		for _, n := range x.addins {
			b = rec(b, recExternName, append(appendShortString(le(0, 0, 0), n), le(2)[0], 0, ptgErr, 0x17))
		}
	}
	xb := le(len(x.xtis))
	for _, t := range x.xtis {
		xb = append(xb, le(t.book, t.first, t.last)...)
	}
	b = rec(b, recExternSheet, xb)
	for _, n := range x.nameRecs {
		itab := 0
		if n.sheet >= 0 {
			itab = n.sheet + 1
		}
		nb := []byte{byte(n.flags), byte(n.flags >> 8), 0, byte(strLen(n.name))}
		nb = append(nb, le(len(n.rgce), 0, itab)...)
		nb = append(nb, 0, 0, 0, 0)
		nb = appendChars(nb, n.name)
		nb = append(nb, n.rgce...)
		nb = append(nb, n.extra...)
		b = rec(b, recName, nb)
	}
	return b
}

// sstRecords writes the shared string table and its index (EXTSST).
func (x *writer) sstRecords(b []byte) []byte {
	dsst := max(8, (len(x.sst)+127)/128)
	var ext []byte
	b = stringRecords(b, recSST, le32(uint32(x.sstUse), uint32(len(x.sst))), x.sst, func(i, pos, recStart int) {
		if i%dsst == 0 {
			ext = append(ext, le32(uint32(pos))...)
			ext = append(ext, le(pos-recStart, 0)...)
		}
	})
	return rec(b, recExtSST, append(le(dsst), ext...))
}

// stringRecords writes strings as XLUnicodeRichExtendedStrings in a record
// with id and its CONTINUE records. A string's 3-byte header never splits;
// when its characters do, the CONTINUE record restates the option byte.
// mark, if set, receives each string's stream position and the position
// of the record holding it.
func stringRecords(b []byte, id int, head []byte, strs []string, mark func(i, pos, recStart int)) []byte {
	recStart := len(b)
	b = append(b, byte(id), byte(id>>8), 0, 0)
	b = append(b, head...)
	room := func() int { return maxRecord - (len(b) - recStart - 4) }
	next := func() {
		binary.LittleEndian.PutUint16(b[recStart+2:], uint16(len(b)-recStart-4))
		recStart = len(b)
		b = append(b, byte(recContinue), byte(recContinue>>8), 0, 0)
	}
	for i, s := range strs {
		wide := !isLatin1(s)
		var units []uint16
		if wide {
			units = utf16Units(s)
		} else {
			for _, c := range s {
				units = append(units, uint16(c))
			}
		}
		csz := 1
		if wide {
			csz = 2
		}
		if room() < 3+min(len(units), 1)*csz {
			next()
		}
		if mark != nil {
			mark(i, len(b), recStart)
		}
		b = binary.LittleEndian.AppendUint16(b, uint16(len(units)))
		b = append(b, byte(csz-1))
		for len(units) > 0 {
			n := min(len(units), room()/csz)
			if n == 0 {
				next()
				b = append(b, byte(csz-1))
				continue
			}
			for _, u := range units[:n] {
				if wide {
					b = binary.LittleEndian.AppendUint16(b, u)
				} else {
					b = append(b, byte(u))
				}
			}
			units = units[n:]
		}
	}
	binary.LittleEndian.PutUint16(b[recStart+2:], uint16(len(b)-recStart-4))
	return b
}

func utf16Units(s string) []uint16 {
	var u []uint16
	for _, c := range s {
		if c >= 0x10000 {
			c -= 0x10000
			u = append(u, uint16(0xD800+(c>>10)), uint16(0xDC00+(c&0x3FF)))
		} else {
			u = append(u, uint16(c))
		}
	}
	return u
}

// --- encoding helpers ---

// rec appends a record; bodies longer than a record can hold continue in
// CONTINUE records.
func rec(b []byte, id int, body []byte) []byte {
	for {
		n := min(len(body), maxRecord)
		b = binary.LittleEndian.AppendUint16(b, uint16(id))
		b = binary.LittleEndian.AppendUint16(b, uint16(n))
		b = append(b, body[:n]...)
		body = body[n:]
		if len(body) == 0 {
			return b
		}
		id = recContinue
	}
}

// le encodes 16-bit little-endian values.
func le(v ...int) []byte {
	b := make([]byte, 0, 2*len(v))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint16(b, uint16(x))
	}
	return b
}

func le32(v ...uint32) []byte {
	b := make([]byte, 0, 4*len(v))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, x)
	}
	return b
}

// rkEncode returns the RK form of v if it is exact.
func rkEncode(v float64) (uint32, bool) {
	var cands []uint32
	if v == math.Trunc(v) && v >= -(1<<29) && v < 1<<29 {
		cands = append(cands, uint32(int32(v))<<2|2)
	}
	if bits := math.Float64bits(v); bits&(1<<34-1) == 0 {
		cands = append(cands, uint32(bits>>32))
	}
	if w := v * 100; w == math.Trunc(w) && w >= -(1<<29) && w < 1<<29 {
		cands = append(cands, uint32(int32(w))<<2|3)
	}
	if bits := math.Float64bits(v * 100); bits&(1<<34-1) == 0 {
		cands = append(cands, uint32(bits>>32)|1)
	}
	for _, rk := range cands {
		if rkValue(rk) == v {
			return rk, true
		}
	}
	return 0, false
}
