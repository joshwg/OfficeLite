package model

import "testing"

func TestResolve(t *testing.T) {
	d := New("Doc")
	p := &Paragraph{Style: "Heading1", Runs: []Run{
		{Text: "plain"},
		{Text: "strong", Style: "Strong"},
		{Text: "direct", Style: "Strong", Props: RunProps{Bold: Off, Size: 20, Color: "FF0000"}},
	}}
	d.Body = []Block{p}
	pp := d.ResolvePara(p)
	if *pp.SpaceAfter != 4 || *pp.SpaceBefore != 12 || pp.KeepNext != On || pp.Line != 1.08 {
		t.Errorf("para = %+v", pp)
	}
	r0 := d.ResolveRun(p, &p.Runs[0])
	if r0.Font != "Calibri Light" || r0.Size != 16 || r0.Bold.Bool(false) {
		t.Errorf("run0 = %+v", r0)
	}
	if r1 := d.ResolveRun(p, &p.Runs[1]); !r1.Bold.Bool(false) || r1.Size != 16 {
		t.Errorf("run1 = %+v", r1)
	}
	if r2 := d.ResolveRun(p, &p.Runs[2]); r2.Bold.Bool(false) || r2.Size != 20 || r2.Color != "FF0000" {
		t.Errorf("run2 = %+v", r2)
	}
	normal := &Paragraph{Runs: []Run{{Text: "x"}}}
	if r := d.ResolveRun(normal, &normal.Runs[0]); r.Font != "Calibri" || r.Size != 11 {
		t.Errorf("normal = %+v", r)
	}
	if got := p.Text(); got != "plainstrongdirect" {
		t.Errorf("text = %q", got)
	}
}

func TestMergeTabs(t *testing.T) {
	base := ParaProps{Tabs: []TabStop{{Pos: 36}, {Pos: 72}}}
	got := base.Merge(ParaProps{Tabs: []TabStop{{Pos: 36, Clear: true}, {Pos: 100, Align: AlignRight}}})
	if len(got.Tabs) != 2 || got.Tabs[0].Pos != 72 || got.Tabs[1].Pos != 100 {
		t.Errorf("tabs = %+v", got.Tabs)
	}
}
