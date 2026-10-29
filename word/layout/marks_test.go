package layout

import (
	"math"
	"testing"

	"officelite/common/pdf"
	"officelite/word/model"
)

func TestMarks(t *testing.T) {
	text := &model.Paragraph{Runs: []model.Run{{Text: "ab c d"}, {Kind: model.RunTab}, {Text: "e"}, {Kind: model.RunBreak}, {Text: "f"}}}
	empty := para("")
	brk := &model.Paragraph{Runs: []model.Run{{Text: "g"}, {Kind: model.RunPageBreak}}}
	cell := func(s string) *model.TableCell { return &model.TableCell{Blocks: []model.Block{para("x"), para(s)}} }
	tbl := &model.Table{Columns: []float64{100, 100}, Borders: true, Rows: []*model.TableRow{{Cells: []*model.TableCell{cell("y"), cell("")}}}}
	e := layoutDoc(docOf(text, empty, tbl, brk))
	pg := e.Pages[0]
	marks := e.Marks(pg.Lines, pg.TableRows)
	count := map[MarkKind]int{}
	for _, m := range marks {
		count[m.Kind]++
	}
	// ¶ ends the two body paragraphs and the first paragraph of each cell
	// (the second gets the end-of-cell mark); the page-break paragraph's
	// ¶ is on the next page.
	want := map[MarkKind]int{MarkPara: 4, MarkCell: 2, MarkRow: 1, MarkSpace: 1, MarkNBSP: 1, MarkTab: 1, MarkBreak: 1, MarkPageBreak: 1}
	for k, n := range want {
		if count[k] != n {
			t.Errorf("kind %d: %d marks, want %d (%+v)", k, count[k], n, count)
		}
	}
	if n := len(e.Marks(e.Pages[1].Lines, nil)); n != 1 {
		t.Errorf("page 2: %d marks", n)
	}

	// Positions on the first line: the space and the NBSP are the third
	// and fifth characters, the tab follows the first word.
	pl := &pg.Lines[0]
	items := pl.Line.Items
	runeX := func(off int) (x, w float64) {
		for _, it := range items {
			if off >= it.Off && off < it.Off+it.N {
				x = pl.X + it.X
				for _, a := range it.Adv[:off-it.Off] {
					x += float64(a)
				}
				return x, float64(it.Adv[off-it.Off])
			}
		}
		t.Fatalf("no item at %d", off)
		return
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.01 }
	base := pl.Y + pl.Line.Baseline
	for _, m := range marks {
		switch m.Kind {
		case MarkSpace, MarkNBSP:
			off := 2
			if m.Kind == MarkNBSP {
				off = 4
			}
			x, w := runeX(off)
			if !near(m.X, x) || !near(m.W, w) || !near(m.Base, base) || w <= 0 {
				t.Errorf("mark %d at %v+%v (base %v), want %v+%v (%v)", m.Kind, m.X, m.W, m.Base, x, w, base)
			}
		case MarkTab:
			x, w := runeX(6)
			if !near(m.X, x) || !near(m.W, w) || w <= 0 {
				t.Errorf("tab mark %v+%v, want %v+%v", m.X, m.W, x, w)
			}
		case MarkBreak:
			x, _ := runeX(8)
			if !near(m.X, x) {
				t.Errorf("break mark at %v, want %v", m.X, x)
			}
		}
	}
	// The ¶ follows the last character: "f" on the second line; the
	// empty paragraph's is at its text's start.
	var paras []Mark
	for _, m := range marks {
		if m.Kind == MarkPara {
			paras = append(paras, m)
		}
	}
	l2 := &pg.Lines[1]
	last := l2.Line.Items[len(l2.Line.Items)-1]
	if x := l2.X + last.X + last.W; !near(paras[0].X, x) || !near(paras[0].Base, l2.Y+l2.Line.Baseline) {
		t.Errorf("¶ at %v,%v, want %v,%v", paras[0].X, paras[0].Base, x, l2.Y+l2.Line.Baseline)
	}
	if l3 := &pg.Lines[2]; !near(paras[1].X, l3.X+l3.Line.X0) || paras[1].Base <= paras[0].Base {
		t.Errorf("empty paragraph's ¶ at %v,%v", paras[1].X, paras[1].Base)
	}
	// The row mark is right of the table.
	for _, m := range marks {
		if m.Kind == MarkRow {
			if r := pg.TableRows[0]; m.X <= r.Edges[len(r.Edges)-1].X || m.Base < r.Y0 || m.Base > r.Y1 {
				t.Errorf("row mark at %v,%v; row %+v", m.X, m.Base, r)
			}
		}
	}
}

func TestShowHidden(t *testing.T) {
	p := &model.Paragraph{Runs: []model.Run{{Text: "seen "}, {Text: "secret ", Props: model.RunProps{Hidden: model.On}}, {Text: "end"}}}
	e := layoutDoc(docOf(p))
	width := func() float64 { return e.cache[p].Lines[0].Width }
	w0 := width()
	pg := e.Pages[0]
	for _, m := range e.Marks(pg.Lines, nil) {
		if m.Kind == MarkHidden {
			t.Error("hidden text marked while not shown")
		}
	}
	e.SetShowHidden(true)
	e.Layout()
	if width() <= w0 {
		t.Errorf("shown hidden text takes no room: %v <= %v", width(), w0)
	}
	n := 0
	for _, m := range e.Marks(e.Pages[0].Lines, nil) {
		if m.Kind == MarkHidden {
			n++
			if m.W <= 0 {
				t.Errorf("hidden mark width %v", m.W)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d hidden marks", n)
	}
	// PDF export lays out without the hidden text and restores it after.
	e.ExportPDF(pdf.New())
	if !e.ShowHidden() || width() <= w0 {
		t.Errorf("after export: shown %v, width %v", e.ShowHidden(), width())
	}
	e.SetShowHidden(false)
	e.Layout()
	if width() != w0 {
		t.Errorf("width after hiding %v, want %v", width(), w0)
	}
}
