// Package docxfile reads and writes .docx documents (Office Open XML
// WordprocessingML), including .docm and .dotx on read.
//
// Reading maps document.xml, styles.xml, numbering.xml, settings.xml and the
// theme onto the model. Content the model doesn't cover (pictures, fields,
// bookmarks, content controls, unknown properties...) is kept as the original
// XML in Opaque runs, Spans and DocxXML fields.
//
// Writing into the package the document came from keeps every other part
// byte for byte and regenerates document.xml, styles.xml and numbering.xml,
// re-using the original XML wherever the model didn't change it. Macros
// (vbaProject) are dropped. Documents from other formats get a new minimal
// package.
package docxfile

import (
	"archive/zip"
	"io"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"officelite/word/model"
)

// Warnings recorded in Document.Warnings while saving.
const (
	warnMacrosRemoved = "Macros were removed: a .docx document can't contain them."
	warnLostContent   = "Some content from the original file (pictures, objects) can't be saved as .docx; a placeholder in brackets was written instead."
)

// Write encodes doc as .docx.
func Write(doc *model.Document, w io.Writer) error {
	wr := &writer{doc: doc}
	if doc.SourceFormat == "docx" && len(doc.Source) > 0 {
		if err := wr.loadSource(); err != nil {
			wr.src, wr.srcRoot, wr.srcStyles, wr.srcNum, wr.th = nil, nil, nil, nil, nil
		}
	}
	if wr.src == nil {
		wr.rels = []rel{
			{"rId1", relTypePrefix + relStyles, "styles.xml", ""},
			{"rId2", relTypePrefix + relSettings, "settings.xml", ""},
		}
		if len(doc.Lists) > 0 {
			wr.rels = append(wr.rels, rel{"rId3", relTypePrefix + relNumbering, "numbering.xml", ""})
		}
	}
	wr.prepareHeaders()
	docXML := wr.documentXML()
	styles := wr.stylesXML()
	numbering := wr.numberingXML()
	if wr.src != nil {
		return wr.writeInto(w, docXML, styles, numbering)
	}
	return wr.writeNew(w, docXML, styles, numbering)
}

type writer struct {
	doc *model.Document
	th  *theme

	// source package (nil when writing from scratch)
	src       *pkg
	main      string // main part name
	srcRoot   *node  // original document.xml root
	srcStyles *node
	srcNum    *node
	rels      []rel // main part relationships (with additions)
	relsDirty bool
	hfParts   []hfPart // header and footer parts (hdrftr.go)
	// pictures written as new media parts (picture.go)
	picRels map[*model.Picture]string
	media   []media
	docPrID int
}

func (w *writer) warn(s string) {
	for _, x := range w.doc.Warnings {
		if x == s {
			return
		}
	}
	w.doc.Warnings = append(w.doc.Warnings, s)
}

func (w *writer) loadSource() error {
	p, err := openPkg(w.doc.Source)
	if err != nil {
		return err
	}
	w.main = p.mainPart()
	data, err := p.read(w.main)
	if err != nil {
		return err
	}
	if w.srcRoot, err = parseXML(data); err != nil {
		return err
	}
	w.src = p
	w.rels = p.rels(w.main)
	for _, rl := range w.rels {
		part := resolve(w.main, rl.Target)
		switch {
		case rl.is(relTheme):
			if b, err := p.read(part); err == nil {
				w.th = parseTheme(b)
			}
		case rl.is(relStyles):
			if b, err := p.read(part); err == nil {
				w.srcStyles, _ = parseXML(b)
			}
		case rl.is(relNumbering):
			if b, err := p.read(part); err == nil {
				w.srcNum, _ = parseXML(b)
			}
		}
	}
	return nil
}

// ---- document.xml ----

// rootTag writes a root start tag named name carrying the standard
// namespace declarations plus those (and other attributes) of orig.
func rootTag(b *strings.Builder, name string, orig *node) {
	b.WriteString("<" + name)
	declared := map[string]bool{}
	var origNS map[string]string
	if orig != nil {
		origNS = orig.ns
	}
	for _, d := range stdNS {
		url := d.url
		if u, ok := origNS[d.prefix]; ok && d.prefix != "w" && d.prefix != "r" {
			url = u
		}
		b.WriteString(` xmlns:` + d.prefix + `="` + escAttr(url) + `"`)
		declared[d.prefix] = true
	}
	var extra []string
	for p, u := range origNS {
		if p == "xml" || declared[p] {
			continue
		}
		if p == "" {
			extra = append(extra, ` xmlns="`+escAttr(u)+`"`)
		} else {
			extra = append(extra, ` xmlns:`+p+`="`+escAttr(u)+`"`)
		}
	}
	sort.Strings(extra)
	b.WriteString(strings.Join(extra, ""))
	ign := false
	if orig != nil {
		for _, a := range orig.attrs {
			n := a.Name.Local
			switch a.Name.Space {
			case "":
			case "mc", "w", "r", "xml":
				n = a.Name.Space + ":" + n
			default:
				continue // attribute in an undeclared-here namespace: drop
			}
			if n == "mc:Ignorable" {
				ign = true
			}
			if n == "w:conformance" {
				continue
			}
			b.WriteString(" " + n + `="` + escAttr(a.Value) + `"`)
		}
	}
	if !ign {
		b.WriteString(` mc:Ignorable="` + stdIgnorable + `"`)
	}
	b.WriteString(">")
}

func (w *writer) documentXML() []byte {
	var b strings.Builder
	b.WriteString(xmlHeader)
	rootTag(&b, "w:document", w.srcRoot)
	var sect *node
	if w.srcRoot != nil {
		for _, k := range w.srcRoot.kids {
			if k.name == "w:body" {
				for _, bk := range k.kids {
					if bk.name == "w:sectPr" {
						sect = bk
					}
				}
				break
			}
			b.Write(k.raw()) // w:background etc.
		}
	}
	b.WriteString("<w:body>")
	w.blocks(&b, w.doc.Body)
	w.sectPr(&b, sect)
	b.WriteString("</w:body></w:document>")
	return []byte(b.String())
}

// wrapStack opens and closes Spans around a sequence of items.
type wrapStack struct {
	w    *writer
	b    *strings.Builder
	open []*model.Span
	ends []string
}

func (s *wrapStack) to(span *model.Span) {
	chain := span.Chain()
	n := 0
	for n < len(chain) && n < len(s.open) && chain[n] == s.open[n] {
		n++
	}
	for len(s.open) > n {
		s.b.WriteString(s.ends[len(s.ends)-1])
		s.open, s.ends = s.open[:len(s.open)-1], s.ends[:len(s.ends)-1]
	}
	for _, sp := range chain[n:] {
		start, end := s.w.spanTags(sp)
		s.b.WriteString(start)
		s.open, s.ends = append(s.open, sp), append(s.ends, end)
	}
}

func (s *wrapStack) close() { s.to(nil) }

// spanTags returns the opening and closing XML for a span.
func (w *writer) spanTags(sp *model.Span) (string, string) {
	if sp.Kind == model.SpanLink {
		if sp.Format == "docx" && len(sp.Start) > 0 {
			nodes := parseFragment(string(sp.Start) + string(sp.End))
			if len(nodes) == 1 {
				n := nodes[0]
				url := ""
				if id := n.attr("r:id"); id != "" {
					for _, rl := range w.rels {
						if rl.ID == id {
							url = rl.Target
						}
					}
					if url == "" {
						goto generate
					}
				}
				if url == sp.URL && n.attr("w:anchor") == sp.Anchor && n.attr("w:tooltip") == sp.Tooltip {
					return string(sp.Start), string(sp.End)
				}
			}
		}
	generate:
		if sp.URL == "" && sp.Anchor == "" {
			return "", ""
		}
		t := "<w:hyperlink"
		if sp.URL != "" {
			t += ` r:id="` + escAttr(w.hyperlinkRel(sp.URL)) + `"`
		}
		if sp.Anchor != "" {
			t += ` w:anchor="` + escAttr(sp.Anchor) + `"`
		}
		if sp.Tooltip != "" {
			t += ` w:tooltip="` + escAttr(sp.Tooltip) + `"`
		}
		return t + ` w:history="1">`, "</w:hyperlink>"
	}
	if sp.Format == "docx" && len(sp.Start) > 0 && w.relsOK(sp.Start) {
		return string(sp.Start), string(sp.End)
	}
	return "", ""
}

func (w *writer) hyperlinkRel(url string) string {
	for _, rl := range w.rels {
		if rl.is(relHyperlink) && rl.Target == url && rl.Mode == "External" {
			return rl.ID
		}
	}
	id := w.newRelID()
	w.rels = append(w.rels, rel{ID: id, Type: relTypePrefix + relHyperlink, Target: url, Mode: "External"})
	w.relsDirty = true
	return id
}

func (w *writer) newRelID() string {
	used := map[string]bool{}
	for _, rl := range w.rels {
		used[rl.ID] = true
	}
	for i := len(w.rels) + 1; ; i++ {
		id := "rId" + strconv.Itoa(i)
		if !used[id] {
			return id
		}
	}
}

var reRelAttr = regexp.MustCompile(`\br:(?:id|embed|link|pict|dm|lo|qs|cs|href)\s*=\s*"([^"]*)"`)

// relsOK reports whether every relationship id the XML refers to exists in
// the package being written.
func (w *writer) relsOK(data []byte) bool {
	ms := reRelAttr.FindAllSubmatch(data, -1)
	if len(ms) == 0 {
		return true
	}
	if w.src == nil {
		return false
	}
	for _, m := range ms {
		found := false
		for _, rl := range w.rels {
			if rl.ID == string(m[1]) {
				found = true
				break
			}
		}
		if !found && len(m[1]) > 0 {
			return false
		}
	}
	return true
}

func (w *writer) opaqueOK(o *model.Opaque) bool {
	return o != nil && o.Format == "docx" && len(o.Data) > 0 && w.relsOK(o.Data)
}

func (w *writer) blocks(b *strings.Builder, blocks []model.Block) {
	ws := &wrapStack{w: w, b: b}
	for _, bl := range blocks {
		switch bl := bl.(type) {
		case *model.Paragraph:
			ws.to(bl.Wrap)
			w.before(b, bl.Before)
			w.para(b, bl)
		case *model.Table:
			ws.to(bl.Wrap)
			w.before(b, bl.Before)
			if len(bl.Rows) > 0 {
				w.table(b, bl)
			}
		}
	}
	ws.close()
}

func (w *writer) before(b *strings.Builder, ops []*model.Opaque) {
	for _, o := range ops {
		if w.opaqueOK(o) {
			b.Write(o.Data)
		}
	}
}

func (w *writer) para(b *strings.Builder, p *model.Paragraph) {
	b.WriteString("<w:p>")
	markStyle := ""
	if p.Mark.DocxXML != "" {
		_, markStyle = parseRPr(parseFragment(p.Mark.DocxXML), w.th)
	}
	if ppr := pPrXML(p.Style, p.Props, rPrXML(markStyle, p.Mark, w.th), w.th); ppr != "" {
		b.WriteString("<w:pPr>" + ppr + "</w:pPr>")
	}
	ws := &wrapStack{w: w, b: b}
	for i := range p.Runs {
		r := &p.Runs[i]
		ws.to(r.Span)
		w.run(b, r)
	}
	ws.close()
	b.WriteString("</w:p>")
}

func (w *writer) run(b *strings.Builder, r *model.Run) {
	if r.Kind == model.RunOpaque {
		if w.opaqueOK(r.Opaque) {
			b.Write(r.Opaque.Data)
			return
		}
		if r.Opaque != nil && r.Opaque.Picture != nil && r.Opaque.Width > 0 && r.Opaque.Height > 0 {
			w.pictureRun(b, r)
			return
		}
		if r.Opaque != nil && r.Opaque.Field != "" {
			b.WriteString(`<w:fldSimple w:instr="` + escAttr(" "+r.Opaque.Field+" ") + `">`)
			w.textRun(b, r, r.Text)
			b.WriteString("</w:fldSimple>")
			return
		}
		text := r.Text
		if text == "" {
			if r.Opaque == nil || len(r.Opaque.Data) == 0 {
				return
			}
			text = "[object]"
		}
		if !strings.HasPrefix(text, "[") {
			text = "[" + text + "]"
		}
		w.warn(warnLostContent)
		w.textRun(b, r, text)
		return
	}
	switch r.Kind {
	case model.RunTab:
		w.textRun(b, r, "\t")
	case model.RunBreak:
		w.textRun(b, r, "\n")
	case model.RunPageBreak:
		w.textRun(b, r, "\f")
	default:
		if r.Text != "" {
			w.textRun(b, r, r.Text)
		}
	}
}

// textRun writes one <w:r> with r's formatting holding text (tabs, line and
// page breaks and special hyphens become their elements).
func (w *writer) textRun(b *strings.Builder, r *model.Run, text string) {
	b.WriteString("<w:r>")
	if rpr := rPrXML(r.Style, r.Props, w.th); rpr != "" {
		b.WriteString("<w:rPr>" + rpr + "</w:rPr>")
	}
	var t strings.Builder
	flush := func() {
		if t.Len() > 0 {
			b.WriteString(`<w:t xml:space="preserve">` + escText(t.String()) + "</w:t>")
			t.Reset()
		}
	}
	for _, c := range text {
		switch c {
		case '\t':
			flush()
			b.WriteString("<w:tab/>")
		case '\n', '\v', ' ':
			flush()
			b.WriteString("<w:br/>")
		case '\f':
			flush()
			b.WriteString(`<w:br w:type="page"/>`)
		case '‑':
			flush()
			b.WriteString("<w:noBreakHyphen/>")
		case '­':
			flush()
			b.WriteString("<w:softHyphen/>")
		case '\r':
		default:
			t.WriteRune(c)
		}
	}
	flush()
	b.WriteString("</w:r>")
}

// ---- tables ----

var tblPrOrder = order("tblStyle tblpPr tblOverlap bidiVisual tblStyleRowBandSize tblStyleColBandSize tblW jc tblCellSpacing tblInd tblBorders shd tblLayout tblCellMar tblLook tblCaption tblDescription tblPrChange")
var trPrOrder = order("cnfStyle divId gridBefore gridAfter wBefore wAfter cantSplit trHeight tblHeader tblCellSpacing jc hidden ins del trPrChange")
var tcPrOrder = order("cnfStyle tcW gridSpan hMerge vMerge tcBorders shd noWrap tcMar textDirection tcFitText vAlign hideMark headers cellIns cellDel cellMerge tcPrChange")
var sectPrOrder = order("headerReference footerReference footnotePr endnotePr type pgSz pgMar paperSrc pgBorders lnNumType pgNumType cols formProt vAlign noEndnote titlePg textDirection bidi rtlGutter docGrid printerSettings sectPrChange")

func has(nodes []*node, name string) bool {
	for _, n := range nodes {
		if n.name == name {
			return true
		}
	}
	return false
}

func (w *writer) table(b *strings.Builder, t *model.Table) {
	orig := parseFragment(t.DocxXML)
	o := parseTblPr(orig, &w.doc.Styles)
	var align string
	switch t.Align {
	case model.AlignLeft:
		align = "left"
	case model.AlignCenter:
		align = "center"
	case model.AlignRight:
		align = "right"
	}
	var ind string
	if t.Indent != 0 {
		ind = `<w:tblInd w:w="` + ptTwips(t.Indent) + `" w:type="dxa"/>`
	}
	var borders string
	if t.Borders != styleBorders(t.Style, &w.doc.Styles) || has(orig, "w:tblBorders") {
		v := "none"
		if t.Borders {
			v = "single"
		}
		borders = "<w:tblBorders>"
		for _, e := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
			borders += `<w:` + e + ` w:val="` + v + `" w:sz="4" w:space="0" w:color="auto"/>`
		}
		borders += "</w:tblBorders>"
	}
	groups := []propGroup{
		{[]string{"w:tblStyle"}, t.Style == o.style, valXML("tblStyle", t.Style)},
		{[]string{"w:tblW"}, has(orig, "w:tblW"), `<w:tblW w:w="0" w:type="auto"/>`},
		{[]string{"w:jc"}, t.Align == o.align, valXML("jc", align)},
		{[]string{"w:tblInd"}, t.Indent == o.indent, ind},
		{[]string{"w:tblBorders"}, t.Borders == o.borders, borders},
		{[]string{"w:tblLook"}, len(orig) > 0, `<w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="1" w:lastColumn="0" w:noHBand="0" w:noVBand="1"/>`},
	}
	b.WriteString("<w:tbl><w:tblPr>" + mergeProps(orig, groups, tblPrOrder) + "</w:tblPr><w:tblGrid>")
	cols := t.Columns
	if len(cols) == 0 {
		n := 0
		for _, r := range t.Rows {
			c := 0
			for _, cell := range r.Cells {
				c += max(cell.Span, 1)
			}
			n = max(n, c)
		}
		if n == 0 {
			n = 1
		}
		for range n {
			cols = append(cols, (w.doc.Page.Width-w.doc.Page.MarginLeft-w.doc.Page.MarginRight)/float64(n))
		}
	}
	for _, c := range cols {
		b.WriteString(`<w:gridCol w:w="` + ptTwips(c) + `"/>`)
	}
	b.WriteString("</w:tblGrid>")
	for _, r := range t.Rows {
		w.row(b, r)
	}
	b.WriteString("</w:tbl>")
}

func (w *writer) row(b *strings.Builder, r *model.TableRow) {
	b.WriteString("<w:tr>")
	var orig []*node
	for _, k := range parseFragment(r.DocxXML) {
		if k.name == "w:tblPrEx" {
			b.Write(k.raw())
		} else {
			orig = append(orig, k)
		}
	}
	oh, ohdr := parseTrPr(orig)
	var hgen, hdr string
	if r.Height > 0 {
		hgen = `<w:trHeight w:val="` + ptTwips(r.Height) + `"/>`
	}
	if r.Header {
		hdr = "<w:tblHeader/>"
	}
	trpr := mergeProps(orig, []propGroup{
		{[]string{"w:trHeight"}, r.Height == oh, hgen},
		{[]string{"w:tblHeader"}, r.Header == ohdr, hdr},
	}, trPrOrder)
	if trpr != "" {
		b.WriteString("<w:trPr>" + trpr + "</w:trPr>")
	}
	for _, c := range r.Cells {
		w.cell(b, c)
	}
	b.WriteString("</w:tr>")
}

func (w *writer) cell(b *strings.Builder, c *model.TableCell) {
	orig := parseFragment(c.DocxXML)
	o := parseTcPr(orig, w.th)
	var width, span, vm, shd, va string
	if c.Width > 0 {
		width = `<w:tcW w:w="` + ptTwips(c.Width) + `" w:type="dxa"/>`
	}
	if c.Span > 1 {
		span = valXML("gridSpan", itoa(c.Span))
	}
	switch c.VMerge {
	case 1:
		vm = valXML("vMerge", "restart")
	case 2:
		vm = "<w:vMerge/>"
	}
	if c.Shading != "" {
		shd = `<w:shd w:val="clear" w:color="auto" w:fill="` + escAttr(c.Shading) + `"/>`
	}
	switch c.VAlign {
	case 1:
		va = valXML("vAlign", "center")
	case 2:
		va = valXML("vAlign", "bottom")
	}
	tcpr := mergeProps(orig, []propGroup{
		{[]string{"w:tcW"}, c.Width == o.width, width},
		{[]string{"w:gridSpan"}, max(c.Span, 1) == max(o.span, 1), span},
		{[]string{"w:vMerge"}, c.VMerge == o.vmerge, vm},
		{[]string{"w:shd"}, c.Shading == o.shading, shd},
		{[]string{"w:vAlign"}, c.VAlign == o.valign, va},
	}, tcPrOrder)
	b.WriteString("<w:tc>")
	if tcpr != "" {
		b.WriteString("<w:tcPr>" + tcpr + "</w:tcPr>")
	}
	blocks := c.Blocks
	if len(blocks) == 0 {
		blocks = []model.Block{&model.Paragraph{}}
	} else if _, ok := blocks[len(blocks)-1].(*model.Paragraph); !ok {
		blocks = append(blocks[:len(blocks):len(blocks)], &model.Paragraph{})
	}
	w.blocks(b, blocks)
	b.WriteString("</w:tc>")
}

// ---- section ----

func (w *writer) sectPr(b *strings.Builder, sect *node) {
	var orig []*node
	if sect != nil {
		orig = sect.kids
	}
	pg := w.doc.Page
	o := parsePage(orig)
	if pg.Width <= 0 || pg.Height <= 0 {
		pg = model.LetterPage()
	}
	size := `<w:pgSz w:w="` + ptTwips(pg.Width) + `" w:h="` + ptTwips(pg.Height) + `"`
	if pg.Landscape {
		size += ` w:orient="landscape"`
	}
	size += "/>"
	mar := `<w:pgMar w:top="` + ptTwips(pg.MarginTop) + `" w:right="` + ptTwips(pg.MarginRight) +
		`" w:bottom="` + ptTwips(pg.MarginBottom) + `" w:left="` + ptTwips(pg.MarginLeft) +
		`" w:header="` + ptTwips(pg.Header) + `" w:footer="` + ptTwips(pg.Footer) + `" w:gutter="0"/>`
	groups := []propGroup{
		{[]string{"w:pgSz"}, has(orig, "w:pgSz") && pg.Width == o.Width && pg.Height == o.Height && pg.Landscape == o.Landscape, size},
		{[]string{"w:pgMar"}, has(orig, "w:pgMar") && pg.MarginTop == o.MarginTop && pg.MarginBottom == o.MarginBottom &&
			pg.MarginLeft == o.MarginLeft && pg.MarginRight == o.MarginRight && pg.Header == o.Header && pg.Footer == o.Footer, mar},
	}
	groups = append(groups, w.hfGroups(orig)...)
	if sect == nil {
		groups = append(groups,
			propGroup{[]string{"w:cols"}, false, `<w:cols w:space="720"/>`},
			propGroup{[]string{"w:docGrid"}, false, `<w:docGrid w:linePitch="360"/>`})
	}
	b.WriteString("<w:sectPr>" + mergeProps(orig, groups, sectPrOrder) + "</w:sectPr>")
}

// ---- styles.xml ----

var styleOrder = order("name aliases basedOn next link autoRedefine hidden uiPriority semiHidden unhideWhenUsed qFormat locked personal personalCompose personalReply rsid pPr rPr tblPr trPr tcPr tblStylePr")

var styleTypeNames = map[model.StyleType]string{
	model.ParagraphStyle: "paragraph", model.CharacterStyle: "character",
	model.TableStyle: "table", model.NumberingStyle: "numbering",
}

func (w *writer) stylesXML() []byte {
	ss := &w.doc.Styles
	var b strings.Builder
	b.WriteString(xmlHeader)
	rootTag(&b, "w:styles", w.srcStyles)
	b.WriteString("<w:docDefaults><w:rPrDefault><w:rPr>" + rPrXML("", ss.DefaultRun, w.th) +
		"</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr>" + pPrXML("", ss.DefaultPara, "", w.th) +
		"</w:pPr></w:pPrDefault></w:docDefaults>")
	if w.srcStyles != nil {
		for _, k := range w.srcStyles.kids {
			if k.name != "w:docDefaults" && k.name != "w:style" {
				b.Write(k.raw()) // latentStyles and anything unknown
			}
		}
	}
	for _, st := range ss.Styles {
		if st.ID == "" {
			continue
		}
		b.WriteString(`<w:style w:type="` + styleTypeNames[st.Type] + `"`)
		if st.Default {
			b.WriteString(` w:default="1"`)
		}
		if st.Custom {
			b.WriteString(` w:customStyle="1"`)
		}
		b.WriteString(` w:styleId="` + escAttr(st.ID) + `">`)
		orig := parseFragment(st.DocxXML)
		var o model.Style
		parseStyleKids(&o, orig)
		var hidden, ppr, rpr string
		if st.Hidden {
			hidden = "<w:semiHidden/>"
		}
		if x := pPrXML("", st.Para, "", w.th); x != "" && st.Type != model.CharacterStyle {
			ppr = "<w:pPr>" + x + "</w:pPr>"
		}
		if x := rPrXML("", st.Run, w.th); x != "" {
			rpr = "<w:rPr>" + x + "</w:rPr>"
		}
		b.WriteString(mergeProps(orig, []propGroup{
			{[]string{"w:name"}, st.Name == o.Name, valXML("name", st.Name)},
			{[]string{"w:basedOn"}, st.BasedOn == o.BasedOn, valXML("basedOn", st.BasedOn)},
			{[]string{"w:next"}, st.Next == o.Next, valXML("next", st.Next)},
			{[]string{"w:semiHidden", "w:hidden"}, st.Hidden == o.Hidden, hidden},
			{[]string{"w:pPr"}, false, ppr},
			{[]string{"w:rPr"}, false, rpr},
		}, styleOrder))
		b.WriteString("</w:style>")
	}
	b.WriteString("</w:styles>")
	return []byte(b.String())
}

// ---- numbering.xml ----

// numberingXML returns numbering.xml, or nil if the document needs none.
// Lists unchanged from the source keep their original definitions.
func (w *writer) numberingXML() []byte {
	var origLists map[int]*model.List
	if w.srcNum != nil {
		origLists = parseNumbering(w.srcNum, &w.doc.Styles)
	}
	if w.srcNum == nil && len(w.doc.Lists) == 0 {
		return nil
	}
	var head, abstracts, nums, tail []string
	maxAbs := -1
	numXML := map[int]string{}
	var numOrder []int
	if w.srcNum != nil {
		for _, k := range w.srcNum.kids {
			switch k.name {
			case "w:abstractNum":
				abstracts = append(abstracts, string(k.raw()))
				maxAbs = max(maxAbs, atoi(k.attr("w:abstractNumId")))
			case "w:num":
				id := atoi(k.attr("w:numId"))
				numXML[id] = string(k.raw())
				numOrder = append(numOrder, id)
			case "w:numPicBullet":
				head = append(head, string(k.raw()))
			default:
				tail = append(tail, string(k.raw()))
			}
		}
	}
	ids := make([]int, 0, len(w.doc.Lists))
	for id := range w.doc.Lists {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		l := w.doc.Lists[id]
		if l == nil || id <= 0 {
			continue
		}
		if o, ok := origLists[id]; ok && reflect.DeepEqual(o, l) {
			continue
		}
		maxAbs++
		abstracts = append(abstracts, abstractNumXML(maxAbs, l))
		if _, ok := numXML[id]; !ok {
			numOrder = append(numOrder, id)
		}
		numXML[id] = `<w:num w:numId="` + itoa(id) + `"><w:abstractNumId w:val="` + itoa(maxAbs) + `"/></w:num>`
	}
	for _, id := range numOrder {
		nums = append(nums, numXML[id])
	}
	var b strings.Builder
	b.WriteString(xmlHeader)
	rootTag(&b, "w:numbering", w.srcNum)
	for _, part := range [][]string{head, abstracts, nums, tail} {
		for _, s := range part {
			b.WriteString(s)
		}
	}
	b.WriteString("</w:numbering>")
	return []byte(b.String())
}

func abstractNumXML(id int, l *model.List) string {
	var b strings.Builder
	b.WriteString(`<w:abstractNum w:abstractNumId="` + itoa(id) + `"><w:multiLevelType w:val="hybridMultilevel"/>`)
	for i, lv := range l.Levels {
		f := lv.Format
		if f == "" {
			f = "decimal"
		}
		b.WriteString(`<w:lvl w:ilvl="` + itoa(i) + `">`)
		if lv.Start != 0 {
			b.WriteString(valXML("start", itoa(lv.Start)))
		}
		b.WriteString(valXML("numFmt", f))
		b.WriteString(`<w:lvlText w:val="` + escAttr(lv.Text) + `"/><w:lvlJc w:val="left"/>`)
		if lv.Indent != 0 || lv.Hang != 0 {
			b.WriteString(`<w:pPr><w:ind w:left="` + ptTwips(lv.Indent) + `"`)
			if lv.Hang >= 0 {
				b.WriteString(` w:hanging="` + ptTwips(lv.Hang) + `"`)
			} else {
				b.WriteString(` w:firstLine="` + ptTwips(-lv.Hang) + `"`)
			}
			b.WriteString("/></w:pPr>")
		}
		if lv.Font != "" {
			b.WriteString(`<w:rPr><w:rFonts w:ascii="` + escAttr(lv.Font) + `" w:hAnsi="` + escAttr(lv.Font) + `" w:hint="default"/></w:rPr>`)
		}
		b.WriteString("</w:lvl>")
	}
	b.WriteString("</w:abstractNum>")
	return b.String()
}

// ---- packages ----

func deflate(zw *zip.Writer, name string, data []byte) error {
	f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

// writeInto writes the document into a copy of its source package.
func (w *writer) writeInto(out io.Writer, docXML, styles, numbering []byte) error {
	p := w.src
	replace := map[string][]byte{w.main: docXML}
	drop := map[string]bool{}
	ctData, err := p.read("[Content_Types].xml")
	if err != nil {
		return err
	}
	ct := parseContentTypes(ctData)
	ctDirty := false

	// macros and toolbars
	var keep []rel
	for _, rl := range w.rels {
		if rl.is(relVBA) || rl.is(relToolbars) {
			part := resolve(w.main, rl.Target)
			drop[part] = true
			drop[relsPath(part)] = true
			for _, sub := range p.rels(part) {
				if sub.Mode != "External" {
					drop[resolve(part, sub.Target)] = true
				}
			}
			w.relsDirty = true
			continue
		}
		keep = append(keep, rl)
	}
	w.rels = keep
	if len(drop) > 0 {
		w.warn(warnMacrosRemoved)
		for part := range drop {
			ct.removeOverride(part)
		}
		defs := ct.defaults[:0]
		for _, d := range ct.defaults {
			if !strings.Contains(d[1], "vbaProject") {
				defs = append(defs, d)
			}
		}
		ct.defaults = defs
		ctDirty = true
	}
	for i, o := range ct.overrides {
		if strings.EqualFold(o[0], "/"+w.main) && o[1] != ctMain {
			ct.overrides[i][1] = ctMain
			ctDirty = true
		}
	}

	stylesPart, numPart := "", ""
	for _, rl := range w.rels {
		switch {
		case rl.is(relStyles):
			stylesPart = resolve(w.main, rl.Target)
		case rl.is(relNumbering):
			numPart = resolve(w.main, rl.Target)
		}
	}
	addPart := func(name, typ, ctype string) string {
		dir := path.Dir(w.main)
		part := path.Join(dir, name)
		if dir == "." {
			part = name
		}
		w.rels = append(w.rels, rel{ID: w.newRelID(), Type: relTypePrefix + typ, Target: name})
		w.relsDirty = true
		ct.setOverride(part, ctype)
		ctDirty = true
		return part
	}
	if stylesPart == "" {
		stylesPart = addPart("styles.xml", relStyles, ctStyles)
	}
	replace[stylesPart] = styles
	if numbering != nil {
		if numPart == "" {
			numPart = addPart("numbering.xml", relNumbering, ctNumbering)
		}
		replace[numPart] = numbering
	}
	if w.addHFParts(replace, ct) {
		ctDirty = true
	}
	if w.addMedia(ct, replace) {
		ctDirty = true
	}
	if w.relsDirty {
		replace[relsPath(w.main)] = relsXML(w.rels)
	}
	if ctDirty {
		replace["[Content_Types].xml"] = ct.xml()
	}

	zw := zip.NewWriter(out)
	written := map[string]bool{}
	for _, f := range p.zr.File {
		name := strings.TrimPrefix(f.Name, "/")
		if drop[name] || written[name] {
			continue
		}
		written[name] = true
		if data, ok := replace[name]; ok {
			if err := deflate(zw, f.Name, data); err != nil {
				return err
			}
			continue
		}
		if err := zw.Copy(f); err != nil {
			return err
		}
	}
	var extra []string
	for name := range replace {
		if !written[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		if err := deflate(zw, name, replace[name]); err != nil {
			return err
		}
	}
	return zw.Close()
}

// writeNew writes a new minimal package.
func (w *writer) writeNew(out io.Writer, docXML, styles, numbering []byte) error {
	ct := &contentTypes{defaults: [][2]string{{"rels", ctRels}, {"xml", "application/xml"}}}
	ct.setOverride("word/document.xml", ctMain)
	ct.setOverride("word/styles.xml", ctStyles)
	ct.setOverride("word/settings.xml", ctSettings)
	if numbering != nil {
		ct.setOverride("word/numbering.xml", ctNumbering)
	}
	ct.setOverride("docProps/core.xml", ctCore)
	ct.setOverride("docProps/app.xml", ctApp)

	pkgRels := []rel{
		{"rId1", relTypePrefix + relOfficeDoc, "word/document.xml", ""},
		{"rId2", "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties", "docProps/core.xml", ""},
		{"rId3", relTypePrefix + "/extended-properties", "docProps/app.xml", ""},
	}
	docRels := w.rels // styles, settings, numbering and hyperlinks

	tab := w.doc.DefaultTabStop
	if tab <= 0 {
		tab = 36
	}
	settings := xmlHeader + `<w:settings xmlns:w="` + nsW + `"><w:defaultTabStop w:val="` + ptTwips(tab) +
		`"/><w:characterSpacingControl w:val="doNotCompress"/><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>`
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	core := xmlHeader + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified></cp:coreProperties>`
	app := xmlHeader + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>OfficeLite</Application></Properties>`

	mediaParts := map[string][]byte{}
	w.addMedia(ct, mediaParts)
	for _, p := range w.hfParts {
		ct.setOverride(p.name, p.ctype)
	}
	if w.doc.HF.EvenOdd {
		settings = strings.Replace(settings, "<w:characterSpacingControl", "<w:evenAndOddHeaders/><w:characterSpacingControl", 1)
	}

	zw := zip.NewWriter(out)
	parts := []struct {
		name string
		data []byte
	}{
		{"[Content_Types].xml", ct.xml()},
		{"_rels/.rels", relsXML(pkgRels)},
		{"word/document.xml", docXML},
		{"word/_rels/document.xml.rels", relsXML(docRels)},
		{"word/styles.xml", styles},
		{"word/settings.xml", []byte(settings)},
		{"docProps/core.xml", []byte(core)},
		{"docProps/app.xml", []byte(app)},
	}
	if numbering != nil {
		parts = append(parts, struct {
			name string
			data []byte
		}{"word/numbering.xml", numbering})
	}
	for _, p := range parts {
		if err := deflate(zw, p.name, p.data); err != nil {
			return err
		}
	}
	for _, m := range w.media {
		if err := deflate(zw, m.part, mediaParts[m.part]); err != nil {
			return err
		}
	}
	for _, p := range w.hfParts {
		if err := deflate(zw, p.name, p.data); err != nil {
			return err
		}
		if p.rels != nil {
			if err := deflate(zw, relsPath(p.name), p.rels); err != nil {
				return err
			}
		}
	}
	return zw.Close()
}
