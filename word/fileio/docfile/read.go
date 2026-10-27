package docfile

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"

	"officelite/common/ole"
	"officelite/word/model"
)

// piece is one entry of the piece table: CPs [cp, cpEnd) stored at byte
// offset fc, compressed (cp1252, one byte per character) or UTF-16.
type piece struct {
	cp, cpEnd  int
	fc         uint32
	compressed bool
	prm        uint16
}

// fkpRun is a run of FCs with one CHPX or PAPX.
type fkpRun struct {
	start, end uint32
	istd       uint16 // PAPX only
	grpprl     []byte
}

type reader struct {
	doc   *model.Document
	wd    []byte
	table []byte
	data  []byte
	fib   *fib

	prcs    [][]byte // RgPrc grpprls
	pieces  []piece
	text    []uint16 // main document text, indexed by CP
	cpFc    []uint32 // byte offset of each CP's character
	cpPiece []int32

	chpx []fkpRun
	papx []fkpRun

	fonts    []string
	styles   []*stdInfo // by istd
	styleIDs []string   // istd → model style ID

	// fields
	skip    []bool      // CPs not shown (field codes of flattened fields)
	unitEnd map[int]int // field begin CP → end CP for fields kept as a unit
	fldAt   map[int][2]byte

	footnoteRefs map[int]int // CP → footnote number
	endnoteRefs  map[int]int

	chpCache map[[2]int]*chpInfo
	warned   map[string]bool
	sedCPs   map[int]bool // CPs that start a new section
}

// Read decodes a Word 97-2003 .doc document.
func Read(data []byte) (doc *model.Document, err error) {
	defer func() {
		if r := recover(); r != nil {
			doc, err = nil, fmt.Errorf("doc: malformed document (%v)", r)
		}
	}()
	names, err := ole.Streams(data)
	if err != nil {
		return nil, fmt.Errorf("not a valid compound file: %w", err)
	}
	r := &reader{warned: map[string]bool{}, chpCache: map[[2]int]*chpInfo{}}
	_, r.wd, err = ole.ReadStream(data, "WordDocument")
	if err != nil {
		return nil, errors.New("not a Word document (no WordDocument stream)")
	}
	if r.fib, err = parseFib(r.wd); err != nil {
		return nil, err
	}
	tableName := "0Table"
	if r.fib.whichTable {
		tableName = "1Table"
	}
	if _, r.table, err = ole.ReadStream(data, tableName); err != nil {
		return nil, fmt.Errorf("doc: missing %s stream", tableName)
	}
	_, r.data, _ = ole.ReadStream(data, "Data")

	r.doc = &model.Document{Lists: map[int]*model.List{}, Page: model.LetterPage()}
	for _, n := range names {
		switch strings.ToLower(n) {
		case "macros", "_vba_project_cur", "vba":
			r.warn("The document contains macros; they are never run and are not kept when saving.")
		case "objectpool":
			r.warn("Embedded objects are shown as their pictures only.")
		}
	}
	if err := r.readPieces(); err != nil {
		return nil, err
	}
	r.readFkps()
	r.readFonts()
	r.readStyles()
	r.readLists()
	r.readSection()
	if dop := r.fib.blob(r.table, fcDop); len(dop) >= 12 {
		if tab := u16(dop[10:]); tab > 0 {
			r.doc.DefaultTabStop = twips(int(tab))
		}
	}
	r.readNotes()
	r.readFields()
	r.subdocWarnings()
	r.buildBody()
	r.doc.Source = data
	r.doc.SourceFormat = "doc"
	return r.doc, nil
}

func (r *reader) warn(msg string) {
	if !r.warned[msg] {
		r.warned[msg] = true
		r.doc.Warnings = append(r.doc.Warnings, msg)
	}
}

// readPieces parses the Clx and decodes the main document text.
func (r *reader) readPieces() error {
	clx := r.fib.blob(r.table, fcClx)
	if clx == nil {
		return errors.New("doc: missing piece table")
	}
	for len(clx) > 0 && clx[0] == 0x01 {
		if len(clx) < 3 {
			return errors.New("doc: bad piece table")
		}
		n := int(i16(clx[1:]))
		if n < 0 || 3+n > len(clx) {
			return errors.New("doc: bad piece table")
		}
		r.prcs = append(r.prcs, clx[3:3+n])
		clx = clx[3+n:]
	}
	if len(clx) < 5 || clx[0] != 0x02 {
		return errors.New("doc: bad piece table")
	}
	lcb := int(u32(clx[1:]))
	plc := clx[5:]
	if lcb < 4 || lcb > len(plc) {
		return errors.New("doc: bad piece table")
	}
	plc = plc[:lcb]
	n := (lcb - 4) / 12
	ccp := int(r.fib.lw[lwCcpText])
	if ccp < 0 {
		return errors.New("doc: bad text length")
	}
	r.text = make([]uint16, 0, ccp)
	r.cpFc = make([]uint32, 0, ccp)
	r.cpPiece = make([]int32, 0, ccp)
	for i := 0; i < n; i++ {
		cp, cpEnd := int(u32(plc[4*i:])), int(u32(plc[4*i+4:]))
		pcd := plc[4*(n+1)+8*i:]
		fcc := u32(pcd[2:])
		p := piece{cp: cp, cpEnd: cpEnd, compressed: fcc&0x40000000 != 0, prm: u16(pcd[6:])}
		if p.compressed {
			p.fc = (fcc & 0x3FFFFFFF) / 2
		} else {
			p.fc = fcc & 0x3FFFFFFF
		}
		r.pieces = append(r.pieces, p)
		for c := cp; c < cpEnd && c < ccp; c++ {
			if c != len(r.text) {
				// Pieces are in CP order; anything else is corrupt.
				return errors.New("doc: piece table out of order")
			}
			var ch uint16
			var fc uint32
			if p.compressed {
				fc = p.fc + uint32(c-cp)
				if int(fc) >= len(r.wd) {
					return errors.New("doc: text outside the WordDocument stream")
				}
				ch = cp1252(r.wd[fc])
			} else {
				fc = p.fc + 2*uint32(c-cp)
				if int(fc)+2 > len(r.wd) {
					return errors.New("doc: text outside the WordDocument stream")
				}
				ch = u16(r.wd[fc:])
			}
			r.text = append(r.text, ch)
			r.cpFc = append(r.cpFc, fc)
			r.cpPiece = append(r.cpPiece, int32(i))
		}
	}
	if len(r.text) < ccp {
		return errors.New("doc: piece table shorter than the text")
	}
	return nil
}

func cp1252(b byte) uint16 {
	if b < 0x80 || b >= 0xA0 {
		return uint16(b)
	}
	return uint16(charmap.Windows1252.DecodeByte(b))
}

// pieceSprms returns the property modifiers a piece's Prm adds.
func (r *reader) pieceSprms(pi int) []byte {
	prm := r.pieces[pi].prm
	if prm&1 != 0 {
		if i := int(prm >> 1); i < len(r.prcs) {
			return r.prcs[i]
		}
		return nil
	}
	isprm, val := byte(prm>>1&0x7F), byte(prm>>8)
	if prm == 0 || isprm == 0 {
		return nil
	}
	if op, ok := prm0Sprms[isprm]; ok {
		var g grpprl
		g.b(op, val)
		return g
	}
	return nil
}

// prm0Sprms maps the isprm of a Prm0 to its one-byte-operand sprm (the
// common ones; [MS-DOC] 2.2.6.1).
var prm0Sprms = map[byte]uint16{
	0x05: sprmPJc80, 0x07: sprmPFKeep, 0x08: sprmPFKeepFollow, 0x09: sprmPFPageBreakBefore,
	0x18: sprmPFInTable, 0x19: sprmPFTtp, 0x35: sprmCFBold, 0x36: sprmCFItalic, 0x37: sprmCFStrike,
	0x3A: sprmCFSmallCaps, 0x3B: sprmCFCaps, 0x3C: sprmCFVanish, 0x3E: sprmCKul, 0x42: sprmCIco,
	0x48: sprmCIss,
}

// readFkps loads every CHPX and PAPX FKP referenced by the bin tables.
func (r *reader) readFkps() {
	r.chpx = r.readBte(fcPlcfBteChpx, false)
	r.papx = r.readBte(fcPlcfBtePapx, true)
}

func (r *reader) readBte(idx int, papx bool) []fkpRun {
	plc := r.fib.blob(r.table, idx)
	if len(plc) < 8 {
		return nil
	}
	n := (len(plc) - 4) / 8
	var runs []fkpRun
	for i := 0; i < n; i++ {
		pn := u32(plc[4*(n+1)+4*i:]) & 0x3FFFFF
		off := int(pn) * 512
		if off+512 > len(r.wd) {
			continue
		}
		page := r.wd[off : off+512]
		cnt := int(page[511])
		if papx {
			if 4*(cnt+1)+13*cnt > 511 {
				continue
			}
		} else if 5*cnt+4 > 511 {
			continue
		}
		for j := 0; j < cnt; j++ {
			run := fkpRun{start: u32(page[4*j:]), end: u32(page[4*j+4:])}
			if papx {
				b := int(page[4*(cnt+1)+13*j]) * 2
				if b > 0 && b < 511 {
					size, at := 0, 0
					if cb := int(page[b]); cb != 0 {
						size, at = 2*cb-1, b+1
					} else {
						size, at = 2*int(page[b+1]), b+2
					}
					if at+size <= 512 && size >= 2 {
						run.istd = u16(page[at:])
						run.grpprl = r.expandHuge(page[at+2 : at+size])
					}
				}
			} else {
				b := int(page[4*(cnt+1)+j]) * 2
				if b > 0 && b < 511 {
					cb := int(page[b])
					if b+1+cb <= 512 {
						run.grpprl = page[b+1 : b+1+cb]
					}
				}
			}
			runs = append(runs, run)
		}
	}
	sort.SliceStable(runs, func(a, b int) bool { return runs[a].start < runs[b].start })
	return runs
}

// expandHuge replaces a sprmPHugePapx by the grpprl it points at in the
// Data stream.
func (r *reader) expandHuge(g []byte) []byte {
	if len(g) >= 6 && u16(g) == sprmPHugePapx {
		fc := int(u32(g[2:]))
		if fc+2 <= len(r.data) {
			cb := int(u16(r.data[fc:]))
			if fc+2+cb <= len(r.data) {
				return r.data[fc+2 : fc+2+cb]
			}
		}
		return nil
	}
	return g
}

// findRun returns the index of the run containing fc, or -1.
func findRun(runs []fkpRun, fc uint32) int {
	i := sort.Search(len(runs), func(i int) bool { return runs[i].end > fc })
	if i < len(runs) && runs[i].start <= fc {
		return i
	}
	return -1
}

// readFonts parses SttbfFfn.
func (r *reader) readFonts() {
	b := r.fib.blob(r.table, fcSttbfFfn)
	if len(b) < 4 {
		return
	}
	n := int(u16(b))
	p := 4
	if n == 0xFFFF { // extended STTB (not expected for fonts)
		n = int(u16(b[2:]))
		p = 6
	}
	for i := 0; i < n && p < len(b); i++ {
		cb := int(b[p])
		end := p + 1 + cb
		if end > len(b) {
			break
		}
		ffn := b[p+1 : end]
		name := ""
		if len(ffn) > 39 {
			name = utf16z(ffn[39:])
		}
		r.fonts = append(r.fonts, name)
		p = end
	}
}

// utf16z decodes a NUL-terminated little-endian UTF-16 string.
func utf16z(b []byte) string {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		c := u16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

func (r *reader) font(ftc uint16) string {
	if int(ftc) < len(r.fonts) {
		return r.fonts[ftc]
	}
	return ""
}

// readSection takes the page setup from the last section.
func (r *reader) readSection() {
	plc := r.fib.blob(r.table, fcPlcfSed)
	r.sedCPs = map[int]bool{}
	pg := model.PageSetup{Width: 612, Height: 792, MarginLeft: 90, MarginRight: 90, MarginTop: 72, MarginBottom: 72, Header: 36, Footer: 36}
	if len(plc) < 4+12 {
		r.doc.Page = pg
		return
	}
	n := (len(plc) - 4) / 16
	for i := 1; i < n; i++ {
		r.sedCPs[int(u32(plc[4*i:]))] = true
	}
	if n > 1 {
		r.warn(fmt.Sprintf("The document has %d sections; WordLite keeps the last section's page setup and turns section breaks into page breaks.", n))
	}
	sed := plc[4*(n+1)+12*(n-1):]
	fc := u32(sed[2:])
	if fc != 0xFFFFFFFF && int(fc)+2 <= len(r.wd) {
		cb := int(u16(r.wd[fc:]))
		if int(fc)+2+cb <= len(r.wd) {
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
				}
			})
		}
	}
	r.doc.Page = pg
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// readNotes collects footnote and endnote reference positions.
func (r *reader) readNotes() {
	r.footnoteRefs = plcPositions(r.fib.blob(r.table, fcPlcffndRef), 2)
	r.endnoteRefs = plcPositions(r.fib.blob(r.table, fcPlcfendRef), 2)
}

// plcPositions numbers the CPs of a PLC with data elements of size cb.
func plcPositions(plc []byte, cb int) map[int]int {
	out := map[int]int{}
	if len(plc) < 4 {
		return out
	}
	n := (len(plc) - 4) / (4 + cb)
	for i := 0; i < n; i++ {
		out[int(u32(plc[4*i:]))] = i + 1
	}
	return out
}

// subdocWarnings reports stories WordLite doesn't show.
func (r *reader) subdocWarnings() {
	lw := r.fib.lw
	if lw[lwCcpFtn] > 0 {
		r.warn("Footnotes are not shown and are not kept when saving.")
	}
	if lw[lwCcpEdn] > 0 {
		r.warn("Endnotes are not shown and are not kept when saving.")
	}
	if lw[lwCcpAtn] > 0 {
		r.warn("Comments are not shown and are not kept when saving.")
	}
	if lw[lwCcpTxbx] > 0 || lw[lwCcpHdrTxbx] > 0 {
		r.warn("Text boxes are not shown and are not kept when saving.")
	}
	if lw[lwCcpHdd] > 0 && r.headerHasText() {
		r.warn("Headers and footers are not shown and are not kept when saving.")
	}
}

// headerHasText reports whether the header story holds anything besides
// paragraph marks (Word always stores separators there).
func (r *reader) headerHasText() bool {
	lw := r.fib.lw
	start := int(lw[lwCcpText] + lw[lwCcpFtn])
	end := start + int(lw[lwCcpHdd])
	plcHdd := r.fib.blob(r.table, fcPlcfHdd)
	// The first six stories are footnote/endnote separators.
	if len(plcHdd) >= 4*8 {
		start += int(u32(plcHdd[4*6:]))
	}
	for cp := start; cp < end; cp++ {
		c, ok := r.charAt(cp)
		if !ok {
			return false
		}
		if c != 0x0D && c != 0x07 && c != 0x03 && c != 0x04 {
			return true
		}
	}
	return false
}

// charAt reads any CP of the document (including subdocuments).
func (r *reader) charAt(cp int) (uint16, bool) {
	for _, p := range r.pieces {
		if cp >= p.cp && cp < p.cpEnd {
			if p.compressed {
				fc := int(p.fc) + cp - p.cp
				if fc < len(r.wd) {
					return cp1252(r.wd[fc]), true
				}
			} else {
				fc := int(p.fc) + 2*(cp-p.cp)
				if fc+2 <= len(r.wd) {
					return u16(r.wd[fc:]), true
				}
			}
			return 0, false
		}
	}
	return 0, false
}
