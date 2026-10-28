package docxfile

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"officelite/word/model"
)

// Warnings shown to the user for content WordLite reads but doesn't show.
const (
	warnNotes     = "Footnotes and endnotes aren't shown; only their reference marks are (they are kept when saving as .docx)."
	warnComments  = "Comments aren't shown (they are kept when saving as .docx)."
	warnTracked   = "Tracked changes aren't shown: insertions appear as normal text and deletions are hidden."
	warnColumns   = "Text columns aren't shown; the text is laid out in one column."
	warnPictures  = "Shapes, charts, equations and pictures in formats WordLite can't draw (such as EMF) are shown as placeholders."
	warnFields    = "Fields (page numbers, tables of contents...) show their last result and aren't updated."
	warnMacros    = "This document contains macros. WordLite never runs them, and they are removed when the document is saved."
	warnTextBoxes = "Text boxes are shown as placeholders."
)

// Read decodes a .docx (or .docm/.dotx) document.
func Read(data []byte) (*model.Document, error) {
	p, err := openPkg(data)
	if err != nil {
		return nil, err
	}
	main := p.mainPart()
	docXML, err := p.read(main)
	if err != nil {
		return nil, fmt.Errorf("not a Word document: %w", err)
	}
	root, err := parseXML(docXML)
	if err != nil {
		return nil, fmt.Errorf("document.xml: %w", err)
	}
	r := &reader{doc: &model.Document{Page: model.LetterPage(), Lists: map[int]*model.List{}}, warned: map[string]bool{}}
	r.pkg, r.main = p, main
	r.rels = map[string]rel{}
	for _, rl := range p.rels(main) {
		r.rels[rl.ID] = rl
		part := resolve(main, rl.Target)
		switch {
		case rl.is(relTheme):
			if b, err := p.read(part); err == nil {
				r.th = parseTheme(b)
			}
		case rl.is(relVBA):
			r.warn(warnMacros)
		}
	}
	for _, rl := range r.rels {
		part := resolve(main, rl.Target)
		switch {
		case rl.is(relStyles):
			if b, err := p.read(part); err == nil {
				if sr, err := parseXML(b); err == nil {
					r.doc.Styles = parseStyles(sr, r.th)
				}
			}
		case rl.is(relSettings):
			if b, err := p.read(part); err == nil {
				if sr, err := parseXML(b); err == nil {
					if v, ok := twipsPt(sr.child("w:defaultTabStop").attr("w:val")); ok && v > 0 {
						r.doc.DefaultTabStop = v
					}
				}
			}
		}
	}
	for _, rl := range r.rels {
		if rl.is(relNumbering) {
			if b, err := p.read(resolve(main, rl.Target)); err == nil {
				if nr, err := parseXML(b); err == nil {
					r.doc.Lists = parseNumbering(nr, &r.doc.Styles)
				}
			}
		}
	}
	body := root.child("w:body")
	if body == nil {
		return nil, fmt.Errorf("document.xml has no body")
	}
	r.doc.Body = r.blocks(body, nil, true)
	if len(r.doc.Body) == 0 {
		p := &model.Paragraph{}
		r.attach(p)
		r.doc.Body = []model.Block{p}
	}
	r.flushTrailing(r.doc.Body)
	r.readHeaders(r.sects)
	if r.sections > 1 {
		r.warn(fmt.Sprintf("This document has %d sections; WordLite uses the last section's page size and margins for all of them.", r.sections))
	}
	r.doc.Source = data
	r.doc.SourceFormat = "docx"
	return r.doc, nil
}

type reader struct {
	doc    *model.Document
	th     *theme
	rels   map[string]rel
	pkg    *pkg   // the package, for reading pictures
	main   string // main part name
	warned map[string]bool

	fields    []fieldState // open complex fields that span paragraphs
	markers   []model.Run  // range markers waiting for the next paragraph
	before    []*model.Opaque
	lastPara  *model.Paragraph
	footnotes int
	endnotes  int
	sections  int
	sects     []*node // every w:sectPr, in order (for headers and footers)
}

type fieldState struct{ result bool }

func (r *reader) warn(s string) {
	if !r.warned[s] {
		r.warned[s] = true
		r.doc.Warnings = append(r.doc.Warnings, s)
	}
}

func opaque(data []byte) *model.Opaque { return &model.Opaque{Format: "docx", Data: data} }

// rangeMarkers are elements allowed both between and inside paragraphs that
// mark a position (bookmarks, comment ranges...).
var rangeMarkers = map[string]bool{
	"w:bookmarkStart": true, "w:bookmarkEnd": true, "w:commentRangeStart": true, "w:commentRangeEnd": true,
	"w:permStart": true, "w:permEnd": true, "w:moveFromRangeStart": true, "w:moveFromRangeEnd": true,
	"w:moveToRangeStart": true, "w:moveToRangeEnd": true, "w:customXmlInsRangeStart": true,
	"w:customXmlInsRangeEnd": true, "w:customXmlDelRangeStart": true, "w:customXmlDelRangeEnd": true,
	"w:customXmlMoveFromRangeStart": true, "w:customXmlMoveFromRangeEnd": true,
	"w:customXmlMoveToRangeStart": true, "w:customXmlMoveToRangeEnd": true,
}

// ignored are elements that only cache state Word recomputes.
var ignored = map[string]bool{"w:proofErr": true, "w:lastRenderedPageBreak": true}

func (r *reader) markerRun(k *node) model.Run {
	if strings.HasPrefix(k.name, "w:comment") {
		r.warn(warnComments)
	}
	if strings.HasPrefix(k.name, "w:move") {
		r.warn(warnTracked)
	}
	return model.Run{Kind: model.RunOpaque, Opaque: opaque(k.raw())}
}

// attach gives a new block the content waiting for it.
func (r *reader) attach(b model.Block) {
	switch b := b.(type) {
	case *model.Paragraph:
		b.Before = r.before
		if len(r.markers) > 0 {
			b.Runs = append(append([]model.Run(nil), r.markers...), b.Runs...)
		}
		r.markers = nil
		r.lastPara = b
	case *model.Table:
		b.Before = r.before
	}
	r.before = nil
}

// flushTrailing puts content left after the last block of a container into
// it: markers at the end of the last paragraph, other content before the
// last block.
func (r *reader) flushTrailing(blocks []model.Block) {
	if len(blocks) == 0 {
		return
	}
	if len(r.markers) > 0 && r.lastPara != nil {
		r.lastPara.Runs = append(r.lastPara.Runs, r.markers...)
		r.markers = nil
	}
	if len(r.before) > 0 {
		switch b := blocks[len(blocks)-1].(type) {
		case *model.Paragraph:
			b.Before = append(b.Before, r.before...)
		case *model.Table:
			b.Before = append(b.Before, r.before...)
		}
		r.before = nil
	}
}

// wrapSpan makes a Span for an element that wraps content: Start is
// everything up to the first content child (properties included).
func wrapSpan(k, firstContent *node, parent *model.Span) *model.Span {
	startEnd, endStart := k.open, k.close
	if firstContent != nil {
		startEnd = firstContent.start
	}
	return &model.Span{Format: "docx", Start: k.rawRange(k.start, startEnd), End: append([]byte(nil), k.src[endStart:k.end]...), Parent: parent}
}

// contentOf returns the children of a wrapper that hold its content: the
// sdtContent's children for content controls, otherwise the children after
// the properties element.
func contentOf(k *node) (kids []*node, span *node, first *node) {
	switch k.name {
	case "w:sdt":
		c := k.child("w:sdtContent")
		if c == nil || c.open == c.end {
			return nil, nil, nil
		}
		return c.kids, c, nil
	}
	for i, c := range k.kids {
		if !strings.HasSuffix(c.name, "Pr") {
			return k.kids[i:], nil, c
		}
	}
	return nil, nil, nil
}

func (r *reader) wrapper(k *node, parent *model.Span) ([]*node, *model.Span) {
	kids, sc, first := contentOf(k)
	if len(kids) == 0 {
		return nil, nil
	}
	if sc != nil { // content control: Start runs to the end of <w:sdtContent>
		return kids, &model.Span{Format: "docx", Start: k.rawRange(k.start, sc.open),
			End: append([]byte(nil), k.src[sc.close:k.end]...), Parent: parent}
	}
	return kids, wrapSpan(k, first, parent)
}

// blocks reads block-level content (body, table cell, block content control).
func (r *reader) blocks(container *node, wrap *model.Span, isBody bool) []model.Block {
	var out []model.Block
	for _, k := range container.kids {
		switch k.name {
		case "w:p":
			p := r.para(k)
			p.Wrap = wrap
			r.attach(p)
			out = append(out, p)
		case "w:tbl":
			t := r.table(k)
			t.Wrap = wrap
			out = append(out, t)
		case "w:sdt", "w:customXml":
			kids, span := r.wrapper(k, wrap)
			var inner []model.Block
			if kids != nil {
				inner = r.blocks(&node{kids: kids}, span, false)
			}
			if len(inner) == 0 {
				r.before = append(r.before, opaque(k.raw()))
				continue
			}
			out = append(out, inner...)
		case "w:sectPr":
			if isBody {
				r.section(k)
				r.doc.Page = parsePage(k.kids)
			} else {
				r.before = append(r.before, opaque(k.raw()))
			}
		case "w:tcPr":
		default:
			switch {
			case ignored[k.name]:
			case rangeMarkers[k.name]:
				r.markers = append(r.markers, r.markerRun(k))
			default:
				r.before = append(r.before, opaque(k.raw()))
			}
		}
	}
	return out
}

func (r *reader) section(s *node) {
	r.sections++
	r.sects = append(r.sects, s)
	for _, k := range s.kids {
		switch k.name {
		case "w:cols":
			if atoi(k.attr("w:num")) > 1 || len(k.children("w:col")) > 1 {
				r.warn(warnColumns)
			}
		}
	}
}

func parsePage(kids []*node) model.PageSetup {
	pg := model.LetterPage()
	for _, k := range kids {
		switch k.name {
		case "w:pgSz":
			if v, ok := twipsPt(k.attr("w:w")); ok {
				pg.Width = v
			}
			if v, ok := twipsPt(k.attr("w:h")); ok {
				pg.Height = v
			}
			pg.Landscape = k.attr("w:orient") == "landscape"
		case "w:pgMar":
			for _, f := range []struct {
				a string
				p *float64
			}{{"w:top", &pg.MarginTop}, {"w:bottom", &pg.MarginBottom}, {"w:left", &pg.MarginLeft},
				{"w:right", &pg.MarginRight}, {"w:header", &pg.Header}, {"w:footer", &pg.Footer}} {
				if v, ok := twipsPt(k.attr(f.a)); ok {
					*f.p = v
				}
			}
		}
	}
	return pg
}

// ---- paragraphs ----

func (r *reader) para(pn *node) *model.Paragraph {
	p := &model.Paragraph{}
	var content []*node
	for _, k := range pn.kids {
		if k.name != "w:pPr" {
			content = append(content, k)
			continue
		}
		p.Props, p.Style = parsePPr(k.kids, r.th)
		p.Props.DocxXML = rawKids(k, "w:rPr")
		if rp := k.child("w:rPr"); rp != nil {
			p.Mark, _ = parseRPr(rp.kids, r.th)
			p.Mark.DocxXML = rawKids(rp)
			if rp.child("w:ins") != nil || rp.child("w:del") != nil {
				r.warn(warnTracked)
			}
		}
		if s := k.child("w:sectPr"); s != nil {
			r.section(s)
		}
		if k.child("w:pPrChange") != nil {
			r.warn(warnTracked)
		}
	}
	p.Runs = r.fieldRuns(r.inline(content, nil))
	return p
}

// token is one inline element: a w:r (possibly split into several model
// runs) or another element shown as one opaque run.
type token struct {
	runs  []model.Run
	raw   []byte
	fld   int // fldBegin...
	instr string
	span  *model.Span
}

const (
	fldNone = iota
	fldBegin
	fldSeparate
	fldEnd
	fldInstr
)

func (r *reader) inline(kids []*node, span *model.Span) []token {
	var out []token
	single := func(k *node, run model.Run) {
		run.Span = span
		out = append(out, token{runs: []model.Run{run}, raw: k.raw(), span: span})
	}
	for _, k := range kids {
		switch k.name {
		case "w:r":
			out = append(out, r.runToken(k, span))
		case "w:hyperlink":
			kids, ls := r.wrapper(k, span)
			if kids == nil {
				single(k, model.Run{Kind: model.RunOpaque, Opaque: opaque(k.raw())})
				continue
			}
			ls.Kind = model.SpanLink
			ls.Start = k.rawRange(k.start, k.open)
			ls.End = append([]byte(nil), k.src[k.close:k.end]...)
			ls.Anchor = k.attr("w:anchor")
			ls.Tooltip = k.attr("w:tooltip")
			if rl, ok := r.rels[k.attr("r:id")]; ok {
				ls.URL = rl.Target
			}
			out = append(out, r.inline(kids, ls)...)
		case "w:fldSimple":
			r.warn(warnFields)
			inner := r.inline(k.kids, nil)
			run := model.Run{Kind: model.RunOpaque, Opaque: opaque(k.raw())}
			run.Opaque.Field = strings.TrimSpace(k.attr("w:instr"))
			first := true
			var text strings.Builder
			for _, t := range inner {
				for _, ir := range t.runs {
					if first {
						run.Style, run.Props, first = ir.Style, ir.Props, false
					}
					text.WriteString(runText(ir))
					if ir.Opaque != nil && run.Opaque.Width == 0 {
						run.Opaque.Width, run.Opaque.Height = ir.Opaque.Width, ir.Opaque.Height
					}
				}
			}
			run.Text = text.String()
			single(k, run)
		case "w:sdt", "w:smartTag", "w:customXml", "w:ins", "w:moveTo", "w:dir", "w:bdo":
			if k.name == "w:ins" || k.name == "w:moveTo" {
				r.warn(warnTracked)
			}
			kids, ws := r.wrapper(k, span)
			if len(kids) == 0 {
				single(k, model.Run{Kind: model.RunOpaque, Opaque: opaque(k.raw())})
				continue
			}
			out = append(out, r.inline(kids, ws)...)
		case "w:del", "w:moveFrom":
			r.warn(warnTracked)
			single(k, model.Run{Kind: model.RunOpaque, Opaque: opaque(k.raw())})
		case "m:oMath", "m:oMathPara":
			r.warn(warnPictures)
			text := k.allText("m:t")
			if text == "" {
				text = "[equation]"
			}
			single(k, model.Run{Kind: model.RunOpaque, Text: text, Opaque: opaque(k.raw())})
		default:
			switch {
			case ignored[k.name]:
			case rangeMarkers[k.name]:
				single(k, r.markerRun(k))
			default:
				run := model.Run{Kind: model.RunOpaque, Opaque: opaque(k.raw())}
				r.describe(k, &run)
				single(k, run)
			}
		}
	}
	return out
}

func runText(ir model.Run) string {
	switch ir.Kind {
	case model.RunTab:
		return "\t"
	case model.RunBreak:
		return "\n"
	case model.RunPageBreak:
		return "\f"
	}
	return ir.Text
}

// runToken reads a <w:r>.
func (r *reader) runToken(rn *node, span *model.Span) token {
	tok := token{raw: rn.raw(), span: span}
	rpr := rn.child("w:rPr")
	props, style := parseRPr(rpr.childList(), r.th)
	props.DocxXML = rawKids(rpr)
	if rpr.child("w:ins") != nil || rpr.child("w:del") != nil || rpr.child("w:rPrChange") != nil {
		r.warn(warnTracked)
	}
	for _, k := range rn.kids {
		switch k.name {
		case "w:fldChar":
			r.warn(warnFields)
			switch k.attr("w:fldCharType") {
			case "begin":
				tok.fld = fldBegin
			case "separate":
				tok.fld = fldSeparate
			case "end":
				tok.fld = fldEnd
			}
		case "w:instrText", "w:delInstrText":
			if tok.fld == fldNone {
				tok.fld = fldInstr
			}
			tok.instr += k.text
		}
	}
	if tok.fld != fldNone {
		tok.runs = []model.Run{{Kind: model.RunOpaque, Style: style, Props: props, Span: span, Opaque: opaque(tok.raw)}}
		return tok
	}
	var text strings.Builder
	add := func(run model.Run) {
		run.Style, run.Props, run.Span = style, props, span
		tok.runs = append(tok.runs, run)
	}
	flush := func() {
		if text.Len() > 0 {
			add(model.Run{Text: text.String()})
			text.Reset()
		}
	}
	for _, k := range rn.kids {
		switch k.name {
		case "w:rPr":
		case "w:t":
			text.WriteString(k.text)
		case "w:noBreakHyphen":
			text.WriteString("‑")
		case "w:softHyphen":
			text.WriteString("­")
		case "w:tab":
			flush()
			add(model.Run{Kind: model.RunTab})
		case "w:br":
			flush()
			if k.attr("w:type") == "page" {
				add(model.Run{Kind: model.RunPageBreak})
			} else {
				add(model.Run{Kind: model.RunBreak})
			}
		case "w:cr":
			flush()
			add(model.Run{Kind: model.RunBreak})
		default:
			if ignored[k.name] {
				continue
			}
			flush()
			data := "<w:r>"
			if rpr != nil {
				data += string(rpr.raw())
			}
			data += string(k.raw()) + "</w:r>"
			run := model.Run{Kind: model.RunOpaque, Opaque: opaque([]byte(data))}
			r.describe(k, &run)
			add(run)
			if k.name == "w:sym" { // add() overwrote Props
				tok.runs[len(tok.runs)-1].Props.Font = k.attr("w:font")
			}
		}
	}
	flush()
	return tok
}

func (n *node) childList() []*node {
	if n == nil {
		return nil
	}
	return n.kids
}

var reStylePt = regexp.MustCompile(`(?i)(width|height)\s*:\s*([0-9.]+)\s*(pt|in|px|cm|mm)?`)

// describe sets the placeholder text and size of an opaque inline element.
func (r *reader) describe(k *node, run *model.Run) {
	o := run.Opaque
	switch k.name {
	case "w:drawing", "mc:AlternateContent":
		run.Text = "[picture]"
		if ext := k.find("wp:extent"); ext != nil {
			cx, _ := strconv.ParseFloat(ext.attr("cx"), 64)
			cy, _ := strconv.ParseFloat(ext.attr("cy"), 64)
			o.Width, o.Height = cx/12700, cy/12700
		}
		if gd := k.find("a:graphicData"); gd != nil {
			uri := gd.attr("uri")
			switch {
			case strings.HasSuffix(uri, "/chart") || strings.Contains(uri, "chartex"):
				run.Text = "[chart]"
			case strings.HasSuffix(uri, "/diagram"):
				run.Text = "[SmartArt]"
			case strings.Contains(uri, "wordprocessingShape") || strings.Contains(uri, "wordprocessingGroup") || strings.Contains(uri, "wordprocessingCanvas"):
				run.Text = "[shape]"
			}
		}
		if k.find("w:txbxContent") != nil {
			run.Text = "[text box]"
		}
		if k.name == "mc:AlternateContent" && k.find("wp:extent") == nil {
			r.vmlSize(k, run)
		}
		if run.Text == "[picture]" {
			if blip := k.find("a:blip"); blip != nil {
				o.Picture = r.picture(blip.attr("r:embed"))
			}
		}
		if o.Picture == nil {
			r.warn(warnPictures)
		}
	case "w:pict", "w:object":
		run.Text = "[picture]"
		if k.name == "w:object" {
			run.Text = "[object]"
			if w, ok := twipsPt(k.attr("w:dxaOrig")); ok {
				o.Width = w
			}
			if h, ok := twipsPt(k.attr("w:dyaOrig")); ok {
				o.Height = h
			}
		}
		if k.find("v:imagedata") == nil && k.name == "w:pict" {
			run.Text = "[shape]"
		}
		if k.find("w:txbxContent") != nil {
			run.Text = "[text box]"
		}
		r.vmlSize(k, run)
		if run.Text == "[picture]" {
			if im := k.find("v:imagedata"); im != nil {
				o.Picture = r.picture(im.attr("r:id"))
			}
		}
		if o.Picture == nil {
			r.warn(warnPictures)
		}
	case "w:footnoteReference":
		r.warn(warnNotes)
		r.footnotes++
		run.Text = strconv.Itoa(r.footnotes)
	case "w:endnoteReference":
		r.warn(warnNotes)
		r.endnotes++
		run.Text = roman(r.endnotes)
	case "w:commentReference":
		r.warn(warnComments)
	case "w:sym":
		if v, err := strconv.ParseUint(k.attr("w:char"), 16, 32); err == nil {
			if v < 0x100 && v >= 0x20 {
				v += 0xF000
			}
			run.Text = string(rune(v))
		}
	case "w:ptab":
		run.Text = "\t"
	case "w:ruby":
		run.Text = k.child("w:rubyBase").allText("w:t")
	case "w:delText":
		r.warn(warnTracked)
	}
}

func (r *reader) vmlSize(k *node, run *model.Run) {
	if run.Opaque.Width > 0 {
		return
	}
	var style string
	for _, n := range []string{"v:shape", "v:rect", "v:roundrect", "v:oval", "v:group", "v:line"} {
		if s := k.find(n); s != nil {
			style = s.attr("style")
			break
		}
	}
	for _, m := range reStylePt.FindAllStringSubmatch(style, -1) {
		v, _ := strconv.ParseFloat(m[2], 64)
		switch strings.ToLower(m[3]) {
		case "in":
			v *= 72
		case "px":
			v *= 0.75
		case "cm":
			v *= 72 / 2.54
		case "mm":
			v *= 72 / 25.4
		}
		if strings.EqualFold(m[1], "width") {
			run.Opaque.Width = v
		} else {
			run.Opaque.Height = v
		}
	}
}

func roman(n int) string {
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

// fieldRuns turns a paragraph's tokens into runs. A complex field whose
// begin and end are in this paragraph, in the same container, becomes one
// opaque run showing its result; a field that spans paragraphs keeps its
// result as ordinary runs and its field characters/instruction as hidden
// opaque runs.
func (r *reader) fieldRuns(toks []token) []model.Run {
	var out []model.Run
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		hidden := false
		for _, f := range r.fields {
			if !f.result {
				hidden = true
			}
		}
		if t.fld == fldBegin && !hidden {
			if j := matchField(toks, i); j > 0 {
				out = append(out, fieldUnit(toks[i:j+1]))
				i = j
				continue
			}
		}
		switch t.fld {
		case fldBegin:
			r.fields = append(r.fields, fieldState{})
			hidden = true
		case fldSeparate:
			if n := len(r.fields); n > 0 {
				r.fields[n-1].result = true
			}
			hidden = true
		case fldEnd:
			if n := len(r.fields); n > 0 {
				r.fields = r.fields[:n-1]
			}
			hidden = true
		case fldInstr:
			hidden = true
		}
		if !hidden {
			out = append(out, t.runs...)
			continue
		}
		run := model.Run{Kind: model.RunOpaque, Span: t.span, Opaque: opaque(t.raw)}
		if len(t.runs) > 0 {
			run.Style, run.Props = t.runs[0].Style, t.runs[0].Props
		}
		if t.fld == fldInstr {
			run.Opaque.Field = strings.TrimSpace(t.instr)
		}
		out = append(out, run)
	}
	return out
}

// matchField returns the index of the end of the field beginning at i, or
// -1 if it isn't in toks or crosses a container boundary.
func matchField(toks []token, i int) int {
	depth := 0
	for j := i; j < len(toks); j++ {
		if toks[j].span != toks[i].span {
			return -1
		}
		switch toks[j].fld {
		case fldBegin:
			depth++
		case fldEnd:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func fieldUnit(toks []token) model.Run {
	run := model.Run{Kind: model.RunOpaque, Span: toks[0].span, Opaque: &model.Opaque{Format: "docx"}}
	var data []byte
	var text, instr strings.Builder
	var stack []bool // per open field: in result
	gotProps := false
	for _, t := range toks {
		data = append(data, t.raw...)
		switch t.fld {
		case fldBegin:
			stack = append(stack, false)
			continue
		case fldSeparate:
			stack[len(stack)-1] = true
			continue
		case fldEnd:
			stack = stack[:len(stack)-1]
			continue
		case fldInstr:
			if len(stack) == 1 {
				instr.WriteString(t.instr)
			}
			continue
		}
		visible := true
		for _, s := range stack {
			visible = visible && s
		}
		if !visible {
			continue
		}
		for _, ir := range t.runs {
			if !gotProps {
				run.Style, run.Props, gotProps = ir.Style, ir.Props, true
			}
			text.WriteString(runText(ir))
			if ir.Opaque != nil && run.Opaque.Width == 0 {
				run.Opaque.Width, run.Opaque.Height = ir.Opaque.Width, ir.Opaque.Height
			}
		}
	}
	if !gotProps && len(toks[0].runs) > 0 {
		run.Style, run.Props = toks[0].runs[0].Style, toks[0].runs[0].Props
	}
	run.Text = text.String()
	run.Opaque.Data = data
	run.Opaque.Field = strings.TrimSpace(instr.String())
	return run
}

// ---- tables ----

func (r *reader) table(tn *node) *model.Table {
	t := &model.Table{Before: r.before}
	r.before = nil
	if pr := tn.child("w:tblPr"); pr != nil {
		tp := parseTblPr(pr.kids, &r.doc.Styles)
		t.Style, t.Align, t.Indent, t.Borders = tp.style, tp.align, tp.indent, tp.borders
		t.DocxXML = rawKids(pr)
	}
	for _, g := range tn.child("w:tblGrid").childList() {
		if g.name == "w:gridCol" {
			w, _ := twipsPt(g.attr("w:w"))
			t.Columns = append(t.Columns, w)
		}
	}
	r.rows(tn.kids, t)
	return t
}

func (r *reader) rows(kids []*node, t *model.Table) {
	for _, k := range kids {
		switch k.name {
		case "w:tblPr", "w:tblGrid":
		case "w:tr":
			t.Rows = append(t.Rows, r.row(k))
		case "w:sdt", "w:customXml":
			if c, _, _ := contentOf(k); c != nil {
				r.rows(c, t)
			}
		default:
			if rangeMarkers[k.name] {
				r.markers = append(r.markers, r.markerRun(k))
			} else if !ignored[k.name] {
				r.before = append(r.before, opaque(k.raw()))
			}
		}
	}
}

func (r *reader) row(rn *node) *model.TableRow {
	row := &model.TableRow{}
	var extra []byte
	if ex := rn.child("w:tblPrEx"); ex != nil {
		extra = append(extra, ex.raw()...)
	}
	if pr := rn.child("w:trPr"); pr != nil {
		row.Height, row.Header = parseTrPr(pr.kids)
		extra = append(extra, rawKids(pr)...)
		if pr.child("w:ins") != nil || pr.child("w:del") != nil {
			r.warn(warnTracked)
		}
	}
	row.DocxXML = string(extra)
	r.cells(rn.kids, row)
	return row
}

func (r *reader) cells(kids []*node, row *model.TableRow) {
	for _, k := range kids {
		switch k.name {
		case "w:tblPrEx", "w:trPr":
		case "w:tc":
			c := &model.TableCell{Span: 1}
			if pr := k.child("w:tcPr"); pr != nil {
				tc := parseTcPr(pr.kids, r.th)
				c.Width, c.Span, c.VMerge, c.Shading, c.VAlign = tc.width, tc.span, tc.vmerge, tc.shading, tc.valign
				c.DocxXML = rawKids(pr)
			}
			c.Blocks = r.blocks(k, nil, false)
			r.flushTrailing(c.Blocks)
			if len(c.Blocks) == 0 {
				p := &model.Paragraph{}
				r.attach(p)
				c.Blocks = []model.Block{p}
			}
			row.Cells = append(row.Cells, c)
		case "w:sdt", "w:customXml":
			if c, _, _ := contentOf(k); c != nil {
				r.cells(c, row)
			}
		default:
			if rangeMarkers[k.name] {
				r.markers = append(r.markers, r.markerRun(k))
			} else if !ignored[k.name] {
				r.before = append(r.before, opaque(k.raw()))
			}
		}
	}
}

type tblProps struct {
	style   string
	align   model.Align
	indent  float64
	borders bool
}

// parseTblPr reads a <w:tblPr>; borders come from the table or, if it has
// none, its style.
func parseTblPr(kids []*node, ss *model.StyleSheet) tblProps {
	var tp tblProps
	explicit := false
	for _, k := range kids {
		switch k.name {
		case "w:tblStyle":
			tp.style = k.attr("w:val")
		case "w:jc":
			switch k.attr("w:val") {
			case "center":
				tp.align = model.AlignCenter
			case "right", "end":
				tp.align = model.AlignRight
			case "left", "start":
				tp.align = model.AlignLeft
			}
		case "w:tblInd":
			if t := k.attr("w:type"); t == "" || t == "dxa" {
				tp.indent, _ = twipsPt(k.attr("w:w"))
			}
		case "w:tblBorders":
			explicit = true
			tp.borders = hasBorders(k)
		}
	}
	if !explicit {
		tp.borders = styleBorders(tp.style, ss)
	}
	return tp
}

func hasBorders(k *node) bool {
	for _, b := range k.kids {
		if v := b.attr("w:val"); v != "" && v != "nil" && v != "none" {
			return true
		}
	}
	return false
}

func styleBorders(id string, ss *model.StyleSheet) bool {
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		st := ss.Find(id)
		if st == nil {
			if def := ss.Default(model.TableStyle); def != nil && !seen[def.ID] {
				st = def
			} else {
				return false
			}
		}
		for _, k := range parseFragment(st.DocxXML) {
			if k.name == "w:tblPr" {
				if b := k.child("w:tblBorders"); b != nil {
					return hasBorders(b)
				}
			}
		}
		id = st.BasedOn
	}
	return false
}

func parseTrPr(kids []*node) (height float64, header bool) {
	for _, k := range kids {
		switch k.name {
		case "w:trHeight":
			height, _ = twipsPt(k.attr("w:val"))
		case "w:tblHeader":
			header = onOff(k) == model.On
		}
	}
	return
}

type tcProps struct {
	width   float64
	span    int
	vmerge  int8
	shading string
	valign  int8
}

func parseTcPr(kids []*node, th *theme) tcProps {
	tc := tcProps{span: 1}
	for _, k := range kids {
		switch k.name {
		case "w:tcW":
			if t := k.attr("w:type"); t == "" || t == "dxa" {
				tc.width, _ = twipsPt(k.attr("w:w"))
			}
		case "w:gridSpan":
			if v := atoi(k.attr("w:val")); v > 1 {
				tc.span = v
			}
		case "w:vMerge":
			if k.attr("w:val") == "restart" {
				tc.vmerge = 1
			} else {
				tc.vmerge = 2
			}
		case "w:shd":
			tc.shading = shdColor(k, th)
		case "w:vAlign":
			switch k.attr("w:val") {
			case "center":
				tc.valign = 1
			case "bottom":
				tc.valign = 2
			}
		}
	}
	return tc
}

// ---- theme, styles, numbering ----

func parseTheme(data []byte) *theme {
	root, err := parseXML(data)
	if err != nil {
		return nil
	}
	th := &theme{colors: map[string]string{}}
	if fs := root.find("a:fontScheme"); fs != nil {
		th.major = fs.child("a:majorFont").child("a:latin").attr("typeface")
		th.minor = fs.child("a:minorFont").child("a:latin").attr("typeface")
	}
	scheme := map[string]string{}
	if cs := root.find("a:clrScheme"); cs != nil {
		for _, c := range cs.kids {
			name := strings.TrimPrefix(c.name, "a:")
			if s := c.child("a:srgbClr"); s != nil {
				scheme[name] = strings.ToUpper(s.attr("val"))
			} else if s := c.child("a:sysClr"); s != nil {
				scheme[name] = strings.ToUpper(s.attr("lastClr"))
			}
		}
	}
	for w, a := range map[string]string{
		"dark1": "dk1", "light1": "lt1", "dark2": "dk2", "light2": "lt2",
		"text1": "dk1", "background1": "lt1", "text2": "dk2", "background2": "lt2",
		"accent1": "accent1", "accent2": "accent2", "accent3": "accent3", "accent4": "accent4",
		"accent5": "accent5", "accent6": "accent6", "hyperlink": "hlink", "followedHyperlink": "folHlink",
	} {
		if v := scheme[a]; v != "" {
			th.colors[w] = v
		}
	}
	return th
}

var styleTypes = map[string]model.StyleType{
	"paragraph": model.ParagraphStyle, "character": model.CharacterStyle,
	"table": model.TableStyle, "numbering": model.NumberingStyle,
}

func parseStyles(root *node, th *theme) model.StyleSheet {
	var ss model.StyleSheet
	if dd := root.child("w:docDefaults"); dd != nil {
		if rp := dd.child("w:rPrDefault").child("w:rPr"); rp != nil {
			ss.DefaultRun, _ = parseRPr(rp.kids, th)
			ss.DefaultRun.DocxXML = rawKids(rp)
		}
		if pp := dd.child("w:pPrDefault").child("w:pPr"); pp != nil {
			ss.DefaultPara, _ = parsePPr(pp.kids, th)
			ss.DefaultPara.DocxXML = rawKids(pp, "w:rPr")
		}
	}
	for _, k := range root.children("w:style") {
		st := &model.Style{ID: k.attr("w:styleId")}
		st.Type = styleTypes[k.attr("w:type")]
		if k.attr("w:type") == "" {
			st.Type = model.ParagraphStyle
		}
		st.Default = onOffAttr(k.attr("w:default"))
		st.Custom = onOffAttr(k.attr("w:customStyle"))
		parseStyleKids(st, k.kids)
		if pp := k.child("w:pPr"); pp != nil {
			st.Para, _ = parsePPr(pp.kids, th)
			st.Para.DocxXML = rawKids(pp, "w:rPr")
		}
		if rp := k.child("w:rPr"); rp != nil {
			st.Run, _ = parseRPr(rp.kids, th)
			st.Run.DocxXML = rawKids(rp)
		}
		st.DocxXML = rawKids(k, "w:pPr", "w:rPr")
		ss.Styles = append(ss.Styles, st)
	}
	return ss
}

// parseStyleKids sets the fields of st that come from <w:style> children
// other than pPr/rPr.
func parseStyleKids(st *model.Style, kids []*node) {
	st.Name, st.BasedOn, st.Next, st.Hidden = "", "", "", false
	for _, c := range kids {
		switch c.name {
		case "w:name":
			st.Name = c.attr("w:val")
		case "w:basedOn":
			st.BasedOn = c.attr("w:val")
		case "w:next":
			st.Next = c.attr("w:val")
		case "w:semiHidden", "w:hidden":
			if onOff(c) == model.On {
				st.Hidden = true
			}
		}
	}
}

func onOffAttr(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "on":
		return true
	}
	return false
}

// parseNumbering reads numbering.xml into lists keyed by numId.
func parseNumbering(root *node, ss *model.StyleSheet) map[int]*model.List {
	abstract := map[string]*node{}
	nums := map[string]*node{}
	for _, k := range root.kids {
		switch k.name {
		case "w:abstractNum":
			abstract[k.attr("w:abstractNumId")] = k
		case "w:num":
			nums[k.attr("w:numId")] = k
		}
	}
	// abstractFor follows numStyleLink to the numbering style's list.
	var abstractFor func(n *node, depth int) *node
	abstractFor = func(n *node, depth int) *node {
		a := abstract[n.child("w:abstractNumId").attr("w:val")]
		if a == nil || depth > 4 {
			return a
		}
		if link := a.child("w:numStyleLink").attr("w:val"); link != "" {
			if st := ss.Find(link); st != nil && st.Para.List != nil {
				if n2 := nums[strconv.Itoa(st.Para.List.NumID)]; n2 != nil && n2 != n {
					if a2 := abstractFor(n2, depth+1); a2 != nil {
						return a2
					}
				}
			}
		}
		return a
	}
	lists := map[int]*model.List{}
	for id, n := range nums {
		numID, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		l := &model.List{}
		if a := abstractFor(n, 0); a != nil {
			for _, lv := range a.children("w:lvl") {
				setLevel(l, lv)
			}
		}
		for _, ov := range n.children("w:lvlOverride") {
			if lv := ov.child("w:lvl"); lv != nil {
				if lv.attr("w:ilvl") == "" {
					lv.attrs = append(lv.attrs, ov.attrs...)
				}
				setLevel(l, lv)
			}
			if so := ov.child("w:startOverride"); so != nil {
				i := atoi(ov.attr("w:ilvl"))
				if i >= 0 && i < 9 {
					for len(l.Levels) <= i {
						l.Levels = append(l.Levels, model.ListLevel{})
					}
					l.Levels[i].Start = atoi(so.attr("w:val"))
				}
			}
		}
		lists[numID] = l
	}
	return lists
}

func setLevel(l *model.List, lv *node) {
	i := atoi(lv.attr("w:ilvl"))
	if i < 0 || i > 8 {
		return
	}
	for len(l.Levels) <= i {
		l.Levels = append(l.Levels, model.ListLevel{})
	}
	lvl := model.ListLevel{
		Format: lv.child("w:numFmt").attr("w:val"),
		Text:   lv.child("w:lvlText").attr("w:val"),
		Start:  atoi(lv.child("w:start").attr("w:val")),
	}
	if ind := lv.child("w:pPr").child("w:ind"); ind != nil {
		for _, a := range []string{"w:left", "w:start"} {
			if v, ok := twipsPt(ind.attr(a)); ok {
				lvl.Indent = v
				break
			}
		}
		if v, ok := twipsPt(ind.attr("w:hanging")); ok {
			lvl.Hang = v
		} else if v, ok := twipsPt(ind.attr("w:firstLine")); ok {
			lvl.Hang = -v
		}
	}
	if f := lv.child("w:rPr").child("w:rFonts"); f != nil {
		lvl.Font = f.attr("w:ascii")
		if lvl.Font == "" {
			lvl.Font = f.attr("w:hAnsi")
		}
	}
	l.Levels[i] = lvl
}

// picture reads the image part relationship id points at, or returns nil.
func (r *reader) picture(id string) *model.Picture {
	rl, ok := r.rels[id]
	if !ok || id == "" || rl.Mode == "External" || r.pkg == nil {
		return nil
	}
	part := resolve(r.main, rl.Target)
	data, err := r.pkg.read(part)
	if err != nil || len(data) == 0 {
		return nil
	}
	pic := &model.Picture{Data: data, Ext: model.PictureExt(part), Name: path.Base(part)}
	if !pic.Decodable() {
		return nil // EMF, WMF...: a placeholder
	}
	return pic
}
