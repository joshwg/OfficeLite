package layout

import (
	"math"
	"strings"
	"testing"
	"time"

	"gioui.org/font/gofont"
	"gioui.org/text"

	"officelite/word/model"
)

func testShaper() *text.Shaper {
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
}

func para(s string) *model.Paragraph {
	p := &model.Paragraph{}
	if s != "" {
		p.Runs = []model.Run{{Text: s}}
	}
	return p
}

func docOf(blocks ...model.Block) *model.Document {
	d := model.New("t")
	d.Body = blocks
	return d
}

func layoutDoc(d *model.Document) *Engine {
	e := New(d, testShaper())
	e.Layout()
	return e
}

const lorem = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat."

func TestLineBreaking(t *testing.T) {
	d := docOf(para(lorem))
	e := layoutDoc(d)
	pl := e.cache[d.Body[0].(*model.Paragraph)]
	if len(pl.Lines) < 2 {
		t.Fatalf("expected wrapping, got %d lines", len(pl.Lines))
	}
	width := e.textWidth()
	rs := []rune(lorem)
	next := 0
	for i, l := range pl.Lines {
		if l.Start != next {
			t.Errorf("line %d starts at %d, want %d", i, l.Start, next)
		}
		next = l.End
		if l.Width > width+0.01 {
			t.Errorf("line %d too wide: %v > %v", i, l.Width, width)
		}
		// Lines break after spaces, never inside a word.
		if !l.Last && rs[l.End-1] != ' ' {
			t.Errorf("line %d ends inside a word: %q", i, string(rs[l.Start:l.End]))
		}
	}
	if next != len(rs) || !pl.Lines[len(pl.Lines)-1].Last {
		t.Errorf("lines end at %d of %d", next, len(rs))
	}
}

func TestLongWordAndHyphen(t *testing.T) {
	long := strings.Repeat("x", 400)
	d := docOf(para(long), para("well-known "+strings.Repeat("word ", 30)))
	e := layoutDoc(d)
	pl := e.cache[d.Body[0].(*model.Paragraph)]
	if len(pl.Lines) < 3 {
		t.Fatalf("long word in %d lines", len(pl.Lines))
	}
	for i, l := range pl.Lines {
		if l.Width > e.textWidth()+0.01 || l.End <= l.Start {
			t.Errorf("line %d: width %v, %d..%d", i, l.Width, l.Start, l.End)
		}
	}
	if got := splitWords("well-known a  b"); strings.Join(got, "|") != "well-|known |a  |b" {
		t.Errorf("splitWords = %q", got)
	}
}

func TestAlignment(t *testing.T) {
	mk := func(a model.Align, s string) *model.Paragraph {
		p := para(s)
		p.Props.Align = a
		return p
	}
	d := docOf(mk(model.AlignCenter, "centered"), mk(model.AlignRight, "right"), mk(model.AlignJustify, lorem))
	e := layoutDoc(d)
	w := e.textWidth()
	c := e.cache[d.Body[0].(*model.Paragraph)].Lines[0]
	if mid := c.X0 + (c.Width-c.X0)/2; math.Abs(mid-w/2) > 0.5 {
		t.Errorf("center: text middle at %v, want %v", mid, w/2)
	}
	r := e.cache[d.Body[1].(*model.Paragraph)].Lines[0]
	if math.Abs(r.Width-w) > 0.01 {
		t.Errorf("right: ends at %v, want %v", r.Width, w)
	}
	j := e.cache[d.Body[2].(*model.Paragraph)]
	if math.Abs(j.Lines[0].Width-w) > 0.01 {
		t.Errorf("justify: first line ends at %v, want %v", j.Lines[0].Width, w)
	}
	last := j.Lines[len(j.Lines)-1]
	if last.Width >= w-1 {
		t.Errorf("justify: last line should not stretch (%v)", last.Width)
	}
}

func TestTabs(t *testing.T) {
	plain := &model.Paragraph{Runs: []model.Run{{Text: "a"}, {Kind: model.RunTab}, {Text: "b"}}}
	// Word ignores default stops left of a custom stop.
	p := &model.Paragraph{Runs: []model.Run{{Text: "a"}, {Kind: model.RunTab}, {Text: "right"}, {Kind: model.RunTab}, {Text: "c"}}}
	p.Props.Tabs = []model.TabStop{{Pos: 300, Align: model.AlignRight}}
	d := docOf(plain, p)
	e := layoutDoc(d)
	if x := e.cache[plain].Lines[0].Items[2].X; x != 36 {
		t.Errorf("default tab: b at %v, want 36", x)
	}
	items := e.cache[p].Lines[0].Items
	if end := items[2].X + items[2].W; math.Abs(end-300) > 0.01 {
		t.Errorf("right tab: text ends at %v, want 300", end)
	}
	if x := items[4].X; x != 324 {
		t.Errorf("default stop after the custom one: %v, want 324", x)
	}
	// Hanging indent acts as a tab stop.
	q := &model.Paragraph{Runs: []model.Run{{Text: "1."}, {Kind: model.RunTab}, {Text: "item"}}}
	q.Props.IndentLeft, q.Props.IndentFirst = model.F(54), model.F(-18)
	d2 := docOf(q)
	e2 := layoutDoc(d2)
	if it := e2.cache[q].Lines[0].Items[2]; it.X != 54 {
		t.Errorf("hanging tab: item at %v, want 54", it.X)
	}
}

func TestListNumbering(t *testing.T) {
	d := model.New("t")
	d.Body = nil
	for _, s := range []string{"one", "two", "sub a", "sub b", "three"} {
		d.Body = append(d.Body, para(s))
	}
	ed := model.NewEditor(d)
	ed.SelectAll()
	ed.ToggleList(false)
	ed.Select(model.Pos{Para: 2}, model.Pos{Para: 3, Off: 1})
	ed.Indent(1)
	e := layoutDoc(d)
	var got []string
	for _, b := range d.Body {
		got = append(got, e.labels[b.(*model.Paragraph)])
	}
	if strings.Join(got, " ") != "1. 2. a. b. 3." {
		t.Errorf("labels %q", got)
	}
	// The text of a list paragraph starts at the level's indent.
	l := e.cache[d.Body[0].(*model.Paragraph)].Lines[0]
	if l.Label == nil || l.Label.X != 18 || l.Items[0].X != 36 {
		t.Errorf("label at %v, text at %v", l.Label, l.Items[0].X)
	}
	for n, want := range map[int]string{1: "i", 4: "iv", 9: "ix", 14: "xiv", 1994: "mcmxciv"} {
		if got := model.FormatNumber(n, "lowerRoman"); got != want {
			t.Errorf("roman %d = %q", n, got)
		}
	}
	if got := model.FormatNumber(28, "upperLetter"); got != "BB" {
		t.Errorf("letter 28 = %q", got)
	}
}

func TestPagination(t *testing.T) {
	var blocks []model.Block
	for range 40 {
		blocks = append(blocks, para(lorem))
	}
	pb := &model.Paragraph{Runs: []model.Run{{Text: "before"}, {Kind: model.RunPageBreak}, {Text: "after"}}}
	blocks = append(blocks, pb)
	h := para("Heading")
	h.Style = "Heading1" // keep with next
	blocks = append(blocks, h, para(lorem))
	d := docOf(blocks...)
	e := layoutDoc(d)
	if len(e.Pages) < 3 {
		t.Fatalf("%d pages", len(e.Pages))
	}
	for _, pg := range e.Pages {
		for _, pl := range pg.Lines {
			if pl.Y+pl.Line.Height > e.textBottom()+0.01 {
				t.Errorf("page %d: line below the margin (%v)", pg.Index, pl.Y+pl.Line.Height)
			}
		}
	}
	// "after" starts a new page.
	refs := e.paraLines[40]
	if len(refs) != 2 || refs[0].Page == refs[1].Page {
		t.Fatalf("page break: %v", refs)
	}
	if e.line(refs[1]).Y != e.textTop() {
		t.Errorf("after page break: y %v", e.line(refs[1]).Y)
	}
	// The heading is on the same page as the next paragraph's first line.
	if e.paraLines[41][0].Page != e.paraLines[42][0].Page {
		t.Errorf("keep with next: heading page %d, next %d", e.paraLines[41][0].Page, e.paraLines[42][0].Page)
	}
	// Page positions in the view.
	if e.Pages[1].Top != e.Pages[0].Top+e.Pages[0].Height+PageGap {
		t.Error("page stacking")
	}
}

func TestTable(t *testing.T) {
	cell := func(s string) *model.TableCell {
		return &model.TableCell{Blocks: []model.Block{para(s)}}
	}
	tall := cell(lorem)
	merged := cell("merged")
	merged.VMerge = 1
	cont := &model.TableCell{VMerge: 2, Blocks: []model.Block{para("")}}
	tbl := &model.Table{Columns: []float64{100, 200, 100}, Borders: true, Rows: []*model.TableRow{
		{Cells: []*model.TableCell{cell("a"), tall, merged}},
		{Cells: []*model.TableCell{cell("b"), cell("c"), cont}},
		{Cells: []*model.TableCell{{Span: 2, Blocks: []model.Block{para("wide")}}, cell("z")}, Height: 50},
	}}
	d := docOf(para("x"), tbl, para("y"))
	e := layoutDoc(d)
	rows, _ := e.layoutTable(tbl, e.textWidth())
	if rows[0].height <= rows[1].height {
		t.Errorf("tall cell row %v vs %v", rows[0].height, rows[1].height)
	}
	if rows[2].height != 50 {
		t.Errorf("row height %v", rows[2].height)
	}
	if rows[0].cells[2].rowSpan != 2 || !rows[1].cells[2].cont {
		t.Errorf("vertical merge: span %d", rows[0].cells[2].rowSpan)
	}
	if w := rows[2].cells[0].w; w != 300 {
		t.Errorf("span width %v", w)
	}
	// Cell text starts inside the cell margin and wraps within the column.
	pc := e.paraLines[2] // the tall cell
	if len(pc) < 3 {
		t.Fatalf("tall cell lines %d", len(pc))
	}
	l := e.line(pc[0])
	if l.X != e.textLeft()+100+cellMargin || l.W != 200-2*cellMargin {
		t.Errorf("cell line at %v width %v", l.X, l.W)
	}
	if len(e.Pages[0].Rules) == 0 {
		t.Error("no borders")
	}
	// "y" comes after the whole table.
	y := e.line(e.paraLines[e.ParaCount()-1][0])
	if y.Y < e.textTop()+rows[0].height+rows[1].height+rows[2].height {
		t.Errorf("paragraph after table at %v", y.Y)
	}
	// Hit-testing inside the second column finds the tall cell.
	pos := e.HitTest(0, l.X+10, l.Y+2)
	if pos.Para != 2 {
		t.Errorf("hit %v", pos)
	}
}

func TestCaretAndHit(t *testing.T) {
	d := docOf(para(lorem), para("second"))
	e := layoutDoc(d)
	for _, off := range []int{0, 5, 60, 100, len([]rune(lorem))} {
		c, ok := e.CaretAt(model.Pos{Para: 0, Off: off})
		if !ok {
			t.Fatal("no caret")
		}
		got := e.HitTest(c.Page, c.X, c.Y+c.H/2)
		if got.Para != 0 || got.Off != off {
			// A caret at a wrapped line's end shows at the next line's start.
			if pl, _ := e.lineFor(model.Pos{Off: off}); pl.Line.Start != off {
				t.Errorf("offset %d: caret %+v hits %v", off, c, got)
			}
		}
	}
	p := model.Pos{Para: 0, Off: 3}
	c, _ := e.CaretAt(p)
	down, _, ok := e.Vertical(p, 1, c.X, false)
	if !ok || down.Para != 0 || down.Off <= 3 {
		t.Errorf("down: %v", down)
	}
	a, b, _ := e.LineBounds(p, false)
	if a.Off != 0 || b.Off <= 3 {
		t.Errorf("line bounds %v %v", a, b)
	}
}

func TestIncremental(t *testing.T) {
	var blocks []model.Block
	for i := range 1500 {
		s := lorem
		if i%10 == 0 {
			s = "Heading"
		}
		p := para(s)
		if i%10 == 0 {
			p.Style = "Heading2"
		}
		blocks = append(blocks, p)
	}
	d := docOf(blocks...)
	ed := model.NewEditor(d)
	e := New(d, testShaper())
	start := time.Now()
	e.Layout()
	full := time.Since(start)
	t.Logf("full layout: %d paragraphs, %d pages in %v", e.ParaCount(), len(e.Pages), full)
	ed.TakeDirty()
	ed.SetCaret(model.Pos{Para: 700, Off: 10}, false)
	var worst time.Duration
	for _, c := range "typing" {
		ed.InsertText(string(c))
		ps, all := ed.TakeDirty()
		e.Invalidate(ps, all)
		start = time.Now()
		e.Layout()
		worst = max(worst, time.Since(start))
		if e.LastRelaid != 1 {
			t.Errorf("relaid %d paragraphs after typing", e.LastRelaid)
		}
	}
	t.Logf("incremental layout after a keystroke: %v (worst)", worst)
	if worst > full/3 && worst > 20*time.Millisecond {
		t.Errorf("incremental layout too slow: %v vs full %v", worst, full)
	}
}

// Lists that share an abstract definition count together unless one
// overrides its start; a list may start at 0.
func TestListCounters(t *testing.T) {
	lvl := func(start int, restart bool) []model.ListLevel {
		return []model.ListLevel{{Format: "decimal", Text: "%1.", Start: start, Restart: restart, Indent: 36, Hang: 18}}
	}
	item := func(num int) *model.Paragraph {
		p := para("item")
		p.Props.List = &model.ListRef{NumID: num}
		return p
	}
	d := docOf(item(1), item(1), item(2), item(3), item(3), item(4))
	d.Lists = map[int]*model.List{
		1: {Abstract: "0", Levels: lvl(1, false)},
		2: {Abstract: "0", Levels: lvl(1, false)}, // continues list 1
		3: {Abstract: "0", Levels: lvl(1, true)},  // restarts at 1
		4: {Levels: lvl(0, false)},                // starts at 0
	}
	e := layoutDoc(d)
	var got []string
	for _, b := range d.Body {
		got = append(got, e.labels[b.(*model.Paragraph)])
	}
	if strings.Join(got, " ") != "1. 2. 3. 1. 2. 0." {
		t.Errorf("labels %q", got)
	}
	if model.FormatNumber(0, "lowerRoman") != "0" || model.FormatNumber(5000, "upperRoman") != "5000" || model.FormatNumber(-2, "lowerLetter") != "-2" {
		t.Errorf("out-of-range numbers")
	}
}

// A row (here a vertically merged cell) taller than the rest of the page
// is split between pages: no line runs past the page bottom.
func TestTableRowSplitsAcrossPages(t *testing.T) {
	long := strings.Repeat(lorem+" ", 40)
	for _, merged := range []bool{false, true} {
		big := &model.TableCell{Blocks: []model.Block{para(long)}}
		rows := []*model.TableRow{{Cells: []*model.TableCell{{Blocks: []model.Block{para("a")}}, big}}}
		if merged {
			big.VMerge = 1
			rows = append(rows, &model.TableRow{Cells: []*model.TableCell{{Blocks: []model.Block{para("b")}},
				{VMerge: 2, Blocks: []model.Block{para("")}}}})
		}
		tbl := &model.Table{Columns: []float64{100, 300}, Borders: true, Rows: rows}
		var blocks []model.Block
		for range 10 {
			blocks = append(blocks, para("x"))
		}
		blocks = append(blocks, tbl, para("after"))
		e := layoutDoc(docOf(blocks...))
		if len(e.Pages) < 2 {
			t.Fatalf("merged=%v: %d pages", merged, len(e.Pages))
		}
		for _, pg := range e.Pages {
			for _, pl := range pg.Lines {
				if pl.Y+pl.Line.Height > e.textBottom()+0.01 {
					t.Fatalf("merged=%v: line of paragraph %d on page %d ends at %v, below %v", merged, pl.Para, pg.Index, pl.Y+pl.Line.Height, e.textBottom())
				}
			}
			for _, r := range pg.Rules {
				if r.Y1 > e.textBottom()+1 {
					t.Fatalf("merged=%v: border at %v on page %d", merged, r.Y1, pg.Index)
				}
			}
		}
		// The big cell starts on the first page, right after the "x"s.
		if ref := e.paraLines[11][0]; ref.Page != 0 {
			t.Errorf("merged=%v: cell starts on page %d", merged, ref.Page)
		}
		// Every line of the cell is placed once.
		n := len(e.cache[big.Blocks[0].(*model.Paragraph)].Lines)
		if got := len(e.paraLines[11]); got != n {
			t.Errorf("merged=%v: %d of %d lines placed", merged, got, n)
		}
	}
	// A row that can't split moves to the next page whole.
	cell := &model.TableCell{Blocks: []model.Block{para(strings.Repeat(lorem+" ", 3))}}
	tbl := &model.Table{Columns: []float64{400}, Rows: []*model.TableRow{{CantSplit: true, Cells: []*model.TableCell{cell}}}}
	var blocks []model.Block
	for range 44 {
		blocks = append(blocks, para("x"))
	}
	e := layoutDoc(docOf(append(blocks, tbl, para("y"))...))
	refs := e.paraLines[44]
	if refs[0].Page != refs[len(refs)-1].Page {
		t.Errorf("can't-split row on pages %d..%d", refs[0].Page, refs[len(refs)-1].Page)
	}
}

// Up and Down in a table go to the cell above or below, not the next cell
// in the row.
func TestVerticalInTable(t *testing.T) {
	cell := func(s string) *model.TableCell { return &model.TableCell{Blocks: []model.Block{para(s)}} }
	tbl := &model.Table{Columns: []float64{150, 150}, Rows: []*model.TableRow{
		{Cells: []*model.TableCell{cell("A"), cell("B")}},
		{Cells: []*model.TableCell{cell("C"), cell("D")}},
	}}
	e := layoutDoc(docOf(para("top"), tbl, para("end")))
	for _, tc := range []struct {
		from, dir, want int
	}{{1, 1, 3}, {2, 1, 4}, {3, -1, 1}, {4, -1, 2}, {3, 1, 5}, {1, -1, 0}} {
		p := model.Pos{Para: tc.from}
		c, _ := e.CaretAt(p)
		got, _, ok := e.Vertical(p, tc.dir, c.X, false)
		if !ok || got.Para != tc.want {
			t.Errorf("from %d dir %d: %v, want paragraph %d", tc.from, tc.dir, got, tc.want)
		}
	}
	if _, _, ok := e.Vertical(model.Pos{Para: 5}, 1, 80, false); ok {
		t.Error("down from the last line")
	}
}

// End on a line wrapped after a hyphen (or inside a long word) goes to the
// line's very end, shown at the end of that line.
func TestEndOfHyphenatedLine(t *testing.T) {
	text := strings.Repeat("abcdefghij-", 30)
	d := docOf(para(text))
	e := layoutDoc(d)
	l := e.cache[d.Body[0].(*model.Paragraph)].Lines[0]
	if l.Last || []rune(text)[l.End-1] != '-' {
		t.Fatalf("first line ends at %d", l.End)
	}
	_, end, eol := e.LineBounds(model.Pos{Off: 2}, false)
	if end.Off != l.End || !eol {
		t.Fatalf("End: %v eol %v, want %d", end, eol, l.End)
	}
	c, _ := e.CaretAtEOL(end, true)
	first, _ := e.CaretAt(model.Pos{Off: 0})
	if c.Y != first.Y || c.X < first.X+l.Width-1 {
		t.Errorf("caret at %+v, line starts at %+v width %v", c, first, l.Width)
	}
	// End again stays; Home from there goes to this line's start.
	if a, b, _ := e.LineBounds(end, true); a.Off != 0 || b != end {
		t.Errorf("again: %v %v", a, b)
	}
	// Without affinity the same offset is the next line's start.
	if a, _, _ := e.LineBounds(end, false); a.Off != l.End {
		t.Errorf("next line start %v", a)
	}
	// A line ending in a space still stops before it.
	d = docOf(para(lorem))
	e = layoutDoc(d)
	l = e.cache[d.Body[0].(*model.Paragraph)].Lines[0]
	if _, end, eol := e.LineBounds(model.Pos{}, false); end.Off != l.End-1 || eol {
		t.Errorf("space-wrapped End %v eol %v (line end %d)", end, eol, l.End)
	}
}
