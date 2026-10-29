package docxfile

import (
	"strconv"
	"strings"

	"officelite/common/opc"
	"officelite/word/model"
)

// Sections and notes. Each w:sectPr (in the pPr of a section's last
// paragraph, and the body's last child for the last section) holds a
// section's page setup, break type, columns, page numbering and header
// references; the properties the model doesn't cover are kept in
// Section.DocxXML. Footnotes and endnotes live in footnotes.xml and
// endnotes.xml, referenced by id; their separator notes are kept as
// they were.

const (
	ctFootnotes = "application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml"
	ctEndnotes  = "application/vnd.openxmlformats-officedocument.wordprocessingml.endnotes+xml"
)

// parseSection reads a w:sectPr (without its header references, which
// readHeaders resolves).
func parseSection(s *node) *model.Section {
	sec := &model.Section{Page: parsePage(s.kids), DocxXML: rawKids(s)}
	for _, k := range s.kids {
		switch k.name {
		case "w:type":
			for i, n := range model.BreakNames {
				if k.attr("w:val") == n {
					sec.Break = model.SectionBreak(i)
				}
			}
		case "w:cols":
			c := &sec.Columns
			c.Count = atoi(k.attr("w:num"))
			if v, ok := twipsPt(k.attr("w:space")); ok {
				c.Space = v
			}
			c.Separator = onOffAttr(k.attr("w:sep"))
			if cols := k.children("w:col"); len(cols) > 1 && !(k.attr("w:equalWidth") != "" && onOffAttr(k.attr("w:equalWidth"))) {
				for _, col := range cols {
					w, _ := twipsPt(col.attr("w:w"))
					sp, _ := twipsPt(col.attr("w:space"))
					c.Widths, c.Spaces = append(c.Widths, w), append(c.Spaces, sp)
				}
				c.Count = len(cols)
			}
		case "w:pgNumType":
			if v := k.attr("w:start"); v != "" {
				sec.PageNumRestart, sec.PageNumStart = true, atoi(v)
			}
			if f := k.attr("w:fmt"); f != "decimal" {
				sec.PageNumFormat = f
			}
		case "w:titlePg":
			sec.HF.TitlePage = onOff(k) == model.On
		}
	}
	return sec
}

// sectPrXML writes section i's w:sectPr, keeping the original properties
// the model doesn't cover.
func (w *writer) sectPrXML(s *model.Section, i int) string {
	var orig []*node
	switch {
	case s.DocxXML != "":
		orig = parseFragment(s.DocxXML)
	case s == &w.doc.Section && w.srcRoot != nil:
		if body := w.srcRoot.child("w:body"); body != nil {
			if sp := body.child("w:sectPr"); sp != nil && body.kids[len(body.kids)-1] == sp {
				orig = sp.kids
			}
		}
	}
	pg := s.Page
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
	os := parseSection(&node{kids: orig})
	typ := ""
	if s.Break != model.BreakNextPage {
		typ = `<w:type w:val="` + model.BreakNames[s.Break] + `"/>`
	}
	groups := []propGroup{
		{[]string{"w:pgSz"}, has(orig, "w:pgSz") && pg.Width == o.Width && pg.Height == o.Height && pg.Landscape == o.Landscape, size},
		{[]string{"w:pgMar"}, has(orig, "w:pgMar") && pg.MarginTop == o.MarginTop && pg.MarginBottom == o.MarginBottom &&
			pg.MarginLeft == o.MarginLeft && pg.MarginRight == o.MarginRight && pg.Header == o.Header && pg.Footer == o.Footer, mar},
		{[]string{"w:type"}, os.Break == s.Break, typ},
		{[]string{"w:cols"}, len(orig) > 0 && sameColumns(os.Columns, s.Columns), colsXML(s.Columns)},
		{[]string{"w:pgNumType"}, os.PageNumRestart == s.PageNumRestart && os.PageNumStart == s.PageNumStart && os.PageNumFormat == s.PageNumFormat,
			pgNumXML(s)},
	}
	groups = append(groups, w.hfGroups(orig, s, i)...)
	if len(orig) == 0 {
		groups = append(groups, propGroup{[]string{"w:docGrid"}, false, `<w:docGrid w:linePitch="360"/>`})
	}
	return "<w:sectPr>" + mergeProps(orig, groups, sectPrOrder) + "</w:sectPr>"
}

func sameColumns(a, b model.Columns) bool {
	if a.N() != b.N() || a.Space != b.Space || a.Separator != b.Separator || len(a.Widths) != len(b.Widths) {
		return false
	}
	for i := range a.Widths {
		if a.Widths[i] != b.Widths[i] || i < len(a.Spaces) && i < len(b.Spaces) && a.Spaces[i] != b.Spaces[i] {
			return false
		}
	}
	return true
}

func colsXML(c model.Columns) string {
	sp := c.Space
	if sp <= 0 {
		sp = 36
	}
	s := `<w:cols`
	if n := c.N(); n > 1 {
		s += ` w:num="` + strconv.Itoa(n) + `"`
	}
	if c.Separator {
		s += ` w:sep="1"`
	}
	s += ` w:space="` + ptTwips(sp) + `"`
	if len(c.Widths) > 1 {
		s += ` w:equalWidth="0">`
		for i, cw := range c.Widths {
			s += `<w:col w:w="` + ptTwips(cw) + `"`
			if i < len(c.Spaces) && i < len(c.Widths)-1 {
				s += ` w:space="` + ptTwips(c.Spaces[i]) + `"`
			}
			s += "/>"
		}
		return s + "</w:cols>"
	}
	return s + "/>"
}

func pgNumXML(s *model.Section) string {
	if !s.PageNumRestart && s.PageNumFormat == "" {
		return ""
	}
	x := "<w:pgNumType"
	if s.PageNumFormat != "" {
		x += ` w:fmt="` + escAttr(s.PageNumFormat) + `"`
	}
	if s.PageNumRestart {
		x += ` w:start="` + strconv.Itoa(s.PageNumStart) + `"`
	}
	return x + "/>"
}

// ---- notes ----

// noteParts holds the notes of footnotes.xml and endnotes.xml by id.
type noteParts struct {
	part  [2]string           // part names (footnotes, endnotes)
	notes [2]map[string]*node // w:footnote / w:endnote elements by w:id
	read  map[*node]*model.Note
}

// loadNotes reads the note parts the main part refers to, and the note
// settings in settings.xml.
func (r *reader) loadNotes() {
	r.np = noteParts{read: map[*node]*model.Note{}}
	for _, rl := range r.rels {
		k := -1
		switch {
		case rl.is(relFootnotes):
			k = 0
		case rl.is(relEndnotes):
			k = 1
		default:
			continue
		}
		part := opc.Resolve(r.main, rl.Target)
		b, err := r.pkg.read(part)
		if err != nil {
			continue
		}
		root, err := parseXML(b)
		if err != nil {
			continue
		}
		r.np.part[k] = part
		r.np.notes[k] = map[string]*node{}
		for _, n := range root.kids {
			if n.name != "w:footnote" && n.name != "w:endnote" {
				continue
			}
			if t := n.attr("w:type"); t != "" && t != "normal" {
				if k == 0 {
					r.doc.Notes.DocxFootSep = append(r.doc.Notes.DocxFootSep, n.raw())
				} else {
					r.doc.Notes.DocxEndSep = append(r.doc.Notes.DocxEndSep, n.raw())
				}
				continue
			}
			r.np.notes[k][n.attr("w:id")] = n
		}
	}
	for _, rl := range r.rels {
		if !rl.is(relSettings) {
			continue
		}
		b, err := r.pkg.read(opc.Resolve(r.main, rl.Target))
		if err != nil {
			continue
		}
		sr, err := parseXML(b)
		if err != nil {
			continue
		}
		ns := &r.doc.Notes
		for _, k := range []struct {
			name  string
			fmt   *string
			start *int
		}{{"w:footnotePr", &ns.FootFormat, &ns.FootStart}, {"w:endnotePr", &ns.EndFormat, &ns.EndStart}} {
			if pr := sr.child(k.name); pr != nil {
				*k.fmt = pr.child("w:numFmt").attr("w:val")
				*k.start = atoi(pr.child("w:numStart").attr("w:val"))
			}
		}
	}
}

// note returns the note a w:footnoteReference or w:endnoteReference
// refers to, read with its part's relationships.
func (r *reader) note(k *node, endnote bool) *model.Note {
	i := 0
	if endnote {
		i = 1
	}
	n := r.np.notes[i][k.attr("w:id")]
	if n == nil {
		return &model.Note{Endnote: endnote, Blocks: []model.Block{&model.Paragraph{}}}
	}
	if note := r.np.read[n]; note != nil {
		return note
	}
	note := &model.Note{Endnote: endnote, DocxID: n.attr("w:id")}
	r.np.read[n] = note
	note.Blocks = r.partBlocks(r.np.part[i], n)
	return note
}

// partBlocks reads the blocks of container, which is in another part
// (a header, a note), with that part's relationships, without disturbing
// the reading of the paragraph in progress.
func (r *reader) partBlocks(part string, container *node) []model.Block {
	rels, main := r.rels, r.main
	markers, before, lastPara, fields := r.markers, r.before, r.lastPara, r.fields
	if part != "" && part != r.main {
		r.rels, r.main = map[string]rel{}, part
		for _, rl := range r.pkg.rels(part) {
			r.rels[rl.ID] = rl
		}
	}
	r.markers, r.before, r.lastPara, r.fields = nil, nil, nil, nil
	blocks := r.blocks(container, nil, false)
	if len(blocks) == 0 {
		p := &model.Paragraph{}
		r.attach(p)
		blocks = []model.Block{p}
	}
	r.flushTrailing(blocks)
	r.rels, r.main = rels, main
	r.markers, r.before, r.lastPara, r.fields = markers, before, lastPara, fields
	return blocks
}

// notesXML writes footnotes.xml (or endnotes.xml): the separator notes
// as they were (or Word's), then the document's notes numbered from the
// separators' ids on, recording each note's id for its reference.
func (w *writer) notesXML(endnote bool, notes []*model.Note, orig *node, part string) ([]byte, []byte) {
	tag, root := "w:footnote", "w:footnotes"
	seps := w.doc.Notes.DocxFootSep
	if endnote {
		tag, root, seps = "w:endnote", "w:endnotes", w.doc.Notes.DocxEndSep
	}
	var b strings.Builder
	b.WriteString(xmlHeader)
	rootTag(&b, root, orig)
	next := 1
	if len(seps) == 0 {
		b.WriteString("<" + tag + ` w:type="separator" w:id="-1"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:separator/></w:r></w:p></` + tag + ">")
		b.WriteString("<" + tag + ` w:type="continuationSeparator" w:id="0"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:continuationSeparator/></w:r></w:p></` + tag + ">")
	}
	used := map[string]bool{}
	for _, s := range seps {
		b.Write(s)
		if ns := parseFragment(string(s)); len(ns) == 1 {
			next = max(next, atoi(ns[0].attr("w:id"))+1)
			used[ns[0].attr("w:id")] = true
		}
	}
	for _, n := range notes {
		next = max(next, atoi(n.DocxID)+1)
	}
	mainRels, mainDirty, mainPart, mainPics := w.rels, w.relsDirty, w.main, w.picRels
	var partRels []rel
	if w.src != nil && orig != nil {
		partRels = w.src.rels(part)
	}
	w.rels, w.relsDirty, w.main, w.picRels = partRels, false, part, nil
	w.inEndnote = endnote
	for _, n := range notes {
		id := n.DocxID
		if id == "" || used[id] {
			id = strconv.Itoa(next)
			next++
		}
		used[id] = true
		w.noteIDs[n] = id
		b.WriteString(w.noteXML(tag, id, n))
	}
	w.inEndnote = false
	b.WriteString("</" + root + ">")
	var relsData []byte
	if w.relsDirty {
		relsData = relsXML(w.rels)
	}
	w.rels, w.relsDirty, w.main, w.picRels = mainRels, mainDirty, mainPart, mainPics
	return []byte(b.String()), relsData
}

// noteXML writes one note element.
func (w *writer) noteXML(tag, id string, n *model.Note) string {
	var b strings.Builder
	b.WriteString("<" + tag + ` w:id="` + id + `">`)
	blocks := n.Blocks
	if len(blocks) == 0 {
		blocks = []model.Block{&model.Paragraph{}}
	} else if _, ok := blocks[len(blocks)-1].(*model.Paragraph); !ok {
		blocks = append(blocks[:len(blocks):len(blocks)], &model.Paragraph{})
	}
	w.blocks(&b, blocks)
	b.WriteString("</" + tag + ">")
	return b.String()
}

// unchangedNotes reports whether notes are the notes of the source's
// part (same ids, in order, with the same content), which is then
// written back as it was.
func (w *writer) unchangedNotes(endnote bool, notes []*model.Note, part string) bool {
	r := &reader{doc: &model.Document{Styles: w.doc.Styles, Lists: map[int]*model.List{}}, th: w.th,
		warned: map[string]bool{}, pkg: w.src, main: w.main, rels: map[string]rel{}}
	for _, rl := range w.src.rels(w.main) {
		r.rels[rl.ID] = rl
	}
	r.loadNotes()
	k, tag := 0, "w:footnote"
	if endnote {
		k, tag = 1, "w:endnote"
	}
	if len(r.np.notes[k]) != len(notes) {
		return false
	}
	mainRels, mainDirty, mainPart, mainPics := w.rels, w.relsDirty, w.main, w.picRels
	defer func() { w.rels, w.relsDirty, w.main, w.picRels = mainRels, mainDirty, mainPart, mainPics }()
	w.rels, w.relsDirty, w.main, w.picRels = w.src.rels(part), false, part, nil
	w.inEndnote = endnote
	defer func() { w.inEndnote = false }()
	for _, n := range notes {
		on := r.np.notes[k][n.DocxID]
		if on == nil {
			return false
		}
		o := r.note(on, endnote)
		o.Mark = n.Mark
		if w.noteXML(tag, n.DocxID, n) != w.noteXML(tag, n.DocxID, o) || w.relsDirty {
			return false
		}
	}
	return true
}

// noteRun writes a note reference or a note's own mark.
func (w *writer) noteRun(b *strings.Builder, r *model.Run) {
	o := r.Opaque
	b.WriteString("<w:r>")
	if rpr := rPrXML(r.Style, r.Props, w.th); rpr != "" {
		b.WriteString("<w:rPr>" + rpr + "</w:rPr>")
	}
	switch {
	case o.Note != nil:
		tag := "w:footnoteReference"
		if o.Note.Endnote {
			tag = "w:endnoteReference"
		}
		b.WriteString("<" + tag + ` w:id="` + w.noteIDs[o.Note] + `"`)
		if o.Note.Mark != "" {
			b.WriteString(` w:customMarkFollows="1"/><w:t xml:space="preserve">` + escText(o.Note.Mark) + "</w:t>")
		} else {
			b.WriteString("/>")
		}
	case w.inEndnote:
		b.WriteString("<w:endnoteRef/>")
	default:
		b.WriteString("<w:footnoteRef/>")
	}
	b.WriteString("</w:r>")
}

// prepareNotes writes footnotes.xml and endnotes.xml when the document
// has notes (or had the parts), before document.xml so references know
// their ids.
func (w *writer) prepareNotes() {
	w.noteIDs = map[*model.Note]string{}
	var lists [2][]*model.Note
	for _, n := range model.NotesIn(w.doc.Body) {
		k := 0
		if n.Endnote {
			k = 1
		}
		lists[k] = append(lists[k], n)
	}
	for k := range 2 {
		part, relID := "", ""
		var orig *node
		if w.src != nil {
			for _, rl := range w.rels {
				if k == 0 && rl.is(relFootnotes) || k == 1 && rl.is(relEndnotes) {
					part, relID = opc.Resolve(w.main, rl.Target), rl.ID
					if b, err := w.src.read(part); err == nil {
						orig, _ = parseXML(b)
					}
				}
			}
		}
		if len(lists[k]) == 0 && part == "" {
			continue
		}
		name, typ, ct := "footnotes.xml", relFootnotes, ctFootnotes
		if k == 1 {
			name, typ, ct = "endnotes.xml", relEndnotes, ctEndnotes
		}
		np := hfPart{name: part, ctype: ct, relID: relID}
		if part == "" {
			dir := path0(w.main)
			np.name, np.isNew = dir+name, true
			np.relID = w.newRelID()
			w.rels = append(w.rels, rel{ID: np.relID, Type: relTypePrefix + typ, Target: name})
			w.relsDirty = true
		}
		np.data, np.rels = w.notesXML(k == 1, lists[k], orig, np.name)
		if orig != nil && w.unchangedNotes(k == 1, lists[k], np.name) {
			if b, err := w.src.read(np.name); err == nil {
				np.data, np.rels = b, nil
			}
		}
		w.noteParts = append(w.noteParts, np)
	}
}

// path0 is the directory of the main part with a trailing slash ("word/").
func path0(main string) string {
	if main == "" {
		return "word/"
	}
	if i := strings.LastIndex(main, "/"); i >= 0 {
		return main[:i+1]
	}
	return ""
}

// noteSettingsXML is the settings.xml elements that list the separator
// notes, for a new package.
func (w *writer) noteSettingsXML() string {
	ns := &w.doc.Notes
	s := ""
	for _, k := range []struct {
		ct, tag, fmt string
		start        int
	}{{ctFootnotes, "footnote", ns.FootFormat, ns.FootStart}, {ctEndnotes, "endnote", ns.EndFormat, ns.EndStart}} {
		for _, p := range w.noteParts {
			if p.ctype != k.ct {
				continue
			}
			s += "<w:" + k.tag + "Pr>"
			if k.fmt != "" {
				s += `<w:numFmt w:val="` + escAttr(k.fmt) + `"/>`
			}
			if k.start > 1 {
				s += `<w:numStart w:val="` + strconv.Itoa(k.start) + `"/>`
			}
			s += "<w:" + k.tag + ` w:id="-1"/><w:` + k.tag + ` w:id="0"/></w:` + k.tag + "Pr>"
		}
	}
	return s
}
