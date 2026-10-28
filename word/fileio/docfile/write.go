package docfile

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"unicode/utf16"

	"officelite/common/ole"
	"officelite/word/model"
)

// wrun starts a CHPX run at CP cp.
type wrun struct {
	cp     int
	grpprl []byte
}

// wpara is a paragraph ending (exclusive) at CP end.
type wpara struct {
	end    int
	istd   uint16
	grpprl []byte
}

type wfld struct {
	cp  int
	fld [2]byte
}

type writer struct {
	doc     *model.Document
	text    []uint16
	runs    []wrun
	paras   []wpara
	fonts   []string
	fontIdx map[string]int
	istds   map[string]int // style ID → istd
	wstyles []*wstyle      // by istd (nil = empty slot)
	ilfo    map[int]int    // model NumID → LFO index (1-based)
	listIDs []int
	flds    []wfld
	data    []byte // Data stream
	hasPic  bool
	warns   []string
	warned  map[string]bool
}

const textStart = 0x400

// Write encodes doc as a Word 97-2003 .doc. Content the format can't
// carry is dropped or simplified; Warnings lists what.
func Write(doc *model.Document, w io.Writer) error {
	wr, err := encode(doc)
	if err != nil {
		return err
	}
	_, err = w.Write(wr)
	return err
}

// WarnHeadersLost is the warning for headers and footers, which the .doc
// writer doesn't save.
const WarnHeadersLost = "Headers and footers can't be saved in a .doc file by WordLite and are left out."

// Warnings returns what saving doc as .doc loses (empty when nothing is).
func Warnings(doc *model.Document) []string {
	wr := newWriter(doc)
	wr.body()
	return wr.warns
}

func newWriter(doc *model.Document) *writer {
	wr := &writer{doc: doc, fontIdx: map[string]int{}, istds: map[string]int{}, ilfo: map[int]int{}, warned: map[string]bool{}}
	for _, f := range []string{"Times New Roman", "Symbol", "Arial"} {
		wr.font(f)
	}
	wr.assignStyles()
	wr.assignLists()
	return wr
}

func (w *writer) warn(msg string) {
	if !w.warned[msg] {
		w.warned[msg] = true
		w.warns = append(w.warns, msg)
	}
}

func (w *writer) font(name string) uint16 {
	if i, ok := w.fontIdx[name]; ok {
		return uint16(i)
	}
	w.fontIdx[name] = len(w.fonts)
	w.fonts = append(w.fonts, name)
	return uint16(len(w.fonts) - 1)
}

func encode(doc *model.Document) ([]byte, error) {
	w := newWriter(doc)
	w.body()
	stsh := w.stsh() // may add fonts
	lst, lfo := w.listTables()

	// WordDocument: FIB, text, FKPs, SEPX.
	wd := make([]byte, textStart, textStart+2*len(w.text)+4096)
	for _, c := range w.text {
		wd = binary.LittleEndian.AppendUint16(wd, c)
	}
	textEnd := len(wd)
	pad512 := func() {
		for len(wd)%512 != 0 {
			wd = append(wd, 0)
		}
	}
	pad512()
	chpPages, chpFcs := w.chpxFkps(textEnd)
	chpPn := make([]uint32, len(chpPages))
	for i, p := range chpPages {
		chpPn[i] = uint32(len(wd) / 512)
		wd = append(wd, p...)
	}
	papPages, papFcs := w.papxFkps()
	papPn := make([]uint32, len(papPages))
	for i, p := range papPages {
		papPn[i] = uint32(len(wd) / 512)
		wd = append(wd, p...)
	}
	sepxFc := len(wd)
	sepx := w.sepx()
	wd = binary.LittleEndian.AppendUint16(wd, uint16(len(sepx)))
	wd = append(wd, sepx...)

	// 1Table.
	var tbl []byte
	fcl := make([][2]uint32, nFcLcb97)
	put := func(idx int, b []byte) {
		fcl[idx] = [2]uint32{uint32(len(tbl)), uint32(len(b))}
		tbl = append(tbl, b...)
	}
	put(fcStshf, stsh)
	fcl[fcStshfOrig] = fcl[fcStshf]
	ccp := len(w.text)
	{ // PlcfSed
		var b []byte
		b = binary.LittleEndian.AppendUint32(b, 0)
		b = binary.LittleEndian.AppendUint32(b, uint32(ccp))
		b = binary.LittleEndian.AppendUint16(b, 0)
		b = binary.LittleEndian.AppendUint32(b, uint32(sepxFc))
		b = binary.LittleEndian.AppendUint16(b, 0)
		b = binary.LittleEndian.AppendUint32(b, 0xFFFFFFFF)
		put(fcPlcfSed, b)
	}
	put(fcPlcfBteChpx, plcBte(chpFcs, chpPn))
	put(fcPlcfBtePapx, plcBte(papFcs, papPn))
	put(fcSttbfFfn, w.sttbfFfn())
	if len(w.flds) > 0 {
		var b []byte
		for _, f := range w.flds {
			b = binary.LittleEndian.AppendUint32(b, uint32(f.cp))
		}
		b = binary.LittleEndian.AppendUint32(b, uint32(ccp))
		for _, f := range w.flds {
			b = append(b, f.fld[0], f.fld[1])
		}
		put(fcPlcfFldMom, b)
	}
	put(fcDop, dop(w.doc))
	{ // Clx: one Unicode piece
		var b []byte
		b = append(b, 0x02)
		b = binary.LittleEndian.AppendUint32(b, 4*2+8)
		b = binary.LittleEndian.AppendUint32(b, 0)
		b = binary.LittleEndian.AppendUint32(b, uint32(ccp))
		b = binary.LittleEndian.AppendUint16(b, 0)
		b = binary.LittleEndian.AppendUint32(b, textStart)
		b = binary.LittleEndian.AppendUint16(b, 0)
		put(fcClx, b)
	}
	if lst != nil {
		put(fcPlfLst, lst[:2+28*int(u16(lst))])
		tbl = append(tbl, lst[2+28*int(u16(lst)):]...) // LVLs follow the PlfLst
		put(fcPlfLfo, lfo)
	}

	writeFib(wd, w, fcl, ccp, textEnd)
	streams := []ole.Stream{{Name: "WordDocument", Data: wd}, {Name: "1Table", Data: tbl}}
	if len(w.data) > 0 {
		streams = append(streams, ole.Stream{Name: "Data", Data: w.data})
	}
	var out bytes.Buffer
	if err := ole.Write(&out, streams); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeFib(wd []byte, w *writer, fcl [][2]uint32, ccp, textEnd int) {
	le := binary.LittleEndian
	le.PutUint16(wd[0:], wIdent)
	le.PutUint16(wd[2:], nFib97)
	le.PutUint16(wd[6:], 0x0409)
	flags := uint16(0x1000 | 0x0200) // fExtChar, fWhichTblStm (1Table)
	if w.hasPic {
		flags |= 0x0008
	}
	le.PutUint16(wd[10:], flags)
	le.PutUint16(wd[12:], 0x00BF) // nFibBack
	le.PutUint32(wd[24:], textStart)
	le.PutUint32(wd[28:], uint32(textEnd))
	p := fibBaseSize
	le.PutUint16(wd[p:], 14) // csw
	p += 2
	le.PutUint16(wd[p+2*13:], 0x0409) // lidFE
	p += 28
	le.PutUint16(wd[p:], nLw97)
	p += 2
	lw := make([]uint32, nLw97)
	lw[lwCbMac] = uint32(len(wd))
	lw[lwCcpText] = uint32(ccp)
	lw[11], lw[14], lw[17] = 0xFFFFF, 0xFFFFF, 0xFFFFF
	for i, v := range lw {
		le.PutUint32(wd[p+4*i:], v)
	}
	p += 4 * nLw97
	le.PutUint16(wd[p:], nFcLcb97)
	p += 2
	for i, v := range fcl {
		le.PutUint32(wd[p+8*i:], v[0])
		le.PutUint32(wd[p+8*i+4:], v[1])
	}
	p += 8 * nFcLcb97
	le.PutUint16(wd[p:], 0) // cswNew
}

func plcBte(fcs []uint32, pns []uint32) []byte {
	var b []byte
	for _, fc := range fcs {
		b = binary.LittleEndian.AppendUint32(b, fc)
	}
	for _, pn := range pns {
		b = binary.LittleEndian.AppendUint32(b, pn)
	}
	return b
}

// chpxFkps packs the character runs into FKPs; it returns the pages and
// the PlcBteChpx FCs (one more than pages).
func (w *writer) chpxFkps(textEnd int) ([][]byte, []uint32) {
	type run struct {
		start, end uint32
		g          []byte
	}
	var runs []run
	for i, r := range w.runs {
		end := len(w.text)
		if i+1 < len(w.runs) {
			end = w.runs[i+1].cp
		}
		runs = append(runs, run{textStart + 2*uint32(r.cp), textStart + 2*uint32(end), r.grpprl})
	}
	_ = textEnd
	var pages [][]byte
	var fcs []uint32
	for len(runs) > 0 {
		page := make([]byte, 512)
		var offs []byte
		bodies := map[string]byte{}
		top := 511
		n := 0
		for n < len(runs) && n < 0x65 {
			g := runs[n].g
			cost := 0
			if _, ok := bodies[string(g)]; !ok && len(g) > 0 {
				cost = 1 + len(g)
			}
			newTop := top
			if cost > 0 {
				newTop = (top - cost) &^ 1
			}
			if 4*(n+2)+(n+1) > newTop {
				break
			}
			var off byte
			if len(g) > 0 {
				if o, ok := bodies[string(g)]; ok {
					off = o
				} else {
					top = newTop
					page[top] = byte(len(g))
					copy(page[top+1:], g)
					off = byte(top / 2)
					bodies[string(g)] = off
				}
			}
			offs = append(offs, off)
			n++
		}
		if n == 0 {
			n = 1 // a single oversized CHPX: write the run without properties
			offs = []byte{0}
		}
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint32(page[4*i:], runs[i].start)
		}
		binary.LittleEndian.PutUint32(page[4*n:], runs[n-1].end)
		copy(page[4*(n+1):], offs)
		page[511] = byte(n)
		pages = append(pages, page)
		fcs = append(fcs, runs[0].start)
		runs = runs[n:]
	}
	fcs = append(fcs, textStart+2*uint32(len(w.text)))
	return pages, fcs
}

// papxFkps packs the paragraphs into FKPs.
func (w *writer) papxFkps() ([][]byte, []uint32) {
	type para struct {
		start, end uint32
		body       []byte // PapxInFkp
	}
	var paras []para
	prev := 0
	for _, p := range w.paras {
		g := p.grpprl
		if 2+len(g) > 400 {
			// Too big for an FKP: store it in the Data stream.
			fc := len(w.data)
			w.data = binary.LittleEndian.AppendUint16(w.data, uint16(len(g)))
			w.data = append(w.data, g...)
			var h grpprl
			h.l(sprmPHugePapx, uint32(fc))
			g = h
		}
		gi := binary.LittleEndian.AppendUint16(nil, p.istd)
		gi = append(gi, g...)
		var body []byte
		if len(gi)%2 == 1 {
			body = append([]byte{byte((len(gi) + 1) / 2)}, gi...)
		} else {
			body = append([]byte{0, byte(len(gi) / 2)}, gi...)
		}
		paras = append(paras, para{textStart + 2*uint32(prev), textStart + 2*uint32(p.end), body})
		prev = p.end
	}
	var pages [][]byte
	var fcs []uint32
	for len(paras) > 0 {
		page := make([]byte, 512)
		top := 511
		n := 0
		var offs []byte
		bodies := map[string]byte{}
		for n < len(paras) && n < 0x1D {
			b := paras[n].body
			newTop := top
			_, dup := bodies[string(b)]
			if !dup {
				newTop = (top - len(b)) &^ 1
			}
			if 4*(n+2)+13*(n+1) > newTop {
				break
			}
			if dup {
				offs = append(offs, bodies[string(b)])
			} else {
				top = newTop
				copy(page[top:], b)
				bodies[string(b)] = byte(top / 2)
				offs = append(offs, byte(top/2))
			}
			n++
		}
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint32(page[4*i:], paras[i].start)
			page[4*(n+1)+13*i] = offs[i]
		}
		binary.LittleEndian.PutUint32(page[4*n:], paras[n-1].end)
		page[511] = byte(n)
		pages = append(pages, page)
		fcs = append(fcs, paras[0].start)
		paras = paras[n:]
	}
	fcs = append(fcs, textStart+2*uint32(len(w.text)))
	return pages, fcs
}

// chars appends characters with one CHPX.
func (w *writer) chars(u []uint16, chpx []byte) {
	if len(u) == 0 {
		return
	}
	if n := len(w.runs); n == 0 || !bytes.Equal(w.runs[n-1].grpprl, chpx) {
		w.runs = append(w.runs, wrun{len(w.text), chpx})
	}
	w.text = append(w.text, u...)
}

func (w *writer) endPara(mark uint16, markChpx []byte, istd int, papx []byte) {
	w.chars([]uint16{mark}, markChpx)
	w.paras = append(w.paras, wpara{len(w.text), uint16(istd), papx})
}

// body writes the document's blocks.
func (w *writer) body() {
	if w.doc.HF.Any() {
		w.warn(WarnHeadersLost)
	}
	blocks := w.doc.Body
	if len(blocks) == 0 {
		blocks = []model.Block{&model.Paragraph{}}
	}
	for _, b := range blocks {
		switch b := b.(type) {
		case *model.Paragraph:
			w.para(b, 0x0D, nil, false)
		case *model.Table:
			w.table(b)
		}
	}
	if _, ok := blocks[len(blocks)-1].(*model.Table); ok {
		w.para(&model.Paragraph{}, 0x0D, nil, false) // Word needs a paragraph after a table
	}
}

func (w *writer) paraIstd(id string) int {
	if i, ok := w.istds[id]; ok {
		if s := w.doc.Styles.Find(id); s != nil && s.Type == model.ParagraphStyle {
			return i
		}
	}
	return 0
}

func (w *writer) para(p *model.Paragraph, mark uint16, extra []byte, inTable bool) {
	for _, o := range p.Before {
		if o != nil && o.Format != opaqueFormat {
			w.warn("Some block content (such as content controls' extras) can't be saved in a .doc file and is left out.")
		}
	}
	var link *model.Span // open hyperlink field
	closeLink := func() {
		if link != nil {
			w.fieldChar(0x15, nil, 0x80)
			link = nil
		}
	}
	for i := range p.Runs {
		r := &p.Runs[i]
		if l := linkSpan(r.Span); l != link {
			closeLink()
			if l != nil {
				w.fieldStart(hyperlinkInstr(l), w.encChp(r.Props, r.Style), 88)
				link = l
			}
		}
		w.run(r, inTable)
	}
	closeLink()
	papx := append(w.encPap(p.Props), extra...)
	w.endPara(mark, w.encChp(p.Mark, ""), w.paraIstd(p.Style), papx)
}

// linkSpan returns the outermost hyperlink span around a run, or nil.
func linkSpan(s *model.Span) *model.Span {
	for _, sp := range s.Chain() {
		if sp.Kind == model.SpanLink && (sp.URL != "" || sp.Anchor != "") {
			return sp
		}
	}
	return nil
}

func hyperlinkInstr(l *model.Span) string {
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, "") + `"` }
	in := " HYPERLINK "
	if l.URL != "" {
		in += q(l.URL) + " "
	}
	if l.Anchor != "" {
		in += `\l ` + q(l.Anchor) + " "
	}
	if l.Tooltip != "" {
		in += `\o ` + q(l.Tooltip) + " "
	}
	return in
}

// fieldChar writes a field begin/separator/end character.
func (w *writer) fieldChar(c uint16, chpx []byte, grf byte) {
	w.flds = append(w.flds, wfld{len(w.text), [2]byte{byte(c), grf}})
	var g grpprl
	g = append(g, chpx...)
	g.b(sprmCFSpec, 1)
	w.chars([]uint16{c}, g)
}

// fieldStart writes a field's begin character, instruction and separator;
// the result and fieldChar(0x15...) follow.
func (w *writer) fieldStart(instr string, chpx []byte, flt byte) {
	w.fieldChar(0x13, chpx, flt)
	w.chars(textUnits(instr, true), chpx)
	w.fieldChar(0x14, chpx, 0xFF)
}

// fieldTypes maps field names to their flt.
var fieldTypes = map[string]byte{
	"REF": 3, "SET": 6, "IF": 7, "INDEX": 8, "STYLEREF": 10, "SEQ": 12, "TOC": 13, "INFO": 14,
	"TITLE": 15, "SUBJECT": 16, "AUTHOR": 17, "KEYWORDS": 18, "COMMENTS": 19, "LASTSAVEDBY": 20,
	"CREATEDATE": 21, "SAVEDATE": 22, "PRINTDATE": 23, "REVNUM": 24, "EDITTIME": 25, "NUMPAGES": 26,
	"NUMWORDS": 27, "NUMCHARS": 28, "FILENAME": 29, "TEMPLATE": 30, "DATE": 31, "TIME": 32, "PAGE": 33,
	"=": 34, "QUOTE": 35, "PAGEREF": 37, "ASK": 38, "FILLIN": 39, "MERGEFIELD": 59, "DOCVARIABLE": 64,
	"SECTION": 65, "SECTIONPAGES": 66, "INCLUDETEXT": 68, "INCLUDEPICTURE": 67, "HYPERLINK": 88,
	"SYMBOL": 57, "LISTNUM": 90, "DOCPROPERTY": 85, "NOTEREF": 72,
}

// textUnits converts model text to .doc characters, dropping control
// characters that would change the document structure.
func textUnits(s string, inTable bool) []uint16 {
	var out []uint16
	for _, c := range s {
		switch {
		case c == '\t':
			out = append(out, 0x09)
		case c == '\n' || c == '\v':
			out = append(out, 0x0B)
		case c == '\f':
			if inTable {
				out = append(out, 0x0B)
			} else {
				out = append(out, 0x0C)
			}
		case c < 0x20 || c == 0xFFFC:
			continue
		case c == 0x2011:
			out = append(out, 0x1E)
		case c == 0x00AD:
			out = append(out, 0x1F)
		default:
			out = append(out, utf16.Encode([]rune{c})...)
		}
	}
	return out
}

func (w *writer) run(r *model.Run, inTable bool) {
	chpx := w.encChp(r.Props, r.Style)
	switch r.Kind {
	case model.RunText:
		w.chars(textUnits(r.Text, inTable), chpx)
	case model.RunTab:
		w.chars([]uint16{0x09}, chpx)
	case model.RunBreak:
		w.chars([]uint16{0x0B}, chpx)
	case model.RunPageBreak:
		if inTable {
			w.chars([]uint16{0x0B}, chpx)
		} else {
			w.chars([]uint16{0x0C}, chpx)
		}
	case model.RunOpaque:
		w.opaque(r, chpx, inTable)
	}
}

// opaque writes a preserved run: .doc fields and pictures go back as they
// were; anything else becomes its placeholder text.
func (w *writer) opaque(r *model.Run, chpx []byte, inTable bool) {
	if r.Opaque != nil && r.Opaque.Format == opaqueFormat {
		var blob opaqueBlob
		if json.Unmarshal(r.Opaque.Data, &blob) == nil {
			switch blob.Kind {
			case "field", "picture":
				if w.writeBlob(&blob, r.Style) {
					return
				}
			case "note":
				w.warn("Footnote and endnote references are saved as plain text (the notes are not kept).")
				w.chars(textUnits(r.Text, inTable), chpx)
				return
			case "shape":
				w.warn("Floating pictures and drawings are not kept.")
				return
			}
		}
	}
	if r.Opaque != nil && r.Opaque.Field != "" {
		// A field from another format: write it as a field with its last result.
		instr := strings.TrimSpace(r.Opaque.Field)
		name := strings.ToUpper(strings.Fields(instr + " x")[0])
		flt, ok := fieldTypes[name]
		if !ok {
			flt = 1
		}
		w.fieldStart(" "+instr+" ", chpx, flt)
		w.chars(textUnits(r.Text, true), chpx)
		w.fieldChar(0x15, chpx, 0x80)
		return
	}
	what := "Some content"
	if r.Text != "" {
		what = "Content such as " + r.Text
	}
	w.warn(what + " can't be saved in a .doc file and is written as its placeholder text.")
	w.chars(textUnits(r.Text, inTable), chpx)
}

// writeBlob writes the characters of a .doc opaque run; it reports false
// (writing nothing) if the blob is inconsistent.
func (w *writer) writeBlob(b *opaqueBlob, style string) bool {
	nf, depth := 0, 0
	for _, s := range b.Segs {
		if s.Pic != nil && (len(s.Text) != 1 || s.Text[0] != 0x01) {
			return false
		}
		for _, c := range s.Text {
			switch c {
			case 0x13:
				depth++
				nf++
			case 0x14:
				nf++
			case 0x15:
				depth--
				nf++
			case 0x0D, 0x07, 0x0C:
				return false
			}
			if depth < 0 {
				return false
			}
		}
	}
	if depth != 0 || nf != len(b.Flds) {
		return false
	}
	k := 0
	for _, s := range b.Segs {
		var g grpprl
		if i, ok := w.istds[style]; ok && style != "" {
			if st := w.doc.Styles.Find(style); st != nil && st.Type == model.CharacterStyle {
				g.w(sprmCIstd, uint16(i))
			}
		}
		eachSprm(s.Chpx, func(op uint16, a []byte) {
			switch op {
			case sprmCRgFtc0, sprmCRgFtc1, sprmCRgFtc2, sprmCFtcBi, sprmCFtcDefault:
				name := ""
				if i := int(u16(a)); i < len(b.Fonts) {
					name = b.Fonts[i]
				}
				if name != "" {
					g.w(op, w.font(name))
				}
				return
			}
			g.op(op)
			if op>>13 == 6 {
				g = append(g, byte(len(a)))
			}
			g = append(g, a...)
		})
		if s.Pic != nil {
			loc := len(w.data)
			w.data = append(w.data, s.Pic...)
			w.hasPic = true
			g.l(sprmCPicLocation, uint32(loc))
		}
		for i, c := range s.Text {
			if c >= 0x13 && c <= 0x15 {
				w.flds = append(w.flds, wfld{len(w.text) + i, b.Flds[k]})
				k++
			}
		}
		w.chars(s.Text, g)
	}
	return true
}

// table writes a table as cell paragraphs plus a row-end mark per row.
func (w *writer) table(t *model.Table) {
	x0 := toTwips(t.Indent)
	textWidth := toTwips(w.doc.Page.Width - w.doc.Page.MarginLeft - w.doc.Page.MarginRight)
	if textWidth <= 0 {
		textWidth = 9360
	}
	for _, row := range t.Rows {
		cells := row.Cells
		if len(cells) > 63 {
			w.warn("Table rows with more than 63 cells are cut to 63 cells.")
			cells = cells[:63]
		}
		if len(cells) == 0 {
			continue
		}
		centers := []int{x0}
		col := 0
		for _, c := range cells {
			span := c.Span
			if span < 1 {
				span = 1
			}
			wd := 0
			if col+span <= len(t.Columns) {
				for _, cw := range t.Columns[col : col+span] {
					wd += toTwips(cw)
				}
			} else if c.Width > 0 {
				wd = toTwips(c.Width)
			} else {
				wd = textWidth / len(cells)
			}
			if wd < 1 {
				wd = 1
			}
			col += span
			centers = append(centers, centers[len(centers)-1]+wd)
		}
		var inCell grpprl
		inCell.b(sprmPFInTable, 1)
		for _, c := range cells {
			var paras []*model.Paragraph
			for _, b := range c.Blocks {
				switch b := b.(type) {
				case *model.Paragraph:
					paras = append(paras, b)
				case *model.Table:
					w.warn("Nested tables are saved as paragraphs inside the outer cell.")
					model.WalkBlocks([]model.Block{b}, func(p *model.Paragraph) bool {
						paras = append(paras, p)
						return true
					})
				}
			}
			if len(paras) == 0 {
				paras = []*model.Paragraph{{}}
			}
			for i, p := range paras {
				mark := uint16(0x0D)
				if i == len(paras)-1 {
					mark = 0x07
				}
				w.para(p, mark, inCell, true)
			}
		}
		w.endPara(0x07, nil, 0, w.tap(t, row, cells, centers))
	}
}

// tap encodes a row's table properties for its row-end paragraph.
func (w *writer) tap(t *model.Table, row *model.TableRow, cells []*model.TableCell, centers []int) []byte {
	var g grpprl
	g.b(sprmPFInTable, 1)
	g.b(sprmPFTtp, 1)
	switch t.Align {
	case model.AlignCenter:
		g.w(sprmTJc90, 1)
	case model.AlignRight:
		g.w(sprmTJc90, 2)
	}
	g.w(sprmTDxaGapHalf, 108)
	if row.Height > 0 {
		g.w(sprmTDyaRowHeight, uint16(toTwips(row.Height)))
	}
	if row.Header {
		g.b(sprmTTableHeader, 1)
	}
	brc := []byte{0, 0, 0, 0}
	if t.Borders {
		brc = []byte{4, 1, 0, 0} // single, 1/2 pt
		var b []byte
		for i := 0; i < 6; i++ {
			b = append(b, brc...)
		}
		g.v(sprmTTableBorders80, b)
	}
	n := len(cells)
	op := make([]byte, 0, 1+2*(n+1)+20*n)
	op = append(op, byte(n))
	for _, x := range centers {
		op = binary.LittleEndian.AppendUint16(op, uint16(int16(x)))
	}
	for i, c := range cells {
		grf := uint16(3 << 9) // ftsWidth = dxa
		switch c.VMerge {
		case 1:
			grf |= 3 << 5
		case 2:
			grf |= 1 << 5
		}
		grf |= uint16(c.VAlign&3) << 7
		op = binary.LittleEndian.AppendUint16(op, grf)
		op = binary.LittleEndian.AppendUint16(op, uint16(centers[i+1]-centers[i]))
		for k := 0; k < 4; k++ {
			op = append(op, brc...)
		}
	}
	g.op(sprmTDefTable)
	g = binary.LittleEndian.AppendUint16(g, uint16(len(op)+1))
	g = append(g, op...)
	shaded := false
	for _, c := range cells {
		if c.Shading != "" {
			shaded = true
		}
	}
	if shaded {
		for k, sop := range []uint16{sprmTDefTableShd, sprmTDefTableShd2nd, sprmTDefTableShd3rd} {
			lo, hi := 22*k, 22*k+22
			if lo >= n {
				break
			}
			if hi > n {
				hi = n
			}
			var b []byte
			for _, c := range cells[lo:hi] {
				if c.Shading == "" {
					b = append(b, 0, 0, 0, 0xFF, 0, 0, 0, 0xFF, 0, 0)
				} else {
					b = append(b, encShd(c.Shading)...)
				}
			}
			g.v(sop, b)
		}
	}
	return g
}

// encChp encodes character formatting (absolute values) and a character style.
func (w *writer) encChp(p model.RunProps, style string) []byte {
	var g grpprl
	if style != "" {
		if i, ok := w.istds[style]; ok {
			if st := w.doc.Styles.Find(style); st != nil && st.Type == model.CharacterStyle && !st.Default {
				g.w(sprmCIstd, uint16(i))
			}
		}
	}
	w.encRun(&g, p)
	return g
}

func (w *writer) encRun(g *grpprl, p model.RunProps) {
	if p.Font != "" {
		f := w.font(p.Font)
		g.w(sprmCRgFtc0, f)
		g.w(sprmCRgFtc1, f)
		g.w(sprmCRgFtc2, f)
	}
	if p.Size > 0 {
		hps := int(p.Size*2 + 0.5)
		if hps < 2 {
			hps = 2
		}
		if hps > 3276 {
			hps = 3276
		}
		g.w(sprmCHps, uint16(hps))
	}
	for t, op := range []uint16{sprmCFBold, sprmCFItalic, sprmCFStrike, sprmCFSmallCaps, sprmCFCaps, sprmCFVanish} {
		switch *toggleField(&p, t) {
		case model.On:
			g.b(op, 1)
		case model.Off:
			g.b(op, 0)
		}
	}
	if p.Underline != model.UnderlineUnset {
		g.b(sprmCKul, kulFromModel(p.Underline))
	}
	if p.Color != "" {
		cv := encColorRef(p.Color)
		if cv[3] == 0 {
			g.l(sprmCCv, binary.LittleEndian.Uint32(cv[:]))
		}
	}
	if p.Highlight != "" {
		if ico, ok := highlightIco(p.Highlight); ok {
			g.b(sprmCHighlight, ico)
		} else if cv := encColorRef(p.Highlight); cv[3] == 0 {
			g.v(sprmCShd, encShd(p.Highlight))
		}
	}
	switch p.Vert {
	case model.VertBaseline:
		g.b(sprmCIss, 0)
	case model.VertSuper:
		g.b(sprmCIss, 1)
	case model.VertSub:
		g.b(sprmCIss, 2)
	}
}

// encPap encodes paragraph formatting.
func (w *writer) encPap(p model.ParaProps) []byte {
	var g grpprl
	switch p.Align {
	case model.AlignLeft, model.AlignCenter, model.AlignRight, model.AlignJustify:
		jc := byte(p.Align - model.AlignLeft)
		g.b(sprmPJc80, jc)
		g.b(sprmPJc, jc)
	}
	if p.SpaceBefore != nil {
		g.w(sprmPDyaBefore, uint16(clampTw(*p.SpaceBefore, 0, 31680)))
	}
	if p.SpaceAfter != nil {
		g.w(sprmPDyaAfter, uint16(clampTw(*p.SpaceAfter, 0, 31680)))
	}
	if p.LineRule != model.LineUnset {
		var dya, mult int
		switch p.LineRule {
		case model.LineAuto:
			dya, mult = int(p.Line*240+0.5), 1
			if dya <= 0 {
				dya = 240
			}
		case model.LineExact:
			dya = -clampTw(p.Line, 1, 31680)
		case model.LineAtLeast:
			dya = clampTw(p.Line, 0, 31680)
		}
		g.l(sprmPDyaLine, uint32(uint16(int16(dya)))|uint32(mult)<<16)
	}
	ind := func(v *float64, op80, op uint16) {
		if v != nil {
			x := uint16(int16(clampTw(*v, -31680, 31680)))
			g.w(op80, x)
			g.w(op, x)
		}
	}
	ind(p.IndentLeft, sprmPDxaLeft80, sprmPDxaLeft)
	ind(p.IndentRight, sprmPDxaRight80, sprmPDxaRight)
	ind(p.IndentFirst, sprmPDxaLeft180, sprmPDxaLeft1)
	for _, t := range []struct {
		v  model.Tri
		op uint16
	}{{p.KeepLines, sprmPFKeep}, {p.KeepNext, sprmPFKeepFollow}, {p.PageBreakBefore, sprmPFPageBreakBefore}, {p.WidowControl, sprmPFWidowControl}} {
		switch t.v {
		case model.On:
			g.b(t.op, 1)
		case model.Off:
			g.b(t.op, 0)
		}
	}
	if p.Shading != "" {
		if cv := encColorRef(p.Shading); cv[3] == 0 {
			g.v(sprmPShd, encShd(p.Shading))
		}
	}
	if len(p.Tabs) > 0 {
		g.v(sprmPChgTabsPapx, encTabs(p.Tabs))
	}
	if p.List != nil {
		if p.List.NumID == 0 {
			g.w(sprmPIlfo, 0)
		} else if ilfo, ok := w.ilfo[p.List.NumID]; ok {
			lvl := p.List.Level
			if lvl < 0 || lvl > 8 {
				lvl = 0
			}
			g.b(sprmPIlvl, byte(lvl))
			g.w(sprmPIlfo, uint16(ilfo))
		}
	}
	if p.OutlineLevel >= 1 && p.OutlineLevel <= 9 {
		g.b(sprmPOutLvl, byte(p.OutlineLevel-1))
	}
	return g
}

func clampTw(pt float64, lo, hi int) int {
	v := toTwips(pt)
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// encTabs encodes tab stops as a sprmPChgTabsPapx operand.
func encTabs(tabs []model.TabStop) []byte {
	var del, add []model.TabStop
	for _, t := range tabs {
		if t.Clear {
			del = append(del, t)
		} else {
			add = append(add, t)
		}
	}
	sort.SliceStable(del, func(i, j int) bool { return del[i].Pos < del[j].Pos })
	sort.SliceStable(add, func(i, j int) bool { return add[i].Pos < add[j].Pos })
	if len(del) > 50 {
		del = del[:50]
	}
	if len(add) > 50 {
		add = add[:50]
	}
	b := []byte{byte(len(del))}
	for _, t := range del {
		b = binary.LittleEndian.AppendUint16(b, uint16(int16(clampTw(t.Pos, -31680, 31680))))
	}
	b = append(b, byte(len(add)))
	for _, t := range add {
		b = binary.LittleEndian.AppendUint16(b, uint16(int16(clampTw(t.Pos, -31680, 31680))))
	}
	for _, t := range add {
		var jc, tlc byte
		switch {
		case t.Decimal:
			jc = 3
		case t.Align == model.AlignCenter:
			jc = 1
		case t.Align == model.AlignRight:
			jc = 2
		}
		switch t.Leader {
		case '.':
			tlc = 1
		case '-':
			tlc = 2
		case '_':
			tlc = 3
		}
		b = append(b, jc|tlc<<3)
	}
	return b
}

// sepx encodes the page setup.
func (w *writer) sepx() []byte {
	pg := w.doc.Page
	if pg.Width <= 0 || pg.Height <= 0 {
		pg = model.LetterPage()
	}
	var g grpprl
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
	return g
}

// sttbfFfn encodes the font table.
func (w *writer) sttbfFfn() []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(len(w.fonts)))
	b = binary.LittleEndian.AppendUint16(b, 0)
	for _, name := range w.fonts {
		u := utf16.Encode([]rune(name))
		if len(u) > 31 {
			u = u[:31]
		}
		ffn := make([]byte, 39)
		ffid := byte(0x02 | 0x04) // variable pitch, TrueType
		symbol := strings.EqualFold(name, "Symbol") || strings.HasPrefix(strings.ToLower(name), "wingdings")
		switch {
		case symbol:
			ffid |= 5 << 4 // decorative
			ffn[3] = 2     // SYMBOL_CHARSET
		case strings.Contains(strings.ToLower(name), "times") || strings.Contains(strings.ToLower(name), "roman"):
			ffid |= 1 << 4
		case strings.Contains(strings.ToLower(name), "courier") || strings.Contains(strings.ToLower(name), "mono"):
			ffid = 0x01 | 0x04 | 3<<4
		default:
			ffid |= 2 << 4
		}
		ffn[0] = ffid
		binary.LittleEndian.PutUint16(ffn[1:], 400)
		// FONTSIGNATURE: Latin, Greek, Cyrillic ranges and code pages.
		fs := ffn[15:39]
		if symbol {
			binary.LittleEndian.PutUint32(fs[16:], 0x80000000)
		} else {
			binary.LittleEndian.PutUint32(fs[0:], 0xE0002EFF)
			binary.LittleEndian.PutUint32(fs[4:], 0xC000785B)
			binary.LittleEndian.PutUint32(fs[8:], 0x00000009)
			binary.LittleEndian.PutUint32(fs[16:], 0x000001FF)
		}
		for _, c := range u {
			ffn = binary.LittleEndian.AppendUint16(ffn, c)
		}
		ffn = append(ffn, 0, 0)
		b = append(b, byte(len(ffn)))
		b = append(b, ffn...)
	}
	return b
}

// dop returns a Word 97 document properties block.
func dop(doc *model.Document) []byte {
	d := make([]byte, 500)
	le := binary.LittleEndian
	tab := 720
	if doc.DefaultTabStop > 0 {
		tab = clampTw(doc.DefaultTabStop, 1, 31680)
	}
	le.PutUint16(d[0:], 0x0002)       // fWidowControl
	le.PutUint16(d[2:], 1<<2)         // nFtn = 1
	d[6] = 0x80                       // fWord97Compat... (as Word writes)
	d[7] = 0x18                       //
	le.PutUint16(d[8:], 0xF000)       // copts60
	le.PutUint16(d[10:], uint16(tab)) // dxaTab
	le.PutUint16(d[14:], 360)         // dxaHotZ
	le.PutUint16(d[32:], 1)           // nRevision
	le.PutUint16(d[52:], 1<<2)        // nEdn = 1
	le.PutUint16(d[82:], 100<<3)      // pctWwdSaved = 100%
	return d
}
