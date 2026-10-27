package main

import "officelite/word/model"

// Sample builds a document exercising styles and layout: title, headings,
// character formatting, lists, tabs, alignment, shading and a table.
func Sample() *model.Document {
	d := model.New("Sample.docx")
	ed := model.NewEditor(d)
	ed.InsertText("WordLite Sample\nStyles and formatting\nThis paragraph uses the Normal style. ")
	add := func(s string, f func()) {
		start := ed.Sel.Caret
		ed.InsertText(s)
		if f == nil {
			f = ed.ClearCharFormatting
		}
		{
			ed.Select(start, ed.Sel.Caret)
			f()
			end := ed.Sel.Caret
			if start.Less(end) {
				ed.SetCaret(end, false)
			} else {
				ed.SetCaret(start, false)
			}
		}
	}
	add("Bold", ed.ToggleBold)
	add(", ", nil)
	add("italic", ed.ToggleItalic)
	add(", ", nil)
	add("underlined", ed.ToggleUnderline)
	add(", ", nil)
	add("struck", ed.ToggleStrike)
	add(", ", nil)
	add("red", func() { ed.SetColor("C00000") })
	add(" and ", nil)
	add("large", func() { ed.SetSize(16) })
	add(" text, ", nil)
	add("Times New Roman", func() { ed.SetFont("Times New Roman") })
	add(", highlighting, x", nil)
	add("2", func() { ed.FormatRuns(func(r *model.Run) { r.Props.Vert = model.VertSuper }) })
	add(" and H", nil)
	add("2", func() { ed.FormatRuns(func(r *model.Run) { r.Props.Vert = model.VertSub }) })
	add("O. Line breaking keeps words whole and wraps long lines at spaces so that this sentence continues onto the next line of the page.", nil)
	ed.Enter()
	add("Lists", nil)
	ed.Enter()
	add("First bullet\nSecond bullet\nNested bullet\nThird bullet", nil)
	ed.Enter()
	add("Step one\nStep two\nSub-step a\nSub-step b\nStep three", nil)
	ed.Enter()
	add("Alignment and tabs", nil)
	ed.Enter()
	add("Centered paragraph\nRight-aligned paragraph\nJustified paragraph: Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.\nName\tQty\tPrice\nApples\t12\t3.50\nA quote in the Quote style.", nil)

	set := func(i int, f func()) {
		ed.SetCaret(model.Pos{Para: i}, false)
		f()
	}
	set(0, func() { ed.SetParaStyle("Title") })
	set(1, func() { ed.SetParaStyle("Heading1") })
	set(3, func() { ed.SetParaStyle("Heading2") })
	ed.Select(model.Pos{Para: 4}, model.Pos{Para: 7, Off: 1})
	ed.ToggleList(true)
	set(6, func() { ed.Indent(1) })
	ed.Select(model.Pos{Para: 8}, model.Pos{Para: 12, Off: 1})
	ed.ToggleList(false)
	ed.Select(model.Pos{Para: 10}, model.Pos{Para: 11, Off: 1})
	ed.Indent(1)
	set(13, func() { ed.SetParaStyle("Heading2") })
	set(14, func() { ed.SetAlign(model.AlignCenter) })
	set(15, func() { ed.SetAlign(model.AlignRight) })
	set(16, func() { ed.SetAlign(model.AlignJustify) })
	for _, i := range []int{17, 18} {
		p := ed.Para(i)
		p.Props.Tabs = []model.TabStop{{Pos: 180, Align: model.AlignRight}, {Pos: 260, Decimal: true, Leader: '.'}}
	}
	set(19, func() { ed.SetParaStyle("Quote") })
	p := ed.Para(2)
	for i := range p.Runs {
		if p.Runs[i].Text == ", highlighting, x" {
			p.Runs[i].Props.Highlight = "yellow"
		}
	}
	shaded := &model.Paragraph{Props: model.ParaProps{Shading: "DEEAF6"}, Runs: []model.Run{{Text: "A shaded paragraph with a picture "},
		{Kind: model.RunOpaque, Text: "[picture]", Opaque: &model.Opaque{Format: "docx", Width: 60, Height: 40}}, {Text: " inline."}}}
	cell := func(s string, shade string) *model.TableCell {
		return &model.TableCell{Shading: shade, Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: s}}}}}
	}
	hdr := func(s string) *model.TableCell {
		c := cell(s, "D9E2F3")
		c.Blocks[0].(*model.Paragraph).Runs[0].Props.Bold = model.On
		return c
	}
	merged := cell("Merged vertically", "")
	merged.VMerge = 1
	tbl := &model.Table{Columns: []float64{150, 180, 138}, Borders: true, Rows: []*model.TableRow{
		{Cells: []*model.TableCell{hdr("Column A"), hdr("Column B"), hdr("Column C")}},
		{Cells: []*model.TableCell{cell("Cell text wraps inside its column when it is long enough", ""), cell("Two", ""), merged}},
		{Cells: []*model.TableCell{{Span: 2, Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: "Spans two columns"}}}}}, {VMerge: 2, Blocks: []model.Block{&model.Paragraph{}}}}},
	}}
	d.Body = append(d.Body, shaded, tbl, &model.Paragraph{Runs: []model.Run{{Text: "After the table."}}})
	return d
}
