package fileio

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/excel/model"
)

func TestFormatFor(t *testing.T) {
	for name, want := range map[string]Format{"a.xlsx": XLSX, "B.XLSM": XLSX, "c.csv": CSV, "d.tsv": TSV, "e.txt": TSV, "f.xls": XLS} {
		if got, ok := FormatFor(name); !ok || got != want {
			t.Errorf("FormatFor(%q) = %v, %v", name, got, ok)
		}
	}
	for _, name := range []string{"b", "c.ods"} {
		if _, ok := FormatFor(name); ok {
			t.Errorf("FormatFor(%q) accepted", name)
		}
	}
	if XLSX.Ext() != ".xlsx" || CSV.Ext() != ".csv" || TSV.Ext() != ".txt" || XLS.Ext() != ".xls" {
		t.Error("Ext")
	}
	if CSV.String() != "CSV (Comma delimited) (*.csv)" {
		t.Errorf("CSV.String() = %q", CSV.String())
	}
	if XLS.String() != "Excel 97-2003 Workbook (*.xls)" || !XLS.MultiSheet() || CSV.MultiSheet() {
		t.Errorf("XLS = %q multi %v", XLS.String(), XLS.MultiSheet())
	}
}

// roundTrip saves wb as .xlsx and reads it back.
func roundTrip(t *testing.T, wb *model.Workbook) *model.Workbook {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(wb, &buf, XLSX, 0); err != nil {
		t.Fatal(err)
	}
	got, err := Read(wb.Name, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSaveSamples(t *testing.T) {
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
			compare(t, want, roundTrip(t, want))
		})
	}
}

// compare checks what a save must keep: sheets, cell text, kind, value and
// formula, cell fonts/fills/borders/alignment, widths, heights, merges,
// panes and names.
func compare(t *testing.T, want, got *model.Workbook) {
	t.Helper()
	if len(got.Sheets) != len(want.Sheets) {
		t.Fatalf("%d sheets, want %d", len(got.Sheets), len(want.Sheets))
	}
	if got.Styles[0].Font != want.Styles[0].Font {
		t.Errorf("default font = %+v, want %+v", got.Styles[0].Font, want.Styles[0].Font)
	}
	if got.Date1904 != want.Date1904 || len(got.Names) != len(want.Names) {
		t.Errorf("date1904 %v names %v, want %v %v", got.Date1904, got.Names, want.Date1904, want.Names)
	}
	for si, ws := range want.Sheets {
		gs := got.Sheets[si]
		if gs.Name != ws.Name || gs.Hidden != ws.Hidden {
			t.Errorf("sheet %d = %q hidden %v, want %q hidden %v", si, gs.Name, gs.Hidden, ws.Name, ws.Hidden)
		}
		errs := 0
		report := func(format string, args ...any) {
			if errs++; errs <= 10 {
				t.Errorf(ws.Name+"!"+format, args...)
			}
		}
		for r := 0; r < max(ws.UsedRows(), gs.UsedRows()); r++ {
			for c := 0; c < max(ws.RowLen(r), gs.RowLen(r)); c++ {
				w, g := ws.Get(r, c), gs.Get(r, c)
				ref := model.CellName(r, c)
				if w.Text != g.Text || w.Kind != g.Kind || w.Num != g.Num || w.Formula != g.Formula {
					report("%s = %q kind %d num %v f %q; want %q kind %d num %v f %q",
						ref, g.Text, g.Kind, g.Num, g.Formula, w.Text, w.Kind, w.Num, w.Formula)
				}
				ww, gg := want.Styles[w.Style], got.Styles[g.Style]
				if ww.Font != gg.Font || ww.Fill != gg.Fill || ww.Border != gg.Border || ww.HAlign != gg.HAlign ||
					ww.VAlign != gg.VAlign || ww.Wrap != gg.Wrap || ww.Indent != gg.Indent || ww.Rotation != gg.Rotation {
					report("%s style = %+v, want %+v", ref, gg, ww)
				}
			}
		}
		if len(gs.Merges) != len(ws.Merges) {
			report("merges = %v, want %v", gs.Merges, ws.Merges)
		}
		for i := range min(len(gs.Merges), len(ws.Merges)) {
			if gs.Merges[i] != ws.Merges[i] {
				report("merge %d = %v, want %v", i, gs.Merges[i], ws.Merges[i])
			}
		}
		if len(gs.ColWidth) != len(ws.ColWidth) {
			report("%d column widths, want %d", len(gs.ColWidth), len(ws.ColWidth))
		}
		for c, w := range ws.ColWidth {
			if g := gs.ColWidth[c]; g != w {
				report("col %s width = %v, want %v", model.ColName(c), g, w)
			}
		}
		for r := 0; r < ws.UsedRows()+5; r++ {
			if g, w := gs.RowHeightPx(r), ws.RowHeightPx(r); g != w {
				report("row %d height = %v, want %v", r+1, g, w)
			}
		}
		if gs.FreezeRows != ws.FreezeRows || gs.FreezeCols != ws.FreezeCols || gs.ShowGridLines != ws.ShowGridLines ||
			gs.ActiveRow != ws.ActiveRow || gs.ActiveCol != ws.ActiveCol || gs.DefaultColWidth != ws.DefaultColWidth {
			report("view: freeze %d,%d grid %v active %d,%d defw %v; want %d,%d %v %d,%d %v",
				gs.FreezeRows, gs.FreezeCols, gs.ShowGridLines, gs.ActiveRow, gs.ActiveCol, gs.DefaultColWidth,
				ws.FreezeRows, ws.FreezeCols, ws.ShowGridLines, ws.ActiveRow, ws.ActiveCol, ws.DefaultColWidth)
		}
		if len(gs.Spills) != len(ws.Spills) {
			report("spills = %v, want %v", gs.Spills, ws.Spills)
		}
	}
}

func TestSaveFile(t *testing.T) {
	dir := t.TempDir()
	wb, err := Open("xlsfile/testdata/sample.xls")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"out.xlsx", "out.csv", "out.txt", "out.xls"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, _ := FormatFor(name)
		if err := SaveFile(wb, path, f, 0); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(path)
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, %v", name, fi.Mode(), err)
		}
		got, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if a, b := got.Sheets[0].Get(0, 0).Text, wb.Sheets[0].Get(0, 0).Text; a != b {
			t.Errorf("%s: A1 = %q, want %q", name, a, b)
		}
		if a, b := got.Sheets[0].Get(4, 0).Text, "Ünïcödé ✓"; a != b {
			t.Errorf("%s: A5 = %q, want %q", name, a, b)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".*")); len(m) != 0 {
		t.Errorf("temp files left: %v", m)
	}
	if err := SaveFile(wb, filepath.Join(dir, "x.csv"), CSV, 5); err == nil {
		t.Error("bad sheet index accepted")
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".*")); len(m) != 0 {
		t.Errorf("temp files left after a failed save: %v", m)
	}
}
