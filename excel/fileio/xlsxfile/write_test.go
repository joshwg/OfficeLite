package xlsxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"officelite/common/colors"
	"officelite/excel/formula"
	"officelite/excel/model"
)

func buildModel() *model.Workbook {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}, Date1904: true}
	cache := map[model.Style]uint32{}
	bold := model.DefaultStyle()
	bold.Font = model.Font{Family: "Arial", SizePt: 14, Bold: true, Italic: true, Underline: true, Strike: true, Color: colors.RGB(0xFF0000)}
	bold.HAlign, bold.VAlign, bold.Wrap, bold.Indent, bold.Rotation = model.HAlignCenter, model.VAlignTop, true, 2, 45
	boxed := model.DefaultStyle()
	boxed.Fill = colors.RGB(0xFFFF00)
	boxed.Border[model.Bottom] = model.Edge{Style: model.BorderMedium, Color: colors.RGB(0x0000FF)}
	boxed.Border[model.Left] = model.Edge{Style: model.BorderThin, Color: colors.RGB(0x000000)}
	money := model.DefaultStyle()
	money.NumFmtID = 7
	pct := model.DefaultStyle()
	pct.NumFmtID = 10
	custom := model.DefaultStyle()
	custom.NumFmtID, custom.NumFmtCode = 164, "0.000"
	sBold, sBoxed := wb.AddStyle(bold, cache), wb.AddStyle(boxed, cache)
	sMoney, sPct, sCustom := wb.AddStyle(money, cache), wb.AddStyle(pct, cache), wb.AddStyle(custom, cache)

	sh := model.NewSheet("Data")
	sh.Set(0, 0, model.Cell{Text: "Title & <more>", Kind: model.KindText, Style: sBold})
	sh.Set(0, 1, model.Cell{Text: "  padded\nline2 _x0041_ \x01", Kind: model.KindText})
	sh.Set(1, 0, model.Cell{Num: 1234.5, Text: "$1,234.50 ", Kind: model.KindNumber, Style: sMoney})
	sh.Set(1, 1, model.Cell{Num: 0.25, Text: "25.00%", Kind: model.KindNumber, Style: sPct})
	sh.Set(1, 2, model.Cell{Num: 45000, Text: "45000.000", Kind: model.KindNumber, Style: sCustom})
	sh.Set(1, 3, model.Cell{Num: 1.0 / 3, Text: "0.333333333", Kind: model.KindNumber})
	sh.Set(2, 0, model.Cell{Text: "TRUE", Num: 1, Kind: model.KindBool})
	sh.Set(2, 1, model.Cell{Text: "FALSE", Kind: model.KindBool})
	sh.Set(2, 2, model.Cell{Text: "#N/A", Kind: model.KindError})
	sh.Set(2, 3, model.Cell{Style: sBoxed})
	// Formulas with cached values of each kind.
	sh.Set(3, 0, model.Cell{Formula: "A2*2", Num: 2469, Text: "2469", Kind: model.KindNumber})
	sh.Set(3, 1, model.Cell{Formula: `"a"&"b"`, Text: "ab", Kind: model.KindText})
	sh.Set(3, 2, model.Cell{Formula: "1>0", Text: "TRUE", Num: 1, Kind: model.KindBool})
	sh.Set(3, 3, model.Cell{Formula: "1/0", Text: "#DIV/0!", Kind: model.KindError})
	sh.Set(3, 4, model.Cell{Formula: "NOW()"}) // not calculated yet
	// A dynamic array spilling into A6:A8, and a single-cell one.
	sh.Set(5, 0, model.Cell{Formula: "_xlfn.SEQUENCE(3)", Num: 1, Text: "1", Kind: model.KindNumber, Dynamic: true})
	sh.Set(6, 0, model.Cell{Num: 2, Text: "2", Kind: model.KindNumber})
	sh.Set(7, 0, model.Cell{Num: 3, Text: "3", Kind: model.KindNumber})
	sh.Spills[[2]int{5, 0}] = model.Range{R1: 5, C1: 0, R2: 7, C2: 0}
	sh.Set(5, 2, model.Cell{Formula: "SUM(A6#)", Num: 6, Text: "6", Kind: model.KindNumber, Dynamic: true})
	sh.Set(5, 3, model.Cell{Formula: "XLOOKUP(1,A:A,B:B)", Num: 1, Text: "1", Kind: model.KindNumber})
	sh.Set(9, 0, model.Cell{Text: "merged", Kind: model.KindText})
	sh.Merges = []model.Range{{R1: 9, C1: 0, R2: 10, C2: 2}}
	sh.ColWidth[0] = 20
	sh.ColWidth[1] = 12.5
	sh.ColWidth[2] = 12.5
	sh.ColWidth[4] = 0
	sh.RowHeight[7] = 40
	sh.RowHeight[8] = 0
	sh.RowHeight[20] = 30 // a row with only a height
	sh.FreezeRows, sh.FreezeCols = 1, 1
	sh.ActiveRow, sh.ActiveCol = 3, 2
	sh.ShowGridLines = false
	sh.DefaultColWidth = 10

	second := model.NewSheet("Second")
	second.Set(0, 0, model.Cell{Formula: "Data!A2", Num: 1234.5, Text: "1234.5", Kind: model.KindNumber})
	second.ActiveRow, second.ActiveCol = 4, 1
	hidden := model.NewSheet("Hidden")
	hidden.Hidden = true
	hidden.Set(0, 0, model.Cell{Text: "secret", Kind: model.KindText})

	wb.Sheets = []*model.Sheet{sh, second, hidden}
	wb.Names = []model.DefinedName{
		{Name: "TaxRate", Sheet: -1, Formula: "Data!$B$2"},
		{Name: "Local", Sheet: 1, Formula: "Second!$A$1"},
	}
	return wb
}

func roundTrip(t *testing.T, wb *model.Workbook) (*model.Workbook, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	// excelize itself must accept the package.
	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("excelize cannot open the written file: %v", err)
	}
	f.Close()
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return got, buf.Bytes()
}

func TestWriteRoundTrip(t *testing.T) {
	want := buildModel()
	got, data := roundTrip(t, want)

	if len(got.Sheets) != 3 || got.Sheets[0].Name != "Data" || got.Sheets[1].Name != "Second" ||
		got.Sheets[2].Name != "Hidden" || !got.Sheets[2].Hidden || got.Sheets[0].Hidden {
		t.Fatalf("sheets = %v", got.Sheets)
	}
	if !got.Date1904 {
		t.Error("Date1904 lost")
	}
	for si, ws := range want.Sheets {
		gs := got.Sheets[si]
		for r := 0; r < ws.UsedRows(); r++ {
			for c := 0; c < ws.RowLen(r); c++ {
				w, g := ws.Get(r, c), gs.Get(r, c)
				// The file carries the prefixes Excel needs (_xlfn.XLOOKUP).
				if w.Kind != g.Kind || w.Num != g.Num || formula.AddFuturePrefixes(w.Formula) != g.Formula || w.Dynamic != g.Dynamic {
					t.Errorf("%s!%s = %+v, want %+v", ws.Name, model.CellName(r, c), *g, *w)
				}
				if w.Kind != model.KindNumber && w.Text != g.Text {
					t.Errorf("%s!%s text = %q, want %q", ws.Name, model.CellName(r, c), g.Text, w.Text)
				}
				if w.Kind == model.KindNumber && w.Formula == "" && w.Text != g.Text {
					t.Errorf("%s!%s formatted = %q, want %q", ws.Name, model.CellName(r, c), g.Text, w.Text)
				}
				ww, gg := want.Styles[w.Style], got.Styles[g.Style]
				if ww.NumFmtCode == "" && gg.NumFmtCode != "" {
					gg.NumFmtID, gg.NumFmtCode = ww.NumFmtID, "" // locale currency written as a code
				} else if ww.NumFmtCode != "" {
					gg.NumFmtID = ww.NumFmtID // custom ids are renumbered
				}
				if ww != gg {
					t.Errorf("%s!%s style = %+v, want %+v", ws.Name, model.CellName(r, c), gg, ww)
				}
			}
		}
	}

	sh := got.Sheets[0]
	if len(sh.Spills) != 2 || sh.Spills[[2]int{5, 0}] != (model.Range{R1: 5, C1: 0, R2: 7, C2: 0}) {
		t.Errorf("spills = %v", sh.Spills)
	}
	if len(sh.Merges) != 1 || sh.Merges[0] != (model.Range{R1: 9, C1: 0, R2: 10, C2: 2}) {
		t.Errorf("merges = %v", sh.Merges)
	}
	for c, w := range map[int]float32{0: 20, 1: 12.5, 2: 12.5, 4: 0} {
		if got := sh.ColWidth[c]; got != w {
			t.Errorf("col %s width = %v, want %v", model.ColName(c), got, w)
		}
	}
	if _, ok := sh.ColWidth[3]; ok {
		t.Errorf("col D has a width")
	}
	for r, h := range map[int]float32{7: 40, 8: 0, 20: 30, 0: 20} {
		if got := sh.RowHeightPx(r); got != h {
			t.Errorf("row %d height = %v, want %v", r+1, got, h)
		}
	}
	if sh.FreezeRows != 1 || sh.FreezeCols != 1 || sh.ShowGridLines || sh.ActiveRow != 3 || sh.ActiveCol != 2 {
		t.Errorf("view: freeze %d,%d grid %v active %d,%d", sh.FreezeRows, sh.FreezeCols, sh.ShowGridLines, sh.ActiveRow, sh.ActiveCol)
	}
	if sh.DefaultColWidth != 10 {
		t.Errorf("default col width = %v", sh.DefaultColWidth)
	}
	if s := got.Sheets[1]; s.ActiveRow != 4 || s.ActiveCol != 1 || !s.ShowGridLines {
		t.Errorf("second sheet view: active %d,%d grid %v", s.ActiveRow, s.ActiveCol, s.ShowGridLines)
	}
	if len(got.Names) != 2 || got.Names[0] != want.Names[0] || got.Names[1] != want.Names[1] {
		t.Errorf("names = %v", got.Names)
	}

	// The dynamic-array plumbing Excel needs to spill instead of showing "@".
	parts := zipParts(t, data)
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], `<f>_xlfn.XLOOKUP(1,A:A,B:B)</f>`) {
		t.Error("XLOOKUP written without its _xlfn. prefix")
	}
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], `cm="1"><f t="array" ref="A6:A8">_xlfn.SEQUENCE(3)</f><v>1</v></c>`) {
		t.Error("dynamic array anchor not written")
	}
	if !strings.Contains(parts["[Content_Types].xml"], "/xl/metadata.xml") ||
		!strings.Contains(parts["xl/_rels/workbook.xml.rels"], `Target="metadata.xml"`) ||
		!strings.Contains(parts["xl/metadata.xml"], "XLDAPR") {
		t.Error("dynamic array metadata part missing")
	}
}

func zipParts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

// A workbook whose default font is not Calibri 11 keeps it as the Normal
// style, which sets the column-width unit.
func TestWriteNormalFont(t *testing.T) {
	wb := &model.Workbook{Styles: []model.Style{{Font: model.Font{Family: "Arial", SizePt: 10, Color: colors.RGB(0)}}}}
	sh := model.NewSheet("S")
	sh.Set(0, 0, model.Cell{Text: "x", Kind: model.KindText})
	wb.Sheets = []*model.Sheet{sh}
	got, _ := roundTrip(t, wb)
	if f := got.Styles[0].Font; f.Family != "Arial" || f.SizePt != 10 {
		t.Errorf("normal font = %+v", f)
	}
	if f := got.Styles[got.Sheets[0].Get(0, 0).Style].Font; f.Family != "Arial" || f.SizePt != 10 {
		t.Errorf("A1 font = %+v", f)
	}
}

func TestWriteEmptyAndOddNames(t *testing.T) {
	wb := &model.Workbook{}
	long := model.NewSheet("a very long csv file name: with [bad] chars")
	wb.Sheets = []*model.Sheet{long, model.NewSheet("A VERY LONG CSV FILE NAME_ WITH _BAD_ CHARS")}
	got, _ := roundTrip(t, wb)
	if len(got.Sheets) != 2 || got.Sheets[0].Name != "a very long csv file name_ with" ||
		got.Sheets[1].Name != "A VERY LONG CSV FILE NAME_ (2)" {
		t.Errorf("names = %q, %q", got.Sheets[0].Name, got.Sheets[1].Name)
	}
	if _, err := Read(mustWrite(t, &model.Workbook{})); err != nil {
		t.Errorf("empty workbook: %v", err)
	}
}

func mustWrite(t *testing.T, wb *model.Workbook) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWriteReadSample(t *testing.T) {
	// The reader's own excelize-built sample survives a write.
	want, err := Read(buildSample(t))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := roundTrip(t, want)
	compareWorkbooks(t, want, got)
}

// compareWorkbooks checks what a save must keep: cell values and formulas,
// fonts, fills, borders, alignment, widths, heights, merges and panes.
func compareWorkbooks(t *testing.T, want, got *model.Workbook) {
	t.Helper()
	if len(got.Sheets) != len(want.Sheets) {
		t.Fatalf("%d sheets, want %d", len(got.Sheets), len(want.Sheets))
	}
	if got.Styles[0].Font != want.Styles[0].Font {
		t.Errorf("default font = %+v, want %+v", got.Styles[0].Font, want.Styles[0].Font)
	}
	for si, ws := range want.Sheets {
		gs := got.Sheets[si]
		if gs.Name != ws.Name || gs.Hidden != ws.Hidden {
			t.Errorf("sheet %d = %q hidden %v, want %q hidden %v", si, gs.Name, gs.Hidden, ws.Name, ws.Hidden)
		}
		errs := 0
		for r := 0; r < max(ws.UsedRows(), gs.UsedRows()) && errs < 10; r++ {
			for c := 0; c < max(ws.RowLen(r), gs.RowLen(r)) && errs < 10; c++ {
				w, g := ws.Get(r, c), gs.Get(r, c)
				where := ws.Name + "!" + model.CellName(r, c)
				if w.Text != g.Text || w.Kind != g.Kind || w.Num != g.Num || w.Formula != g.Formula {
					t.Errorf("%s = %q kind %d num %v f %q; want %q kind %d num %v f %q",
						where, g.Text, g.Kind, g.Num, g.Formula, w.Text, w.Kind, w.Num, w.Formula)
					errs++
				}
				ww, gg := want.Styles[w.Style], got.Styles[g.Style]
				if ww.Font != gg.Font || ww.Fill != gg.Fill || ww.Border != gg.Border || ww.HAlign != gg.HAlign ||
					ww.VAlign != gg.VAlign || ww.Wrap != gg.Wrap || ww.Indent != gg.Indent || ww.Rotation != gg.Rotation {
					t.Errorf("%s style = %+v, want %+v", where, gg, ww)
					errs++
				}
			}
		}
		if len(gs.Merges) != len(ws.Merges) {
			t.Errorf("%s merges = %v, want %v", ws.Name, gs.Merges, ws.Merges)
		}
		for i := range min(len(gs.Merges), len(ws.Merges)) {
			if gs.Merges[i] != ws.Merges[i] {
				t.Errorf("%s merge %d = %v, want %v", ws.Name, i, gs.Merges[i], ws.Merges[i])
			}
		}
		for c, w := range ws.ColWidth {
			if g := gs.ColWidth[c]; g != w {
				t.Errorf("%s col %s width = %v, want %v", ws.Name, model.ColName(c), g, w)
			}
		}
		for r := 0; r < ws.UsedRows()+5; r++ {
			if g, w := gs.RowHeightPx(r), ws.RowHeightPx(r); g != w {
				t.Errorf("%s row %d height = %v, want %v", ws.Name, r+1, g, w)
			}
		}
		if gs.FreezeRows != ws.FreezeRows || gs.FreezeCols != ws.FreezeCols {
			t.Errorf("%s freeze = %d,%d, want %d,%d", ws.Name, gs.FreezeRows, gs.FreezeCols, ws.FreezeRows, ws.FreezeCols)
		}
	}
}

func TestPatchSheet(t *testing.T) {
	src := `<sheetFormatPr/><sheetData><row r="1"><c r="A1"><v>1</v></c><c r="A10" s="2"></c><c r="B1" s="3"></c></row></sheetData>`
	var out bytes.Buffer
	if err := patchSheet(&out, strings.NewReader(src), &sheetPatch{cols: "<cols/>", cells: []rawCell{{"B1", "<X/>"}}}); err != nil {
		t.Fatal(err)
	}
	if want := `<sheetFormatPr/><cols/><sheetData><row r="1"><c r="A1"><v>1</v></c><c r="A10" s="2"></c><X/></row></sheetData>`; out.String() != want {
		t.Errorf("patched = %s", out.String())
	}
	if err := patchSheet(io.Discard, strings.NewReader(src), &sheetPatch{cells: []rawCell{{"C1", "<X/>"}}}); err == nil {
		t.Error("missing cell not reported")
	}
}

// Excel refuses files whose <v> holds hundreds of digits, so very large
// and very small numbers must be written with an exponent.
func TestWriteExtremeNumbers(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	vals := []float64{1e300, -1e300, 1e-300, 1.5e21, 123.25}
	for i, v := range vals {
		sh.Set(i, 0, model.Cell{Kind: model.KindNumber, Num: v})
	}
	wb := &model.Workbook{Name: "x.xlsx", Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, _ := f.Open()
		xml, _ := io.ReadAll(rc)
		rc.Close()
		if strings.Contains(string(xml), "0000000000000000000000") {
			t.Errorf("number written without exponent: %s", xml)
		}
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range vals {
		if c := got.Sheets[0].Get(i, 0); c.Num != v {
			t.Errorf("row %d = %v, want %v", i, c.Num, v)
		}
	}
}
