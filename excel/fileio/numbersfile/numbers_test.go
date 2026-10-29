package numbersfile

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/iwork"
	"officelite/excel/model"
)

func readSample(t *testing.T, name string) *model.Workbook {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "Samples", name))
	if err != nil {
		t.Skip(err)
	}
	wb, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	return wb
}

func TestDumpSamples(t *testing.T) {
	if os.Getenv("NUMBERS_DUMP") == "" {
		t.Skip("set NUMBERS_DUMP")
	}
	for _, name := range []string{"Sample1.numbers", "Sample2.numbers", "Sample3.numbers"} {
		wb := readSample(t, name)
		t.Logf("%s: warnings %q", name, wb.Warnings)
		for _, sh := range wb.Sheets {
			t.Logf("  sheet %q %dx%d merges %v freeze %d/%d", sh.Name, sh.UsedRows(), sh.UsedCols(), sh.Merges, sh.FreezeRows, sh.FreezeCols)
			for r := range sh.UsedRows() {
				line := ""
				for c := range sh.RowLen(r) {
					cell := sh.Get(r, c)
					st := wb.Styles[cell.Style]
					line += "|" + cell.Text
					if st.NumFmtCode != "" {
						line += "{" + st.NumFmtCode + "}"
					}
					if st.Font.Bold {
						line += "*"
					}
					if cell.Formula != "" {
						line += " =" + cell.Formula
					}
				}
				t.Logf("    %d: %s", r, line)
			}
		}
	}
}

func TestSample1Calendar(t *testing.T) {
	wb := readSample(t, "Sample1.numbers")
	if len(wb.Sheets) != 17 {
		t.Errorf("%d sheets, want 17 (one per table)", len(wb.Sheets))
	}
	sh := wb.Sheets[0]
	if sh.Name != "Year - Month 1" {
		t.Errorf("first sheet %q", sh.Name)
	}
	if c := sh.Get(0, 0); c.Text != "September" || !wb.Styles[c.Style].Font.Bold {
		t.Errorf("A1 = %q bold %v", c.Text, wb.Styles[c.Style].Font.Bold)
	}
	if c := sh.Get(0, 5); c.Kind != model.KindNumber || c.Num != 2024 {
		t.Errorf("F1 = %+v", c)
	}
	// Dates: September 1, 2024 is the first Sunday, shown as the day only.
	c := sh.Get(2, 0)
	if c.Kind != model.KindNumber || c.Num != 45536 || c.Text != "1" {
		t.Errorf("A3 = %v %q, want serial 45536 shown as 1", c.Num, c.Text)
	}
	if len(sh.Merges) != 2 || sh.Merges[0] != (model.Range{R1: 0, C1: 0, R2: 0, C2: 4}) {
		t.Errorf("merges %v", sh.Merges)
	}
	if h := sh.RowHeightPx(8); h != 0 {
		t.Errorf("hidden helper row 9 is %vpx high", h)
	}
	var notes *model.Sheet
	for _, s := range wb.Sheets {
		if s.Name == "Year - Notes" {
			notes = s
		}
	}
	if notes == nil {
		t.Fatal("no Year - Notes sheet")
	}
	if c := notes.Get(1, 0); c.Text != "9/2" || c.Num != 45537 {
		t.Errorf("Notes A2 = %v %q", c.Num, c.Text)
	}
	if c := notes.Get(1, 1); c.Text != "School Starts" {
		t.Errorf("Notes B2 = %q", c.Text)
	}
	if notes.FreezeRows != 1 {
		t.Errorf("Notes frozen rows %d", notes.FreezeRows)
	}
}

func TestSample2Table(t *testing.T) {
	wb := readSample(t, "Sample2.numbers")
	if len(wb.Sheets) != 1 || wb.Sheets[0].Name != "Table 1" {
		t.Fatalf("sheets %v", len(wb.Sheets))
	}
	sh := wb.Sheets[0]
	want := [][]string{
		{"", "Category A", "Category B"},
		{"Item 1", "5", "7"},
		{"Item 5", "16", "21"},
	}
	for i, r := range []int{0, 1, 5} {
		for c, w := range want[i] {
			if got := sh.Get(r, c).Text; got != w {
				t.Errorf("%s = %q, want %q", model.CellName(r, c), got, w)
			}
		}
	}
	if !wb.Styles[sh.Get(1, 0).Style].Font.Bold {
		t.Error("header column cell A2 isn't bold")
	}
	if !hasWarning(wb, "charts were not imported") {
		t.Errorf("no chart warning in %q", wb.Warnings)
	}
}

func TestSample3Budget(t *testing.T) {
	wb := readSample(t, "Sample3.numbers")
	var names []string
	for _, sh := range wb.Sheets {
		names = append(names, sh.Name)
	}
	if len(names) != 3 || names[0] != "Budget - Income" || names[1] != "Budget - Expenses" || names[2] != "Budget - Net" {
		t.Fatalf("sheets %q", names)
	}
	exp := wb.Sheets[1]
	if c := exp.Get(1, 1); c.Num != 1500 || c.Text != "$1,500" {
		t.Errorf("Expenses B2 = %v %q", c.Num, c.Text)
	}
	if c := exp.Get(7, 1); c.Num != 2750 || c.Text != "$2,750" || !wb.Styles[c.Style].Font.Bold {
		t.Errorf("Expenses B8 (total) = %v %q", c.Num, c.Text)
	}
	if c := wb.Sheets[2].Get(1, 1); c.Num != 1250 {
		t.Errorf("Net B2 = %v", c.Num)
	}
	// The title row has a green fill and white text.
	st := wb.Styles[wb.Sheets[0].Get(0, 0).Style]
	if st.Fill.A == 0 || st.Fill.G < 200 || st.Font.Color.R != 255 {
		t.Errorf("Income A1 fill %v text %v", st.Fill, st.Font.Color)
	}
	// Banded rows: Groceries (row 3) gets the band fill, Housing doesn't.
	if f2, f3 := wb.Styles[exp.Get(1, 1).Style].Fill, wb.Styles[exp.Get(2, 1).Style].Fill; f2.A != 0 || f3.A == 0 {
		t.Errorf("banding: B2 fill %v, B3 fill %v", f2, f3)
	}
}

func TestNotNumbers(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "Samples", "PagesInvoice.pages"))
	if err != nil {
		t.Skip(err)
	}
	if _, err := Read(data); err == nil || !strings.Contains(err.Error(), "Pages") {
		t.Errorf("reading a Pages document: %v", err)
	}
}

func hasWarning(wb *model.Workbook, s string) bool {
	for _, w := range wb.Warnings {
		if strings.Contains(w, s) {
			return true
		}
	}
	return false
}

func TestFormulas(t *testing.T) {
	wb := readSample(t, "Sample3.numbers")
	for _, tt := range []struct {
		sheet, row, col int
		want            string
	}{
		{0, 3, 1, "SUM(B$2:B$3)"}, // a column of the table: its body rows
		{1, 7, 1, "SUM(B$2:B$7)"},
		{2, 1, 1, "'Budget - Income'!B4-'Budget - Expenses'!B8"}, // other tables
	} {
		if got := wb.Sheets[tt.sheet].Get(tt.row, tt.col).Formula; got != tt.want {
			t.Errorf("sheet %d %s formula %q, want %q", tt.sheet, model.CellName(tt.row, tt.col), got, tt.want)
		}
	}
	// Numbers-only functions (DURATION, MONTHNAME...) keep their values.
	wb = readSample(t, "Sample1.numbers")
	if !hasWarning(wb, "imported as their values") {
		t.Errorf("no warning for untranslated formulas in %q", wb.Warnings)
	}
}

// A group that contains itself (directly or through another group)
// overflowed the stack while tallying dropped objects.
func TestCountDroppedGroupCycle(t *testing.T) {
	children := func(ids ...uint64) iwork.Message {
		var m iwork.Message
		for _, id := range ids {
			m = append(m, iwork.Field{Num: 2, Wire: 2, B: binary.AppendUvarint([]byte{1 << 3}, id)})
		}
		return m
	}
	r := &reader{doc: &iwork.Document{Objects: map[uint64]*iwork.Object{
		1: {ID: 1, Type: typeGroup, Msg: children(1, 2)},
		2: {ID: 2, Type: typeGroup, Msg: children(1, 3)},
		3: {ID: 3, Type: typeImage},
	}}}
	dropped := map[string]int{}
	r.countDropped(1, dropped, map[uint64]bool{})
	if dropped["pictures"] != 1 || len(dropped) != 1 {
		t.Errorf("dropped = %v", dropped)
	}
}
