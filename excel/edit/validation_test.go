package edit

import (
	"reflect"
	"testing"

	"officelite/excel/model"
)

func TestValidationRules(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "H1", "Red")
	b.enter(t, 0, "H2", "Green")
	b.enter(t, 0, "H4", "10")
	b.enter(t, 0, "I1", "5")
	b.enter(t, 0, "I2", "50")
	b.enter(t, 1, "A1", "North")
	b.enter(t, 1, "A2", "South")
	b.WB.Names = append(b.WB.Names, model.DefinedName{Name: "Regions", Sheet: -1, Formula: "Data!$A$1:$A$2"})

	set := func(ref string, v model.Validation) {
		r, c := pos(ref)
		b.SetValidation(0, []model.Range{{R1: r, C1: c, R2: r + 1, C2: c}}, &v)
	}
	whole := func(op model.ValidationOp, f1, f2 string) model.Validation {
		return model.Validation{Type: model.ValidWhole, Op: op, Formula1: f1, Formula2: f2}
	}
	check := func(ref, text string, want bool) {
		t.Helper()
		r, c := pos(ref)
		if got := b.CheckEntry(0, r, c, text) == nil; got != want {
			t.Errorf("%s %q: valid %v, want %v", ref, text, got, want)
		}
	}
	// Whole number with every comparison.
	for _, tc := range []struct {
		v     model.Validation
		cases map[string]bool
	}{
		{whole(model.ValidBetween, "1", "10"), map[string]bool{"1": true, "10": true, "11": false, "5.5": false, "abc": false}},
		{whole(model.ValidNotBetween, "1", "10"), map[string]bool{"0": true, "5": false, "11": true}},
		{whole(model.ValidEqual, "7", ""), map[string]bool{"7": true, "8": false}},
		{whole(model.ValidNotEqual, "7", ""), map[string]bool{"7": false, "8": true}},
		{whole(model.ValidGreater, "7", ""), map[string]bool{"7": false, "8": true}},
		{whole(model.ValidLess, "7", ""), map[string]bool{"6": true, "7": false}},
		{whole(model.ValidGreaterEqual, "7", ""), map[string]bool{"7": true, "6": false}},
		{whole(model.ValidLessEqual, "7", ""), map[string]bool{"7": true, "8": false}},
		// Bounds from cells (relative to the rule's top-left: A2 reads I2)
		// and formulas.
		{whole(model.ValidBetween, "$I$1", "I1*2"), map[string]bool{"5": true, "10": true, "11": false, "4": false}},
		{model.Validation{Type: model.ValidDecimal, Op: model.ValidBetween, Formula1: "0", Formula2: "1"}, map[string]bool{"0.5": true, "1.5": false, "50%": true}},
		{model.Validation{Type: model.ValidDate, Op: model.ValidGreaterEqual, Formula1: "45292"}, map[string]bool{"1/1/2024": true, "12/31/2023": false, "x": false}},
		{model.Validation{Type: model.ValidTime, Op: model.ValidLess, Formula1: "0.5"}, map[string]bool{"9:00": true, "13:00": false}},
		{model.Validation{Type: model.ValidTextLength, Op: model.ValidLessEqual, Formula1: "3"}, map[string]bool{"abc": true, "abcd": false, "123": true, "1234": false}},
		{model.Validation{Type: model.ValidList, Formula1: `"Yes,No, Maybe,1"`}, map[string]bool{"yes": true, "NO": true, "Maybe": true, "1": true, "1.0": true, "y": false}},
		{model.Validation{Type: model.ValidList, Formula1: "$H$1:$H$4"}, map[string]bool{"red": true, "Green": true, "10": true, "Blue": false}},
		{model.Validation{Type: model.ValidList, Formula1: "Regions"}, map[string]bool{"south": true, "East": false}},
		{model.Validation{Type: model.ValidCustom, Formula1: "ISNUMBER(A1)"}, map[string]bool{"12": true, "x": false}},
		{model.Validation{Type: model.ValidCustom, Formula1: "COUNTIF($H$1:$H$2,A1)=0"}, map[string]bool{"Red": false, "Blue": true}},
		{model.Validation{Type: model.ValidAny}, map[string]bool{"x": true}},
	} {
		set("A1", tc.v)
		for text, want := range tc.cases {
			check("A1", text, want)
		}
	}
	// A2 is under the rule too, its relative references moved one row.
	set("A1", model.Validation{Type: model.ValidCustom, Formula1: "A1<>$H$1"})
	check("A2", "Red", false)
	set("A1", whole(model.ValidBetween, "I1", "I1*2"))
	check("A2", "60", true)
	check("A2", "101", false)
	// Blanks: allowed only with Ignore blank; a blank bound with Ignore
	// blank allows anything.
	check("A1", "", false)
	set("A1", model.Validation{Type: model.ValidWhole, Op: model.ValidLess, Formula1: "Z9", AllowBlank: true})
	check("A1", "", true)
	check("A1", "999", true)
	check("C1", "x", true) // no rule

	// Drop-down items: a range shows the cells as displayed.
	set("A1", model.Validation{Type: model.ValidList, Formula1: "$H$1:$H$4"})
	if got := b.ListItems(0, 0, 0); !reflect.DeepEqual(got, []string{"Red", "Green", "10"}) {
		t.Errorf("range items %q", got)
	}
	set("A1", model.Validation{Type: model.ValidList, Formula1: `"a, b,c"`})
	if got := b.ListItems(0, 1, 0); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("text items %q", got)
	}
	set("A1", model.Validation{Type: model.ValidList, Formula1: "Regions", NoDropDown: true})
	if got := b.ListItems(0, 0, 0); got != nil {
		t.Errorf("no drop-down: %q", got)
	}

	// Circle Invalid Data.
	b.enter(t, 0, "A1", "West")
	b.enter(t, 0, "A2", "North")
	if got := b.InvalidCells(0); !reflect.DeepEqual(got, [][2]int{{0, 0}}) {
		t.Errorf("invalid %v", got)
	}
	if b.CellValid(0, 0, 0) || !b.CellValid(0, 1, 0) {
		t.Error("CellValid")
	}
}

func TestValidationUndoAndMove(t *testing.T) {
	b := newBook()
	sh := b.WB.Sheets[0]
	v := model.Validation{Type: model.ValidWhole, Op: model.ValidGreater, Formula1: "$C$5", ShowError: true}
	b.SetValidation(0, []model.Range{rng("B2", "B6")}, &v)
	// A second rule over part of it takes those cells.
	b.SetValidation(0, []model.Range{rng("B4", "C4")}, &model.Validation{Type: model.ValidList, Formula1: `"a"`})
	if len(sh.Validations) != 2 || !reflect.DeepEqual(sh.Validations[0].Ranges, []model.Range{rng("B2", "B3"), rng("B5", "B6")}) {
		t.Fatalf("after second rule: %+v", sh.Validations)
	}
	b.Undo()
	if len(sh.Validations) != 1 || sh.Validations[0].Ranges[0] != rng("B2", "B6") {
		t.Fatalf("undo: %+v", sh.Validations)
	}
	b.Redo()
	b.Undo()

	// Rows inserted above move the rule and its reference; deleted rows
	// shrink it.
	if _, err := b.InsertRows(0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if got := sh.Validations[0]; got.Ranges[0] != rng("B4", "B8") || got.Formula1 != "$C$7" {
		t.Errorf("insert rows: %+v", got)
	}
	if _, err := b.DeleteRows(0, 4, 2); err != nil {
		t.Fatal(err)
	}
	if got := sh.Validations[0].Ranges[0]; got != rng("B4", "B6") {
		t.Errorf("delete rows: %v", got)
	}
	b.Undo()
	b.Undo()
	if got := sh.Validations[0]; got.Ranges[0] != rng("B2", "B6") || got.Formula1 != "$C$5" {
		t.Errorf("undo structure: %+v", got)
	}
	if _, err := b.InsertCols(0, 0, 1); err != nil {
		t.Fatal(err)
	}
	if got := sh.Validations[0]; got.Ranges[0] != rng("C2", "C6") || got.Formula1 != "$D$5" {
		t.Errorf("insert cols: %+v", got)
	}
	if _, err := b.DeleteCols(0, 2, 1); err != nil {
		t.Fatal(err)
	}
	if len(sh.Validations) != 0 {
		t.Errorf("rule's column deleted: %+v", sh.Validations)
	}
	b.Undo()
	b.Undo()

	// Insert Cells shifting down in the rule's column.
	if _, err := b.InsertCells(0, rng("B3", "B3"), ShiftDown); err != nil {
		t.Fatal(err)
	}
	if got := sh.Validations[0].Ranges; !reflect.DeepEqual(got, []model.Range{rng("B2", "B7")}) {
		t.Errorf("shift down: %v", got)
	}
	b.Undo()

	// Moving cells carries their rule.
	if _, _, err := b.MoveCells(0, rng("B2", "B3"), 0, 3, false); err != nil {
		t.Fatal(err)
	}
	if got := sh.Validations[0].Ranges; !reflect.DeepEqual(got, []model.Range{rng("B4", "B6"), rng("E2", "E3")}) {
		t.Errorf("move: %v", got)
	}
	b.Undo()

	// Sorting moves the rules with the rows.
	b.SetValidation(0, []model.Range{rng("A3", "A3")}, &model.Validation{Type: model.ValidList, Formula1: `"x"`})
	for i, s := range []string{"c", "a", "b"} {
		b.enter(t, 0, "A"+itoa(i+2), s)
	}
	if _, err := b.Sort(0, SortSpec{Range: rng("A2", "A4"), Keys: []SortKey{{Col: 0}}}); err != nil {
		t.Fatal(err)
	}
	if v, ok := sh.ValidationAt(1, 0); !ok || v.Formula1 != `"x"` {
		t.Errorf("sort: A2 rule %+v", v)
	}
	if _, ok := sh.ValidationAt(2, 0); ok {
		t.Error("sort: A3 kept its rule")
	}
	// Clear All.
	b.SetValidation(0, []model.Range{rng("A1", "Z100")}, nil)
	if len(sh.Validations) != 0 {
		t.Errorf("clear all: %+v", sh.Validations)
	}
}

func TestValidationBound(t *testing.T) {
	for _, tc := range []struct {
		in       string
		list     bool
		want     string
		ok       bool
		typ      model.ValidationType
		readBack string
	}{
		{"10", false, "10", true, model.ValidWhole, "10"},
		{"1/1/2024", false, "45292", true, model.ValidDate, "1/1/2024"},
		{"9:30", false, "0.395833333333333", true, model.ValidTime, "9:30"},
		{"=$A$1:$A$5", true, "$A$1:$A$5", true, model.ValidList, "=$A$1:$A$5"},
		{"Yes,No", true, `"Yes,No"`, true, model.ValidList, "Yes,No"},
		{"abc", false, "", false, model.ValidWhole, ""},
		{"", false, "", false, model.ValidWhole, ""},
	} {
		got, ok := ValidationBound(tc.in, tc.list, false)
		if len(got) > len(tc.want) && tc.want != "" {
			got = got[:len(tc.want)]
		}
		if got != tc.want || ok != tc.ok {
			t.Errorf("%q: %q %v", tc.in, got, ok)
			continue
		}
		if ok {
			f, _ := ValidationBound(tc.in, tc.list, false)
			if back := ValidationBoundText(f, tc.typ, false); back != tc.readBack {
				t.Errorf("%q: shown as %q", tc.in, back)
			}
		}
	}
}
