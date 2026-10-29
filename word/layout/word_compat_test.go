package layout

import (
	"math"
	"testing"

	"officelite/common/colors"
	"officelite/word/model"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// Paragraphs of a style with contextual spacing get no space between
// them, but keep it toward paragraphs of other styles.
func TestContextualSpacing(t *testing.T) {
	d := model.New("t")
	d.Styles.Styles = append(d.Styles.Styles, &model.Style{ID: "ListBullet", Name: "List Bullet",
		Type: model.ParagraphStyle, BasedOn: "Normal", Para: model.ParaProps{ContextualSpacing: model.On}})
	item := func(s string) *model.Paragraph {
		p := para(s)
		p.Style = "ListBullet"
		p.Props.SpaceBefore, p.Props.SpaceAfter = model.F(6), model.F(10)
		return p
	}
	intro := para("intro")
	intro.Props.SpaceAfter = model.F(10)
	a, b, after := item("a"), item("b"), para("after")
	d.Body = []model.Block{intro, a, b, after}
	e := layoutDoc(d)
	y := func(i int) float64 { return e.line(e.paraLines[i][0]).Y }
	bottom := func(i int) float64 { return y(i) + e.line(e.paraLines[i][0]).Line.Height }
	if got := y(1) - bottom(0); !near(got, 10+6) {
		t.Errorf("intro to a: %v, want after + before", got)
	}
	if got := y(2) - bottom(1); !near(got, 0) {
		t.Errorf("a to b: %v, want 0", got)
	}
	if got := y(3) - bottom(2); !near(got, 10) {
		t.Errorf("b to after: %v, want b's space after", got)
	}
	// Without it, the spaces add up.
	a.Props.ContextualSpacing, b.Props.ContextualSpacing = model.Off, model.Off
	e.Invalidate([]*model.Paragraph{a, b}, false)
	e.Layout()
	if got := y(2) - bottom(1); !near(got, 16) {
		t.Errorf("a to b without contextual spacing: %v, want 16", got)
	}
}

// A one-cell table with jc=center is centered on its cell width, and the
// cell's own borders are drawn in their width and color, centered on its
// edges; the table has no borders of its own.
func TestCenteredTableCellBorders(t *testing.T) {
	cell := &model.TableCell{Width: 396, Shading: "F4F6F4", Blocks: []model.Block{para("J&A Solar")}}
	green := &model.Border{Width: 0.75, Color: "135A28"}
	cell.Borders = [4]*model.Border{green, {Width: 3, Color: "135A28"}, green, green}
	tbl := &model.Table{Columns: []float64{396}, Align: model.AlignCenter, Rows: []*model.TableRow{{Cells: []*model.TableCell{cell}}}}
	d := docOf(para("x"), tbl, para("y"))
	e := layoutDoc(d)
	pg := e.Pages[0]
	if len(pg.Rules) != 0 {
		t.Errorf("table borders drawn: %v", pg.Rules)
	}
	x0 := e.textLeft() + (e.textWidth()-396)/2
	l := e.line(e.paraLines[1][0])
	if !near(l.X, x0+cellMargin) {
		t.Errorf("cell text at %v, want %v", l.X, x0+cellMargin)
	}
	c, _ := colors.ParseHex("135A28")
	var left, top, shade *Fill
	for i := range pg.Fills {
		f := &pg.Fills[i]
		switch {
		case f.C == c && near(f.R.X1-f.R.X0, 3):
			left = f
		case f.C == c && near(f.R.Y1-f.R.Y0, 0.75):
			if top == nil {
				top = f
			}
		case f.C != c:
			shade = f
		}
	}
	if left == nil || !near(left.R.X0, x0-1.5) || !near(left.R.X1, x0+1.5) {
		t.Fatalf("left border %+v, want 3pt centered on %v", left, x0)
	}
	if top == nil || !near(top.R.X0, x0-1.5) || !near(top.R.X1, x0+396+0.375) {
		t.Errorf("top border %+v", top)
	}
	if shade == nil || !near(shade.R.X0, x0) || !near(shade.R.X1, x0+396) {
		t.Errorf("shading %+v", shade)
	}
	// An edge set to none hides the table's border there.
	tbl.Borders = true
	cell.Borders = [4]*model.Border{{}, nil, nil, nil}
	e.Invalidate(nil, true)
	e.Layout()
	if n := len(e.Pages[0].Rules); n != 3 {
		t.Errorf("%d table border rules, want 3 (top edge none)", n)
	}
}

// Cell paragraphs take Table Grid's single spacing and no space after,
// but a table whose style sets no paragraph spacing keeps Normal's.
func TestTableStyleSpacing(t *testing.T) {
	d := model.New("t")
	d.Styles.DefaultPara = model.ParaProps{SpaceAfter: model.F(8), Line: 1.15, LineRule: model.LineAuto}
	grid := &model.Table{Style: "TableGrid", Columns: []float64{200}, Rows: []*model.TableRow{{Cells: []*model.TableCell{{Blocks: []model.Block{para("a")}}}}}}
	plain := &model.Table{Columns: []float64{200}, Rows: []*model.TableRow{{Cells: []*model.TableCell{{Blocks: []model.Block{para("b")}}}}}}
	d.Body = []model.Block{grid, para(""), plain, para("")}
	e := layoutDoc(d)
	pa := e.cache[grid.Rows[0].Cells[0].Blocks[0].(*model.Paragraph)]
	pb := e.cache[plain.Rows[0].Cells[0].Blocks[0].(*model.Paragraph)]
	if pa.After != 0 || pa.Props.Line != 1 {
		t.Errorf("Table Grid cell: after %v line %v, want 0 and single", pa.After, pa.Props.Line)
	}
	if pb.After != 8 || pb.Props.Line != 1.15 {
		t.Errorf("plain table cell: after %v line %v, want Normal's 8 and 1.15", pb.After, pb.Props.Line)
	}
}

// A Symbol bullet falls back to the Unicode bullet when the shaper has no
// Symbol font.
func TestSymbolBulletFallback(t *testing.T) {
	d := model.New("t")
	d.Lists = map[int]*model.List{1: {Levels: []model.ListLevel{{Format: "bullet", Text: "", Indent: 18, Hang: 18, Font: "Symbol"}}}}
	p := para("item")
	p.Props.List = &model.ListRef{NumID: 1}
	d.Body = []model.Block{p}
	e := layoutDoc(d)
	lb := e.cache[p].Lines[0].Label
	if lb == nil || lb.Text != "•" {
		t.Fatalf("label %+v", lb)
	}
}

// A paragraph's bottom border is drawn its space below the text and takes
// room; neighbors with the same borders share one box.
func TestParagraphBorders(t *testing.T) {
	rule := &model.Border{Width: 1.5, Space: 1, Color: "135A28"}
	ruled := func(s string) *model.Paragraph {
		p := para(s)
		p.Props.Borders[model.EdgeBottom] = rule
		return p
	}
	a, b := ruled("a"), para("b")
	d := docOf(a, b)
	d.Styles.DefaultPara = model.ParaProps{}
	e := layoutDoc(d)
	la := e.line(e.paraLines[0][0])
	lb := e.line(e.paraLines[1][0])
	end := la.Y + la.Line.Height
	if got := lb.Y - end; !near(got, 2.5) {
		t.Errorf("paragraph after the border at +%v, want width+space 2.5", got)
	}
	c, _ := colors.ParseHex("135A28")
	var fills []Fill
	for _, f := range e.Pages[0].Fills {
		if f.C == c {
			fills = append(fills, f)
		}
	}
	if len(fills) != 1 || !near(fills[0].R.Y0, end+1) || !near(fills[0].R.Y1, end+2.5) || !near(fills[0].R.X1-fills[0].R.X0, e.textWidth()) {
		t.Errorf("border fills %+v, want one at %v", fills, end+1)
	}
	// Two paragraphs with the same top and bottom borders: one box.
	a.Props.Borders[model.EdgeTop] = rule
	b.Props.Borders = a.Props.Borders
	e.Invalidate(nil, true)
	e.Layout()
	fills = fills[:0]
	for _, f := range e.Pages[0].Fills {
		if f.C == c {
			fills = append(fills, f)
		}
	}
	la, lb = e.line(e.paraLines[0][0]), e.line(e.paraLines[1][0])
	if len(fills) != 2 || fills[0].R.Y1 > la.Y || fills[1].R.Y0 < lb.Y+lb.Line.Height || !near(lb.Y, la.Y+la.Line.Height) {
		t.Errorf("grouped borders %+v; lines at %v, %v", fills, la.Y, lb.Y)
	}
}
