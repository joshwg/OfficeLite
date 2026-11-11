// tallline_test.go checks that a line taller than the page's body is placed
// on one page and clipped, as Word does, rather than continued over as many
// pages as its height spans.

package layout

import (
	"strings"
	"testing"
	"time"

	"officelite/word/model"
)

// TestLineTallerThanPage lays out lines far taller than a page — a huge
// exact line height, in the body and in a table cell, and 1,638 pt text in
// a cell one character wide — and checks that each takes one page, that
// every line is still placed once, and that the text after them follows.
func TestLineTallerThanPage(t *testing.T) {
	exactPara := func() *model.Paragraph {
		p := para("one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty")
		p.Props.Line, p.Props.LineRule = 1e4, model.LineExact
		return p
	}
	cellTable := func(p *model.Paragraph, w float64) *model.Table {
		return &model.Table{Columns: []float64{w, 200}, Borders: true, Rows: []*model.TableRow{{Cells: []*model.TableCell{
			{Blocks: []model.Block{p}}, {Blocks: []model.Block{para("beside")}}}}}}
	}
	exact, exactInCell := exactPara(), exactPara()
	huge := para(strings.Repeat("W", 6))
	huge.Runs[0].Props.Size = 1638
	for _, tc := range []struct {
		name  string
		block model.Block
		tall  *model.Paragraph // the paragraph whose lines are taller than a page
	}{
		{"exact line height", exact, exact},
		{"exact line height in a cell", cellTable(exactInCell, 100), exactInCell},
		{"huge font in a narrow cell", cellTable(huge, 20), huge},
	} {
		start := time.Now()
		e := layoutDoc(docOf(para("before"), tc.block, para("after")))
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s: layout took %v", tc.name, d)
		}
		n := len(e.cache[tc.tall].Lines)
		if n < 2 {
			t.Fatalf("%s: %d lines, want several", tc.name, n)
		}
		// One page per tall line, plus the first page ("before") and maybe
		// one for "after".
		if got := len(e.Pages); got > n+2 {
			t.Errorf("%s: %d pages for %d tall lines, want at most %d", tc.name, got, n, n+2)
		}
		if got := len(e.paraLines[1]); got != n {
			t.Errorf("%s: %d of %d tall lines placed", tc.name, got, n)
		}
		last := len(e.paraLines) - 1
		if refs := e.paraLines[last]; len(refs) != 1 || refs[0].Page != len(e.Pages)-1 {
			t.Errorf("%s: \"after\" placed at %v, want once on the last page", tc.name, refs)
		}
	}
}
