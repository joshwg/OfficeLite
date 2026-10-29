package model

import (
	"slices"
	"testing"
)

func TestShadingTargets(t *testing.T) {
	d, e := newDoc("Hello world", "Second", "Third")
	p0 := d.Body[0].(*Paragraph)

	// Part of one paragraph: character shading on the selected text.
	e.Select(Pos{0, 6}, Pos{0, 11})
	if got := e.ShadingTarget(); got != ShadeText {
		t.Fatalf("part of a paragraph: target %v", got)
	}
	e.SetShading("FFC000")
	if len(p0.Runs) != 2 || p0.Runs[0].Props.Shading != "" || p0.Runs[1].Props.Shading != "FFC000" || p0.Props.Shading != "" {
		t.Fatalf("text shading: %+v / %+v", p0.Runs, p0.Props)
	}

	// The caret alone, a whole paragraph, or across paragraphs: paragraph
	// shading.
	e.SetCaret(Pos{1, 2}, false)
	if got := e.ShadingTarget(); got != ShadeParagraph {
		t.Fatalf("caret: target %v", got)
	}
	e.SetShading("DEEAF6")
	if got := e.Para(1).Props.Shading; got != "DEEAF6" {
		t.Errorf("caret paragraph shading = %q", got)
	}
	e.Select(Pos{0, 0}, Pos{0, 11})
	if got := e.ShadingTarget(); got != ShadeParagraph {
		t.Errorf("whole paragraph: target %v", got)
	}
	e.Select(Pos{1, 3}, Pos{2, 2})
	e.SetShading("E2EFDA")
	if e.Para(1).Props.Shading != "E2EFDA" || e.Para(2).Props.Shading != "E2EFDA" || p0.Props.Shading != "" {
		t.Errorf("across paragraphs: %q %q", e.Para(1).Props.Shading, e.Para(2).Props.Shading)
	}

	// No Color removes it; over a shaded style it hides the style's.
	e.SetShading("")
	if e.Para(1).Props.Shading != "" {
		t.Errorf("No Color left %q", e.Para(1).Props.Shading)
	}
	d.Styles.Styles = append(d.Styles.Styles, &Style{ID: "Boxed", Name: "Boxed", Type: ParagraphStyle, Para: ParaProps{Shading: "CCCCCC"}})
	e.Para(2).Style = "Boxed"
	e.SetCaret(Pos{2, 0}, false)
	e.SetShading("")
	if got := d.ResolvePara(e.Para(2)).Shading; got != "auto" {
		t.Errorf("No Color over a shaded style resolves to %q", got)
	}

	// Each command is one undo step named Shading.
	if got := e.UndoLabels(); !slices.Equal(got, []string{"Shading", "Shading", "Shading", "Shading", "Shading"}) {
		t.Errorf("undo labels %q", got)
	}
	for e.CanUndo() {
		e.Undo()
	}
	p0 = d.Body[0].(*Paragraph)
	for i := range 3 {
		if p := e.Para(i); p.Props.Shading != "" {
			t.Errorf("paragraph %d still shaded after undo", i)
		}
	}
	for _, r := range p0.Runs {
		if r.Props.Shading != "" {
			t.Errorf("run %q still shaded after undo", r.Text)
		}
	}
}

func TestShadingCells(t *testing.T) {
	e := typed(t, "Before")
	e.InsertTable(2, 2)
	tb := e.Doc.Body[1].(*Table)
	e.InsertText("A1")
	e.NextCell(1)
	e.InsertText("B1")

	// The caret in a cell, or text inside it: the paragraph or the text.
	e.SetCaret(e.Sel.Caret, false)
	if got := e.ShadingTarget(); got != ShadeParagraph {
		t.Errorf("caret in a cell: target %v", got)
	}

	// A whole cell selected: the cell.
	e.SelectCell()
	if got := e.ShadingTarget(); got != ShadeCells {
		t.Fatalf("whole cell: target %v", got)
	}
	e.SetShading("FFFF00")
	if tb.Rows[0].Cells[1].Shading != "FFFF00" || tb.Rows[0].Cells[0].Shading != "" {
		t.Errorf("cell shading: %q %q", tb.Rows[0].Cells[0].Shading, tb.Rows[0].Cells[1].Shading)
	}
	if p := tb.Rows[0].Cells[1].Blocks[0].(*Paragraph); p.Props.Shading != "" || p.Runs[0].Props.Shading != "" {
		t.Error("whole-cell shading also shaded the paragraph")
	}

	// Across cells: every cell selected.
	e.SelectTable()
	e.SetShading("92D050")
	for r, row := range tb.Rows {
		for c, cell := range row.Cells {
			if cell.Shading != "92D050" {
				t.Errorf("cell %d,%d shading %q", r, c, cell.Shading)
			}
		}
	}
	e.Undo()
	if tb := e.Doc.Body[1].(*Table); tb.Rows[1].Cells[0].Shading != "" || tb.Rows[0].Cells[1].Shading != "FFFF00" {
		t.Errorf("undo cell shading: %q %q", tb.Rows[1].Cells[0].Shading, tb.Rows[0].Cells[1].Shading)
	}
	if got := e.UndoLabels()[0]; got != "Shading" {
		t.Errorf("undo label %q", got)
	}
}
