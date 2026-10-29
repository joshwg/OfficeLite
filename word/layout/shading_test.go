package layout

import (
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"

	"officelite/common/colors"
	"officelite/word/model"
)

// fillsOf returns the page's fills of color c, top to bottom.
func fillsOf(pg *Page, c color.NRGBA) []Fill {
	var out []Fill
	for _, f := range pg.Fills {
		if f.C == c {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b Fill) int { return int(math.Copysign(1, a.R.Y0-b.R.Y0)) })
	return out
}

func TestShadingFills(t *testing.T) {
	blue, gold, green := colors.RGB(0xDEEAF6), colors.RGB(0xFFC000), colors.RGB(0x92D050)
	shaded := func(s, hex string, indent float64) *model.Paragraph {
		p := para(s)
		p.Props.Shading = hex
		p.Props.SpaceAfter = model.F(8)
		if indent > 0 {
			p.Props.IndentLeft = model.F(indent)
		}
		return p
	}
	runShaded := &model.Paragraph{Runs: []model.Run{{Text: "plain "}, {Text: "shaded", Props: model.RunProps{Shading: "FFC000"}}, {Text: " both", Props: model.RunProps{Shading: "FFC000", Highlight: "yellow"}}}}
	cell := &model.TableCell{Shading: "92D050", Blocks: []model.Block{para("cell")}}
	tbl := &model.Table{Columns: []float64{100, 100}, Rows: []*model.TableRow{
		{Cells: []*model.TableCell{{Blocks: []model.Block{para("plain")}}, cell}},
	}}
	d := docOf(para("top"), shaded("one", "DEEAF6", 0), shaded("two", "DEEAF6", 0), shaded("three", "DEEAF6", 36), runShaded, tbl, para(""))
	e := layoutDoc(d)
	pg := e.Pages[0]
	left, right := e.textLeft(), e.textLeft()+e.textWidth()

	// Paragraph shading spans the text column between the indents, and
	// the paragraphs "one" and "two" (same shading and indents) make one
	// block: the space after "one" is shaded too.
	fs := fillsOf(pg, blue)
	if len(fs) != 3 {
		t.Fatalf("%d blue fills: %+v", len(fs), fs)
	}
	one, two, three := e.line(e.paraLines[1][0]), e.line(e.paraLines[2][0]), e.line(e.paraLines[3][0])
	if fs[0].R.X0 != left || fs[0].R.X1 != right || fs[0].R.Y0 != one.Y {
		t.Errorf("first fill %+v, want x %v..%v from y %v", fs[0].R, left, right, one.Y)
	}
	if fs[0].R.Y1 != two.Y || fs[1].R.Y0 != two.Y {
		t.Errorf("gap between grouped paragraphs: %v..%v, second line at %v", fs[0].R.Y1, fs[1].R.Y0, two.Y)
	}
	// "three" is indented: its own block, the gap above it unshaded.
	if fs[1].R.Y1 != two.Y+two.Line.Height {
		t.Errorf("shading below 'two' runs into the gap: %v, line ends %v", fs[1].R.Y1, two.Y+two.Line.Height)
	}
	if fs[2].R.X0 != left+36 || fs[2].R.Y0 != three.Y {
		t.Errorf("indented fill %+v, want x0 %v y0 %v", fs[2].R, left+36, three.Y)
	}

	// Character shading: the shaded runs' items carry the color (drawn
	// behind them like highlighting); highlighting wins where both are set.
	l := e.line(e.paraLines[4][0])
	got := map[string]color.NRGBA{}
	for _, it := range l.Line.Items {
		if it.St != nil {
			got[strings.TrimSpace(it.Text)] = it.St.Highlight
		}
	}
	if got["shaded"] != gold || got["plain"].A != 0 {
		t.Errorf("run shading: %v", got)
	}
	if got["both"] == gold || got["both"].A == 0 {
		t.Errorf("highlight over shading: %v", got["both"])
	}

	// Cell shading fills the second cell.
	cf := fillsOf(pg, green)
	if len(cf) != 1 || cf[0].R.X0 != left+100 || cf[0].R.X1 != left+200 {
		t.Errorf("cell fill %+v, want x %v..%v", cf, left+100, left+200)
	}
}
