package fileio

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"officelite/word/model"
)

// structure describes what the complex-layout features of a document
// look like: sections with their page setup and headers, tables with
// nested tables, notes, floating objects and kept fields.
func structure(d *model.Document) string {
	var b strings.Builder
	secs := d.Sections()
	for i, s := range secs {
		fmt.Fprintf(&b, "section %d %vx%v landscape=%v margins=%v/%v/%v/%v break=%d cols=%d title=%v\n", i, s.Page.Width, s.Page.Height,
			s.Page.Landscape, s.Page.MarginLeft, s.Page.MarginTop, s.Page.MarginRight, s.Page.MarginBottom, s.Break, s.Columns.N(), s.HF.TitlePage)
		for k := range 3 {
			for _, footer := range []bool{false, true} {
				if h := *s.HF.Slot(model.HFKind(k), footer); h != nil {
					fmt.Fprintf(&b, "  hf %d footer=%v %q\n", k, footer, text(h.Blocks))
				}
			}
		}
	}
	var blocks func([]model.Block, string)
	blocks = func(bs []model.Block, ind string) {
		for _, bl := range bs {
			switch bl := bl.(type) {
			case *model.Paragraph:
				// The text without floating objects' anchors and fields'
				// parts (a .doc field start is three .docx runs).
				var txt []string
				for _, r := range bl.Runs {
					if o := r.Opaque; o == nil || o.Float == nil && o.FieldChar == model.FieldNone {
						txt = append(txt, (&model.Paragraph{Runs: []model.Run{r}}).Text())
					}
				}
				fmt.Fprintf(&b, "%sP %q %q sect=%v\n", ind, bl.Style, strings.Join(txt, ""), bl.Sect != nil)
				for _, r := range bl.Runs {
					o := r.Opaque
					switch {
					case o == nil:
					case o.Note != nil:
						fmt.Fprintf(&b, "%s  note endnote=%v mark=%q %q\n", ind, o.Note.Endnote, r.Text, text(o.Note.Blocks))
					case o.Float != nil:
						f := o.Float
						fmt.Fprintf(&b, "%s  float %q %v,%v %vx%v rel=%d/%d wrap=%d fill=%q line=%q/%v %q\n", ind, f.Shape, f.X, f.Y, f.W, f.H,
							f.RelX, f.RelY, f.Wrap, f.Fill, f.Line, f.LineWidth, text(f.Blocks))
					case o.FieldChar == model.FieldStart || o.FieldChar == model.FieldInstr:
						fmt.Fprintf(&b, "%s  field start %q\n", ind, o.Field)
					case o.FieldChar == model.FieldEnd:
						fmt.Fprintf(&b, "%s  field end\n", ind)
					case o.Field != "":
						fmt.Fprintf(&b, "%s  field %q %q\n", ind, o.Field, r.Text)
					}
				}
			case *model.Table:
				fmt.Fprintf(&b, "%sT cols=%v\n", ind, bl.Columns)
				for _, row := range bl.Rows {
					for _, c := range row.Cells {
						fmt.Fprintf(&b, "%s cell span=%d vm=%d shd=%q\n", ind, c.Span, c.VMerge, c.Shading)
						blocks(c.Blocks, ind+"   ")
					}
				}
			}
		}
	}
	blocks(d.Body, "")
	return b.String()
}

func text(bs []model.Block) string {
	var out []string
	model.WalkBlocks(bs, func(p *model.Paragraph) bool { out = append(out, p.Text()); return true })
	return strings.Join(out, "|")
}

func reopen(t *testing.T, d *model.Document, f Format) *model.Document {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(d, &buf, f); err != nil {
		t.Fatal(err)
	}
	back, err := Read("x"+f.Ext(), buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return back
}

// The complex layout sample (sections in portrait and landscape, a
// footer, a nested table, an endnote, a text box and a table of
// contents) keeps its structure through .docx and .doc saves.
func TestComplexLayoutRoundTrips(t *testing.T) {
	data, err := os.ReadFile("docfile/testdata/complex.doc")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Read("complex.doc", data)
	if err != nil {
		t.Fatal(err)
	}
	want := structure(doc)
	for _, s := range []string{"section 0 612x792 landscape=false", "section 1 792x612 landscape=true", `hf 0 footer=true "Footer"`,
		`note endnote=true mark="1" "￼ Endnote text"`, `float "rect" 264,54.35 100x50`, `"Inside box"`, `field start "TOC \\o \"1-3\""`, `P "" "n1"`} {
		if !strings.Contains(want, s) {
			t.Errorf("no %s in\n%s", s, want)
		}
	}
	// .doc → .docx → read: the same structure.
	x := reopen(t, doc, DOCX)
	if got := structure(x); got != want {
		t.Errorf(".docx save:\n%s", diff(want, got))
	}
	// and .docx → .docx again.
	if got := structure(reopen(t, x, DOCX)); got != want {
		t.Errorf("second .docx save:\n%s", diff(want, got))
	}
	// .docx → .doc: everything but the text box (which .doc saving
	// drops).
	noFloat := func(s string) string {
		var keep []string
		for _, l := range strings.Split(s, "\n") {
			if !strings.Contains(l, "float ") {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n")
	}
	if got := structure(reopen(t, x, DOC)); noFloat(got) != noFloat(want) {
		t.Errorf(".doc save of the .docx:\n%s", diff(noFloat(want), noFloat(got)))
	}
}

func diff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range max(len(w), len(g)) {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return fmt.Sprintf("line %d:\nwant %s\ngot  %s", i+1, a, b)
		}
	}
	return ""
}
