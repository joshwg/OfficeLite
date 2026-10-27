package docfile

import (
	"fmt"
	"os"
	"testing"

	"officelite/word/model"
)

func TestDump(t *testing.T) {
	path := os.Getenv("DOCDUMP")
	if path == "" {
		t.Skip()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(dumpDoc(doc))
}

func dumpDoc(doc *model.Document) string {
	s := fmt.Sprintf("warnings: %q\npage: %+v\ndefaults: %+v\n", doc.Warnings, doc.Page, doc.Styles.DefaultRun)
	for _, st := range doc.Styles.Styles {
		s += fmt.Sprintf("style %q %q t=%d base=%q next=%q def=%v hid=%v run=%+v para=%s\n", st.ID, st.Name, st.Type, st.BasedOn, st.Next, st.Default, st.Hidden, st.Run, paraStr(st.Para))
	}
	for id, l := range doc.Lists {
		for i, lv := range l.Levels {
			if i < 2 {
				s += fmt.Sprintf("list %d lvl %d %+v\n", id, i, lv)
			}
		}
	}
	var blocks func(bs []model.Block, ind string)
	blocks = func(bs []model.Block, ind string) {
		for _, b := range bs {
			switch b := b.(type) {
			case *model.Paragraph:
				s += fmt.Sprintf("%sP style=%q %s mark=%+v\n", ind, b.Style, paraStr(b.Props), b.Mark)
				for _, r := range b.Runs {
					o := ""
					if r.Opaque != nil {
						o = fmt.Sprintf(" opaque(%s %d bytes %.1fx%.1f)", r.Opaque.Format, len(r.Opaque.Data), r.Opaque.Width, r.Opaque.Height)
					}
					s += fmt.Sprintf("%s  R%d %q cs=%q %+v%s\n", ind, r.Kind, r.Text, r.Style, r.Props, o)
				}
			case *model.Table:
				s += fmt.Sprintf("%sTABLE cols=%v borders=%v indent=%v align=%v\n", ind, b.Columns, b.Borders, b.Indent, b.Align)
				for _, row := range b.Rows {
					s += fmt.Sprintf("%s ROW h=%v hdr=%v\n", ind, row.Height, row.Header)
					for _, c := range row.Cells {
						s += fmt.Sprintf("%s  CELL span=%d vm=%d w=%v shd=%q va=%d\n", ind, c.Span, c.VMerge, c.Width, c.Shading, c.VAlign)
						blocks(c.Blocks, ind+"    ")
					}
				}
			}
		}
	}
	blocks(doc.Body, "")
	return s
}

func paraStr(p model.ParaProps) string {
	f := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return fmt.Sprint(*v)
	}
	s := fmt.Sprintf("{al=%d sb=%s sa=%s line=%v/%d ind=%s/%s/%s keep=%d/%d pbb=%d wid=%d shd=%q out=%d", p.Align, f(p.SpaceBefore), f(p.SpaceAfter), p.Line, p.LineRule, f(p.IndentLeft), f(p.IndentRight), f(p.IndentFirst), p.KeepNext, p.KeepLines, p.PageBreakBefore, p.WidowControl, p.Shading, p.OutlineLevel)
	if len(p.Tabs) > 0 {
		s += fmt.Sprintf(" tabs=%+v", p.Tabs)
	}
	if p.List != nil {
		s += fmt.Sprintf(" list=%+v", *p.List)
	}
	return s + "}"
}
