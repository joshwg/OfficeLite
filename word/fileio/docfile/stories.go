package docfile

import (
	"officelite/word/model"
)

// Sections, headers and footers, notes and text boxes: the section table
// (PlcfSed with a SEPX per section) and the subdocuments that follow the
// main text in CP order (footnotes, headers, comments, endnotes, text
// boxes, header text boxes), each split into stories by its PLC.

// Section property modifiers.
const (
	sprmSFEvenlySpaced = 0x3005
	sprmSBkc           = 0x3009
	sprmSFTitlePage    = 0x300A
	sprmSCcolumns      = 0x500B
	sprmSDxaColumns    = 0x900C
	sprmSNfcPgn        = 0x300E
	sprmSFPgnRestart   = 0x3011
	sprmSLBetween      = 0x3019
	sprmSPgnStart97    = 0x501C
	sprmSDxaColWidth   = 0xF203
	sprmSDxaColSpacing = 0xF204
	sprmSPgnStart      = 0x7044
)

// pgnFormats are the page number formats by nfc.
var pgnFormats = []string{"decimal", "upperRoman", "lowerRoman", "upperLetter", "lowerLetter"}

// storyStart returns the first CP of subdocument i (0 main, then lwCcp*
// order: footnotes, headers, comments, endnotes, text boxes, header text
// boxes).
func (r *reader) storyStart(lw int) int {
	cp := 0
	for _, k := range []int{lwCcpText, lwCcpFtn, lwCcpHdd, lwCcpAtn, lwCcpEdn, lwCcpTxbx, lwCcpHdrTxbx} {
		if k == lw {
			return cp
		}
		cp += int(r.fib.lw[k])
	}
	return cp
}

// readSections reads every section's properties: the last becomes the
// document's own, the others go to the paragraphs that end them.
func (r *reader) readSections() {
	plc := r.fib.blob(r.table, fcPlcfSed)
	r.sedCPs = map[int]bool{}
	r.sectEnd = map[int]*model.Section{}
	if len(plc) < 4+12 {
		r.doc.Section = defaultSection()
		r.sections = []*model.Section{&r.doc.Section}
		return
	}
	n := (len(plc) - 4) / 16
	for i := 0; i < n; i++ {
		s := r.readSep(plc[4*(n+1)+12*i:])
		end := int(u32(plc[4*i+4:]))
		if i == n-1 {
			r.doc.Section = *s
			s = &r.doc.Section
		} else {
			r.sedCPs[end] = true
			r.sectEnd[end] = s
		}
		r.sections = append(r.sections, s)
	}
}

// sectEnding returns the section that ends just before CP cp.
func (r *reader) sectEnding(cp int) *model.Section { return r.sectEnd[cp] }

func defaultSection() model.Section {
	return model.Section{Page: model.PageSetup{Width: 612, Height: 792, MarginLeft: 90, MarginRight: 90, MarginTop: 72, MarginBottom: 72, Header: 36, Footer: 36}}
}

// readSep decodes one section descriptor's SEPX.
func (r *reader) readSep(sed []byte) *model.Section {
	s := defaultSection()
	pg := &s.Page
	fc := u32(sed[2:])
	if fc == 0xFFFFFFFF || int(fc)+2 > len(r.wd) {
		return &s
	}
	cb := int(u16(r.wd[fc:]))
	if int(fc)+2+cb > len(r.wd) {
		return &s
	}
	cols := &s.Columns
	even := true
	eachSprm(r.wd[fc+2:int(fc)+2+cb], func(op uint16, a []byte) {
		switch op {
		case sprmSXaPage:
			pg.Width = twips(int(u16(a)))
		case sprmSYaPage:
			pg.Height = twips(int(u16(a)))
		case sprmSDxaLeft:
			pg.MarginLeft = twips(int(u16(a)))
		case sprmSDxaRight:
			pg.MarginRight = twips(int(u16(a)))
		case sprmSDyaTop:
			pg.MarginTop = twips(abs(int(i16(a))))
		case sprmSDyaBottom:
			pg.MarginBottom = twips(abs(int(i16(a))))
		case sprmSDyaHdrTop:
			pg.Header = twips(int(u16(a)))
		case sprmSDyaHdrBottom:
			pg.Footer = twips(int(u16(a)))
		case sprmSBOrientation:
			pg.Landscape = u8(a) == 2
		case sprmSBkc:
			s.Break = [...]model.SectionBreak{model.BreakContinuous, model.BreakNextColumn, model.BreakNextPage,
				model.BreakEvenPage, model.BreakOddPage}[min(int(u8(a)), 4)]
		case sprmSFTitlePage:
			s.HF.TitlePage = u8(a) != 0
		case sprmSCcolumns:
			cols.Count = int(u16(a)) + 1
		case sprmSDxaColumns:
			cols.Space = twips(int(u16(a)))
		case sprmSFEvenlySpaced:
			even = u8(a) != 0
		case sprmSDxaColWidth, sprmSDxaColSpacing:
			if len(a) >= 3 {
				i := int(a[0])
				list := &cols.Widths
				if op == sprmSDxaColSpacing {
					list = &cols.Spaces
				}
				for len(*list) <= i && i < 64 {
					*list = append(*list, 0)
				}
				if i < 64 {
					(*list)[i] = twips(int(u16(a[1:])))
				}
			}
		case sprmSLBetween:
			cols.Separator = u8(a) != 0
		case sprmSNfcPgn:
			if v := int(u8(a)); v < len(pgnFormats) {
				s.PageNumFormat = pgnFormats[v]
			}
		case sprmSFPgnRestart:
			s.PageNumRestart = u8(a) != 0
		case sprmSPgnStart97:
			s.PageNumStart = int(u16(a))
		case sprmSPgnStart:
			s.PageNumStart = int(u32(a))
		}
	})
	if even || cols.Count <= 1 || len(cols.Widths) != cols.Count {
		cols.Widths, cols.Spaces = nil, nil
	}
	if s.PageNumFormat == "decimal" {
		s.PageNumFormat = ""
	}
	return &s
}

// Header story slots of a section in PlcfHdd, after the six note
// separator stories.
var hddSlots = [6]struct {
	kind   model.HFKind
	footer bool
}{{model.HFEven, false}, {model.HFDefault, false}, {model.HFEven, true}, {model.HFDefault, true},
	{model.HFFirst, false}, {model.HFFirst, true}}

// story returns the CP range [a, b) of story i of a PLC of story starts
// (relative to base), without its last paragraph mark when trim is set
// (header and text box stories end with an extra one).
func (r *reader) story(plc []byte, i, base int, trim bool) (int, int, bool) {
	n := len(plc)/4 - 1
	if i+1 > n {
		return 0, 0, false
	}
	a, b := base+int(u32(plc[4*i:])), base+int(u32(plc[4*i+4:]))
	if b <= a || b > len(r.text) {
		return 0, 0, false
	}
	if trim && b-a > 1 && r.text[b-1] == 0x0D {
		b--
	}
	return a, b, true
}

// readHeaders reads each section's headers and footers from the header
// subdocument. An empty story is linked to the previous section's.
func (r *reader) readHeaders() {
	plc := r.fib.blob(r.table, fcPlcfHdd)
	if r.fib.lw[lwCcpHdd] <= 0 || len(plc) < 8 {
		return
	}
	base := r.storyStart(lwCcpHdd)
	for si, s := range r.sections {
		for j, slot := range hddSlots {
			a, b, ok := r.story(plc, 6+6*si+j, base, true)
			if !ok {
				continue
			}
			blocks := r.blocks(a, b, false)
			if len(blocks) == 0 {
				blocks = []model.Block{&model.Paragraph{}}
			}
			*s.HF.Slot(slot.kind, slot.footer) = &model.HeaderFooter{Blocks: blocks}
		}
	}
	if dop := r.fib.blob(r.table, fcDop); len(dop) > 0 {
		r.doc.HF.EvenOdd = dop[0]&1 != 0 // fFacingPages
	}
}

// readNotes reads the footnotes and endnotes: the reference CPs (and
// whether each is numbered automatically) and their text stories.
func (r *reader) readNotes() {
	r.noteAt = map[int]*model.Note{}
	if dop := r.fib.blob(r.table, fcDop); len(dop) >= 492 {
		nfc := func(v uint16) string {
			if int(v) < len(pgnFormats) {
				return pgnFormats[v]
			}
			return ""
		}
		r.doc.Notes.FootFormat = nfc(u16(dop[488:]))
		r.doc.Notes.EndFormat = nfc(u16(dop[490:]))
		r.doc.Notes.FootStart = int(u16(dop[2:]) >> 2)
		r.doc.Notes.EndStart = int(u16(dop[52:]) >> 2)
	}
	for _, k := range []struct {
		ref, txt, lw int
		endnote      bool
	}{{fcPlcffndRef, fcPlcffndTxt, lwCcpFtn, false}, {fcPlcfendRef, fcPlcfendTxt, lwCcpEdn, true}} {
		ref, txt := r.fib.blob(r.table, k.ref), r.fib.blob(r.table, k.txt)
		if len(ref) < 4 || r.fib.lw[k.lw] <= 0 {
			continue
		}
		n := (len(ref) - 4) / 6
		base := r.storyStart(k.lw)
		for i := 0; i < n; i++ {
			cp := int(u32(ref[4*i:]))
			note := &model.Note{Endnote: k.endnote}
			if auto := i16(ref[4*(n+1)+2*i:]); auto == 0 && cp < len(r.text) {
				note.Mark = string(rune(r.text[cp]))
			}
			if a, b, ok := r.story(txt, i, base, false); ok {
				r.inNote = true
				note.Blocks = r.blocks(a, b, false)
				r.inNote = false
			}
			if len(note.Blocks) == 0 {
				note.Blocks = []model.Block{&model.Paragraph{}}
			}
			r.noteAt[cp] = note
		}
	}
}

// subdocWarnings reports stories WordLite doesn't show.
func (r *reader) subdocWarnings() {
	if r.fib.lw[lwCcpAtn] > 0 {
		r.warn("Comments are not shown and are not kept when saving.")
	}
	if r.fib.lw[lwCcpHdrTxbx] > 0 {
		r.warn("Text boxes in headers and footers are not shown and are not kept when saving.")
	}
}

// fieldCharRun is the opaque run of a field's begin (with its code and
// separator, [b, e]) or end character, kept around a field result that
// spans paragraphs.
func (r *reader) fieldCharRun(b, e int, kind, paraStyle string) model.Run {
	props, cs := r.runProps(r.chpAt(b), paraStyle)
	blob := &opaqueBlob{Kind: kind}
	r.segs(b, e, blob)
	o := r.opaqueRun(model.Run{Props: props, Style: cs}, "", blob)
	if kind == "fieldbegin" {
		o.Opaque.FieldChar = model.FieldStart
		o.Opaque.Field = r.fieldInstr(b, e)
	} else {
		o.Opaque.FieldChar = model.FieldEnd
	}
	return o
}
