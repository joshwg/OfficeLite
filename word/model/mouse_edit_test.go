package model

import (
	"math"
	"testing"
)

func TestMoveText(t *testing.T) {
	d, e := newDoc("one two three", "four five")
	// Make "two " bold so the move keeps formatting.
	e.Select(Pos{0, 4}, Pos{0, 8})
	e.ToggleBold()
	// Move "two " to the start of the second paragraph.
	if !e.MoveText(Pos{1, 0}, false) {
		t.Fatal("not moved")
	}
	if got := texts(d); got != "one three|two four five" {
		t.Fatalf("moved: %q", got)
	}
	if a, b := e.Sel.Ordered(); a != (Pos{1, 0}) || b != (Pos{1, 4}) {
		t.Errorf("selection %v %v", a, b)
	}
	if r := e.Para(1).Runs[0]; r.Text != "two " || r.Props.Bold != On {
		t.Errorf("formatting lost: %+v", r)
	}
	// One undo step restores it.
	e.Undo()
	if got := texts(d); got != "one two three|four five" {
		t.Fatalf("undo: %q", got)
	}
	// Moving forward within a paragraph: "one " after "three".
	e.Select(Pos{0, 0}, Pos{0, 4})
	e.MoveText(Pos{0, 13}, false)
	if got := texts(d); got != "two threeone |four five" {
		t.Fatalf("forward: %q", got)
	}
	if a, b := e.Sel.Ordered(); a != (Pos{0, 9}) || b != (Pos{0, 13}) {
		t.Errorf("forward selection %v %v", a, b)
	}
	e.Undo()
	// Across paragraphs, forward onto the last selected paragraph's line:
	// "three|four " goes after "five".
	e.Select(Pos{0, 8}, Pos{1, 5})
	e.MoveText(Pos{1, 9}, false)
	if got := texts(d); got != "one two fivethree|four " {
		t.Fatalf("multi: %q", got)
	}
	e.Undo()
	// Ctrl+drag copies.
	e.Select(Pos{0, 0}, Pos{0, 3})
	e.MoveText(Pos{1, 9}, true)
	if got := texts(d); got != "one two three|four fiveone" {
		t.Fatalf("copy: %q", got)
	}
	e.Undo()
	// Dropping inside the selection does nothing.
	e.Select(Pos{0, 0}, Pos{0, 7})
	v := e.Version
	if e.MoveText(Pos{0, 3}, false) || e.Version != v {
		t.Error("drop inside the selection changed the document")
	}
}

func TestMoveTextRefusesTables(t *testing.T) {
	e := typed(t, "before", "after")
	e.SetCaret(Pos{1, 0}, false)
	e.InsertTable(1, 2)
	n := e.NumParas()
	e.Select(Pos{0, 0}, Pos{n - 1, 2})
	if e.CanMoveSelection() || e.MoveText(Pos{n - 1, 5}, false) {
		t.Error("a selection holding a table moved")
	}
}

func TestMoveColumnBorder(t *testing.T) {
	e := typed(t, "")
	e.InsertTable(2, 3)
	tb := e.Doc.Body[0].(*Table)
	w := tb.Columns[0]
	if !e.MoveColumnBorder(tb, 1, 30) {
		t.Fatal("not resized")
	}
	if math.Abs(tb.Columns[0]-(w+30)) > 1e-9 || math.Abs(tb.Columns[1]-(w-30)) > 1e-9 || tb.Columns[2] != w {
		t.Fatalf("columns %v", tb.Columns)
	}
	if c := tb.Rows[1].Cells[0]; math.Abs(c.Width-(w+30)) > 1e-9 {
		t.Errorf("cell width %v", c.Width)
	}
	// The right edge changes only the last column.
	e.MoveColumnBorder(tb, 3, -20)
	if math.Abs(tb.Columns[2]-(w-20)) > 1e-9 || math.Abs(tb.Columns[1]-(w-30)) > 1e-9 {
		t.Fatalf("right edge: %v", tb.Columns)
	}
	// Columns keep a minimum width.
	e.MoveColumnBorder(tb, 1, 1000)
	if math.Abs(tb.Columns[1]-MinColumnWidth) > 1e-9 {
		t.Errorf("clamped: %v", tb.Columns)
	}
	e.Undo()
	e.Undo()
	e.Undo()
	tb = e.Doc.Body[0].(*Table)
	if tb.Columns[0] != w || tb.Columns[1] != w || tb.Columns[2] != w {
		t.Errorf("undo: %v", tb.Columns)
	}
	if !e.SetRowHeight(tb, 1, 40) || tb.Rows[1].Height != 40 {
		t.Errorf("row height %v", tb.Rows[1].Height)
	}
	e.Undo()
	if tb = e.Doc.Body[0].(*Table); tb.Rows[1].Height != 0 {
		t.Errorf("undo row height %v", tb.Rows[1].Height)
	}
}
