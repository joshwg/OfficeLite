package edit

import (
	"slices"
	"testing"

	"officelite/excel/model"
)

// levels lists the outline levels of rows (columns) lo..hi.
func levels(sh *model.Sheet, rows bool, lo, hi int) []int {
	var out []int
	for i := lo; i <= hi; i++ {
		out = append(out, sh.Level(rows, i))
	}
	return out
}

// hiddenIn lists the 0-based hidden rows (columns) of lo..hi.
func hiddenIn(sh *model.Sheet, rows bool, lo, hi int) []int {
	var out []int
	for i := lo; i <= hi; i++ {
		if sh.IndexHidden(rows, i) {
			out = append(out, i)
		}
	}
	return out
}

func TestGroupUngroup(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	if !b.Group(0, true, 1, 6) || !b.Group(0, true, 2, 3) || !b.Group(0, true, 2, 2) {
		t.Fatal("Group reported no change")
	}
	if got := levels(sh, true, 0, 7); !slices.Equal(got, []int{0, 1, 3, 2, 1, 1, 1, 0}) {
		t.Fatalf("levels %v", got)
	}
	groups := sh.Groups(true)
	want := []model.OutlineGroup{
		{Rows: true, Level: 1, Lo: 1, Hi: 6, Summary: 7},
		{Rows: true, Level: 2, Lo: 2, Hi: 3, Summary: 4},
		{Rows: true, Level: 3, Lo: 2, Hi: 2, Summary: 3},
	}
	if !slices.Equal(groups, want) {
		t.Fatalf("groups %+v", groups)
	}
	// Up to seven levels.
	for range 10 {
		b.Group(0, false, 3, 4)
	}
	if sh.MaxLevel(false) != model.MaxOutlineLevel || b.Group(0, false, 3, 4) {
		t.Fatalf("column levels %d", sh.MaxLevel(false))
	}
	// Collapse the outer row group, then expand the inner one's parent.
	g := groups[0]
	b.SetGroupCollapsed(0, groups[2], true)
	b.SetGroupCollapsed(0, g, true)
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{1, 2, 3, 4, 5, 6}) || !sh.IsCollapsed(true, 7) {
		t.Fatalf("collapsed: hidden %v", got)
	}
	b.SetGroupCollapsed(0, g, false)
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{2}) || sh.IsCollapsed(true, 7) || !sh.IsCollapsed(true, 3) {
		t.Fatalf("expanded: hidden %v (inner stays collapsed)", got)
	}
	// Level buttons.
	b.ShowLevels(0, true, 2)
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{2, 3}) || !sh.IsCollapsed(true, 4) || sh.IsCollapsed(true, 7) {
		t.Fatalf("level 2: hidden %v", got)
	}
	b.ShowLevels(0, true, 1)
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("level 1: hidden %v", got)
	}
	b.ShowLevels(0, true, 4)
	if got := hiddenIn(sh, true, 0, 8); len(got) != 0 || len(sh.Outline.RowCollapsed) != 0 {
		t.Fatalf("level 4: hidden %v, collapsed %v", got, sh.Outline.RowCollapsed)
	}
	// Undo goes back one command at a time.
	b.Undo()
	if got := hiddenIn(sh, true, 0, 8); len(got) != 6 {
		t.Fatalf("undo level 4: hidden %v", got)
	}
	b.Redo()

	// Ungroup shows the rows and takes them a level up.
	b.SetGroupCollapsed(0, sh.Groups(true)[1], true)
	if !b.Ungroup(0, true, 1, 6) {
		t.Fatal("Ungroup")
	}
	if got := levels(sh, true, 0, 7); !slices.Equal(got, []int{0, 0, 2, 1, 0, 0, 0, 0}) || len(hiddenIn(sh, true, 0, 8)) != 0 {
		t.Fatalf("ungrouped: levels %v hidden %v", got, hiddenIn(sh, true, 0, 8))
	}
	if b.Ungroup(0, true, 10, 12) {
		t.Error("ungrouping ungrouped rows changed something")
	}
	if !b.ClearOutline(0) || sh.MaxLevel(true) != 0 || sh.MaxLevel(false) != 0 {
		t.Fatal("Clear Outline")
	}
	b.Undo()
	if sh.MaxLevel(true) != 2 {
		t.Fatal("undo Clear Outline")
	}
}

func TestOutlineKeepsSizesAndMoves(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	b.SetRowHeights(0, []int{3}, []float32{40})
	b.Group(0, true, 2, 4)
	g := sh.Groups(true)[0]
	b.SetGroupCollapsed(0, g, true)
	if sh.RowHeightPx(3) != 0 {
		t.Fatal("row 4 not hidden")
	}
	b.SetGroupCollapsed(0, g, false)
	if sh.RowHeightPx(3) != 40 || sh.RowHeightPx(2) != sh.DefaultRowHeight {
		t.Fatalf("heights back: %v %v", sh.RowHeightPx(3), sh.RowHeightPx(2))
	}

	// Rows inserted inside a group join it; above it, they move it.
	if _, err := b.InsertRows(0, 3, 2); err != nil {
		t.Fatal(err)
	}
	if got := levels(sh, true, 1, 7); !slices.Equal(got, []int{0, 1, 1, 1, 1, 1, 0}) {
		t.Fatalf("insert inside: %v", got)
	}
	b.InsertRows(0, 0, 1)
	if got := levels(sh, true, 2, 8); !slices.Equal(got, []int{0, 1, 1, 1, 1, 1, 0}) {
		t.Fatalf("insert above: %v", got)
	}
	b.SetGroupCollapsed(0, sh.Groups(true)[0], true)
	if !sh.IsCollapsed(true, 8) {
		t.Fatal("summary row 9 not collapsed")
	}
	b.DeleteRows(0, 0, 2)
	if got := levels(sh, true, 0, 6); !slices.Equal(got, []int{0, 1, 1, 1, 1, 1, 0}) || !sh.IsCollapsed(true, 6) {
		t.Fatalf("delete above: %v, collapsed %v", got, sh.Outline.RowCollapsed)
	}
	b.Undo()
	b.Undo()
	if got := levels(sh, true, 2, 8); !slices.Equal(got, []int{0, 1, 1, 1, 1, 1, 0}) || sh.IsCollapsed(true, 8) {
		t.Fatalf("undo: %v", got)
	}
}

func TestShowHideDetail(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	b.Group(0, true, 1, 6)
	b.Group(0, true, 1, 2)
	b.Group(0, false, 1, 2)
	// A cell in an inner detail row hides the inner group, and its
	// column's group.
	if !b.HideDetail(0, rng("B2", "B2")) {
		t.Fatal("Hide Detail")
	}
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{1, 2}) || len(hiddenIn(sh, false, 0, 4)) != 2 {
		t.Fatalf("hidden rows %v cols %v", got, hiddenIn(sh, false, 0, 4))
	}
	// Its summary row (4) is in the outer group: Hide Detail there hides
	// the inner one again (already done), so the outer goes.
	b.HideDetail(0, rng("A8", "A8"))
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("outer: %v", got)
	}
	// Show Detail on the summary row brings back the outer group only.
	whole := model.Range{R1: 7, R2: 7, C2: model.MaxCols - 1}
	if !b.ShowDetail(0, whole) {
		t.Fatal("Show Detail")
	}
	if got := hiddenIn(sh, true, 0, 8); !slices.Equal(got, []int{1, 2}) || len(hiddenIn(sh, false, 0, 4)) != 2 {
		t.Fatalf("show: rows %v cols %v (whole rows leave columns alone)", got, hiddenIn(sh, false, 0, 4))
	}
	if b.ShowDetail(0, rng("J20", "J20")) {
		t.Error("Show Detail outside the outline changed something")
	}
}

// subtotalBook has Region / Item / Qty / Price in A1:D8.
func subtotalBook(t *testing.T) *Book {
	b := newBook()
	rows := [][]string{
		{"Region", "Item", "Qty", "Price"},
		{"East", "a", "1", "10"},
		{"East", "b", "2", "20"},
		{"North", "c", "3", "30"},
		{"West", "d", "4", "40"},
		{"West", "e", "5", "50"},
		{"West", "f", "6", "60"},
	}
	for r, row := range rows {
		for c, v := range row {
			b.enter(t, 0, model.CellName(r, c), v)
		}
	}
	b.enter(t, 0, "F2", "=SUM(C2:C7)")
	return b
}

func TestSubtotal(t *testing.T) {
	b := subtotalBook(t)
	sh := b.WB.Sheets[0]
	spec := SubtotalSpec{Range: rng("A1", "D7"), GroupBy: 0, Func: 0, Cols: []int{2, 3}, Replace: true}
	list, _, err := b.Subtotal(0, spec)
	if err != nil {
		t.Fatal(err)
	}
	if list != rng("A1", "D11") {
		t.Fatalf("list %v", list)
	}
	b.expect(t, "subtotals", map[string]string{
		"A4": "East Total", "C4": "3", "D4": "30",
		"A6": "North Total", "C6": "3",
		"A10": "West Total", "C10": "15", "D10": "150",
		"A11": "Grand Total", "C11": "21", "D11": "210",
		"F2": "27", // SUM counts the two subtotal rows inserted inside its range
	})
	if f := b.cell(0, "C11").Formula; f != "SUBTOTAL(9,C2:C10)" {
		t.Errorf("grand formula %q", f)
	}
	if f := b.cell(0, "C4").Formula; f != "SUBTOTAL(9,C2:C3)" {
		t.Errorf("East formula %q", f)
	}
	if f := b.cell(0, "F2").Formula; f != "SUM(C2:C9)" {
		t.Errorf("F2 not adjusted: %q", f)
	}
	if !b.WB.Styles[b.cell(0, "A4").Style].Font.Bold || !b.WB.Styles[b.cell(0, "A11").Style].Font.Bold {
		t.Error("labels not bold")
	}
	if got := levels(sh, true, 0, 11); !slices.Equal(got, []int{0, 2, 2, 1, 2, 1, 2, 2, 2, 1, 0, 0}) {
		t.Fatalf("levels %v", got)
	}
	// Level 2: only subtotals and the grand total.
	b.ShowLevels(0, true, 2)
	if got := hiddenIn(sh, true, 0, 11); !slices.Equal(got, []int{1, 2, 4, 6, 7, 8}) {
		t.Fatalf("level 2 hides %v", got)
	}
	b.Undo()

	// Again with Replace and Count: the old rows go.
	spec.Range, spec.Func = list, 1
	list, _, err = b.Subtotal(0, spec)
	if err != nil || list != rng("A1", "D11") {
		t.Fatalf("replace: %v %v", list, err)
	}
	b.expect(t, "count", map[string]string{"A4": "East Count", "C4": "2", "A11": "Grand Count", "C11": "6", "A12": ""})

	// Without Replace, nested by Item inside each Region.
	spec.GroupBy, spec.Replace, spec.Func = 1, false, 0
	list, _, err = b.Subtotal(0, spec)
	if err != nil {
		t.Fatal(err)
	}
	if list.R2 != 16 || b.cell(0, "B3").Text != "a Total" || b.cell(0, "A17").Text != "Grand Count" || b.cell(0, "C17").Text != "6" {
		t.Fatalf("nested: list %v, B3 %q A17 %q C17 %q", list, b.cell(0, "B3").Text, b.cell(0, "A17").Text, b.cell(0, "C17").Text)
	}
	if sh.MaxLevel(true) != 3 {
		t.Errorf("nested levels %v", levels(sh, true, 0, 17))
	}

	// One undo step each.
	b.Undo()
	b.Undo()
	b.expect(t, "undone twice", map[string]string{"A4": "East Total", "A11": "Grand Total", "C11": "21"})
	b.Undo()
	b.expect(t, "undone", map[string]string{"A4": "North", "A8": "", "F2": "21"})
	if sh.MaxLevel(true) != 0 || b.cell(0, "F2").Formula != "SUM(C2:C7)" {
		t.Fatalf("undo left levels %d, F2 %q", sh.MaxLevel(true), b.cell(0, "F2").Formula)
	}
	b.Redo()
	b.expect(t, "redo", map[string]string{"A11": "Grand Total", "C11": "21"})

	// Remove All.
	b.ShowLevels(0, true, 1)
	list, _, err = b.RemoveSubtotals(0, rng("A1", "D11"))
	if err != nil || list != rng("A1", "D7") {
		t.Fatalf("remove: %v %v", list, err)
	}
	b.expect(t, "removed", map[string]string{"A4": "North", "A7": "West", "A8": "", "F2": "21"})
	if sh.MaxLevel(true) != 0 || len(hiddenIn(sh, true, 0, 12)) != 0 {
		t.Fatalf("remove left levels %d hidden %v", sh.MaxLevel(true), hiddenIn(sh, true, 0, 12))
	}
	b.Undo()
	b.expect(t, "remove undone", map[string]string{"A11": "Grand Total"})
	if len(hiddenIn(sh, true, 0, 12)) != 9 {
		t.Errorf("hidden after undo %v", hiddenIn(sh, true, 0, 12))
	}
}

func TestSubtotalAbove(t *testing.T) {
	b := subtotalBook(t)
	sh := b.WB.Sheets[0]
	list, _, err := b.Subtotal(0, SubtotalSpec{Range: rng("A1", "D7"), Func: 3, Cols: []int{2}, Replace: true, Above: true})
	if err != nil || list != rng("A1", "D11") {
		t.Fatalf("%v %v", list, err)
	}
	b.expect(t, "above", map[string]string{
		"A2": "Grand Max", "C2": "6",
		"A3": "East Max", "C3": "2", "A4": "East",
		"A6": "North Max", "A8": "West Max", "C8": "6", "A11": "West",
	})
	if !sh.Outline.SummaryAbove {
		t.Error("SummaryAbove not set")
	}
	gs := sh.Groups(true)
	if gs[0] != (model.OutlineGroup{Rows: true, Level: 1, Lo: 2, Hi: 10, Summary: 1}) {
		t.Fatalf("grand group %+v", gs[0])
	}
	for _, g := range gs[1:] {
		if g.Summary != g.Lo-1 {
			t.Errorf("group %+v", g)
		}
	}
	if _, _, err := b.Subtotal(0, SubtotalSpec{Range: rng("A1", "D1"), Cols: []int{2}}); err == nil {
		t.Error("no data: no error")
	}
}

// TestSubtotalReplaceFailureRollsBack checks that a Subtotal that fails
// after removing the old subtotal rows leaves the sheet and undo list alone.
func TestSubtotalReplaceFailureRollsBack(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "Region")
	b.enter(t, 0, "A2", "East Total")
	b.enter(t, 0, "C2", "=SUBTOTAL(9,C1:C1)")
	undo := len(b.UndoLabels())
	_, _, err := b.Subtotal(0, SubtotalSpec{Range: rng("A1", "C2"), GroupBy: 0, Func: 0, Cols: []int{2}, Replace: true})
	if err != ErrSubtotalRange {
		t.Fatalf("err = %v, want ErrSubtotalRange", err)
	}
	if got := b.cell(0, "A2").Text; got != "East Total" {
		t.Errorf("A2 = %q after the failed Subtotal", got)
	}
	if n := len(b.UndoLabels()); n != undo {
		t.Errorf("undo list grew from %d to %d", undo, n)
	}
}
