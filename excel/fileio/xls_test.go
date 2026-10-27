package fileio

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/colors"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// xlsFormulas are formulas of the code-built workbook, written in column
// D of sheet Data from row 1, and compared after a save as .xls.
var xlsFormulas = []string{
	"SUM(A1:A3)",
	`IF(A1>1,"big","small")`,
	"VLOOKUP(2,A1:B3,2,FALSE)",
	"Other!B2*2",
	"SUM('My Sheet'!A1:B2)",
	"TaxRate*A2",
	"_xlfn.XLOOKUP(2,A1:A3,B1:B3)",
	`IFERROR(1/0,"oops")`,
	"EDATE(A1,1)",
	"SUM({1,2;3,4})",
	"-A1^2+A2%",
	`"a"&B1&"c"`,
	"A1<>B1",
	"(A1+A2)*2",
	"SUM( A1, A2) + 1",
	"SUM(A:A)-SUM(1:1)",
	"SUM($A$1:B$2)",
	"CHOOSE(2,A1,A2,A3)",
	"INDEX(A1:B3,2,2)",
	"ROUND(PI(),2)",
	"ISNA(#N/A)",
	"SUM(Data:Other!A1)",
	"COUNTIF(A1:A3,\">1\")",
	"SUMPRODUCT((A1:A3>1)*A1:A3)",
	"MAX(A1:A3)-MIN(A1:A3)+AVERAGE(A1:A3)",
	"Local*2",
	`TEXT(A1,"0.00")&" kg"`,
	"_xlfn.TEXTJOIN(\",\",TRUE,B1:B3)",
	"AND(A1>0,OR(A2>5,NOT(A3=3)))",
	"YEAR(DATE(2024,2,29))",
	`LEN(REPT("x",300))`,
	"_xlfn._xlws.FILTER(A1:A3,A1:A3>1)",
	"SUM(A1:A3*2)",
}

// buildXLSWorkbook makes a workbook with what a .xls file holds: values,
// formulas, styles, layout and names.
func buildXLSWorkbook() *model.Workbook {
	wb := &model.Workbook{Name: "built.xls", Styles: []model.Style{model.DefaultStyle()}}
	cache := map[model.Style]uint32{wb.Styles[0]: 0}
	style := func(f func(*model.Style)) uint32 {
		st := model.DefaultStyle()
		f(&st)
		return wb.AddStyle(st, cache)
	}
	bold := style(func(s *model.Style) {
		s.Font.Bold, s.Font.Color, s.Font.SizePt = true, colors.RGB(0xC00000), 14
		s.HAlign = model.HAlignCenter
	})
	boxed := style(func(s *model.Style) {
		s.Fill = colors.RGB(0x123456)
		s.Border[model.Left] = model.Edge{Style: model.BorderThin, Color: colors.Black}
		s.Border[model.Bottom] = model.Edge{Style: model.BorderDouble, Color: colors.RGB(0x00B050)}
		s.VAlign, s.Wrap, s.Indent, s.Rotation = model.VAlignTop, true, 2, 45
	})
	pct := style(func(s *model.Style) { s.NumFmtCode = "0.00%" })
	kg := style(func(s *model.Style) {
		s.NumFmtCode = `#,##0.000 "kg"`
		s.Font.Family, s.Font.Italic, s.Font.Underline, s.Font.Strike = "Arial", true, true, true
	})

	data := model.NewSheet("Data")
	other := model.NewSheet("Other")
	mine := model.NewSheet("My Sheet")
	hidden := model.NewSheet("Hidden")
	hidden.Hidden = true
	wb.Sheets = []*model.Sheet{data, other, mine, hidden}

	nf := numfmt.New(false)
	defer nf.Close()
	num := func(sh *model.Sheet, r, c int, v float64, st uint32) {
		s := wb.Styles[st]
		sh.Set(r, c, model.Cell{Kind: model.KindNumber, Num: v, Text: nf.Format(v, s.NumFmtID, s.NumFmtCode), Style: st})
	}
	text := func(sh *model.Sheet, r, c int, s string, st uint32) {
		sh.Set(r, c, model.Cell{Kind: model.KindText, Text: s, Style: st})
	}
	num(data, 0, 0, 1, 0)
	num(data, 1, 0, 2.5, pct)
	num(data, 2, 0, 3, kg)
	text(data, 0, 1, "one", bold)
	text(data, 1, 1, "two", boxed)
	text(data, 2, 1, "Ünïcödé ✓ 日本", 0)
	data.Set(3, 0, model.Cell{Kind: model.KindBool, Num: 1, Text: "TRUE"})
	data.Set(4, 0, model.Cell{Kind: model.KindError, Text: "#DIV/0!"})
	data.Set(5, 0, model.Cell{Style: boxed}) // styled blank
	text(data, 6, 0, strings.Repeat("long ", 3000), 0)
	num(data, 7, 0, 1234567.891, 0)
	num(data, 8, 0, -0.01, 0)
	num(data, 9, 0, 1e300, 0)
	for i, f := range xlsFormulas {
		data.Set(i, 3, model.Cell{Formula: f, Kind: model.KindNumber, Num: float64(i), Text: fmt.Sprint(i)})
	}
	data.Set(len(xlsFormulas), 3, model.Cell{Formula: `REPT("ab",5000)`, Kind: model.KindText, Text: strings.Repeat("ab", 5000)})
	data.Set(len(xlsFormulas)+1, 3, model.Cell{Formula: `1>2`, Kind: model.KindBool, Text: "FALSE"})
	data.Set(len(xlsFormulas)+2, 3, model.Cell{Formula: `1/0`, Kind: model.KindError, Text: "#DIV/0!"})
	for r := 100; r < 400; r++ { // many shared strings
		text(data, r, 0, fmt.Sprintf("row %d", r), 0)
	}
	data.ColWidth[0] = 20
	data.ColWidth[1] = 0 // hidden
	data.ColWidth[3] = 12.5
	data.RowHeight[0] = 40
	data.RowHeight[2] = 0 // hidden
	data.Merges = []model.Range{{R1: 10, C1: 0, R2: 11, C2: 2}}
	data.FreezeRows, data.FreezeCols = 1, 1
	data.ActiveRow, data.ActiveCol = 4, 2

	num(other, 1, 1, 21, 0)
	num(other, 0, 0, 5, 0)
	other.ShowGridLines = false
	other.DefaultRowHeight = 25
	num(mine, 0, 0, 1, 0)
	num(mine, 1, 1, 2, 0)
	text(hidden, 0, 0, "secret", 0)

	wb.Names = []model.DefinedName{
		{Name: "TaxRate", Sheet: -1, Formula: "Other!$B$2"},
		{Name: "Local", Sheet: 0, Formula: "Data!$A$1:$A$3"},
		{Name: "Both", Sheet: -1, Formula: "'My Sheet'!$A$1,Other!$A$1"},
	}
	return wb
}

// writeXLS saves wb as .xls and reads it back.
func writeXLS(t *testing.T, wb *model.Workbook) *model.Workbook {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(wb, &buf, XLS, 0); err != nil {
		t.Fatal(err)
	}
	got, err := Read(wb.Name, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestXLSRoundTrip(t *testing.T) {
	wb := buildXLSWorkbook()
	if fits := Fits(wb, XLS); len(fits) != 0 {
		t.Errorf("Fits = %q", fits)
	}
	got := writeXLS(t, wb)
	if len(got.Warnings) != 0 {
		t.Errorf("warnings: %q", got.Warnings)
	}
	compare(t, wb, got)
	for i, n := range wb.Names {
		if i >= len(got.Names) || got.Names[i] != n {
			t.Errorf("names = %+v, want %+v", got.Names, wb.Names)
			break
		}
	}
}

func TestXLSSamples(t *testing.T) {
	paths := []string{"xlsfile/testdata/sample.xls"}
	if m, _ := filepath.Glob("../../Samples/*.xls*"); len(m) > 0 {
		paths = append(paths, m...)
	}
	for _, p := range paths {
		if strings.Contains(p, ":") { // Windows Zone.Identifier streams
			continue
		}
		t.Run(filepath.Base(p), func(t *testing.T) {
			want, err := Open(p)
			if err != nil {
				t.Fatal(err)
			}
			if fits := Fits(want, XLS); len(fits) != 0 {
				t.Errorf("Fits = %q", fits)
			}
			compare(t, want, writeXLS(t, want))
		})
	}
}

func TestXLSFits(t *testing.T) {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	sh := model.NewSheet("A very long sheet name that Excel cuts")
	wb.Sheets = []*model.Sheet{sh}
	sh.Set(70000, 0, model.Cell{Kind: model.KindNumber, Num: 1, Text: "1"})
	sh.Set(0, 300, model.Cell{Kind: model.KindNumber, Num: 1, Text: "1"})
	sh.Set(0, 0, model.Cell{Formula: "SUM(A1:A70000)", Kind: model.KindNumber, Num: 3, Text: "3"})
	sh.Set(1, 0, model.Cell{Formula: "_xlfn.SEQUENCE(3)", Kind: model.KindNumber, Num: 1, Text: "1", Dynamic: true})
	sh.Spills[[2]int{1, 0}] = model.Range{R1: 1, C1: 0, R2: 3, C2: 0}
	sh.Set(2, 0, model.Cell{Formula: "1+1", Kind: model.KindNumber, Num: 2, Text: "2", Dynamic: true}) // typed, no spill
	sh.Set(4, 0, model.Cell{Formula: "_xlfn.LET(_xlpm.x,2,_xlpm.x*3)", Kind: model.KindNumber, Num: 6, Text: "6"})
	fits := Fits(wb, XLS)
	want := []string{"will be named", "cells beyond row 65,536", "A1 (=SUM(A1:A70000)", "A5 (=_xlfn.LET", ": 1 dynamic-array"}
	if len(fits) != len(want)-1 { // A1 and A5 are one item
		t.Errorf("Fits = %q", fits)
	}
	for _, w := range want {
		found := false
		for _, f := range fits {
			found = found || strings.Contains(f, w)
		}
		if !found {
			t.Errorf("Fits = %q, missing %q", fits, w)
		}
	}
	for _, f := range []Format{XLSX, CSV, TSV} {
		if fits := Fits(wb, f); len(fits) != 0 {
			t.Errorf("Fits(%v) = %q", f, fits)
		}
	}

	// What does not fit is left out; the rest is saved.
	got := writeXLS(t, wb)
	sh2 := got.Sheets[0]
	if sh2.UsedRows() > 65536 || sh2.UsedCols() > 256 {
		t.Errorf("saved %d rows × %d columns", sh2.UsedRows(), sh2.UsedCols())
	}
	if c := sh2.Get(0, 0); c.Formula != "" || c.Num != 3 {
		t.Errorf("A1 = %+v, want the value 3 without its formula", c)
	}
	if c := sh2.Get(1, 0); c.Formula != "_xlfn.SEQUENCE(3)" || c.Dynamic {
		t.Errorf("A2 = %+v, want an ordinary formula", c)
	}
}
