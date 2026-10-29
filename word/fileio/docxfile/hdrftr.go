package docxfile

import (
	"path"
	"strconv"
	"strings"

	"officelite/common/opc"
	"officelite/word/model"
)

// Headers and footers: header*.xml / footer*.xml parts referenced from the
// last section's w:sectPr (w:headerReference / w:footerReference with
// w:type default, first or even), w:titlePg in the sectPr and
// w:evenAndOddHeaders in settings.xml.

const (
	ctHeader = "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"
	ctFooter = "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"
)

func hfKind(t string) (model.HFKind, bool) {
	for k, n := range model.HFKindNames {
		if n == t || t == "" && k == 0 {
			return model.HFKind(k), true
		}
	}
	return 0, false
}

// readHeaders reads the headers and footers each section refers to. A
// section without a reference of some type is linked to the previous
// section's (its slot stays nil), as in Word.
func (r *reader) readHeaders(sects []*node) {
	if r.pkg == nil || len(sects) == 0 {
		return
	}
	hf := &r.doc.HF
	read := map[string]*model.HeaderFooter{}
	for i, sp := range sects {
		sec := r.sections[i]
		for _, k := range sp.kids {
			footer := k.name == "w:footerReference"
			if !footer && k.name != "w:headerReference" {
				continue
			}
			kind, ok := hfKind(k.attr("w:type"))
			if !ok {
				continue
			}
			rl, ok := r.rels[k.attr("r:id")]
			if !ok {
				continue
			}
			part := opc.Resolve(r.main, rl.Target)
			h := read[part]
			if h == nil {
				h = r.readHF(part)
				read[part] = h
			}
			*sec.HF.Slot(kind, footer) = h
		}
	}
	for _, rl := range r.rels {
		if rl.is(relSettings) {
			if b, err := r.pkg.read(opc.Resolve(r.main, rl.Target)); err == nil {
				if sr, err := parseXML(b); err == nil {
					hf.EvenOdd = onOff(sr.child("w:evenAndOddHeaders")) == model.On
				}
			}
		}
	}
}

// readHF reads one header or footer part, with that part's relationships
// (its pictures and links).
func (r *reader) readHF(part string) *model.HeaderFooter {
	data, err := r.pkg.read(part)
	if err != nil {
		return nil
	}
	root, err := parseXML(data)
	if err != nil {
		return nil
	}
	return &model.HeaderFooter{Blocks: r.partBlocks(part, root), Part: part}
}

// hfPart is a header or footer part being written.
type hfPart struct {
	name   string // part name
	ctype  string
	data   []byte
	rels   []byte // its relationships part, when changed
	isNew  bool
	relID  string
	kind   model.HFKind
	footer bool
	sec    int  // the section that refers to it
	shared bool // a reference to a part written for another section
}

// prepareHeaders writes the header and footer parts and adds the
// relationships of new ones; it runs before document.xml is written so the
// section can refer to them.
func (w *writer) prepareHeaders() {
	used := map[string]bool{}
	if w.src != nil {
		for name := range w.src.files {
			used[strings.ToLower(name)] = true
		}
	}
	done := map[*model.HeaderFooter]hfPart{} // a header two sections share is written once
	for si, sec := range w.doc.Sections() {
		for _, footer := range []bool{false, true} {
			for k := range 3 {
				h := *sec.HF.Slot(model.HFKind(k), footer)
				if h == nil {
					continue
				}
				if p, ok := done[h]; ok {
					p.sec, p.kind, p.data, p.rels, p.isNew, p.shared = si, model.HFKind(k), nil, nil, false, true
					w.hfParts = append(w.hfParts, p)
					continue
				}
				p := w.hfPart(h, model.HFKind(k), footer, used)
				p.sec = si
				done[h] = p
				w.hfParts = append(w.hfParts, p)
			}
		}
	}
}

// hfPart writes the part of one header or footer.
func (w *writer) hfPart(h *model.HeaderFooter, kind model.HFKind, footer bool, used map[string]bool) hfPart {
	p := hfPart{kind: kind, footer: footer, ctype: ctHeader}
	base := "header"
	if footer {
		p.ctype, base = ctFooter, "footer"
	}
	// An unchanged source part keeps its relationship.
	if w.src != nil && h.Part != "" && w.src.find(h.Part) != nil {
		for _, rl := range w.rels {
			if strings.EqualFold(opc.Resolve(w.main, rl.Target), h.Part) {
				p.name, p.relID = h.Part, rl.ID
				break
			}
		}
	}
	if p.relID == "" {
		dir := path.Dir(w.main)
		if w.main == "" {
			dir = "word"
		}
		for n := 1; ; n++ {
			name := base + strconv.Itoa(n) + ".xml"
			full := path.Join(dir, name)
			if dir == "." {
				full = name
			}
			if !used[strings.ToLower(full)] {
				p.name, p.isNew = full, true
				used[strings.ToLower(full)] = true
				p.relID = w.newRelID()
				typ := relHeader
				if footer {
					typ = relFooter
				}
				w.rels = append(w.rels, rel{ID: p.relID, Type: relTypePrefix + typ, Target: name})
				w.relsDirty = true
				break
			}
		}
	}
	p.data, p.rels = w.hfXML(h, p)
	if !p.isNew {
		// Unchanged content keeps the original part byte for byte.
		if o := w.sourceHF(p.name); o != nil {
			if od, orels := w.hfXML(o, p); string(od) == string(p.data) && string(orels) == string(p.rels) {
				if b, err := w.src.read(p.name); err == nil {
					p.data, p.rels = b, nil
				}
			}
		}
	}
	return p
}

// sourceHF reads a header or footer part of the source package as it was.
func (w *writer) sourceHF(part string) *model.HeaderFooter {
	r := &reader{doc: &model.Document{Styles: w.doc.Styles, Lists: map[int]*model.List{}}, th: w.th,
		warned: map[string]bool{}, pkg: w.src, main: w.main, rels: map[string]rel{}}
	for _, rl := range w.src.rels(w.main) {
		r.rels[rl.ID] = rl
	}
	return r.readHF(part)
}

// hfXML writes a header or footer part (and its relationships part when
// writing it added relationships, e.g. for a new link).
func (w *writer) hfXML(h *model.HeaderFooter, p hfPart) ([]byte, []byte) {
	var orig *node
	var partRels []rel
	if !p.isNew && w.src != nil {
		if b, err := w.src.read(p.name); err == nil {
			orig, _ = parseXML(b)
		}
		partRels = w.src.rels(p.name)
	}
	// Opaque content refers to this part's relationships.
	mainRels, mainDirty, mainPart, mainPics := w.rels, w.relsDirty, w.main, w.picRels
	w.rels, w.relsDirty, w.main, w.picRels = partRels, false, p.name, nil
	tag := "w:hdr"
	if p.footer {
		tag = "w:ftr"
	}
	var b strings.Builder
	b.WriteString(xmlHeader)
	rootTag(&b, tag, orig)
	blocks := h.Blocks
	if len(blocks) == 0 {
		blocks = []model.Block{&model.Paragraph{}}
	} else if _, ok := blocks[len(blocks)-1].(*model.Paragraph); !ok {
		blocks = append(blocks[:len(blocks):len(blocks)], &model.Paragraph{})
	}
	w.blocks(&b, blocks)
	b.WriteString("</" + tag + ">")
	var relsData []byte
	if w.relsDirty {
		relsData = relsXML(w.rels)
	}
	w.rels, w.relsDirty, w.main, w.picRels = mainRels, mainDirty, mainPart, mainPics
	return []byte(b.String()), relsData
}

// hfRefsXML is section sec's header and footer references.
func (w *writer) hfRefsXML(sec int) string {
	var b strings.Builder
	for _, footer := range []bool{false, true} {
		for _, p := range w.hfParts {
			if p.footer != footer || p.sec != sec {
				continue
			}
			tag := "w:headerReference"
			if footer {
				tag = "w:footerReference"
			}
			b.WriteString("<" + tag + ` w:type="` + model.HFKindNames[p.kind] + `" r:id="` + escAttr(p.relID) + `"/>`)
		}
	}
	return b.String()
}

// hfGroups are the sectPr property groups for headers and footers: the
// references (always regenerated) and the different-first-page switch.
func (w *writer) hfGroups(orig []*node, s *model.Section, i int) []propGroup {
	title := ""
	if s.HF.TitlePage {
		title = "<w:titlePg/>"
	}
	origTitle := false
	for _, k := range orig {
		if k.name == "w:titlePg" {
			origTitle = onOff(k) == model.On
		}
	}
	return []propGroup{
		{[]string{"w:headerReference", "w:footerReference"}, false, w.hfRefsXML(i)},
		{[]string{"w:titlePg"}, origTitle == s.HF.TitlePage, title},
	}
}

// addHFParts puts the header and footer parts into a package written from
// the source: replaced or added parts, their relationships and content
// types, and the even/odd switch in settings.xml. It reports whether the
// content types changed.
func (w *writer) addHFParts(replace map[string][]byte, ct *contentTypes) bool {
	ctDirty := false
	for _, p := range append(w.hfParts, w.noteParts...) {
		if p.shared {
			continue
		}
		replace[p.name] = p.data
		if p.rels != nil {
			replace[opc.RelsPath(p.name)] = p.rels
		}
		if p.isNew {
			ct.setOverride(p.name, p.ctype)
			ctDirty = true
		}
	}
	for _, rl := range w.rels {
		if !rl.is(relSettings) {
			continue
		}
		part := opc.Resolve(w.main, rl.Target)
		b, err := w.src.read(part)
		if err != nil {
			continue
		}
		if s, ok := setEvenOdd(b, w.doc.HF.EvenOdd); ok {
			replace[part] = s
		}
	}
	return ctDirty
}

// settingsOrder is the schema order of settings.xml's elements around
// w:evenAndOddHeaders.
var settingsOrder = order("writeProtection view zoom removePersonalInformation removeDateAndTime doNotDisplayPageBoundaries displayBackgroundShape printPostScriptOverText printFractionalCharacterWidth printFormsData embedTrueTypeFonts embedSystemFonts saveSubsetFonts saveFormsData mirrorMargins alignBordersAndEdges bordersDoNotSurroundHeader bordersDoNotSurroundFooter gutterAtTop hideSpellingErrors hideGrammaticalErrors activeWritingStyle proofState formsDesign attachedTemplate linkStyles stylePaneFormatFilter stylePaneSortMethod documentType mailMerge revisionView trackRevisions doNotTrackMoves doNotTrackFormatting documentProtection autoFormatOverride styleLockTheme styleLockQFSet defaultTabStop autoHyphenation consecutiveHyphenLimit hyphenationZone doNotHyphenateCaps showEnvelope summaryLength clickAndTypeStyle defaultTableStyle evenAndOddHeaders bookFoldRevPrinting bookFoldPrinting bookFoldPrintingSheets drawingGridHorizontalSpacing drawingGridVerticalSpacing displayHorizontalDrawingGridEvery displayVerticalDrawingGridEvery doNotUseMarginsForDrawingGridOrigin drawingGridHorizontalOrigin drawingGridVerticalOrigin doNotShadeFormData noPunctuationKerning characterSpacingControl printTwoOnOne strictFirstAndLastChars noLineBreaksAfter noLineBreaksBefore savePreviewPicture doNotValidateAgainstSchema saveInvalidXml ignoreMixedContent alwaysShowPlaceholderText doNotDemarcateInvalidXml saveXmlDataOnly useXSLTWhenSaving saveThroughXslt showXMLTags alwaysMergeEmptyNamespace updateFields hdrShapeDefaults footnotePr endnotePr compat docVars rsids mathPr attachedSchema themeFontLang clrSchemeMapping doNotIncludeSubdocsInStats doNotAutoCompressPictures forceUpgrade captions readModeInkLockDown smartTagType schemaLibrary shapeDefaults doNotEmbedSmartTags decimalSymbol listSeparator")

// setEvenOdd returns settings.xml with w:evenAndOddHeaders set to on, or
// false when it already is.
func setEvenOdd(data []byte, on bool) ([]byte, bool) {
	root, err := parseXML(data)
	if err != nil {
		return nil, false
	}
	cur := onOff(root.child("w:evenAndOddHeaders")) == model.On
	if cur == on {
		return nil, false
	}
	gen := ""
	if on {
		gen = "<w:evenAndOddHeaders/>"
	}
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.Write(root.src[root.start:root.open])
	b.WriteString(mergeProps(root.kids, []propGroup{{[]string{"w:evenAndOddHeaders"}, false, gen}}, settingsOrder))
	b.WriteString(string(root.src[root.close:root.end]))
	return []byte(b.String()), true
}
