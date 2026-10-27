package edit

import (
	"reflect"
	"testing"
	"time"

	"officelite/excel/model"
)

func TestParseInput(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Input
	}{
		{"", Input{}},
		{"42", Input{Kind: model.KindNumber, Num: 42}},
		{" -3.5 ", Input{Kind: model.KindNumber, Num: -3.5}},
		{"1,234", Input{Kind: model.KindNumber, Num: 1234, Format: "#,##0"}},
		{"1,234.50", Input{Kind: model.KindNumber, Num: 1234.5, Format: "#,##0.00"}},
		{"(12)", Input{Kind: model.KindNumber, Num: -12}},
		{"12%", Input{Kind: model.KindNumber, Num: 0.12, Format: "0%"}},
		{"12.5%", Input{Kind: model.KindNumber, Num: 0.125, Format: "0.00%"}},
		{"$5", Input{Kind: model.KindNumber, Num: 5, Format: `"$"#,##0_);\("$"#,##0\)`}},
		{"-$1,000.25", Input{Kind: model.KindNumber, Num: -1000.25, Format: `"$"#,##0.00_);\("$"#,##0.00\)`}},
		{"($3)", Input{Kind: model.KindNumber, Num: -3, Format: `"$"#,##0_);\("$"#,##0\)`}},
		{"1E3", Input{Kind: model.KindNumber, Num: 1000, Format: "0.00E+00"}},
		{"1/15/2026", Input{Kind: model.KindNumber, Num: 46037, Format: "m/d/yyyy"}},
		{"2026-01-15", Input{Kind: model.KindNumber, Num: 46037, Format: "m/d/yyyy"}},
		{"15-Jan-2026", Input{Kind: model.KindNumber, Num: 46037, Format: "d-mmm-yy"}},
		{"Jan 2026", Input{Kind: model.KindNumber, Num: 46023, Format: "mmm-yy"}},
		{"3:45 PM", Input{Kind: model.KindNumber, Num: 0.65625, Format: "h:mm AM/PM"}},
		{"15:45", Input{Kind: model.KindNumber, Num: 0.65625, Format: "h:mm"}},
		{"15:45:30", Input{Kind: model.KindNumber, Num: (15*3600 + 45*60 + 30) / 86400.0, Format: "h:mm:ss"}},
		{"1/15/2026 3:45 PM", Input{Kind: model.KindNumber, Num: 46037.65625, Format: "m/d/yyyy h:mm"}},
		{"true", Input{Kind: model.KindBool, Num: 1, Text: "TRUE"}},
		{"FALSE", Input{Kind: model.KindBool, Text: "FALSE"}},
		{"#n/a", Input{Kind: model.KindError, Text: "#N/A"}},
		{"'0042", Input{Kind: model.KindText, Text: "0042"}},
		{"'=A1", Input{Kind: model.KindText, Text: "=A1"}},
		{"hello world", Input{Kind: model.KindText, Text: "hello world"}},
		{"1,23", Input{Kind: model.KindText, Text: "1,23"}},
		{"=", Input{Kind: model.KindText, Text: "="}},
		{"=sum(a1:a3)", Input{Formula: "SUM(A1:A3)"}},
		{"+a1*2", Input{Formula: "+A1*2"}},
		{"-B2", Input{Formula: "-B2"}},
		{"- hello", Input{Kind: model.KindText, Text: "- hello"}},
	} {
		got, err := ParseInput(tc.in, false)
		if err != nil || got != tc.want {
			t.Errorf("ParseInput(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	if _, err := ParseInput("=1+", false); err == nil {
		t.Error("=1+: no error")
	}
}

var now = time.Date(2026, 10, 15, 13, 30, 0, 0, time.UTC)

func newBook() *Book {
	wb := &model.Workbook{
		Sheets: []*model.Sheet{model.NewSheet("Sheet1"), model.NewSheet("Data")},
		Styles: []model.Style{model.DefaultStyle()},
	}
	b := New(wb)
	b.Now = func() time.Time { return now }
	return b
}

// pos parses an A1 reference.
func pos(ref string) (row, col int) {
	i := 0
	for ref[i] >= 'A' {
		i++
	}
	n := 0
	for _, d := range ref[i:] {
		n = n*10 + int(d-'0')
	}
	return n - 1, model.ParseColName(ref[:i])
}

func rng(a, b string) model.Range {
	r1, c1 := pos(a)
	r2, c2 := pos(b)
	return model.Range{R1: r1, C1: c1, R2: r2, C2: c2}
}

func (b *Book) enter(t *testing.T, sheet int, ref, text string) {
	t.Helper()
	r, c := pos(ref)
	if _, err := b.Enter(sheet, r, c, text); err != nil {
		t.Fatalf("Enter %s %q: %v", ref, text, err)
	}
}

func (b *Book) cell(sheet int, ref string) *model.Cell {
	r, c := pos(ref)
	return b.WB.Sheets[sheet].Get(r, c)
}

// expect checks display texts of cells on sheet 0.
func (b *Book) expect(t *testing.T, what string, want map[string]string) {
	t.Helper()
	for ref, w := range want {
		if got := b.cell(0, ref).Text; got != w {
			t.Errorf("%s: %s = %q, want %q", what, ref, got, w)
		}
	}
}

func TestEnterAndUndo(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "2")
	b.enter(t, 0, "A2", "3")
	b.enter(t, 0, "B1", "=a1*a2")
	b.enter(t, 0, "C1", "=sequence(2)*B1")
	b.expect(t, "entered", map[string]string{"B1": "6", "C1": "6", "C2": "12"})
	if f := b.cell(0, "B1"); f.Formula != "A1*A2" || !f.Dynamic {
		t.Errorf("B1 formula %q dynamic %v", f.Formula, f.Dynamic)
	}
	b.enter(t, 0, "A1", "10%")
	b.expect(t, "percent", map[string]string{"A1": "10%", "B1": "0.3"})

	// Typing into the spill blocks it; undo lets it spill again.
	b.enter(t, 0, "C2", "x")
	b.expect(t, "blocked", map[string]string{"C1": "#SPILL!", "C2": "x"})
	b.Undo()
	b.expect(t, "unblocked", map[string]string{"C1": "0.3", "C2": "0.6"})

	b.Undo() // A1 back to 2
	b.expect(t, "undo", map[string]string{"A1": "2", "B1": "6", "C2": "12"})
	b.Redo()
	b.expect(t, "redo", map[string]string{"A1": "10%", "B1": "0.3"})

	// A bad formula changes nothing.
	if _, err := b.Enter(0, 0, 1, "=A1+"); err == nil {
		t.Error("bad formula accepted")
	}
	b.expect(t, "bad formula", map[string]string{"B1": "0.3"})

	// Clearing the anchor clears its spill.
	r, c := pos("C1")
	b.Clear(0, model.Range{R1: r, C1: c, R2: r, C2: c})
	if b.cell(0, "C2").Kind != model.KindEmpty {
		t.Errorf("C2 = %q after clearing C1", b.cell(0, "C2").Text)
	}
	b.Undo()
	b.expect(t, "undo clear", map[string]string{"C1": "0.3", "C2": "0.6"})
}

func TestEnterKeepsStyle(t *testing.T) {
	b := newBook()
	bold := model.DefaultStyle()
	bold.Font.Bold = true
	b.WB.Styles = append(b.WB.Styles, bold)
	b.WB.Sheets[0].Set(0, 0, model.Cell{Style: 1})
	b.enter(t, 0, "A1", "1/15/2026")
	c := b.cell(0, "A1")
	st := b.WB.Styles[c.Style]
	if !st.Font.Bold || st.NumFmtCode != "m/d/yyyy" || c.Text != "1/15/2026" {
		t.Errorf("A1: style %+v text %q", st, c.Text)
	}
	// A formatted cell keeps its format.
	b.enter(t, 0, "A1", "46000")
	if got := b.cell(0, "A1").Text; got != "12/9/2025" {
		t.Errorf("A1 = %q, want a date", got)
	}
	// Delete keeps the style.
	b.Clear(0, rng("A1", "A1"))
	if c := b.cell(0, "A1"); c.Kind != model.KindEmpty || !b.WB.Styles[c.Style].Font.Bold {
		t.Errorf("after Clear: %+v", c)
	}
}

func TestCopyPaste(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "1")
	b.enter(t, 0, "A2", "2")
	b.enter(t, 0, "B1", "=A1*10")
	b.enter(t, 0, "C1", "=$A$1+A2")
	clip := b.Copy(0, rng("B1", "C1"), false)

	// Pasting one row down shifts relative references.
	if _, _, err := b.Paste(clip, 0, rng("B2", "B2")); err != nil {
		t.Fatal(err)
	}
	if got := b.cell(0, "B2").Formula + " " + b.cell(0, "C2").Formula; got != "A2*10 $A$1+A3" {
		t.Errorf("pasted formulas: %s", got)
	}
	b.expect(t, "paste", map[string]string{"B2": "20", "C2": "1"})

	// A selection that is a multiple of the copied size is tiled.
	dest, _, _ := b.Paste(b.Copy(0, rng("A1", "A1"), false), 0, rng("E1", "F2"))
	if dest != rng("E1", "F2") {
		t.Errorf("tiled into %+v", dest)
	}
	b.expect(t, "fill", map[string]string{"E1": "1", "F2": "1"})

	// Cut and paste moves cells, and references to them follow.
	b.Paste(b.Copy(0, rng("A1", "A2"), true), 0, rng("H5", "H5"))
	if got := b.cell(0, "B1").Formula; got != "H5*10" {
		t.Errorf("B1 = %q after moving A1", got)
	}
	if got := b.cell(0, "C1").Formula; got != "$H$5+H6" {
		t.Errorf("C1 = %q after moving A1:A2", got)
	}
	if b.cell(0, "A1").Kind != model.KindEmpty {
		t.Error("A1 not emptied by the cut")
	}
	b.expect(t, "cut", map[string]string{"B1": "10", "H5": "1"})
	b.Undo()
	b.expect(t, "undo cut", map[string]string{"A1": "1", "B1": "10"})
	if got := b.cell(0, "B1").Formula; got != "A1*10" {
		t.Errorf("B1 = %q after undoing the move", got)
	}
}

func TestPasteText(t *testing.T) {
	b := newBook()
	if got := SplitText("a\tb\r\n\"x\ny\"\t\"q\"\"\"\r\n"); !reflect.DeepEqual(got, [][]string{{"a", "b"}, {"x\ny", `q"`}}) {
		t.Errorf("SplitText: %q", got)
	}
	b.PasteText("1,000\t12%\n=A1*2\tTRUE\n", 0, rng("A1", "A1"))
	b.expect(t, "text", map[string]string{"A1": "1,000", "B1": "12%", "A2": "2000", "B2": "TRUE"})
	if b.cell(0, "A1").Num != 1000 {
		t.Error("A1 is not the number 1000")
	}
	// One value fills the selection.
	b.PasteText("7", 0, rng("D1", "D3"))
	b.expect(t, "fill", map[string]string{"D1": "7", "D3": "7"})
	if got := CopyText(b.WB.Sheets[0], rng("A1", "B1")); got != "1,000\t12%\r\n" {
		t.Errorf("CopyText = %q", got)
	}
}

func TestInsertDeleteRows(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	b.enter(t, 0, "A1", "1")
	b.enter(t, 0, "A2", "2")
	b.enter(t, 0, "A3", "3")
	b.enter(t, 0, "B4", "=SUM(A1:A3)")
	b.enter(t, 1, "A1", "=Sheet1!A3*2")
	b.WB.Names = []model.DefinedName{{Name: "Top", Sheet: -1, Formula: "Sheet1!$A$3"}}
	sh.Merges = []model.Range{rng("C2", "D3")}
	sh.RowHeight[2] = 30
	sh.FreezeRows = 2
	saved := sh.Clone()

	if _, err := b.InsertRows(0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if got := b.cell(0, "B6").Formula; got != "SUM(A1:A5)" {
		t.Errorf("B6 = %q", got)
	}
	if got := b.cell(1, "A1").Formula; got != "Sheet1!A5*2" {
		t.Errorf("Data!A1 = %q", got)
	}
	if got := b.WB.Names[0].Formula; got != "Sheet1!$A$5" {
		t.Errorf("name = %q", got)
	}
	if sh.Merges[0] != rng("C4", "D5") || sh.RowHeight[4] != 30 || sh.FreezeRows != 4 {
		t.Errorf("merges %v, heights %v, freeze %d", sh.Merges, sh.RowHeight, sh.FreezeRows)
	}
	b.expect(t, "insert", map[string]string{"A4": "2", "B6": "6"})
	b.Undo()
	if !reflect.DeepEqual(sh, saved) {
		t.Error("undo of insert did not restore the sheet")
	}

	// Deleting row 3 removes A3: references to it become #REF!.
	if _, err := b.DeleteRows(0, 2, 1); err != nil {
		t.Fatal(err)
	}
	if got := b.cell(0, "B3").Formula; got != "SUM(A1:A2)" {
		t.Errorf("B3 = %q", got)
	}
	if got := b.cell(1, "A1"); got.Formula != "#REF!*2" || got.Text != "#REF!" {
		t.Errorf("Data!A1 = %q (%q)", got.Formula, got.Text)
	}
	b.expect(t, "delete", map[string]string{"B3": "3"})
	b.Undo()
	if !reflect.DeepEqual(sh, saved) {
		t.Error("undo of delete did not restore the sheet")
	}
	if got := b.cell(1, "A1"); got.Formula != "Sheet1!A3*2" || got.Text != "6" {
		t.Errorf("Data!A1 after undo = %q (%q)", got.Formula, got.Text)
	}
	b.Redo()
	if got := b.cell(0, "B3").Formula; got != "SUM(A1:A2)" {
		t.Errorf("redo: B3 = %q", got)
	}
}

func TestInsertDeleteCols(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "5")
	b.enter(t, 0, "C1", "=A1+B1")
	b.WB.Sheets[0].ColWidth[1] = 20
	b.InsertCols(0, 1, 1)
	if got := b.cell(0, "D1").Formula; got != "A1+C1" {
		t.Errorf("D1 = %q", got)
	}
	if b.WB.Sheets[0].ColWidth[2] != 20 {
		t.Errorf("widths %v", b.WB.Sheets[0].ColWidth)
	}
	b.DeleteCols(0, 0, 1)
	if got := b.cell(0, "C1"); got.Formula != "#REF!+B1" || got.Text != "#REF!" {
		t.Errorf("C1 = %q (%q)", got.Formula, got.Text)
	}
}

func TestSheets(t *testing.T) {
	b := newBook()
	b.enter(t, 1, "A1", "4")
	b.enter(t, 0, "A1", "=Data!A1*2")
	name, _ := b.InsertSheet(1)
	if name != "Sheet3" || b.WB.Sheets[1].Name != "Sheet3" || b.WB.Sheets[2].Name != "Data" {
		t.Errorf("inserted %q: %v", name, b.WB.Sheets)
	}
	if _, err := b.RenameSheet(2, "My Data"); err != nil {
		t.Fatal(err)
	}
	if got := b.cell(0, "A1").Formula; got != "'My Data'!A1*2" {
		t.Errorf("after rename: %q", got)
	}
	if _, err := b.RenameSheet(2, "sheet1"); err == nil {
		t.Error("duplicate name accepted")
	}
	if _, err := b.RenameSheet(2, "a[1]"); err == nil {
		t.Error("bad name accepted")
	}
	b.DeleteSheet(2)
	if got := b.cell(0, "A1"); got.Formula != "#REF!*2" || got.Text != "#REF!" {
		t.Errorf("after delete: %q (%q)", got.Formula, got.Text)
	}
	b.Undo()
	if got := b.cell(0, "A1"); got.Formula != "'My Data'!A1*2" || got.Text != "8" {
		t.Errorf("undo delete: %q (%q)", got.Formula, got.Text)
	}
	b.Undo()
	b.Undo()
	if len(b.WB.Sheets) != 2 || b.cell(0, "A1").Formula != "Data!A1*2" {
		t.Errorf("after undoing everything: %d sheets, A1 %q", len(b.WB.Sheets), b.cell(0, "A1").Formula)
	}
	b.DeleteSheet(1)
	if _, err := b.DeleteSheet(0); err == nil {
		t.Error("deleted the last sheet")
	}
}
