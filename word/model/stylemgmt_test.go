package model

import "testing"

func TestNewStyleFromSelection(t *testing.T) {
	d, e := newDoc("Fancy paragraph", "other")
	p := d.Body[0].(*Paragraph)
	p.Props.Align = AlignRight
	p.Props.SpaceAfter = F(20)
	p.Runs[0].Props = RunProps{Bold: On, Color: "00B050"}
	e.reindex()
	e.SetCaret(Pos{0, 3}, false)
	id, err := e.NewStyle(StyleSpec{Name: "My Fancy", Type: ParagraphStyle, Next: "Normal"})
	if err != nil || id != "MyFancy" {
		t.Fatalf("new style: %q %v", id, err)
	}
	st := d.Styles.Find(id)
	if st == nil || !st.Custom || st.BasedOn != "Normal" || st.Next != "Normal" || st.Para.Align != AlignRight || st.Run.Bold != On {
		t.Fatalf("style: %+v", st)
	}
	if p.Style != id || p.Props.Align != AlignUnset || p.Runs[0].Props.Bold != Unset {
		t.Errorf("not applied cleanly: %q %+v %+v", p.Style, p.Props, p.Runs[0].Props)
	}
	if rp := d.ResolveRun(p, &p.Runs[0]); rp.Bold != On || rp.Color != "00B050" {
		t.Errorf("resolved: %+v", rp)
	}
	if _, err := e.NewStyle(StyleSpec{Name: "my fancy"}); err != ErrStyleExists {
		t.Errorf("duplicate name: %v", err)
	}
	if _, err := e.NewStyle(StyleSpec{Name: " "}); err != ErrStyleName {
		t.Errorf("empty name: %v", err)
	}
	e.Undo()
	if d.Styles.Find(id) != nil || d.Body[0].(*Paragraph).Style != "" {
		t.Errorf("undo didn't remove the style")
	}
}

func TestCharStyleUpdateDelete(t *testing.T) {
	d, e := newDoc("one two three")
	e.Select(Pos{0, 4}, Pos{0, 7})
	e.SetColor("C00000")
	id, err := e.NewStyle(StyleSpec{Name: "Red", Type: CharacterStyle})
	if err != nil {
		t.Fatal(err)
	}
	p := d.Body[0].(*Paragraph)
	if len(p.Runs) != 3 || p.Runs[1].Style != id || p.Runs[1].Props.Color != "" {
		t.Fatalf("char style applied: %+v", p.Runs)
	}
	// Update to match: make the selection italic too, then update.
	e.Select(Pos{0, 4}, Pos{0, 7})
	e.ToggleItalic()
	e.UpdateStyleToMatch(id)
	if st := d.Styles.Find(id); st.Run.Italic != On || st.Run.Color != "C00000" {
		t.Errorf("update: %+v", st.Run)
	}
	if p.Runs[1].Props.Italic != Unset {
		t.Errorf("direct formatting kept: %+v", p.Runs[1].Props)
	}
	// Deleting it: the runs lose the style.
	if err := e.DeleteStyle(id); err != nil {
		t.Fatal(err)
	}
	// The run that lost the style looks like its neighbours, so they merge.
	if d.Styles.Find(id) != nil || len(p.Runs) != 1 || p.Runs[0].Style != "" {
		t.Errorf("delete: %+v", p.Runs)
	}
	if err := e.DeleteStyle("Normal"); err != ErrStyleBuiltIn {
		t.Errorf("deleting Normal: %v", err)
	}
	e.Undo()
	if d.Styles.Find(id) == nil || d.Body[0].(*Paragraph).Runs[1].Style != id {
		t.Errorf("undo delete")
	}
}

func TestDeleteParaStyleFallsBack(t *testing.T) {
	d, e := newDoc("a", "b")
	d.Styles.Styles = append(d.Styles.Styles,
		&Style{ID: "Mine", Name: "Mine", Type: ParagraphStyle, BasedOn: "Heading1", Custom: true},
		&Style{ID: "Child", Name: "Child", Type: ParagraphStyle, BasedOn: "Mine", Next: "Mine", Custom: true})
	d.Body[0].(*Paragraph).Style = "Mine"
	hf := d.NewHeaderFooter(HFDefault, false)
	hf.Blocks[0].(*Paragraph).Style = "Mine"
	if err := e.DeleteStyle("Mine"); err != nil {
		t.Fatal(err)
	}
	if d.Body[0].(*Paragraph).Style != "Heading1" || hf.Blocks[0].(*Paragraph).Style != "Heading1" {
		t.Errorf("fallback: %q", d.Body[0].(*Paragraph).Style)
	}
	if c := d.Styles.Find("Child"); c.BasedOn != "Heading1" || c.Next != "" {
		t.Errorf("child: %+v", c)
	}
}

func TestHeadersFooters(t *testing.T) {
	d := New("t")
	hf := d.NewHeaderFooter(HFDefault, false)
	if hf != d.NewHeaderFooter(HFDefault, false) || d.Styles.Find("Header") == nil {
		t.Fatal("header not created once with its style")
	}
	first := d.NewHeaderFooter(HFFirst, true)
	if h, k := d.HF.For(0, true); h != nil || k != HFDefault {
		t.Errorf("first page without titlePg: %v %v", h, k)
	}
	d.HF.TitlePage = true
	if h, k := d.HF.For(0, true); h != first || k != HFFirst {
		t.Errorf("first page: %v", k)
	}
	d.HF.EvenOdd = true
	if _, k := d.HF.For(1, false); k != HFEven {
		t.Errorf("page 2 kind %v", k)
	}
	if h, _ := d.HF.For(2, false); h != hf {
		t.Errorf("page 3 header")
	}
	c := d.Clone()
	if c.HF.Headers[0] == hf || c.HF.Headers[0].Blocks[0] == hf.Blocks[0] {
		t.Errorf("clone shares headers")
	}
	sub := d.SubDocument(hf.Blocks)
	e := NewEditor(sub)
	e.InsertText("Page ")
	e.InsertSimpleField("PAGE", "1")
	if got := sub.Body[0].(*Paragraph).Text(); got != "Page ￼" || !HasPageFields(sub.Body) {
		t.Errorf("field: %q", got)
	}
	if FieldName(` page \* MERGEFORMAT `) != "PAGE" {
		t.Errorf("FieldName")
	}
}

// Undoing Delete Style gives header and footer paragraphs their style back.
func TestDeleteStyleUndoHeaders(t *testing.T) {
	d, e := newDoc("body")
	id := "Mine"
	d.Styles.Styles = append(d.Styles.Styles, &Style{ID: id, Name: id, Type: ParagraphStyle, Custom: true})
	hf := d.NewHeaderFooter(HFDefault, false)
	hp := hf.Blocks[0].(*Paragraph)
	hp.Style = id
	d.Body[0].(*Paragraph).Style = id
	if err := e.DeleteStyle(id); err != nil {
		t.Fatal(err)
	}
	if hp.Style == id {
		t.Fatalf("header paragraph kept the deleted style")
	}
	e.Undo()
	h := d.HF.Headers[HFDefault].Blocks[0].(*Paragraph)
	if h.Style != id || d.Body[0].(*Paragraph).Style != id {
		t.Fatalf("undo: header style %q, body style %q", h.Style, d.Body[0].(*Paragraph).Style)
	}
	e.Redo()
	if h := d.HF.Headers[HFDefault].Blocks[0].(*Paragraph); h.Style == id {
		t.Fatalf("redo: header style %q", h.Style)
	}
}
