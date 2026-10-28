package model

import "testing"

func TestFontCommands(t *testing.T) {
	d, e := newDoc("hello world. this IS fine")
	e.SelectAll()
	e.ChangeCase(SentenceCase)
	if got := texts(d); got != "Hello world. This is fine" {
		t.Errorf("sentence case: %q", got)
	}
	e.ChangeCase(CapitalizeWords)
	if got := texts(d); got != "Hello World. This Is Fine" {
		t.Errorf("capitalize: %q", got)
	}
	e.ChangeCase(ToggleCase)
	if got := texts(d); got != "hELLO wORLD. tHIS iS fINE" {
		t.Errorf("toggle: %q", got)
	}
	e.ChangeCase(UpperCase)
	e.Select(Pos{0, 0}, Pos{0, 5})
	e.ChangeCase(LowerCase)
	if got := texts(d); got != "hello WORLD. THIS IS FINE" {
		t.Errorf("upper/lower: %q", got)
	}
	e.Select(Pos{0, 0}, Pos{0, 5})
	e.GrowFont(1)
	p := d.Body[0].(*Paragraph)
	if p.Runs[0].Props.Size != 12 {
		t.Errorf("grow 11 -> %v", p.Runs[0].Props.Size)
	}
	e.GrowFont(-1)
	e.GrowFont(-1)
	if p := d.Body[0].(*Paragraph); p.Runs[0].Props.Size != 10 {
		t.Errorf("shrink -> %v", p.Runs[0].Props.Size)
	}
	e.ToggleVert(VertSuper)
	if p := d.Body[0].(*Paragraph); p.Runs[0].Props.Vert != VertSuper {
		t.Errorf("superscript")
	}
	e.ToggleVert(VertSuper)
	if p := d.Body[0].(*Paragraph); p.Runs[0].Props.Vert == VertSuper {
		t.Errorf("superscript off")
	}
	e.SetHighlight("yellow")
	if p := d.Body[0].(*Paragraph); p.Runs[0].Props.Highlight != "yellow" {
		t.Errorf("highlight")
	}
}
