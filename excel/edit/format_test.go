package edit

import (
	"path/filepath"
	"slices"
	"testing"

	"officelite/excel/fileio"
	"officelite/excel/model"
)

func (b *Book) styleOf(ref string) model.Style { return b.style(b.cell(0, ref).Style) }

func TestFormatAndUndo(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "1234.5")
	b.enter(t, 0, "A2", "=A1*2")
	b.Format(0, rng("A1", "B2"), func(st *model.Style, _, _ int) { st.Font.Bold = true })
	for _, ref := range []string{"A1", "A2", "B1", "B2"} {
		if !b.styleOf(ref).Font.Bold {
			t.Errorf("%s not bold", ref)
		}
	}
	if b.cell(0, "A1").Text != "1234.5" || b.cell(0, "A2").Formula != "A1*2" {
		t.Errorf("content changed: %+v %+v", b.cell(0, "A1"), b.cell(0, "A2"))
	}
	// A number format redoes the displayed text, formula results too.
	b.Format(0, rng("A1", "A2"), func(st *model.Style, _, _ int) { st.NumFmtID, st.NumFmtCode = 4, "" })
	b.expect(t, "#,##0.00", map[string]string{"A1": "1,234.50", "A2": "2,469.00"})
	b.Undo()
	b.expect(t, "undo format", map[string]string{"A1": "1234.5", "A2": "2469"})
	b.Undo()
	if b.styleOf("A1").Font.Bold || b.styleOf("B2").Font.Bold {
		t.Error("bold not undone")
	}
	b.Redo()
	if !b.styleOf("B2").Font.Bold {
		t.Error("bold not redone")
	}

	// Whole columns format only the used rows.
	b.Format(0, model.Range{R1: 0, C1: 3, R2: model.MaxRows - 1, C2: 3}, func(st *model.Style, _, _ int) { st.Font.Italic = true })
	if n := b.WB.Sheets[0].UsedRows(); n != 2 {
		t.Errorf("used rows after formatting column D = %d", n)
	}
	if !b.styleOf("D2").Font.Italic {
		t.Error("D2 not italic")
	}
}

func TestBordersAndMerge(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "x")
	b.enter(t, 0, "B2", "y")
	b.Borders(0, rng("A1", "B2"), BordersOutside)
	if a, c := b.styleOf("A1").Border, b.styleOf("B2").Border; a[model.Top].Style != model.BorderThin ||
		a[model.Left].Style != model.BorderThin || a[model.Right].Style != 0 || c[model.Bottom].Style != model.BorderThin {
		t.Errorf("outside borders: A1 %+v B2 %+v", a, c)
	}
	b.Borders(0, rng("A1", "B2"), BordersNone)
	if a := b.styleOf("A1").Border; a != [4]model.Edge{} {
		t.Errorf("no border: %+v", a)
	}

	sh := b.WB.Sheets[0]
	_, lost := b.Merge(0, rng("A1", "B2"), MergeCenter)
	if !lost || len(sh.Merges) != 1 || sh.Merges[0] != rng("A1", "B2") {
		t.Fatalf("merge: lost %v merges %v", lost, sh.Merges)
	}
	if b.cell(0, "B2").Kind != model.KindEmpty || b.styleOf("A1").HAlign != model.HAlignCenter {
		t.Errorf("merged: B2 %+v A1 align %v", b.cell(0, "B2"), b.styleOf("A1").HAlign)
	}
	b.Merge(0, rng("A1", "A1"), Unmerge)
	if len(sh.Merges) != 0 {
		t.Errorf("unmerge: %v", sh.Merges)
	}
	b.Undo()
	b.Undo()
	if len(sh.Merges) != 0 || b.cell(0, "B2").Text != "y" {
		t.Errorf("undo merge: %v, B2 %q", sh.Merges, b.cell(0, "B2").Text)
	}
	b.Merge(0, rng("A1", "C2"), MergeAcross)
	if !slices.Equal(sh.Merges, []model.Range{rng("A1", "C1"), rng("A2", "C2")}) {
		t.Errorf("merge across: %v", sh.Merges)
	}
	b.ClearFormats(0, rng("A1", "C2"), false)
	if len(sh.Merges) != 0 || b.cell(0, "A1").Style != 0 || b.cell(0, "A1").Text != "x" {
		t.Errorf("clear formats: %v %+v", sh.Merges, b.cell(0, "A1"))
	}
}

func TestStepDecimals(t *testing.T) {
	for _, tc := range []struct {
		id         int
		code, text string
		delta      int
		want       string
	}{
		{0, "", "1.5", 1, "0.00"},
		{0, "", "12", 1, "0.0"},
		{0, "", "1.5", -1, "0"},
		{2, "", "", 1, "0.000"},
		{2, "", "", -1, "0.0"},
		{0, "0.0", "", -1, "0"},
		{9, "", "", 1, "0.0%"},
		{3, "", "", 1, "#,##0.0"},
		{44, "", "", -1, `_("$"* #,##0.0_);_("$"* \(#,##0.0\);_("$"* "-"??_);_(@_)`},
		{11, "", "", 1, "0.000E+00"},
	} {
		got, ok := StepDecimals(tc.id, tc.code, tc.text, tc.delta)
		if !ok || got != tc.want {
			t.Errorf("StepDecimals(%d, %q, %q, %d) = %q, %v; want %q", tc.id, tc.code, tc.text, tc.delta, got, ok, tc.want)
		}
	}
	if _, ok := StepDecimals(14, "", "", 1); ok {
		t.Error("date format got decimals")
	}
}

func TestPasteSpecial(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "2")
	b.enter(t, 0, "A2", "=A1*3")
	b.Format(0, rng("A1", "A2"), func(st *model.Style, _, _ int) { st.Font.Bold = true })
	clip := b.Copy(0, rng("A1", "A2"), false)
	b.PasteSpecial(clip, 0, rng("B1", "B1"), PasteValues)
	if c := b.cell(0, "B2"); c.Formula != "" || c.Text != "6" || b.styleOf("B2").Font.Bold {
		t.Errorf("paste values: %+v", c)
	}
	b.PasteSpecial(clip, 0, rng("C1", "C1"), PasteFormulas)
	if c := b.cell(0, "C2"); c.Formula != "C1*3" || b.styleOf("C2").Font.Bold {
		t.Errorf("paste formulas: %+v", c)
	}
	b.PasteSpecial(b.CopyFormats(0, rng("A1", "A1")), 0, rng("D1", "D3"), PasteFormats)
	if !b.styleOf("D3").Font.Bold || b.cell(0, "D3").Kind != model.KindEmpty {
		t.Errorf("paste formats: %+v", b.cell(0, "D3"))
	}
}

// TestFormatsSave checks that formats made by the commands survive a
// save and reload in both Excel formats.
func TestFormatsSave(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "1234.5")
	b.enter(t, 0, "B1", "title")
	b.Format(0, rng("A1", "B1"), func(st *model.Style, _, _ int) {
		st.Font.Bold, st.Font.Italic, st.Font.Underline, st.Font.Strike = true, true, true, true
		st.Font.Family, st.Font.SizePt = "Arial", 14
		st.Font.Color = hex(0xFF0000)
		st.Fill = hex(0xFFFF00)
		st.HAlign, st.VAlign, st.Wrap, st.Indent = model.HAlignRight, model.VAlignTop, true, 2
	})
	b.Format(0, rng("A1", "A1"), func(st *model.Style, _, _ int) { st.NumFmtID, st.NumFmtCode = 44, "" })
	b.Borders(0, rng("A1", "B1"), BordersThickOutside)
	b.Merge(0, rng("B1", "C1"), MergeCenter)
	b.Format(0, rng("A2", "A2"), func(st *model.Style, _, _ int) { st.NumFmtCode = "0.000" })
	b.enter(t, 0, "A2", "1.5")
	want := map[string]model.Style{}
	for _, ref := range []string{"A1", "B1", "C1", "A2"} {
		want[ref] = b.styleOf(ref)
	}
	for _, f := range []fileio.Format{fileio.XLSX, fileio.XLS} {
		path := filepath.Join(t.TempDir(), "f"+f.Ext())
		if err := fileio.SaveFile(b.WB, path, f, 0); err != nil {
			t.Fatal(err)
		}
		wb, err := fileio.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		sh := wb.Sheets[0]
		for ref, w := range want {
			r, c := pos(ref)
			got := wb.Styles[sh.Get(r, c).Style]
			// Number formats may come back as a code or a built-in id.
			if FormatCode(got.NumFmtID, got.NumFmtCode) != FormatCode(w.NumFmtID, w.NumFmtCode) {
				t.Errorf("%v %s: number format %d %q, want %d %q", f, ref, got.NumFmtID, got.NumFmtCode, w.NumFmtID, w.NumFmtCode)
			}
			got.NumFmtID, got.NumFmtCode, w.NumFmtID, w.NumFmtCode = 0, "", 0, ""
			if got != w {
				t.Errorf("%v %s:\n got %+v\nwant %+v", f, ref, got, w)
			}
		}
		if !slices.Equal(sh.Merges, []model.Range{rng("B1", "C1")}) {
			t.Errorf("%v merges: %v", f, sh.Merges)
		}
		if got := sh.Get(0, 0).Text; got != b.cell(0, "A1").Text {
			t.Errorf("%v A1 text %q, want %q", f, got, b.cell(0, "A1").Text)
		}
	}
}

func TestGrowRows(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "x")
	b.Format(0, rng("A1", "A1"), func(st *model.Style, _, _ int) { st.Font.SizePt = 20 })
	b.GrowRows(0, []int{0}, []float32{34})
	sh := b.WB.Sheets[0]
	if sh.RowHeight[0] != 34 {
		t.Fatalf("height %v", sh.RowHeight)
	}
	b.Undo()
	if _, ok := sh.RowHeight[0]; ok || b.styleOf("A1").Font.SizePt != 11 {
		t.Errorf("undo: heights %v, size %v", sh.RowHeight, b.styleOf("A1").Font.SizePt)
	}
	b.Redo()
	if sh.RowHeight[0] != 34 || b.styleOf("A1").Font.SizePt != 20 {
		t.Errorf("redo: heights %v, size %v", sh.RowHeight, b.styleOf("A1").Font.SizePt)
	}
}
