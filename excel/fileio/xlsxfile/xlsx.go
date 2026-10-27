// Package xlsxfile reads Office Open XML workbooks (.xlsx, .xlsm).
//
// Worksheet XML is streamed directly (excelize would expand every sheet into
// a full in-memory DOM, which is too heavy at 256K rows). excelize is used
// only for what it does well and is small: resolving the style table and
// theme colors.
package xlsxfile

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"officelite/common/colors"
	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

type cellFormat struct {
	style   uint32 // model style index
	fmtID   int
	fmtCode string
}

type reader struct {
	files   map[string]*zip.File // normalized lower-case path -> entry
	wb      *model.Workbook
	sst     []string
	formats []cellFormat // indexed by cellXfs index
	nf      *numfmt.Formatter
}

// Read parses an .xlsx package.
func Read(data []byte) (*model.Workbook, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	r := &reader{files: map[string]*zip.File{}, wb: &model.Workbook{}}
	for _, f := range zr.File {
		r.files[normPath(f.Name)] = f
	}

	wbPath := "xl/workbook.xml"
	if rels, err := r.rels("_rels/.rels", ""); err == nil {
		for _, rel := range rels {
			if strings.HasSuffix(rel.Type, "/officeDocument") {
				wbPath = rel.Target
			}
		}
	}
	var book xWorkbook
	if err := r.unmarshal(wbPath, &book); err != nil {
		return nil, fmt.Errorf("workbook: %w", err)
	}
	rels, err := r.rels(relsPath(wbPath), path.Dir(wbPath))
	if err != nil {
		return nil, fmt.Errorf("workbook relationships: %w", err)
	}
	byID := map[string]xRel{}
	stylesPath := ""
	for _, rel := range rels {
		byID[rel.ID] = rel
		if strings.HasSuffix(rel.Type, "/styles") {
			stylesPath = rel.Target
		}
		if strings.HasSuffix(rel.Type, "/sharedStrings") {
			if r.sst, err = r.readSST(rel.Target); err != nil {
				return nil, fmt.Errorf("shared strings: %w", err)
			}
		}
	}

	r.wb.Date1904 = xmlBool(book.Pr.Date1904)
	r.nf = numfmt.New(r.wb.Date1904)
	defer r.nf.Close()
	r.loadNumFmts(stylesPath)
	r.loadStyles(data)

	// localSheetId in defined names counts all sheets, including chart
	// sheets that are not loaded.
	sheetIndex := map[int]int{}
	for i, s := range book.Sheets {
		rel, ok := byID[s.rid()]
		if !ok || !strings.HasSuffix(rel.Type, "/worksheet") {
			continue // chart sheets, dialog sheets
		}
		sheetIndex[i] = len(r.wb.Sheets)
		sh := model.NewSheet(s.Name)
		sh.Hidden = s.State == "hidden" || s.State == "veryHidden"
		if err := r.readSheet(rel.Target, sh); err != nil {
			return nil, fmt.Errorf("sheet %q: %w", s.Name, err)
		}
		r.wb.Sheets = append(r.wb.Sheets, sh)
	}
	for _, dn := range book.Names {
		if strings.HasPrefix(dn.Name, "_xlnm.") || dn.Hidden == "1" {
			continue // print areas, filter databases ...
		}
		n := model.DefinedName{Name: dn.Name, Sheet: -1, Formula: dn.Formula}
		if dn.LocalSheet != nil {
			idx, ok := sheetIndex[*dn.LocalSheet]
			if !ok {
				continue
			}
			n.Sheet = idx
		}
		r.wb.Names = append(r.wb.Names, n)
	}
	return r.wb, nil
}

// --- package plumbing ---

func normPath(p string) string {
	return strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(p, `\`, "/"), "/"))
}

func relsPath(p string) string {
	return path.Join(path.Dir(p), "_rels", path.Base(p)+".rels")
}

func (r *reader) open(name string) (io.ReadCloser, error) {
	f, ok := r.files[normPath(name)]
	if !ok {
		return nil, fmt.Errorf("missing part %s", name)
	}
	return f.Open()
}

func (r *reader) unmarshal(name string, v any) error {
	rc, err := r.open(name)
	if err != nil {
		return err
	}
	defer rc.Close()
	return xml.NewDecoder(rc).Decode(v)
}

type xRel struct {
	ID     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
	Mode   string `xml:"TargetMode,attr"`
}

// rels reads a relationships part, resolving targets against baseDir.
func (r *reader) rels(name, baseDir string) ([]xRel, error) {
	var v struct {
		Rels []xRel `xml:"Relationship"`
	}
	if err := r.unmarshal(name, &v); err != nil {
		return nil, err
	}
	for i, rel := range v.Rels {
		if rel.Mode == "External" {
			continue
		}
		if strings.HasPrefix(rel.Target, "/") {
			v.Rels[i].Target = rel.Target[1:]
		} else {
			v.Rels[i].Target = path.Join(baseDir, rel.Target)
		}
	}
	return v.Rels, nil
}

type xWorkbook struct {
	Pr struct {
		Date1904 string `xml:"date1904,attr"`
	} `xml:"workbookPr"`
	Sheets []xSheetEntry `xml:"sheets>sheet"`
	Names  []struct {
		Name       string `xml:"name,attr"`
		LocalSheet *int   `xml:"localSheetId,attr"`
		Hidden     string `xml:"hidden,attr"`
		Formula    string `xml:",chardata"`
	} `xml:"definedNames>definedName"`
}

type xSheetEntry struct {
	Name  string     `xml:"name,attr"`
	State string     `xml:"state,attr"`
	Attrs []xml.Attr `xml:",any,attr"`
}

// rid returns the r:id attribute (transitional and strict namespaces differ).
func (s xSheetEntry) rid() string {
	for _, a := range s.Attrs {
		if a.Name.Local == "id" && a.Name.Space != "" {
			return a.Value
		}
	}
	return ""
}

func xmlBool(s string) bool { return s == "1" || s == "true" }

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// unescapeOOXML decodes the _xHHHH_ escapes OOXML uses for control chars.
func unescapeOOXML(s string) string {
	if !strings.Contains(s, "_x") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '_' && i+6 < len(s) && s[i+1] == 'x' && s[i+6] == '_' {
			if v, err := strconv.ParseUint(s[i+2:i+6], 16, 16); err == nil {
				b.WriteRune(rune(v))
				i += 6
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// --- shared strings ---

func (r *reader) readSST(name string) ([]string, error) {
	rc, err := r.open(name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	dec := xml.NewDecoder(rc)
	var (
		out   []string
		cur   strings.Builder
		inT   bool
		inRPh int // depth inside phonetic runs, whose text is not displayed
	)
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				cur.Reset()
			case "t":
				inT = inRPh == 0
			case "rPh":
				inRPh++
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				out = append(out, unescapeOOXML(cur.String()))
			case "t":
				inT = false
			case "rPh":
				inRPh--
			}
		case xml.CharData:
			if inT {
				cur.Write(t)
			}
		}
	}
}

// --- styles ---

// loadStyles converts the cellXfs table into model styles via excelize,
// which resolves theme and indexed colors and tints for us.
func (r *reader) loadStyles(data []byte) {
	def := model.DefaultStyle()
	x, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		r.wb.Styles = []model.Style{def}
		r.wb.Warnings = append(r.wb.Warnings, "styles could not be read; showing unformatted values")
		return
	}
	defer x.Close()

	cache := map[model.Style]uint32{}
	for i := 0; ; i++ {
		st, err := x.GetStyle(i)
		if err != nil {
			break
		}
		ms := convertStyle(x, st, def)
		if i < len(r.formats) {
			ms.NumFmtID, ms.NumFmtCode = r.formats[i].fmtID, r.formats[i].fmtCode
		} else {
			r.formats = append(r.formats, cellFormat{})
		}
		if i == 0 {
			// The Normal style's font is the default for everything else.
			def = ms
			r.wb.Styles = nil
		}
		r.formats[i].style = r.wb.AddStyle(ms, cache)
	}
	if len(r.wb.Styles) == 0 {
		r.wb.Styles = []model.Style{def}
	}
}

// loadNumFmts reads each cell format's number-format id and code straight
// from styles.xml: excelize's GetStyle drops ids it has no code for (the
// locale-dependent currency formats 5-8, for one). It runs before
// loadStyles, which adds the formats to the model styles.
func (r *reader) loadNumFmts(path string) {
	if path == "" {
		return
	}
	var ss struct {
		NumFmts []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Xfs []struct {
			NumFmtID int `xml:"numFmtId,attr"`
		} `xml:"cellXfs>xf"`
	}
	if err := r.unmarshal(path, &ss); err != nil {
		return
	}
	codes := map[int]string{}
	for _, nf := range ss.NumFmts {
		codes[nf.ID] = nf.Code
	}
	for i, xf := range ss.Xfs {
		if i >= len(r.formats) {
			r.formats = append(r.formats, cellFormat{})
		}
		r.formats[i].fmtID = xf.NumFmtID
		r.formats[i].fmtCode = codes[xf.NumFmtID]
	}
}

func convertStyle(x *excelize.File, st *excelize.Style, def model.Style) model.Style {
	ms := model.Style{Font: def.Font}
	if f := st.Font; f != nil {
		if f.Family != "" {
			ms.Font.Family = f.Family
		}
		if f.Size > 0 {
			ms.Font.SizePt = float32(f.Size)
		}
		ms.Font.Bold = f.Bold
		ms.Font.Italic = f.Italic
		ms.Font.Strike = f.Strike
		ms.Font.Underline = f.Underline != "" && f.Underline != "none"
		base := x.GetBaseColor(f.Color, f.ColorIndexed, f.ColorTheme)
		ms.Font.Color = colors.HexOr(strings.TrimPrefix(excelize.ThemeColor(base, f.ColorTint), "FF"), def.Font.Color)
	}
	if fl := st.Fill; len(fl.Color) > 0 && fl.Color[0] != "" && (fl.Type == "gradient" || fl.Pattern > 0) {
		c := colors.HexOr(fl.Color[0], colors.White)
		if fl.Type == "pattern" && fl.Pattern > 1 {
			c = colors.Mix(c, colors.White, 0.5) // hatch patterns: approximate with a lighter solid
		}
		ms.Fill = c
	}
	for _, b := range st.Border {
		side := map[string]int{"left": model.Left, "top": model.Top, "right": model.Right, "bottom": model.Bottom}
		if i, ok := side[b.Type]; ok && b.Style > 0 {
			ms.Border[i] = model.Edge{Style: model.BorderStyle(b.Style), Color: colors.HexOr(b.Color, def.Font.Color)}
		}
	}
	if a := st.Alignment; a != nil {
		ms.HAlign = map[string]model.HAlign{
			"left": model.HAlignLeft, "center": model.HAlignCenter, "right": model.HAlignRight,
			"fill": model.HAlignFill, "justify": model.HAlignJustify,
			"centerContinuous": model.HAlignCenterContinuous, "distributed": model.HAlignDistributed,
		}[a.Horizontal]
		ms.VAlign = map[string]model.VAlign{
			"top": model.VAlignTop, "center": model.VAlignCenter,
			"justify": model.VAlignJustify, "distributed": model.VAlignDistributed,
		}[a.Vertical]
		ms.Wrap = a.WrapText
		ms.Indent = a.Indent
		ms.Rotation = a.TextRotation
	}
	return ms
}

// --- worksheets ---

type sharedFormula struct {
	row, col int
	text     string
}

func (r *reader) readSheet(name string, sh *model.Sheet) error {
	rc, err := r.open(name)
	if err != nil {
		return err
	}
	defer rc.Close()
	dec := xml.NewDecoder(rc)

	var (
		row, col   = -1, -1
		cellT      string
		cellS      int
		val, form  strings.Builder
		inV, inF   bool
		inIsT      bool
		inRPh      int
		fShared    string // si of the current cell's shared formula
		hasFormula bool
		fArray     string // ref of an array / dynamic-array formula
		shared     = map[string]sharedFormula{}
		dropped    int
	)
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sheetFormatPr":
				r.sheetFormat(t, sh)
			case "col":
				r.colInfo(t, sh)
			case "sheetView":
				if attr(t, "showGridLines") == "0" || attr(t, "showGridLines") == "false" {
					sh.ShowGridLines = false
				}
				if rr, cc, ok := parseCellRef(attr(t, "topLeftCell")); ok {
					sh.TopRow, sh.LeftCol = rr, cc
				}
			case "selection":
				if rr, cc, ok := parseCellRef(attr(t, "activeCell")); ok {
					sh.ActiveRow, sh.ActiveCol = rr, cc
				}
			case "pane":
				if st := attr(t, "state"); st == "frozen" || st == "frozenSplit" {
					sh.FreezeCols = int(parseFloat(attr(t, "xSplit")))
					sh.FreezeRows = int(parseFloat(attr(t, "ySplit")))
				}
			case "row":
				if n, err := strconv.Atoi(attr(t, "r")); err == nil {
					row = n - 1
				} else {
					row++
				}
				col = -1
				if row < model.MaxRows {
					if xmlBool(attr(t, "hidden")) {
						sh.RowHeight[row] = 0
					} else if ht := attr(t, "ht"); ht != "" {
						sh.RowHeight[row] = model.PointsToPx(parseFloat(ht))
					}
				}
			case "c":
				if rr, cc, ok := parseCellRef(attr(t, "r")); ok {
					row, col = rr, cc
				} else {
					col++
				}
				cellT = attr(t, "t")
				cellS, _ = strconv.Atoi(attr(t, "s"))
				val.Reset()
				form.Reset()
				hasFormula, fShared, fArray = false, "", ""
			case "v":
				inV = true
			case "f":
				inF, hasFormula = true, true
				switch attr(t, "t") {
				case "shared":
					fShared = attr(t, "si")
				case "array":
					// Legacy CSE arrays and dynamic arrays (which spill)
					// are both written as array formulas over their range.
					fArray = attr(t, "ref")
					if fArray == "" {
						fArray = model.CellName(row, col)
					}
				}
			case "t":
				inIsT = inRPh == 0
			case "rPh":
				inRPh++
			case "mergeCell":
				if m, ok := parseRange(attr(t, "ref")); ok {
					sh.Merges = append(sh.Merges, m)
				}
			}
		case xml.CharData:
			switch {
			case inV:
				val.Write(t)
			case inF:
				form.Write(t)
			case inIsT:
				val.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inV = false
			case "f":
				inF = false
			case "t":
				inIsT = false
			case "rPh":
				inRPh--
			case "c":
				f := form.String()
				if hasFormula && fShared != "" {
					if f != "" {
						shared[fShared] = sharedFormula{row, col, f}
					} else if m, ok := shared[fShared]; ok {
						f = formula.Shift(m.text, row-m.row, col-m.col)
					}
				}
				c := r.makeCell(cellT, cellS, val.String(), f)
				if c.Kind == model.KindEmpty && c.Style == 0 && c.Formula == "" {
					continue
				}
				if fArray != "" && f != "" {
					if rng, ok := parseRange(fArray); ok {
						c.Dynamic = true
						sh.Spills[[2]int{row, col}] = rng
					} else if rr, cc, ok := parseCellRef(fArray); ok {
						c.Dynamic = true
						sh.Spills[[2]int{row, col}] = model.Range{R1: rr, C1: cc, R2: rr, C2: cc}
					}
				}
				if !sh.Set(row, col, c) && c.Kind != model.KindEmpty {
					dropped++
				}
			}
		}
	}
	r.wb.WarnDropped(sh.Name, dropped)
	return nil
}

func (r *reader) makeCell(t string, s int, v, f string) model.Cell {
	var cf cellFormat
	if s >= 0 && s < len(r.formats) {
		cf = r.formats[s]
	}
	c := model.Cell{Style: cf.style, Formula: f}
	switch t {
	case "s":
		if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(r.sst) {
			c.Text, c.Kind = r.sst[i], model.KindText
		}
	case "str", "inlineStr":
		c.Text, c.Kind = unescapeOOXML(v), model.KindText
	case "b":
		c.Kind = model.KindBool
		if v == "1" || v == "true" {
			c.Text, c.Num = "TRUE", 1
		} else {
			c.Text = "FALSE"
		}
	case "e":
		c.Text, c.Kind = v, model.KindError
	case "d":
		c.Text, c.Kind = v, model.KindText // ISO date; rare outside strict files
	default:
		if v == "" {
			break
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			c.Text, c.Kind = v, model.KindText
			break
		}
		c.Num, c.Kind = n, model.KindNumber
		c.Text = r.nf.Format(n, cf.fmtID, cf.fmtCode)
	}
	return c
}

func (r *reader) sheetFormat(t xml.StartElement, sh *model.Sheet) {
	if v := attr(t, "defaultRowHeight"); v != "" {
		sh.DefaultRowHeight = model.PointsToPx(parseFloat(v))
	}
	if v := attr(t, "defaultColWidth"); v != "" {
		sh.DefaultColWidth = float32(parseFloat(v))
	}
	if v := attr(t, "baseColWidth"); v != "" {
		sh.BaseColWidth = float32(parseFloat(v))
	}
}

func (r *reader) colInfo(t xml.StartElement, sh *model.Sheet) {
	lo, _ := strconv.Atoi(attr(t, "min"))
	hi, _ := strconv.Atoi(attr(t, "max"))
	if lo < 1 || hi < lo {
		return
	}
	hi = min(hi, model.MaxCols)
	var w float32 = -1
	if xmlBool(attr(t, "hidden")) {
		w = 0
	} else if v := attr(t, "width"); v != "" {
		w = float32(parseFloat(v))
	}
	if w < 0 {
		return
	}
	for c := lo - 1; c < hi; c++ {
		sh.ColWidth[c] = w
	}
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseCellRef parses "AB12" into zero-based row, col.
func parseCellRef(s string) (row, col int, ok bool) {
	i := 0
	for i < len(s) && (s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z') {
		i++
	}
	if i == 0 || i == len(s) {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil || n < 1 {
		return 0, 0, false
	}
	return n - 1, model.ParseColName(s[:i]), true
}

func parseRange(s string) (model.Range, bool) {
	a, b, _ := strings.Cut(s, ":")
	r1, c1, ok1 := parseCellRef(a)
	r2, c2, ok2 := parseCellRef(b)
	if !ok1 || !ok2 {
		return model.Range{}, false
	}
	return model.Range{R1: r1, C1: c1, R2: r2, C2: c2}, true
}
