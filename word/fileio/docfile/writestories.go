package docfile

import (
	"encoding/binary"
	"strings"

	"officelite/word/model"
)

// Writing sections and subdocuments: a SEPX per section in PlcfSed, and
// after the main text the footnote, header and endnote subdocuments,
// each a list of stories followed by a guard paragraph mark, with the
// PLCs that split them (PlcffndRef/PlcffndTxt, PlcfHdd, PlcfendRef/
// PlcfendTxt) and their own field PLCs.

// WarnFloatsLost is the warning for floating objects, which the .doc
// writer doesn't save.
const WarnFloatsLost = "Text boxes, shapes and floating pictures can't be saved in a .doc file by WordLite and are left out."

// wnoteRef is a note reference written in the main text.
type wnoteRef struct {
	cp   int
	note *model.Note
}

// storyLayout is where the subdocuments went: each one's first CP and
// length, and its stories' starts (relative to it, with the end and the
// PLC's final CP).
type storyLayout struct {
	ftnStart, ftnLen int
	hddStart, hddLen int
	ednStart, ednLen int
	ftnCPs, hddCPs   []int
	ednCPs           []int
}

// noteRun writes a note reference (Opaque.Note) or the mark starting a
// note's text (Opaque.NoteMark): the auto-numbered note character 0x02,
// or a custom mark's text.
func (w *writer) noteRun(r *model.Run, chpx []byte) {
	o := r.Opaque
	if o.Note != nil {
		k := 0
		if o.Note.Endnote {
			k = 1
		}
		w.noteRefs[k] = append(w.noteRefs[k], wnoteRef{len(w.text), o.Note})
		if o.Note.Mark != "" {
			w.chars(textUnits(o.Note.Mark, true), chpx)
			return
		}
	}
	var g grpprl
	g = append(g, chpx...)
	g.b(sprmCFSpec, 1)
	w.chars([]uint16{0x02}, g)
}

// fieldCharRun writes a field character kept from another format (a
// .docx field whose result spans paragraphs).
func (w *writer) fieldCharRun(r *model.Run, chpx []byte) {
	o := r.Opaque
	switch o.FieldChar {
	case model.FieldBegin:
		w.fieldChar(0x13, chpx, w.fieldType(o.Field))
	case model.FieldStart:
		w.fieldChar(0x13, chpx, w.fieldType(o.Field))
		w.chars(textUnits(" "+o.Field+" ", true), chpx)
		w.fieldChar(0x14, chpx, 0xFF)
	case model.FieldInstr:
		w.chars(textUnits(o.Field, true), chpx)
	case model.FieldSep:
		w.fieldChar(0x14, chpx, 0xFF)
	case model.FieldEnd:
		w.fieldChar(0x15, chpx, 0x80)
	}
}

// fieldType is the flt of a field instruction (1 when unknown).
func (w *writer) fieldType(instr string) byte {
	name := strings.ToUpper(strings.Fields(instr + " x")[0])
	if flt, ok := fieldTypes[name]; ok {
		return flt
	}
	return 1
}

// guard ends a subdocument (or a header story) with a paragraph mark.
func (w *writer) guard() { w.endPara(0x0D, nil, 0, nil) }

// subdocs writes the footnote, header and endnote subdocuments after the
// main text, and the final paragraph mark when there are any.
func (w *writer) subdocs() {
	st := &w.stories
	st.ftnStart = len(w.text)
	st.ftnCPs = w.noteStory(w.noteRefs[0])
	st.ftnLen = len(w.text) - st.ftnStart

	st.hddStart = len(w.text)
	if model.AnyHF(w.secs) {
		st.hddCPs = make([]int, 6, 6+6*len(w.secs)+2) // empty separator stories: Word's defaults
		for _, s := range w.secs {
			for _, slot := range hddSlots {
				st.hddCPs = append(st.hddCPs, len(w.text)-st.hddStart)
				if h := *s.HF.Slot(slot.kind, slot.footer); h != nil {
					w.blocks(h.Blocks, false)
					w.guard()
				}
			}
		}
		end := len(w.text) - st.hddStart
		w.guard()
		st.hddCPs = append(st.hddCPs, end, len(w.text)-st.hddStart+2)
	}
	st.hddLen = len(w.text) - st.hddStart

	st.ednStart = len(w.text)
	st.ednCPs = w.noteStory(w.noteRefs[1])
	st.ednLen = len(w.text) - st.ednStart
	if len(w.text) > w.mainLen {
		w.guard()
	}
}

// noteStory writes the notes of refs as one subdocument and returns its
// PlcffndTxt CPs (nil without notes).
func (w *writer) noteStory(refs []wnoteRef) []int {
	if len(refs) == 0 {
		return nil
	}
	start := len(w.text)
	var cps []int
	for _, ref := range refs {
		cps = append(cps, len(w.text)-start)
		w.blocks(ref.note.Blocks, false)
	}
	end := len(w.text) - start
	w.guard()
	return append(cps, end, len(w.text)-start+2)
}

// subdocPLCs adds the subdocuments' PLCs to the table stream.
func (w *writer) subdocPLCs(put func(int, []byte), total int) {
	le := binary.LittleEndian
	cps := func(list []int) []byte {
		var b []byte
		for _, c := range list {
			b = le.AppendUint32(b, uint32(c))
		}
		return b
	}
	for k, idx := range [2][2]int{{fcPlcffndRef, fcPlcffndTxt}, {fcPlcfendRef, fcPlcfendTxt}} {
		refs := w.noteRefs[k]
		txt := w.stories.ftnCPs
		if k == 1 {
			txt = w.stories.ednCPs
		}
		if len(refs) == 0 {
			continue
		}
		var b []byte
		for _, r := range refs {
			b = le.AppendUint32(b, uint32(r.cp))
		}
		b = le.AppendUint32(b, uint32(total))
		for _, r := range refs {
			auto := uint16(1)
			if r.note.Mark != "" {
				auto = 0
			}
			b = le.AppendUint16(b, auto)
		}
		put(idx[0], b)
		put(idx[1], cps(txt))
	}
	if len(w.stories.hddCPs) > 0 {
		put(fcPlcfHdd, cps(w.stories.hddCPs))
	}
}

// fieldPLC returns the PlcFld of the fields in CPs [start, start+n),
// relative to start (nil without fields).
func (w *writer) fieldPLC(start, n int) []byte {
	var in []wfld
	for _, f := range w.flds {
		if f.cp >= start && f.cp < start+n {
			in = append(in, f)
		}
	}
	if len(in) == 0 {
		return nil
	}
	var b []byte
	for _, f := range in {
		b = binary.LittleEndian.AppendUint32(b, uint32(f.cp-start))
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(n))
	for _, f := range in {
		b = append(b, f.fld[0], f.fld[1])
	}
	return b
}

// sepx encodes a section's properties.
func sepx(s *model.Section) []byte {
	pg := s.Page
	if pg.Width <= 0 || pg.Height <= 0 {
		pg = model.LetterPage()
	}
	var g grpprl
	if bkc, ok := map[model.SectionBreak]byte{model.BreakContinuous: 0, model.BreakNextColumn: 1,
		model.BreakEvenPage: 3, model.BreakOddPage: 4}[s.Break]; ok {
		g.b(sprmSBkc, bkc)
	}
	if s.HF.TitlePage {
		g.b(sprmSFTitlePage, 1)
	}
	g.w(sprmSXaPage, uint16(clampTw(pg.Width, 144, 31680)))
	g.w(sprmSYaPage, uint16(clampTw(pg.Height, 144, 31680)))
	g.w(sprmSDxaLeft, uint16(clampTw(pg.MarginLeft, 0, 31680)))
	g.w(sprmSDxaRight, uint16(clampTw(pg.MarginRight, 0, 31680)))
	g.w(sprmSDyaTop, uint16(clampTw(pg.MarginTop, 0, 31680)))
	g.w(sprmSDyaBottom, uint16(clampTw(pg.MarginBottom, 0, 31680)))
	g.w(sprmSDyaHdrTop, uint16(clampTw(pg.Header, 0, 31680)))
	g.w(sprmSDyaHdrBottom, uint16(clampTw(pg.Footer, 0, 31680)))
	if pg.Landscape {
		g.b(sprmSBOrientation, 2)
	}
	c := s.Columns
	if c.Space > 0 && c.N() == 1 {
		g.w(sprmSDxaColumns, uint16(clampTw(c.Space, 0, 31680)))
	}
	if n := c.N(); n > 1 {
		g.w(sprmSCcolumns, uint16(n-1))
		sp := c.Space
		if sp <= 0 {
			sp = 36
		}
		g.w(sprmSDxaColumns, uint16(clampTw(sp, 0, 31680)))
		if len(c.Widths) == n {
			g.b(sprmSFEvenlySpaced, 0)
			for i, cw := range c.Widths {
				g.op(sprmSDxaColWidth)
				g = append(g, byte(i))
				g = binary.LittleEndian.AppendUint16(g, uint16(clampTw(cw, 0, 31680)))
				if i < len(c.Spaces) {
					g.op(sprmSDxaColSpacing)
					g = append(g, byte(i))
					g = binary.LittleEndian.AppendUint16(g, uint16(clampTw(c.Spaces[i], 0, 31680)))
				}
			}
		}
		if c.Separator {
			g.b(sprmSLBetween, 1)
		}
	}
	if s.PageNumRestart {
		g.b(sprmSFPgnRestart, 1)
		g.w(sprmSPgnStart97, uint16(max(s.PageNumStart, 0)))
	}
	for i, f := range pgnFormats {
		if f == s.PageNumFormat && i > 0 {
			g.b(sprmSNfcPgn, byte(i))
		}
	}
	return g
}
