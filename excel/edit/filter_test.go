package edit

import (
	"maps"
	"slices"
	"testing"

	"officelite/excel/model"
)

// filterBook has a table in A1:C8 with a manually hidden row 9 below it.
func filterBook(t *testing.T) *Book {
	b := newBook()
	rows := [][]string{
		{"Name", "Qty", "Date"},
		{"apple", "10", "1/5/2026"},
		{"Banana", "25", "2/10/2026"},
		{"cherry", "5", "1/20/2026"},
		{"apricot", "40", "3/1/2026"},
		{"", "15", "2/14/2026"},
		{"blueberry", "30", "1/5/2026"},
		{"avocado", "20", "4/1/2026"},
	}
	for r, row := range rows {
		for c, v := range row {
			if v != "" {
				b.enter(t, 0, model.CellName(r, c), v)
			}
		}
	}
	b.enter(t, 0, "A11", "after")
	b.ResizeRows(0, []int{10}, 0) // row 11 hidden by hand
	return b
}

// shown lists the 1-based rows of A2:A8 the filter shows.
func shown(b *Book) []int {
	sh := b.WB.Sheets[0]
	var out []int
	for r := 1; r <= 7; r++ {
		if !sh.RowHidden(r) {
			out = append(out, r+1)
		}
	}
	return out
}

func TestFilterRange(t *testing.T) {
	b := filterBook(t)
	sh := b.WB.Sheets[0]
	for _, tc := range []struct {
		sel, want model.Range
	}{
		{rng("B3", "B3"), rng("A1", "C8")},
		{rng("A1", "C1"), rng("A1", "C8")},
		{rng("A1", "B4"), rng("A1", "B4")},
		{model.Range{C1: 0, C2: 2, R2: model.MaxRows - 1}, rng("A1", "C11")},
	} {
		if got, err := FilterRange(sh, tc.sel); err != nil || got != tc.want {
			t.Errorf("FilterRange(%v) = %v, %v; want %v", tc.sel, got, err, tc.want)
		}
	}
	if _, err := FilterRange(sh, rng("F20", "F20")); err == nil {
		t.Error("empty cell: no error")
	}
}

func TestFilterCriteria(t *testing.T) {
	b := filterBook(t)
	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	for _, tc := range []struct {
		name string
		fc   model.FilterColumn
		want []int
	}{
		{"values", model.FilterColumn{Col: 0, Kind: model.FilterValues, Values: []string{"APPLE", "cherry"}}, []int{2, 4}},
		{"values+blanks", model.FilterColumn{Col: 0, Kind: model.FilterValues, Values: []string{"banana"}, Blanks: true}, []int{3, 6}},
		{"begins with", model.FilterColumn{Col: 0, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpEqual, Val: "a*"}}}, []int{2, 5, 8}},
		{"does not contain", model.FilterColumn{Col: 0, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpNotEqual, Val: "*an*"}}}, []int{2, 4, 5, 6, 7, 8}},
		{"ends with ?", model.FilterColumn{Col: 0, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpEqual, Val: "*rr?"}}}, []int{4, 7}},
		{"text greater", model.FilterColumn{Col: 0, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreater, Val: "b"}}}, []int{3, 4, 7}},
		{"between", model.FilterColumn{Col: 1, Kind: model.FilterCustom, And: true,
			Conds: []model.FilterCond{{Op: model.OpGreaterEqual, Val: "15"}, {Op: model.OpLessEqual, Val: "30"}}}, []int{3, 6, 7, 8}},
		{"or", model.FilterColumn{Col: 1, Kind: model.FilterCustom,
			Conds: []model.FilterCond{{Op: model.OpLess, Val: "10"}, {Op: model.OpGreater, Val: "35"}}}, []int{4, 5}},
		{"top 3", model.FilterColumn{Col: 1, Kind: model.FilterTop10, Top10: model.Top10{Top: true, N: 3}}, []int{3, 5, 7}},
		{"bottom 50%", model.FilterColumn{Col: 1, Kind: model.FilterTop10, Top10: model.Top10{Percent: true, N: 50}}, []int{2, 4, 6}},
		{"above average", model.FilterColumn{Col: 1, Kind: model.FilterDynamic, Dynamic: "aboveAverage"}, []int{3, 5, 7}},
		{"below average", model.FilterColumn{Col: 1, Kind: model.FilterDynamic, Dynamic: "belowAverage"}, []int{2, 4, 6, 8}},
		{"date values", model.FilterColumn{Col: 2, Kind: model.FilterValues, Values: []string{"1/5/2026"}}, []int{2, 7}},
		{"date group", model.FilterColumn{Col: 2, Kind: model.FilterValues, Dates: []model.DateGroup{{Year: 2026, Month: 1, Grouping: "month"}}}, []int{2, 4, 7}},
		{"date after", model.FilterColumn{Col: 2, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreater, Val: "46082"}}}, []int{8}},
	} {
		b.FilterColumn(0, tc.fc.Col, &tc.fc)
		if got := shown(b); !slices.Equal(got, tc.want) {
			t.Errorf("%s: rows %v, want %v", tc.name, got, tc.want)
		}
		b.FilterColumn(0, tc.fc.Col, nil)
	}
}

func TestFilterUndoAndClear(t *testing.T) {
	b := filterBook(t)
	sh := b.WB.Sheets[0]
	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	b.FilterColumn(0, 1, &model.FilterColumn{Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreater, Val: "20"}}})
	b.FilterColumn(0, 0, &model.FilterColumn{Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpEqual, Val: "a*"}}})
	if got := shown(b); !slices.Equal(got, []int{5}) {
		t.Fatalf("two columns: %v", got)
	}
	if n, m := FilterCount(sh); n != 1 || m != 7 {
		t.Errorf("FilterCount = %d of %d", n, m)
	}
	b.Undo()
	if got := shown(b); !slices.Equal(got, []int{3, 5, 7}) {
		t.Errorf("undo: %v", got)
	}
	b.Redo()
	if got := shown(b); !slices.Equal(got, []int{5}) {
		t.Errorf("redo: %v", got)
	}

	// Clear shows the filtered rows but not the row hidden by hand.
	b.ClearFilters(0)
	if got := shown(b); len(got) != 7 || !sh.RowHidden(10) || sh.AutoFilter == nil {
		t.Errorf("clear: %v, row 11 hidden %v, filter %v", got, sh.RowHidden(10), sh.AutoFilter)
	}
	b.Undo()
	b.SetAutoFilter(0, nil)
	if sh.AutoFilter != nil || len(sh.Filtered) != 0 || !sh.RowHidden(10) {
		t.Errorf("off: filter %v, filtered %v", sh.AutoFilter, sh.Filtered)
	}
	b.Undo()
	if got := shown(b); !slices.Equal(got, []int{5}) {
		t.Errorf("undo off: %v", got)
	}
}

func TestFilterReapplyAndSort(t *testing.T) {
	b := filterBook(t)
	sh := b.WB.Sheets[0]
	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	b.FilterColumn(0, 1, &model.FilterColumn{Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreaterEqual, Val: "25"}}})
	b.enter(t, 0, "B2", "50") // apple: data changed, still hidden
	b.enter(t, 0, "A9", "date")
	b.enter(t, 0, "B9", "60") // a row added right below
	if !sh.RowHidden(1) {
		t.Error("an edit refiltered")
	}
	b.ReapplyFilter(0)
	if got := shown(b); !slices.Equal(got, []int{2, 3, 5, 7}) || sh.AutoFilter.Range.R2 != 8 || sh.RowHidden(8) {
		t.Errorf("reapply: %v, range %v", got, sh.AutoFilter.Range)
	}

	if _, err := b.SortFiltered(0, 1, true); err != nil {
		t.Fatal(err)
	}
	// B: 60 50 40 30 25 20 15 5 → the four >= 25 first, then 25.
	var qty []string
	for row := 1; row <= 8; row++ {
		if !sh.RowHidden(row) {
			qty = append(qty, sh.Get(row, 1).Text)
		}
	}
	if !slices.Equal(qty, []string{"60", "50", "40", "30", "25"}) {
		t.Errorf("sorted shown %v", qty)
	}
	b.Undo()
	if got := shown(b); !slices.Equal(got, []int{2, 3, 5, 7}) || b.cell(0, "B9").Text != "60" {
		t.Errorf("undo sort: %v, B9 %q", got, b.cell(0, "B9").Text)
	}
}

func TestFilterItems(t *testing.T) {
	b := filterBook(t)
	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	b.FilterColumn(0, 0, &model.FilterColumn{Kind: model.FilterValues, Values: []string{"apple"}, Blanks: true})
	items := b.FilterItems(0, 0)
	got := map[string]bool{}
	var order []string
	for _, it := range items {
		got[it.Text] = it.Checked
		order = append(order, it.Text)
	}
	if !slices.Equal(order, []string{"apple", "apricot", "avocado", "Banana", "blueberry", "cherry", ""}) {
		t.Errorf("order %q", order)
	}
	if want := map[string]bool{"apple": true, "": true, "apricot": false, "avocado": false, "Banana": false, "blueberry": false, "cherry": false}; !maps.Equal(got, want) {
		t.Errorf("checked %v", got)
	}
	// Another column's list holds only the rows this filter shows, numbers
	// in order.
	var qty []string
	for _, it := range b.FilterItems(0, 1) {
		qty = append(qty, it.Text)
	}
	if !slices.Equal(qty, []string{"10", "15"}) {
		t.Errorf("Qty items %q", qty)
	}
	if k := b.FilterColumnKind(0, 2); k != ColumnDate {
		t.Errorf("kind of C = %v", k)
	}
	if k := b.FilterColumnKind(0, 1); k != ColumnNumber {
		t.Errorf("kind of B = %v", k)
	}
}

func TestFilterInsertRows(t *testing.T) {
	b := filterBook(t)
	sh := b.WB.Sheets[0]
	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	b.FilterColumn(0, 1, &model.FilterColumn{Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpLess, Val: "20"}}})
	if _, err := b.InsertRows(0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if sh.AutoFilter.Range != rng("A3", "C10") || !sh.RowHidden(4) || sh.RowHidden(3) {
		t.Errorf("after insert: %v, filtered %v", sh.AutoFilter.Range, sh.Filtered)
	}
	b.Undo()
	if sh.AutoFilter.Range != r || !sh.RowHidden(2) {
		t.Errorf("undo insert: %v, filtered %v", sh.AutoFilter.Range, sh.Filtered)
	}
}

// SUBTOTAL always leaves out rows the filter hid, and 101-111 rows hidden
// by hand too; AGGREGATE's odd options leave out both. They follow filter
// changes and undo.
func TestSubtotalHiddenRows(t *testing.T) {
	b := filterBook(t)
	b.enter(t, 0, "B11", "100") // in the row hidden by hand
	b.enter(t, 0, "E1", "=SUBTOTAL(9,B2:B12)")
	b.enter(t, 0, "E2", "=SUBTOTAL(109,B2:B12)")
	b.enter(t, 0, "E3", "=AGGREGATE(9,5,B2:B12)")
	b.enter(t, 0, "E4", "=AGGREGATE(9,4,B2:B12)")
	unfiltered := map[string]string{"E1": "245", "E2": "145", "E3": "145", "E4": "245"}
	b.expect(t, "no filter", unfiltered)

	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	b.FilterColumn(0, 0, &model.FilterColumn{Col: 0, Kind: model.FilterValues, Values: []string{"apple", "cherry"}})
	b.expect(t, "filtered", map[string]string{"E1": "115", "E2": "15", "E3": "15", "E4": "245"})

	b.Undo()
	b.expect(t, "undo", unfiltered)
	b.Redo()
	b.expect(t, "redo", map[string]string{"E1": "115", "E2": "15"})
	b.ClearFilters(0)
	b.expect(t, "cleared", unfiltered)
}

// Fill, Clear, Copy and formatting skip the rows the AutoFilter hides,
// as Excel's do; a single copied row pastes into the rows shown only.
func TestFilteredRowsLeftAlone(t *testing.T) {
	b := filterBook(t)
	sh := b.WB.Sheets[0]
	r := rng("A1", "C8")
	b.SetAutoFilter(0, &r)
	b.FilterColumn(0, 1, &model.FilterColumn{Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreaterEqual, Val: "25"}}})
	if got := shown(b); !slices.Equal(got, []int{3, 5, 7}) {
		t.Fatalf("shown %v", got)
	}
	b.FillDown(0, rng("B3", "B8"))
	b.expect(t, "fill down", map[string]string{"B3": "25", "B4": "5", "B5": "25", "B6": "15", "B7": "25", "B8": "20"})
	b.Undo()
	// The top row filtered: the first row shown is copied.
	b.FillDown(0, rng("B2", "B6"))
	b.expect(t, "fill from B3", map[string]string{"B2": "10", "B4": "5", "B5": "25", "B6": "15"})
	b.Undo()
	b.FillRight(0, rng("A2", "B8"))
	b.expect(t, "fill right", map[string]string{"B2": "10", "B3": "Banana", "B4": "5", "B5": "apricot"})
	b.Undo()

	b.Clear(0, rng("A2", "A8"))
	b.expect(t, "clear", map[string]string{"A2": "apple", "A3": "", "A4": "cherry", "A5": "", "A7": "", "A8": "avocado"})
	b.Undo()
	b.Clear(0, model.Range{R1: 0, C1: 0, R2: model.MaxRows - 1, C2: 0})
	b.expect(t, "clear column", map[string]string{"A1": "", "A2": "apple", "A3": "", "A11": ""})
	b.Undo()

	b.Format(0, rng("A2", "A8"), func(st *model.Style, _, _ int) { st.Font.Bold = true })
	for row, want := range map[int]bool{1: false, 2: true, 3: false, 4: true, 6: true, 7: false} {
		if got := b.WB.Styles[sh.Get(row, 0).Style].Font.Bold; got != want {
			t.Errorf("format: row %d bold %v", row+1, got)
		}
	}
	b.Undo()

	// Copy takes rows 3, 5 and 7; pasted they are adjacent, formulas
	// shifted from their own rows.
	b.enter(t, 0, "D3", "=B3*2")
	b.enter(t, 0, "D5", "=B5*2")
	clip := b.Copy(0, rng("A2", "D8"), false)
	if len(clip.Cells) != 3 {
		t.Fatalf("copied %d rows", len(clip.Cells))
	}
	if txt := CopyText(sh, rng("A2", "B8")); txt != "Banana\t25\r\napricot\t40\r\nblueberry\t30\r\n" {
		t.Errorf("CopyText %q", txt)
	}
	if _, _, err := b.Paste(clip, 1, rng("A1", "A1")); err != nil {
		t.Fatal(err)
	}
	for ref, want := range map[string]string{"A1": "Banana", "B2": "40", "A3": "blueberry"} {
		if got := b.cell(1, ref).Text; got != want {
			t.Errorf("paste: %s = %q, want %q", ref, got, want)
		}
	}
	if f1, f2 := b.cell(1, "D1").Formula, b.cell(1, "D2").Formula; f1 != "B1*2" || f2 != "B2*2" {
		t.Errorf("pasted formulas %q, %q", f1, f2)
	}
	// One cell pasted over the filtered column fills the rows shown.
	one := b.Copy(0, rng("A11", "A11"), false)
	if _, _, err := b.Paste(one, 0, rng("E2", "E8")); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "paste one", map[string]string{"E2": "", "E3": "after", "E4": "", "E5": "after", "E7": "after", "E8": ""})
}
