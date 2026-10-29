// Package pagesfile imports Apple Pages documents (.pages, 2013 and
// later) read-only: the body text with its paragraph and character styles,
// lists, tables and inline pictures. Text boxes and other floating
// objects can't be placed on the page in WordLite's flow layout: their
// text comes in where they are anchored, or at the end of the document,
// with a warning.
package pagesfile

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"officelite/common/iwork"
	"officelite/word/model"
)

// Object types of a Pages document.
const (
	typeFloating   = 10010 // TP.FloatingDrawablesArchive (objects on pages)
	typeImage      = 3005  // TSD.ImageArchive
	typeShape      = 3004  // TSD.ShapeArchive
	typeGroup      = 3008  // TSD.GroupArchive
	typeChart      = 5021  // TSCH.ChartDrawableArchive
	typeCLine      = 3009  // TSD.ConnectionLineArchive
	typeMovie      = 3007  // TSD.MovieArchive
	typeFootnote   = 2008  // TSWP.FootnoteReferenceAttachmentArchive
	typePageNumber = 2043  // TSWP.NumberAttachmentArchive
	typeTextual1   = 2004  // TSWP.TextualAttachmentArchive (page number, count...)
	typeTextual2   = 2007
	typeTextual3   = 2009
	typeSection    = 10011 // TP.SectionArchive
)

type reader struct {
	doc      *iwork.Document
	out      *model.Document
	styleIDs map[uint64]string // named paragraph style -> Word style ID
	lists    map[uint64]int    // list style -> NumID
	counts   map[string]int    // dropped or moved content, for warnings
	seen     map[uint64]bool   // drawables already placed
}

// Read imports a .pages file.
func Read(data []byte) (_ *model.Document, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the file is damaged or not supported (internal error: %v)", r)
		}
	}()
	doc, err := iwork.Open(data)
	if err != nil {
		return nil, err
	}
	if doc.App != iwork.Pages {
		return nil, fmt.Errorf("this is a %s document, not a Pages document", doc.App)
	}
	out := model.New("")
	out.Body = nil
	out.SourceFormat = "pages"
	r := &reader{doc: doc, out: out, styleIDs: map[uint64]string{}, lists: map[uint64]int{},
		counts: map[string]int{}, seen: map[uint64]bool{}}
	root := doc.Msg(1)
	r.pageSetup(root)
	r.defaults()

	// A page layout document's body holds nothing but page breaks; its
	// content is all on the pages (floating).
	if body := root.Ref(4); strings.TrimFunc(doc.PlainText(body), func(c rune) bool {
		return unicode.IsSpace(c) || c == '\f'
	}) != "" {
		out.Body = r.storage(body)
	}
	if floating := r.floating(root.Ref(3)); len(floating) > 0 {
		r.counts["floating"]++
		out.Body = append(out.Body, floating...)
	}
	r.headersFooters(root.Ref(4))
	if len(out.Body) == 0 {
		out.Body = append(out.Body, &model.Paragraph{})
	}
	if _, ok := out.Body[len(out.Body)-1].(*model.Paragraph); !ok {
		out.Body = append(out.Body, &model.Paragraph{})
	}
	r.warnings()
	return out, nil
}

func (r *reader) warnings() {
	msgs := []struct{ key, text string }{
		{"floating", "Text boxes, pictures and tables placed on the page (not in the text) were added at the end of the document, in page order"},
		{"textbox", "%d text boxes were placed in the text flow"},
		{"shape", "%d shapes were not imported"},
		{"chart", "%d charts were not imported"},
		{"movie", "%d movies or other objects were not imported"},
		{"picture", "%d pictures could not be shown (not in the file, or in a format that can't be read)"},
		{"footnote", "%d footnotes were not imported"},
	}
	for _, m := range msgs {
		n := r.counts[m.key]
		if n == 0 {
			continue
		}
		if strings.Contains(m.text, "%d") {
			r.out.Warnings = append(r.out.Warnings, fmt.Sprintf(m.text, n))
		} else {
			r.out.Warnings = append(r.out.Warnings, m.text)
		}
	}
}

// pageSetup reads the page size and margins (TP.DocumentArchive 30-37).
func (r *reader) pageSetup(root iwork.Message) {
	p := model.LetterPage()
	set := func(n int, dst *float64) {
		if v, ok := root.FloatOK(n); ok && v >= 0 {
			*dst = v
		}
	}
	set(30, &p.Width)
	set(31, &p.Height)
	set(32, &p.MarginLeft)
	set(33, &p.MarginRight)
	set(34, &p.MarginTop)
	set(35, &p.MarginBottom)
	// Pages' header and footer heights (36, 37) are the room above and
	// below the body; Word's header and footer distances measure from the
	// page edge, so put the header in the middle of the top margin.
	p.Header, p.Footer = p.MarginTop/2, p.MarginBottom/2
	if p.Width <= 0 || p.Height <= 0 {
		p = model.LetterPage()
	}
	p.Landscape = root.Uint(42) == 1 || p.Width > p.Height
	r.out.Page = p
}

// defaults makes Word's document defaults neutral (Pages styles carry all
// their properties) and maps the Body style to Normal.
func (r *reader) defaults() {
	ss := &r.out.Styles
	ss.DefaultRun = model.RunProps{Font: "Helvetica Neue", Size: 11}
	ss.DefaultPara = model.ParaProps{SpaceBefore: model.F(0), SpaceAfter: model.F(0), Line: 1, LineRule: model.LineAuto, WidowControl: model.On}
	// The style a paragraph without one gets.
	for _, id := range r.doc.OfType(iwork.TypeParaStyle) {
		st := r.doc.Style(id)
		if st == nil || st.Variation {
			continue
		}
		if st.Ident == "paragraph-style-default" || strings.EqualFold(st.Name, "Body") {
			if n := ss.Find("Normal"); n != nil {
				n.Para, n.Run = paraProps(st.Para), runProps(st.Text)
			}
			if strings.EqualFold(st.Name, "Body") {
				r.styleIDs[id] = "Normal"
			}
		}
	}
}

// styleName is the user-visible name of a style: its name, or what its
// identifier ends with ("text-3-paragraphstyle-Heading 1").
func styleName(st *iwork.Style) string {
	if st.Name != "" {
		return st.Name
	}
	if _, after, ok := strings.Cut(st.Ident, "paragraphstyle-"); ok {
		return after
	}
	return st.Ident
}

// builtinStyle maps Pages style names to Word's built-in styles.
func builtinStyle(name string) (id string, level int) {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "title":
		return "Title", 0
	case "subtitle":
		return "Subtitle", 0
	case "heading", "heading 1":
		return "Heading1", 1
	case "quote":
		return "Quote", 0
	}
	if rest, ok := strings.CutPrefix(n, "heading "); ok && len(rest) == 1 && rest[0] >= '2' && rest[0] <= '9' {
		return "Heading" + rest, int(rest[0] - '0')
	}
	return "", 0
}

// paraStyle returns the Word style for a paragraph's Pages style and the
// direct formatting its variation (if any) adds.
func (r *reader) paraStyle(id uint64) (string, model.ParaProps, model.RunProps) {
	named := r.doc.NamedStyle(id)
	if named == 0 {
		return "", model.ParaProps{}, model.RunProps{}
	}
	wid := r.wordStyle(named)
	var dp model.ParaProps
	var dr model.RunProps
	if id != named {
		base, v := r.doc.Style(named), r.doc.Style(id)
		dp = diffPara(paraProps(base.Para), paraProps(v.Para))
		dr = diffRun(runProps(base.Text), runProps(v.Text))
	}
	return wid, dp, dr
}

// wordStyle creates (once) the Word paragraph style for a named Pages
// paragraph style.
func (r *reader) wordStyle(id uint64) string {
	if wid, ok := r.styleIDs[id]; ok {
		return wid
	}
	st := r.doc.Style(id)
	name := styleName(st)
	ss := &r.out.Styles
	wid, level := builtinStyle(name)
	taken := func(w string) bool {
		for _, v := range r.styleIDs {
			if v == w {
				return true
			}
		}
		return false
	}
	if wid == "" || taken(wid) {
		level = 0
		wid = styleID(name)
		for i := 2; ss.Find(wid) != nil; i++ {
			wid = fmt.Sprintf("%s%d", styleID(name), i)
		}
	}
	ws := ss.Find(wid)
	if ws == nil {
		ws = &model.Style{ID: wid, Name: name, Type: model.ParagraphStyle, Next: "Normal", Custom: true}
		ss.Styles = append(ss.Styles, ws)
	}
	ws.BasedOn = ""
	ws.Para, ws.Run = paraProps(st.Para), runProps(st.Text)
	ws.Para.OutlineLevel = level
	if level > 0 {
		ws.Para.KeepNext = model.On
	}
	r.styleIDs[id] = wid
	return wid
}

// styleID makes a style ID from a name: letters and digits only.
func styleID(name string) string {
	id := strings.Map(func(c rune) rune {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			return c
		}
		return -1
	}, name)
	if id == "" {
		id = "PagesStyle"
	}
	return id
}

// runProps converts iWork character formatting.
func runProps(t iwork.TextProps) model.RunProps {
	var p model.RunProps
	if t.Font != "" {
		family, bold, italic := iwork.FontFamily(t.Font)
		p.Font = family
		if bold {
			p.Bold = model.On
		}
		if italic {
			p.Italic = model.On
		}
	}
	if t.Size > 0 {
		p.Size = math.Round(t.Size*2) / 2
	}
	tri := func(v iwork.Tri) model.Tri {
		switch v {
		case iwork.On:
			return model.On
		case iwork.Off:
			return model.Off
		}
		return model.Unset
	}
	if t.Bold != iwork.Unset && p.Bold == model.Unset {
		p.Bold = tri(t.Bold)
	}
	if t.Italic != iwork.Unset && p.Italic == model.Unset {
		p.Italic = tri(t.Italic)
	}
	switch t.Underline {
	case iwork.On:
		p.Underline = model.UnderlineSingle
	case iwork.Off:
		p.Underline = model.UnderlineNone
	}
	p.Strike = tri(t.Strike)
	if t.Color.Set {
		p.Color = t.Color.Hex()
	}
	switch t.Script {
	case 1:
		p.Vert = model.VertSuper
	case 2:
		p.Vert = model.VertSub
	case 3:
		p.Vert = model.VertBaseline
	}
	switch t.Caps {
	case 1:
		p.Caps, p.SmallCaps = model.Off, model.Off
	case 2:
		p.Caps = model.On
	case 3:
		p.SmallCaps = model.On
	}
	if t.Highlight.Set {
		p.Highlight = t.Highlight.Hex()
	}
	return p
}

// paraProps converts iWork paragraph formatting; unset spacing and
// indents become 0 so Word's defaults don't creep in.
func paraProps(pp iwork.ParaProps) model.ParaProps {
	val := func(v *float64) *float64 {
		if v == nil {
			return model.F(0)
		}
		return model.F(*v)
	}
	p := model.ParaProps{
		SpaceBefore: val(pp.SpaceBefore),
		SpaceAfter:  val(pp.SpaceAfter),
		IndentLeft:  val(pp.LeftIndent),
		IndentRight: val(pp.RightIndent),
		// Pages measures the first line's indent from the margin, Word
		// from the left indent.
		IndentFirst: model.F(*val(pp.FirstIndent) - *val(pp.LeftIndent)),
		Line:        1,
		LineRule:    model.LineAuto,
	}
	if pp.Align != nil {
		switch *pp.Align {
		case iwork.AlignRight:
			p.Align = model.AlignRight
		case iwork.AlignCenter:
			p.Align = model.AlignCenter
		case iwork.AlignJustify:
			p.Align = model.AlignJustify
		default:
			p.Align = model.AlignLeft
		}
	}
	switch pp.LineMode {
	case iwork.LineRelative:
		if pp.LineAmount > 0 {
			p.Line = pp.LineAmount
		}
	case iwork.LineAtLeast:
		p.Line, p.LineRule = pp.LineAmount, model.LineAtLeast
	case iwork.LineExact, iwork.LineMaximum:
		p.Line, p.LineRule = pp.LineAmount, model.LineExact
	case iwork.LineBetween:
		p.Line = 1 + pp.LineAmount/12
	}
	tri := func(v iwork.Tri) model.Tri {
		switch v {
		case iwork.On:
			return model.On
		case iwork.Off:
			return model.Off
		}
		return model.Unset
	}
	p.KeepNext, p.KeepLines, p.PageBreakBefore = tri(pp.KeepNext), tri(pp.KeepLines), tri(pp.PageBreakBefore)
	if pp.Fill.Set {
		p.Shading = pp.Fill.Hex()
	}
	for _, t := range pp.Tabs {
		p.Tabs = append(p.Tabs, model.TabStop{Pos: t, Align: model.AlignLeft})
	}
	return p
}

// diffRun returns the properties of v that differ from base.
func diffRun(base, v model.RunProps) model.RunProps {
	var d model.RunProps
	if v.Font != base.Font {
		d.Font = v.Font
	}
	if v.Size != base.Size {
		d.Size = v.Size
	}
	if v.Bold != base.Bold {
		d.Bold = v.Bold
	}
	if v.Italic != base.Italic {
		d.Italic = v.Italic
	}
	if v.Underline != base.Underline {
		d.Underline = v.Underline
	}
	if v.Strike != base.Strike {
		d.Strike = v.Strike
	}
	if v.Caps != base.Caps {
		d.Caps = v.Caps
	}
	if v.SmallCaps != base.SmallCaps {
		d.SmallCaps = v.SmallCaps
	}
	if v.Color != base.Color {
		d.Color = v.Color
	}
	if v.Highlight != base.Highlight {
		d.Highlight = v.Highlight
	}
	if v.Vert != base.Vert {
		d.Vert = v.Vert
	}
	return d
}

// diffPara returns the properties of v that differ from base.
func diffPara(base, v model.ParaProps) model.ParaProps {
	var d model.ParaProps
	if v.Align != base.Align {
		d.Align = v.Align
	}
	for _, f := range []struct{ b, v, d **float64 }{
		{&base.SpaceBefore, &v.SpaceBefore, &d.SpaceBefore}, {&base.SpaceAfter, &v.SpaceAfter, &d.SpaceAfter},
		{&base.IndentLeft, &v.IndentLeft, &d.IndentLeft}, {&base.IndentRight, &v.IndentRight, &d.IndentRight},
		{&base.IndentFirst, &v.IndentFirst, &d.IndentFirst},
	} {
		if *f.v != nil && (*f.b == nil || **f.b != **f.v) {
			*f.d = model.F(**f.v)
		}
	}
	if v.Line != base.Line || v.LineRule != base.LineRule {
		d.Line, d.LineRule = v.Line, v.LineRule
	}
	if v.Shading != base.Shading {
		d.Shading = v.Shading
	}
	if v.KeepNext != base.KeepNext {
		d.KeepNext = v.KeepNext
	}
	if v.PageBreakBefore != base.PageBreakBefore {
		d.PageBreakBefore = v.PageBreakBefore
	}
	if len(v.Tabs) > 0 && fmt.Sprint(v.Tabs) != fmt.Sprint(base.Tabs) {
		d.Tabs = v.Tabs
	}
	return d
}

// charProps is a character style's formatting.
func (r *reader) charProps(id uint64) model.RunProps {
	if id == 0 {
		return model.RunProps{}
	}
	st := r.doc.Style(id)
	if st == nil {
		return model.RunProps{}
	}
	return runProps(st.Text)
}

// listRef puts a paragraph in the Word list made for its list style.
func (r *reader) listRef(listStyle uint64, level int) *model.ListRef {
	if listStyle == 0 {
		return nil
	}
	levels := r.doc.ListLevels(listStyle)
	if level >= len(levels) || level < 0 || levels[level].Label == 0 {
		return nil
	}
	num, ok := r.lists[listStyle]
	if !ok {
		num = len(r.out.Lists) + 1
		for r.out.Lists[num] != nil {
			num++
		}
		r.out.Lists[num] = wordList(levels)
		r.lists[listStyle] = num
	}
	return &model.ListRef{NumID: num, Level: min(level, 8)}
}

// wordList converts a Pages list style's levels.
func wordList(levels []iwork.ListLevel) *model.List {
	l := &model.List{}
	for i, lv := range levels {
		if i == 9 {
			break
		}
		wl := model.ListLevel{Start: 1, Indent: lv.Indent, Hang: lv.Indent - lv.Label0}
		if wl.Indent == 0 {
			wl.Indent = 18 * float64(i+1)
			wl.Hang = 18
		}
		switch lv.Label {
		case 2, 1: // a string (or a picture: shown as a bullet)
			wl.Format, wl.Text = "bullet", lv.Bullet
			if strings.TrimSpace(wl.Text) == "" || lv.Label == 1 {
				wl.Text = "•"
			}
		case 3:
			format, pre, post := numberFormat(lv.Number)
			text := pre + fmt.Sprintf("%%%d", i+1) + post
			if lv.Tiered {
				text = ""
				for k := 0; k <= i; k++ {
					text += fmt.Sprintf("%%%d.", k+1)
				}
			}
			wl.Format, wl.Text = format, text
		default:
			wl.Format = "none"
		}
		l.Levels = append(l.Levels, wl)
	}
	return l
}

// numberFormat maps a TSWP.ListStyleArchive.NumberType to a Word number
// format and the text around the number.
func numberFormat(t int) (format, pre, post string) {
	format = "decimal"
	switch t / 3 {
	case 1:
		format = "upperRoman"
	case 2:
		format = "lowerRoman"
	case 3:
		format = "upperLetter"
	case 4:
		format = "lowerLetter"
	}
	switch t % 3 {
	case 0:
		return format, "", "."
	case 1:
		return format, "(", ")"
	}
	return format, "", ")"
}

// storage converts a text storage to blocks.
func (r *reader) storage(id uint64) []model.Block {
	var blocks []model.Block
	for _, p := range r.doc.Paragraphs(id) {
		blocks = append(blocks, r.paragraph(p)...)
	}
	return blocks
}

// paragraph converts one paragraph; objects anchored in it that can't be
// inline (text boxes, tables) follow it as blocks of their own.
func (r *reader) paragraph(p iwork.Paragraph) []model.Block {
	wid, dp, dr := r.paraStyle(p.Style)
	para := &model.Paragraph{Style: wid, Props: dp, Mark: dr}
	if ref := r.listRef(p.ListStyle, p.Level); ref != nil {
		para.Props.List = ref
	}
	var after []model.Block
	var link *model.Span
	for _, s := range p.Spans {
		props := dr.Merge(r.charProps(s.CharStyle))
		if s.Link == "" {
			link = nil
		} else if link == nil || link.URL != s.Link {
			link = &model.Span{Kind: model.SpanLink, URL: s.Link}
		}
		switch s.Kind {
		case iwork.SpanText:
			para.Runs = append(para.Runs, model.Run{Kind: model.RunText, Text: s.Text, Props: props, Span: link})
		case iwork.SpanTab:
			para.Runs = append(para.Runs, model.Run{Kind: model.RunTab, Props: props})
		case iwork.SpanLineBreak, iwork.SpanColumnBreak:
			para.Runs = append(para.Runs, model.Run{Kind: model.RunBreak, Props: props})
		case iwork.SpanPageBreak:
			para.Runs = append(para.Runs, model.Run{Kind: model.RunPageBreak, Props: props})
		case iwork.SpanAttachment:
			run, blocks := r.attachment(s.Object)
			if run != nil {
				run.Props = props
				para.Runs = append(para.Runs, *run)
			}
			after = append(after, blocks...)
		}
	}
	// Tables line up with the text they're anchored in.
	if left := r.out.ResolvePara(para).IndentLeft; left != nil && *left > 0 {
		for _, b := range after {
			if t, ok := b.(*model.Table); ok {
				t.Indent = *left
			}
		}
	}
	if len(after) > 0 && len(para.Runs) == 0 {
		return after // the paragraph only anchored the objects
	}
	return append([]model.Block{para}, after...)
}

// attachment converts an object anchored in the text: an inline run (a
// picture) or blocks to place after the paragraph.
func (r *reader) attachment(id uint64) (*model.Run, []model.Block) {
	switch r.doc.Type(id) {
	case iwork.TypeDrawableAttachment:
		drawable := r.doc.Msg(id).Ref(1)
		if r.doc.Type(drawable) == typeImage {
			if run := r.picture(drawable); run != nil {
				return run, nil
			}
			return nil, nil
		}
		return nil, r.drawable(drawable, true)
	case typeFootnote:
		r.counts["footnote"]++
	case typePageNumber, typeTextual1, typeTextual2, typeTextual3:
		m := r.doc.Msg(id)
		kind, ok := m.UintOK(2)
		if r.doc.Type(id) == typePageNumber {
			kind, ok = m.Msg(1).UintOK(2)
		}
		switch {
		case !ok || kind == 0:
			return &model.Run{Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: "PAGE"}}, nil
		case kind == 1:
			return &model.Run{Kind: model.RunOpaque, Text: "1", Opaque: &model.Opaque{Field: "NUMPAGES"}}, nil
		}
	}
	return nil, nil
}

// headersFooters reads the first section's headers and footers (those of
// odd pages, which Pages uses for every page unless they differ).
func (r *reader) headersFooters(body uint64) {
	sections := r.doc.Msg(body).Msg(17).Msgs(1)
	if len(sections) == 0 {
		return
	}
	sec := r.doc.Msg(sections[0].Ref(2))
	if r.doc.Type(sections[0].Ref(2)) != typeSection {
		return
	}
	hf := sec.Ref(25)
	if hf == 0 {
		hf = sec.Ref(23)
	}
	m := r.doc.Msg(hf)
	hdr, ftr := r.hfBlocks(m.Refs(1)), r.hfBlocks(m.Refs(2))
	if hdr != nil {
		r.out.HF.Headers[model.HFDefault] = &model.HeaderFooter{Blocks: hdr}
	}
	if ftr != nil {
		r.out.HF.Footers[model.HFDefault] = &model.HeaderFooter{Blocks: ftr}
	}
	if (hdr != nil || ftr != nil) && sec.Bool(28) { // hidden on the first page
		r.out.HF.TitlePage = true
	}
	for _, e := range sections[1:] {
		other := r.doc.Msg(e.Ref(2))
		if o := other.Ref(25); o != 0 && o != hf && (r.hfBlocks(r.doc.Msg(o).Refs(1)) != nil || r.hfBlocks(r.doc.Msg(o).Refs(2)) != nil) {
			r.out.Warnings = append(r.out.Warnings, "The document has sections with different headers or footers: the first section's are used throughout")
			break
		}
	}
}

// hfBlocks converts a header or footer: its left, center and right parts,
// each a storage, the center and right ones aligned that way.
func (r *reader) hfBlocks(parts []uint64) []model.Block {
	var blocks []model.Block
	for i, id := range parts {
		paras := r.doc.Paragraphs(id)
		empty := true
		for _, p := range paras {
			if len(p.Spans) > 0 {
				empty = false
			}
		}
		if empty {
			continue
		}
		for _, b := range r.storage(id) {
			if p, ok := b.(*model.Paragraph); ok && p.Props.Align == model.AlignUnset {
				switch i {
				case 1:
					p.Props.Align = model.AlignCenter
				case 2:
					p.Props.Align = model.AlignRight
				}
			}
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// drawable converts a text box, table, picture or group to blocks.
func (r *reader) drawable(id uint64, anchored bool) []model.Block {
	if r.seen[id] {
		return nil
	}
	r.seen[id] = true
	m := r.doc.Msg(id)
	switch r.doc.Type(id) {
	case iwork.TypeShapeInfo:
		storage := m.Ref(4)
		if storage == 0 {
			storage = m.Ref(2)
		}
		if storage == 0 || strings.TrimSpace(r.doc.PlainText(storage)) == "" {
			r.counts["shape"]++
			return nil
		}
		if anchored {
			r.counts["textbox"]++
		}
		return r.storage(storage)
	case iwork.TypeTableInfo:
		t, err := r.doc.TableModel(id)
		if err != nil {
			r.out.Warnings = append(r.out.Warnings, "A table could not be read: "+err.Error())
			return nil
		}
		return []model.Block{r.table(t)}
	case typeImage:
		if run := r.picture(id); run != nil {
			return []model.Block{&model.Paragraph{Runs: []model.Run{*run}}}
		}
	case typeGroup:
		var blocks []model.Block
		for _, c := range m.Refs(2) {
			blocks = append(blocks, r.drawable(c, anchored)...)
		}
		return blocks
	case typeShape, typeCLine:
		r.counts["shape"]++
	case typeChart:
		r.counts["chart"]++
	default:
		r.counts["movie"]++
	}
	return nil
}

// picture returns an inline picture run for a TSD.ImageArchive, at the
// size it has on the page.
func (r *reader) picture(id uint64) *model.Run {
	m := r.doc.Msg(id)
	var data []byte
	var name string
	for _, n := range []int{11, 13, 12} { // data, original data, thumbnail
		ref := m.Msg(n).Uint(1)
		if ref == 0 {
			continue
		}
		if b, nm, err := r.doc.Data(ref); err == nil {
			data, name = b, nm
			break
		}
	}
	if data == nil {
		r.counts["picture"]++
		return nil
	}
	pic, w, h, err := model.NewPicture(data, name)
	if err != nil {
		r.counts["picture"]++
		return nil
	}
	if _, _, gw, gh := r.doc.Geometry(id); gw > 0 && gh > 0 {
		w, h = gw, gh
	}
	// Keep the picture inside the text column.
	if maxW := r.out.Page.Width - r.out.Page.MarginLeft - r.out.Page.MarginRight; w > maxW && maxW > 0 {
		h, w = h*maxW/w, maxW
	}
	run := model.PictureRun(pic, w, h)
	return &run
}

// floating converts the objects placed on pages rather than in the text,
// page by page, top to bottom, then left to right.
func (r *reader) floating(id uint64) []model.Block {
	if r.doc.Type(id) != typeFloating {
		return nil
	}
	type placed struct {
		id   uint64
		page int
		x, y float64
	}
	var objs []placed
	for _, pg := range r.doc.Msg(id).Msgs(1) {
		page := int(pg.Uint(1))
		for _, e := range pg.Msgs(4) {
			ref := e.Ref(1)
			x, y, _, _ := r.doc.Geometry(ref)
			objs = append(objs, placed{ref, page, x, y})
		}
	}
	sort.SliceStable(objs, func(i, j int) bool {
		a, b := objs[i], objs[j]
		if a.page != b.page {
			return a.page < b.page
		}
		if a.y != b.y {
			return a.y < b.y
		}
		return a.x < b.x
	})
	var blocks []model.Block
	for _, o := range objs {
		b := r.drawable(o.id, false)
		if len(b) > 0 && len(blocks) > 0 {
			blocks = append(blocks, &model.Paragraph{}) // keep objects apart
		}
		blocks = append(blocks, b...)
	}
	return blocks
}
