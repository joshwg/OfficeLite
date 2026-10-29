// nested_test.go tests deleting a selection that crosses the cells of a
// table nested in a table cell.

package model

import (
	"strings"
	"testing"
)

// TestDeleteAcrossNestedTable checks that a selection from one container
// into another clears the text it covers and leaves every table in place,
// while one that starts and ends in the same container removes the tables
// between, as Word does.
func TestDeleteAcrossNestedTable(t *testing.T) {
	para := func(s string) *Paragraph { return &Paragraph{Runs: []Run{{Text: s}}} }
	for _, c := range []struct {
		name string
		s, t Pos
		want string
	}{
		{"outer into nested cell", Pos{1, 1}, Pos{3, 1}, "x|a||2|bb|y"},
		{"nested cell to outer after", Pos{2, 1}, Pos{4, 1}, "x|aa|n||b|y"},
		{"body into nested cell", Pos{0, 0}, Pos{3, 1}, "|||2|bb|y"},
		{"outer before to after (same cell)", Pos{1, 1}, Pos{4, 1}, "x|ab|y"},
	} {
		nested := &Table{Rows: []*TableRow{{Cells: []*TableCell{{Blocks: []Block{para("n1")}}, {Blocks: []Block{para("n2")}}}}}}
		outer := &Table{Rows: []*TableRow{{Cells: []*TableCell{{Blocks: []Block{para("aa"), nested, para("bb")}}}}}}
		d := New("t")
		d.Body = []Block{para("x"), outer, para("y")}
		e := NewEditor(d)
		e.Select(c.s, c.t)
		e.DeleteSelection()
		var got []string
		for _, l := range e.Locs() {
			got = append(got, paraText(l.P))
		}
		if s := strings.Join(got, "|"); s != c.want {
			t.Errorf("%s: %q, want %q", c.name, s, c.want)
		}
	}
}

// paraText is a paragraph's text.
func paraText(p *Paragraph) string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}
