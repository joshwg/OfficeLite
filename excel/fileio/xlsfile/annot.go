// Notes (NOTE records with the text in the OBJ/TXO pair of their drawing
// object) and hyperlinks (HLINK, HLINKTOOLTIP), read and written, plus the
// OfficeArt drawing that holds note text boxes and AutoFilter buttons.

package xlsfile

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
	"unicode/utf16"

	"officelite/common/textenc"
	"officelite/excel/model"
)

const (
	recNote         = 0x001C
	recObj          = 0x005D
	recTxo          = 0x01B6
	recHLink        = 0x01B8
	recHLinkTooltip = 0x0800

	otNote     = 0x19 // OBJ type of a note's text box
	otDropDown = 0x14 // OBJ type of a drop-down (an AutoFilter button)
)

var (
	clsidStdLink     = []byte{0xD0, 0xC9, 0xEA, 0x79, 0xF9, 0xBA, 0xCE, 0x11, 0x8C, 0x82, 0x00, 0xAA, 0x00, 0x4B, 0xA9, 0x0B}
	clsidURLMoniker  = []byte{0xE0, 0xC9, 0xEA, 0x79, 0xF9, 0xBA, 0xCE, 0x11, 0x8C, 0x82, 0x00, 0xAA, 0x00, 0x4B, 0xA9, 0x0B}
	clsidFileMoniker = []byte{0x03, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}
)

// HLINK flags (MS-OSHARED 2.3.7.1).
const (
	hlHasMoniker     = 0x001
	hlIsAbsolute     = 0x002
	hlSiteGaveName   = 0x004
	hlHasLocation    = 0x008
	hlHasDisplayName = 0x010
	hlHasGUID        = 0x020
	hlHasTime        = 0x040
	hlHasFrame       = 0x080
	hlMonikerString  = 0x100
)

// annotReader collects a sheet's notes and links while its records are
// read.
type annotReader struct {
	lastObj int
	texts   map[int]string // note text by object id

	txo      bool // reading the CONTINUE records of a TXO
	txoObj   int
	txoCch   int // the text's length in UTF-16 code units
	txoGot   int // the code units read so far (a rune beyond the BMP is two)
	txoText  []rune
	notes    []noteRec
	links    []linkRec
	tooltips map[model.Range]string
}

type noteRec struct {
	row, col, obj int
	author        string
}

type linkRec struct {
	rng model.Range
	h   model.Hyperlink
}

// record handles a record that belongs to notes or links and reports
// whether it did.
func (a *annotReader) record(r record) bool {
	b := r.body
	if a.txo {
		if r.id == recContinue {
			if a.txoGot < a.txoCch && len(b) > 0 {
				s, _ := chars(b, 1, a.txoCch-a.txoGot, b[0]&1 != 0)
				for _, r := range s {
					a.txoText = append(a.txoText, r)
					a.txoGot += utf16.RuneLen(r)
				}
			}
			return true // the text, then its formatting runs
		}
		a.txo = false
		if a.texts == nil {
			a.texts = map[int]string{}
		}
		a.texts[a.txoObj] = strings.ReplaceAll(string(a.txoText), "\r\n", "\n") // a lone CR is text
	}
	switch r.id {
	case recObj:
		if len(b) >= 8 && u16(b, 0) == 0x15 {
			a.lastObj = u16(b, 6)
			if u16(b, 4) != otNote {
				a.lastObj = -1
			}
		}
		return true
	case recTxo:
		if len(b) >= 12 && a.lastObj >= 0 {
			a.txo, a.txoObj, a.txoCch, a.txoGot, a.txoText = true, a.lastObj, u16(b, 10), 0, nil
		}
		return true
	case recNote:
		if len(b) >= 11 {
			author, _ := unicodeString(b, 8, 2)
			a.notes = append(a.notes, noteRec{row: u16(b, 0), col: u16(b, 2), obj: u16(b, 6), author: author})
		}
		return true
	case recHLink:
		if l, ok := parseHLink(b); ok {
			a.links = append(a.links, l)
		}
		return true
	case recHLinkTooltip:
		if len(b) >= 10 {
			rng := model.Range{R1: u16(b, 2), R2: u16(b, 4), C1: u16(b, 6), C2: u16(b, 8)}
			if a.tooltips == nil {
				a.tooltips = map[model.Range]string{}
			}
			a.tooltips[rng] = strings.TrimRight(textenc.UTF16LE(b[10:len(b)&^1]), "\x00")
		}
		return true
	}
	return false
}

// maxLinkCells bounds how many cells one HLINK range fills.
const maxLinkCells = 10000

// apply stores what was read in sh.
func (a *annotReader) apply(sh *model.Sheet) {
	if a.txo {
		a.record(record{})
	}
	for _, n := range a.notes {
		if n.row >= model.MaxRows || n.col >= model.MaxCols {
			continue
		}
		sh.SetNote(n.row, n.col, &model.Note{Author: n.author, Text: a.texts[n.obj]})
	}
	for _, l := range a.links {
		if tip, ok := a.tooltips[l.rng]; ok {
			l.h.Tooltip = tip
		}
		n := 0 // bounded as in the .xlsx reader: a damaged range can span the sheet
		for r := l.rng.R1; r <= min(l.rng.R2, model.MaxRows-1) && n < maxLinkCells; r++ {
			for c := l.rng.C1; c <= min(l.rng.C2, model.MaxCols-1) && n < maxLinkCells; c++ {
				sh.SetLink(r, c, &l.h)
				n++
			}
		}
	}
}

// hcursor reads an HLINK body, failing softly past its end.
type hcursor struct {
	b   []byte
	off int
	bad bool
}

// take returns the next n bytes, or nil (marking the cursor bad) when
// they run past the end.
func (c *hcursor) take(n int) []byte {
	if c.bad || n < 0 || c.off+n > len(c.b) {
		c.bad = true
		return nil
	}
	s := c.b[c.off : c.off+n]
	c.off += n
	return s
}

// u32 reads a little-endian uint32, 0 past the end.
func (c *hcursor) u32() int {
	s := c.take(4)
	if s == nil {
		return 0
	}
	return int(binary.LittleEndian.Uint32(s))
}

// u16 reads a little-endian uint16, 0 past the end.
func (c *hcursor) u16() int {
	s := c.take(2)
	if s == nil {
		return 0
	}
	return int(binary.LittleEndian.Uint16(s))
}

// hstring reads a HyperlinkString: a character count (with the
// terminating null) and UTF-16 characters.
func (c *hcursor) hstring() string {
	n := c.u32()
	if n > 1<<16 {
		c.bad = true
		return ""
	}
	return strings.TrimRight(textenc.UTF16LE(c.take(2*n)), "\x00")
}

// parseHLink decodes an HLINK body into its cell range and target or
// location; ok is false when it is damaged or links nowhere.
func parseHLink(b []byte) (linkRec, bool) {
	c := &hcursor{b: b}
	var l linkRec
	l.rng.R1, l.rng.R2, l.rng.C1, l.rng.C2 = c.u16(), c.u16(), c.u16(), c.u16()
	c.take(16) // hlinkClsid
	c.u32()    // streamVersion
	flags := c.u32()
	if flags&hlHasDisplayName != 0 {
		c.hstring()
	}
	if flags&hlHasFrame != 0 {
		c.hstring()
	}
	if flags&hlHasMoniker != 0 {
		if flags&hlMonikerString != 0 {
			l.h.Target = c.hstring()
		} else {
			l.h.Target = c.moniker()
		}
	}
	if flags&hlHasLocation != 0 {
		l.h.Location = c.hstring()
	}
	if c.bad || l.h.Target == "" && l.h.Location == "" {
		return l, false
	}
	return l, true
}

// moniker reads a URL or file moniker's address.
func (c *hcursor) moniker() string {
	clsid := c.take(16)
	switch {
	case bytes.Equal(clsid, clsidURLMoniker):
		n := c.u32()
		s := textenc.UTF16LE(c.take(n &^ 1))
		if i := strings.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		return s
	case bytes.Equal(clsid, clsidFileMoniker):
		anti := c.u16()
		ansi := c.take(c.u32())
		path := strings.TrimRight(textenc.Latin1(ansi), "\x00")
		c.u16()    // endServer
		c.u16()    // versionNumber
		c.take(20) // reserved
		if size := c.u32(); size > 0 {
			n := c.u32()
			c.u16() // usKeyValue
			path = textenc.UTF16LE(c.take(n))
		}
		return strings.Repeat(`..\`, anti) + path
	}
	c.bad = true
	return ""
}

// utf16z is s as UTF-16LE with a terminating null.
func utf16z(s string) []byte { return append(textenc.AppendUTF16LE(nil, s), 0, 0) }

// appendHString appends s as a HyperlinkString: its character count
// (with the null) and null-terminated UTF-16 text.
func appendHString(b []byte, s string) []byte {
	b = binary.LittleEndian.AppendUint32(b, uint32(len(utf16.Encode([]rune(s)))+1))
	return append(b, utf16z(s)...)
}

// isURL reports whether a target is written as a URL moniker rather than
// a file path.
func isURL(s string) bool {
	if i := strings.Index(s, ":"); i > 1 {
		scheme := strings.ToLower(s[:i])
		return strings.HasPrefix(s[i:], "://") || scheme == "mailto" || scheme == "news" || scheme == "tel"
	}
	return false
}

// hlinkRecord encodes one cell's hyperlink as HLINK (plus HLINKTOOLTIP).
func hlinkRecord(b []byte, r, c int, h model.Hyperlink) []byte {
	body := le(r, r, c, c)
	body = append(body, clsidStdLink...)
	body = binary.LittleEndian.AppendUint32(body, 2)
	flags := 0
	if h.Target != "" {
		flags |= hlHasMoniker
		if isURL(h.Target) || strings.Contains(h.Target, `:\`) || strings.HasPrefix(h.Target, `\\`) {
			flags |= hlIsAbsolute
		}
	}
	if h.Location != "" {
		flags |= hlHasLocation
	}
	body = binary.LittleEndian.AppendUint32(body, uint32(flags))
	if h.Target != "" {
		if isURL(h.Target) {
			body = append(body, clsidURLMoniker...)
			s := utf16z(h.Target)
			body = binary.LittleEndian.AppendUint32(body, uint32(len(s)))
			body = append(body, s...)
		} else {
			path := h.Target
			anti := 0
			for strings.HasPrefix(path, `..\`) {
				anti++
				path = path[3:]
			}
			body = append(body, clsidFileMoniker...)
			body = binary.LittleEndian.AppendUint16(body, uint16(anti))
			ansi := latin1Bytes(path)
			body = binary.LittleEndian.AppendUint32(body, uint32(len(ansi)+1))
			body = append(append(body, ansi...), 0)
			body = append(body, le(0xFFFF, 0xDEAD)...)
			body = append(body, make([]byte, 20)...)
			if isLatin1(path) {
				body = binary.LittleEndian.AppendUint32(body, 0)
			} else {
				u := utf16z(path)
				u = u[:len(u)-2]
				body = binary.LittleEndian.AppendUint32(body, uint32(len(u)+6))
				body = binary.LittleEndian.AppendUint32(body, uint32(len(u)))
				body = binary.LittleEndian.AppendUint16(body, 3)
				body = append(body, u...)
			}
		}
	}
	if h.Location != "" {
		body = appendHString(body, h.Location)
	}
	b = rec(b, recHLink, body)
	if h.Tooltip != "" {
		tip := le(recHLinkTooltip, r, r, c, c)
		tip = append(tip, utf16z(truncUTF16(h.Tooltip, 255))...)
		b = rec(b, recHLinkTooltip, tip)
	}
	return b
}

// annotations writes the sheet's hyperlinks.
func (x *writer) annotations(b []byte, sh *model.Sheet) []byte {
	keys := make([][2]int, 0, len(sh.Links))
	for k := range sh.Links {
		if k[0] < xlsMaxRows && k[1] < xlsMaxCols {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	for _, k := range keys {
		b = hlinkRecord(b, k[0], k[1], sh.Links[k])
	}
	return b
}

// latin1Bytes encodes s in Latin-1, other characters as '?'.
func latin1Bytes(s string) []byte {
	var b []byte
	for _, r := range s {
		if r > 0xFF {
			r = '?'
		}
		b = append(b, byte(r))
	}
	return b
}

// Writing notes: each note is a hidden text box in the sheet's OfficeArt
// drawing (MSODRAWING records holding one DgContainer), followed by its
// OBJ and TXO records, then a NOTE record per note; the workbook globals
// carry the drawing group (MSODRAWINGGROUP). The layout follows what
// Excel 2003 writes.

const (
	recMsoDrawingGroup = 0x00EB
	recMsoDrawing      = 0x00EC
)

// noteFOPT is the shape properties of a note's text box as Excel writes
// them (infoBackground fill, shadow, hidden), with lTxid first.
var noteFOPT = []byte{
	0x80, 0x00, 0x00, 0x00, 0x00, 0x00, // lTxid (patched)
	0x85, 0x00, 0x02, 0x00, 0x00, 0x00,
	0x87, 0x00, 0x01, 0x00, 0x00, 0x00,
	0xBF, 0x00, 0x08, 0x00, 0x08, 0x00,
	0x58, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x81, 0x01, 0x50, 0x00, 0x00, 0x08,
	0x83, 0x01, 0x50, 0x00, 0x00, 0x08,
	0xBF, 0x01, 0x10, 0x00, 0x10, 0x00,
	0xCB, 0x01, 0x90, 0x24, 0x00, 0x00,
	0xD0, 0x01, 0x01, 0x00, 0x00, 0x00,
	0xFF, 0x01, 0x18, 0x00, 0x18, 0x00,
	0x3F, 0x02, 0x03, 0x00, 0x03, 0x00,
	0xBF, 0x03, 0x02, 0x00, 0x0A, 0x00,
}

// officeArt appends an OfficeArt record header and data.
func officeArt(b []byte, verInst, typ int, length int, data ...byte) []byte {
	b = binary.LittleEndian.AppendUint16(b, uint16(verInst))
	b = binary.LittleEndian.AppendUint16(b, uint16(typ))
	b = binary.LittleEndian.AppendUint32(b, uint32(length))
	return append(b, data...)
}

// noteKeys are the cells of sh's notes that .xls can hold, in order.
func noteKeys(sh *model.Sheet) [][2]int {
	keys := make([][2]int, 0, len(sh.Notes))
	for k := range sh.Notes {
		if k[0] < xlsMaxRows && k[1] < xlsMaxCols {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	return keys
}

// dropDowns are the header columns of sh's AutoFilter that get a
// drop-down button object.
func dropDowns(sh *model.Sheet) []int {
	if !filterFits(sh) {
		return nil
	}
	f := sh.AutoFilter
	var cols []int
	for c := f.Range.C1; c <= f.Range.C2; c++ {
		if fc, ok := f.Column(c); !ok || !fc.HideButton {
			cols = append(cols, c)
		}
	}
	return cols
}

// noteDrawings lists, for each sheet with notes or AutoFilter buttons,
// its shape count; a sheet's drawing id is its 1-based position in this
// list.
func (x *writer) noteDrawings() (dg map[int]int, counts []int) {
	dg = map[int]int{}
	for si, sh := range x.sheets {
		if n := len(noteKeys(sh)) + len(dropDowns(sh)); n > 0 {
			counts = append(counts, n)
			dg[si] = len(counts)
		}
	}
	return dg, counts
}

// drawingGroup writes MSODRAWINGGROUP when some sheet has notes or
// AutoFilter buttons.
func (x *writer) drawingGroup(b []byte) []byte {
	_, counts := x.noteDrawings()
	if len(counts) == 0 {
		return b
	}
	shapes := 0
	var fidcl []byte
	for i, n := range counts {
		fidcl = append(fidcl, le32(uint32(i+1), uint32(n+1))...)
		shapes += n + 1
	}
	fdgg := le32(uint32((len(counts)+1)*0x400+1), uint32(len(counts)+1), uint32(shapes), uint32(len(counts)))
	fdgg = append(fdgg, fidcl...)
	opt := []byte{0xBF, 0x00, 0x08, 0x00, 0x08, 0x00, 0x81, 0x01, 0x09, 0x00, 0x00, 0x08, 0xC0, 0x01, 0x40, 0x00, 0x00, 0x08}
	split := []byte{0x0D, 0x00, 0x00, 0x08, 0x0C, 0x00, 0x00, 0x08, 0x17, 0x00, 0x00, 0x08, 0xF7, 0x00, 0x00, 0x10}
	var body []byte
	inner := officeArt(nil, 0, 0xF006, len(fdgg), fdgg...)
	inner = officeArt(inner, 0x0033, 0xF00B, len(opt), opt...)
	inner = officeArt(inner, 0x0040, 0xF11E, len(split), split...)
	body = officeArt(body, 0x000F, 0xF000, len(inner), inner...)
	return rec(b, recMsoDrawingGroup, body)
}

// notes writes the drawing objects and NOTE records of sh's notes, and
// the drop-down buttons of its AutoFilter.
func (x *writer) notes(b []byte, si int, sh *model.Sheet) []byte {
	keys := noteKeys(sh)
	drops := dropDowns(sh)
	if len(keys) == 0 && len(drops) == 0 {
		return b
	}
	dgs, _ := x.noteDrawings()
	dg := dgs[si]
	k, nd := len(keys), len(drops)
	const groupLen, noteLen, dropLen = 48, 152, 96 // SpContainers with headers
	spgr := groupLen + k*noteLen + nd*dropLen
	base := dg * 0x400

	var first []byte
	first = officeArt(first, 0x000F, 0xF002, 16+8+spgr)                                    // DgContainer
	first = officeArt(first, dg<<4, 0xF008, 8, le32(uint32(k+nd+1), uint32(base+k+nd))...) // FDG
	first = officeArt(first, 0x000F, 0xF003, spgr)                                         // SpgrContainer
	first = officeArt(first, 0x000F, 0xF004, groupLen-8)                                   // group shape
	first = officeArt(first, 0x0001, 0xF009, 16, make([]byte, 16)...)                      // FSPGR
	first = officeArt(first, 0x0002, 0xF00A, 8, le32(uint32(base), 0x05)...)               // FSP: group, patriarch
	for i, key := range keys {
		var d []byte
		if i == 0 {
			d = first
		}
		row, col := key[0], key[1]
		opt := slices.Clone(noteFOPT)
		binary.LittleEndian.PutUint32(opt[2:], uint32(base+i+1)<<16)
		r1 := max(row-1, 0)
		anchor := le(2, min(col+1, 255), 15, r1, 10, min(col+3, 255), 15, min(r1+4, xlsMaxRows-1), 10)
		d = officeArt(d, 0x000F, 0xF004, noteLen-8)
		d = officeArt(d, 0x0CA2, 0xF00A, 8, le32(uint32(base+i+1), 0x0A00)...) // FSP: text box, anchored
		d = officeArt(d, 0x00D3, 0xF00B, len(opt), opt...)
		d = officeArt(d, 0x0000, 0xF010, len(anchor), anchor...)
		d = officeArt(d, 0x0000, 0xF011, 0)
		b = rec(b, recMsoDrawing, d)

		obj := le(0x15, 0x12, otNote, i+1, 0)
		obj = append(obj, make([]byte, 12)...)
		obj = append(obj, le(0x0D, 0x16)...)
		obj = append(obj, make([]byte, 22)...)
		obj = append(obj, le(0, 0)...)
		b = rec(b, recObj, obj)
		b = rec(b, recMsoDrawing, officeArt(nil, 0, 0xF00D, 0))

		text := []rune(truncUTF16(strings.ReplaceAll(sh.Notes[key].Text, "\r\n", "\n"), maxText))
		units := utf16.Encode(text)
		b = rec(b, recTxo, le(0x0012, 0, 0, 0, 0, len(units), 16, 0, 0))
		if len(units) == 0 {
			continue
		}
		wide := !isLatin1(string(text))
		per := maxRecord - 1
		if wide {
			per /= 2
		}
		for start := 0; start < len(units); start += per {
			chunk := units[start:min(start+per, len(units))]
			body := []byte{0}
			if wide {
				body[0] = 1
				for _, u := range chunk {
					body = binary.LittleEndian.AppendUint16(body, u)
				}
			} else {
				for _, u := range chunk {
					body = append(body, byte(u))
				}
			}
			b = rec(b, recContinue, body)
		}
		b = rec(b, recContinue, le(0, 0, 0, 0, len(units), 0, 0, 0))
	}
	for j, col := range drops {
		var d []byte
		if k == 0 && j == 0 {
			d = first
		}
		b = x.dropDown(b, d, base+k+j+1, k+j+1, sh.AutoFilter.Range.R1, col, sh)
	}
	for i, key := range keys {
		author := truncUTF16(sh.Notes[key].Author, 54)
		b = rec(b, recNote, append(appendString(le(key[0], key[1], 0, i+1), author), 0))
	}
	return b
}

// dropDownFOPT is the shape properties of an AutoFilter button as Excel
// writes them: locked against grouping, fit text, no fill hit test, no
// line, not printed.
var dropDownFOPT = []byte{
	0x7F, 0x00, 0x04, 0x01, 0x04, 0x01,
	0xBF, 0x00, 0x08, 0x00, 0x08, 0x00,
	0xBF, 0x01, 0x00, 0x00, 0x01, 0x00,
	0xFF, 0x01, 0x00, 0x00, 0x08, 0x00,
	0xBF, 0x03, 0x00, 0x00, 0x0A, 0x00,
}

// dropDown writes the drop-down button of an AutoFilter header cell: a
// host-control shape over the cell and its OBJ record (a combo box marked
// as an AutoFilter's, filtered when the column has criteria). d holds
// the drawing's start when this is its first shape.
func (x *writer) dropDown(b, d []byte, spid, objID, row, col int, sh *model.Sheet) []byte {
	anchor := le(1, col, 0, row, 0, min(col+1, 255), 0, min(row+1, xlsMaxRows-1), 0)
	d = officeArt(d, 0x000F, 0xF004, 96-8)
	d = officeArt(d, 0x0C92, 0xF00A, 8, le32(uint32(spid), 0x0A00)...) // FSP: host control, anchored
	d = officeArt(d, 0x0053, 0xF00B, len(dropDownFOPT), dropDownFOPT...)
	d = officeArt(d, 0x0000, 0xF010, len(anchor), anchor...)
	d = officeArt(d, 0x0000, 0xF011, 0)
	b = rec(b, recMsoDrawing, d)

	style := 2 // simple drop-down
	if _, ok := sh.AutoFilter.Column(col); ok {
		style |= 0x0008 // filtered: Excel shows the funnel
	}
	obj := le(0x15, 0x12, otDropDown, objID, 0x2101)
	obj = append(obj, make([]byte, 12)...)
	obj = append(obj, le(0x0C, 0x14, 0, 0, 0, 0, 0x64, 1, 0x0A, 0, 0x10, 1)...) // ftSbs
	obj = append(obj, le(0x13, 0x1FEE, 0, 0, 0, 0x0301, 0, style, 20, 130)...)  // ftLbsData: AutoFilter list
	obj = append(obj, le(0, 0)...)
	return rec(b, recObj, obj)
}
