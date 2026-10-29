// datatools_test.go tests Text to Columns, date-order parsing and Remove
// Duplicates.

package edit

import (
	"reflect"
	"testing"

	"officelite/excel/model"
)

// TestSplitFields checks delimited, qualified and fixed-width splitting.
func TestSplitFields(t *testing.T) {
	csv := TextToColsSpec{Delims: ",", Qualifier: '"'}
	for _, tc := range []struct {
		in   string
		spec TextToColsSpec
		want []string
	}{
		{"a,b,c", csv, []string{"a", "b", "c"}},
		{`"Smith, J",42,"say ""hi"""`, csv, []string{"Smith, J", "42", `say "hi"`}},
		{"a,,b", csv, []string{"a", "", "b"}},
		{"a,,b,", TextToColsSpec{Delims: ",", Consecutive: true}, []string{"a", "b"}},
		{"a; b\tc", TextToColsSpec{Delims: ";\t"}, []string{"a", " b", "c"}},
		{"'x y' z", TextToColsSpec{Delims: " ", Qualifier: '\''}, []string{"x y", "z"}},
		{`"x,y"`, TextToColsSpec{Delims: ","}, []string{`"x`, `y"`}},
		{"a|b", TextToColsSpec{Delims: "|"}, []string{"a", "b"}},
		{"ABC  123 xyz", TextToColsSpec{Fixed: true, Breaks: []int{5, 3}}, []string{"ABC", "", "123 xyz"}},
		{"1234567", TextToColsSpec{Fixed: true, Breaks: []int{2, 4}}, []string{"12", "34", "567"}},
		{"12", TextToColsSpec{Fixed: true, Breaks: []int{4}}, []string{"12"}},
	} {
		if got := SplitFields(tc.in, tc.spec); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestParseDateOrder checks dates read in each order, and impossible ones
// refused.
func TestParseDateOrder(t *testing.T) {
	for _, tc := range []struct {
		in   string
		f    ColFormat
		want float64
		ok   bool
	}{
		{"3/15/2024", ColDateMDY, 45366, true},
		{"15/3/2024", ColDateDMY, 45366, true},
		{"15.03.24", ColDateDMY, 45366, true},
		{"2024-03-15", ColDateYMD, 45366, true},
		{"2024-15-03", ColDateYDM, 45366, true},
		{"15/3/2024", ColDateMDY, 0, false},
		{"31/4/2024", ColDateDMY, 0, false},
		{"hello", ColDateDMY, 0, false},
	} {
		got, ok := ParseDateOrder(tc.in, tc.f, false)
		if ok != tc.ok || ok && got != tc.want {
			t.Errorf("%q: %v %v, want %v %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestTextToColumns checks TextToColumns' formats, overwrite check, undo
// and a fixed-width split to another destination.
func TestTextToColumns(t *testing.T) {
	b := newBook()
	b.enter(t, 0, "A1", "Name,Qty,Date,Code,Note")
	b.enter(t, 0, "A2", `"Smith, J","1,500",15/3/2024,007,x`)
	b.enter(t, 0, "A3", "Lee,12.5,1/2/2023,042,y")
	b.enter(t, 0, "D5", "keep")
	spec := TextToColsSpec{Delims: ",", Qualifier: '"',
		Formats: []ColFormat{ColGeneral, ColGeneral, ColDateDMY, ColText, ColSkip}}
	if _, err := b.TextToColumns(0, rng("A1", "B3"), 0, 0, spec); err != ErrTextToColsShape {
		t.Fatalf("two columns: %v", err)
	}
	if b.TextToColumnsOverwrites(0, rng("A1", "A3"), 0, 0, spec) {
		t.Error("nothing to overwrite")
	}
	if !b.TextToColumnsOverwrites(0, rng("A1", "A3"), 2, 0, spec) {
		t.Error("D5 would be overwritten")
	}
	if _, err := b.TextToColumns(0, rng("A1", "A3"), 0, 0, spec); err != nil {
		t.Fatal(err)
	}
	b.expect(t, "split", map[string]string{
		"A1": "Name", "B1": "Qty", "C1": "Date", "D1": "Code", "E1": "",
		"A2": "Smith, J", "B2": "1,500", "C2": "3/15/2024", "D2": "007",
		"A3": "Lee", "B3": "12.5", "C3": "2/1/2023", "D3": "042",
	})
	if c := b.cell(0, "B2"); c.Kind != model.KindNumber || c.Num != 1500 {
		t.Errorf("B2 %+v", c)
	}
	if c := b.cell(0, "D2"); c.Kind != model.KindText || b.style(c.Style).NumFmtCode != "@" {
		t.Errorf("D2 %+v", c)
	}
	if c := b.cell(0, "C3"); c.Kind != model.KindNumber || c.Num != 44958 {
		t.Errorf("C3 %+v", c)
	}
	b.Undo()
	b.expect(t, "undo", map[string]string{"A2": `"Smith, J","1,500",15/3/2024,007,x`, "B2": "", "C3": ""})

	// To another destination, fixed width.
	b.enter(t, 0, "F1", "AB 12")
	if _, err := b.TextToColumns(0, rng("F1", "F1"), 9, 6, TextToColsSpec{Fixed: true, Breaks: []int{2}}); err != nil {
		t.Fatal(err)
	}
	if b.cell(0, "G10").Text != "AB" || b.cell(0, "H10").Num != 12 || b.cell(0, "F1").Text != "AB 12" {
		t.Errorf("fixed: %q %v %q", b.cell(0, "G10").Text, b.cell(0, "H10").Num, b.cell(0, "F1").Text)
	}
}

// TestRemoveDuplicates checks RemoveDuplicates by all columns, one column
// and without headers, with undo.
func TestRemoveDuplicates(t *testing.T) {
	b := newBook()
	rows := [][]string{
		{"Name", "City", "Qty"},
		{"Ann", "Oslo", "1"},
		{"bob", "Rome", "2"},
		{"ANN", "oslo", "1.0"},
		{"Bob", "Rome", "3"},
		{"Cy", "Oslo", "1"},
	}
	for r, row := range rows {
		for c, v := range row {
			b.enter(t, 0, model.CellName(r, c), v)
		}
	}
	b.enter(t, 0, "D6", "outside")
	b.enter(t, 0, "A8", "below")
	all := rng("A1", "C6")
	if !b.GuessHeader(0, all) {
		t.Error("header not detected")
	}
	removed, remain, _ := b.RemoveDuplicates(0, all, []int{0, 1, 2}, true)
	if removed != 1 || remain != 4 {
		t.Fatalf("all columns: %d removed, %d remain", removed, remain)
	}
	b.expect(t, "all", map[string]string{"A4": "Bob", "A5": "Cy", "A6": "", "C6": "", "D6": "outside", "A8": "below"})
	b.Undo()
	b.expect(t, "undo", map[string]string{"A4": "ANN", "A6": "Cy"})

	// Name only: bob/Bob and Ann/ANN repeat.
	removed, remain, _ = b.RemoveDuplicates(0, all, []int{0}, true)
	if removed != 2 || remain != 3 {
		t.Fatalf("name: %d removed, %d remain", removed, remain)
	}
	b.expect(t, "name", map[string]string{"A2": "Ann", "A3": "bob", "A4": "Cy", "B4": "Oslo", "A5": ""})
	b.Undo()

	// Without headers, City and Qty: the header row takes part.
	removed, remain, _ = b.RemoveDuplicates(0, all, []int{1, 2}, false)
	if removed != 2 || remain != 4 {
		t.Fatalf("city+qty: %d removed, %d remain", removed, remain)
	}
	b.expect(t, "city+qty", map[string]string{"A1": "Name", "A2": "Ann", "A3": "bob", "A4": "Bob", "A5": ""})
	if removed, _, _ := b.RemoveDuplicates(0, rng("A1", "C4"), []int{0, 1, 2}, true); removed != 0 {
		t.Errorf("no duplicates: %d", removed)
	}
}
