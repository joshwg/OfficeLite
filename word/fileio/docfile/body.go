package docfile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"officelite/word/model"
)

// opaqueBlob is the Data of a RunOpaque read from a .doc: the characters
// with their character formatting, so the writer can put them back.
type opaqueBlob struct {
	Kind  string      `json:"k"` // "field", "picture", "shape", "note", "comment"
	Segs  []opaqueSeg `json:"s,omitempty"`
	Flds  [][2]byte   `json:"d,omitempty"` // Fld of each 0x13/0x14/0x15, in order
	Fonts []string    `json:"f,omitempty"` // fonts referenced by Segs[].Chpx
}

// opaqueSeg is a stretch of characters with one CHPX. Font sprms index
// Fonts; sprmCIstd and sprmCPicLocation are removed (Pic carries the
// picture of a 0x01 character).
type opaqueSeg struct {
	Text []uint16 `json:"t"`
	Chpx []byte   `json:"c,omitempty"`
	Pic  []byte   `json:"p,omitempty"`
}

const opaqueFormat = "doc"

// readFields finds field begin/separator/end characters and decides which
// fields are kept as one opaque run and which (spanning paragraphs) are
// flattened into their result text.
func (r *reader) readFields() {
	r.skip = make([]bool, len(r.text))
	r.unitEnd = map[int]int{}
	r.fldAt = map[int][2]byte{}
	if plc := r.fib.blob(r.table, fcPlcfFldMom); len(plc) >= 4 {
		n := (len(plc) - 4) / 6
		for i := 0; i < n; i++ {
			cp := int(u32(plc[4*i:]))
			d := plc[4*(n+1)+2*i:]
			r.fldAt[cp] = [2]byte{d[0], d[1]}
		}
	}
	type open struct{ b, s int }
	type fld struct{ b, s, e, depth int }
	var stack []open
	var all []fld
	for cp, c := range r.text {
		switch c {
		case 0x13:
			stack = append(stack, open{cp, -1})
		case 0x14:
			if len(stack) == 0 {
				r.skip[cp] = true
			} else if stack[len(stack)-1].s < 0 {
				stack[len(stack)-1].s = cp
			}
		case 0x15:
			if len(stack) == 0 {
				r.skip[cp] = true
				continue
			}
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			all = append(all, fld{o.b, o.s, cp, len(stack)})
		}
	}
	for _, o := range stack {
		r.skip[o.b] = true
		if o.s >= 0 {
			r.skip[o.s] = true
		}
	}
	flattened := 0
	var flat []fld
	for _, f := range all {
		if f.depth != 0 {
			continue
		}
		unit := true
		for cp := f.b; cp <= f.e; cp++ {
			if c := r.text[cp]; c == 0x0D || c == 0x07 || (c == 0x0C && r.sedCPs[cp+1]) {
				unit = false
				break
			}
		}
		if unit {
			r.unitEnd[f.b] = f.e
		} else {
			flattened++
			flat = append(flat, f)
		}
	}
	// Flatten: hide codes of the field and of every field nested in it.
	for _, f := range all {
		inFlat := false
		for _, t := range flat {
			if f.b >= t.b && f.e <= t.e {
				inFlat = true
			}
		}
		if !inFlat {
			continue
		}
		end := f.s
		if end < 0 {
			end = f.e
		}
		for cp := f.b; cp <= end; cp++ {
			if c := r.text[cp]; c != 0x0D && c != 0x07 {
				r.skip[cp] = true
			}
		}
		r.skip[f.e] = true
	}
	if flattened > 0 {
		r.warn(fmt.Sprintf("%d field(s) spanning several paragraphs (such as a table of contents) were converted to plain text.", flattened))
	}
}

// chpAt returns the decoded character formatting of the character at cp.
func (r *reader) chpAt(cp int) *chpInfo {
	ri := findRun(r.chpx, r.cpFc[cp])
	pi := int(r.cpPiece[cp])
	key := [2]int{ri, pi}
	if c, ok := r.chpCache[key]; ok {
		return c
	}
	c := &chpInfo{istd: -1}
	var g []byte
	if ri >= 0 {
		g = append(g, r.chpx[ri].grpprl...)
	}
	g = append(g, r.pieceSprms(pi)...)
	c.grpprl = g
	r.decodeChp(c, g)
	r.chpCache[key] = c
	return c
}

// papAt returns the paragraph properties of the paragraph whose mark is at cp.
func (r *reader) papAt(cp int) (istd int, g []byte) {
	if ri := findRun(r.papx, r.cpFc[cp]); ri >= 0 {
		istd = int(r.papx[ri].istd)
		g = append(g, r.papx[ri].grpprl...)
	}
	return istd, append(g, r.pieceSprms(int(r.cpPiece[cp]))...)
}

// runProps resolves a character's formatting for a paragraph style.
func (r *reader) runProps(c *chpInfo, paraStyle string) (model.RunProps, string) {
	cs := r.styleID(c.istd)
	if c.istd == 10 || (c.istd >= 0 && c.istd < len(r.styles) && r.styles[c.istd] != nil && r.styles[c.istd].sti == 65) {
		cs = ""
	}
	if s := r.doc.Styles.Find(cs); s == nil || s.Type != model.CharacterStyle {
		cs = ""
	}
	base := r.doc.ResolveRun(&model.Paragraph{Style: paraStyle}, &model.Run{Style: cs})
	return resolveToggles(c, base), cs
}

// tableState collects table rows while walking paragraphs.
type tableState struct {
	table *model.Table
	cells []*model.TableCell
	cell  *model.TableCell
	rows  []rowTap
}

type rowTap struct {
	row *model.TableRow
	tap []byte
}

// buildBody walks the main text paragraph by paragraph.
func (r *reader) buildBody() {
	var ts *tableState
	flush := func() {
		if ts == nil {
			return
		}
		if ts.cell != nil && len(ts.cell.Blocks) > 0 {
			ts.cells = append(ts.cells, ts.cell)
		}
		if len(ts.cells) > 0 {
			ts.rows = append(ts.rows, rowTap{&model.TableRow{Cells: ts.cells}, nil})
		}
		r.finishTable(ts)
		r.doc.Body = append(r.doc.Body, ts.table)
		ts = nil
	}
	start := 0
	nested := false
	for cp := 0; cp < len(r.text); cp++ {
		c := r.text[cp]
		section := c == 0x0C && r.sedCPs[cp+1] && cp+1 < len(r.text)
		if c != 0x0D && c != 0x07 && !section {
			continue
		}
		istd, g := r.papAt(cp)
		var pi papInfo
		r.decodePap(&pi, g)
		itap := 0
		if pi.inTable {
			itap = 1
		}
		if pi.hasItap {
			itap = pi.itap
		}
		para := r.paragraph(start, cp, istd, &pi)
		if section {
			para.Runs = append(para.Runs, model.Run{Kind: model.RunPageBreak})
		}
		start = cp + 1
		if itap <= 0 {
			flush()
			r.doc.Body = append(r.doc.Body, para)
			continue
		}
		if ts == nil {
			ts = &tableState{table: &model.Table{}}
		}
		if ts.cell == nil {
			ts.cell = &model.TableCell{Span: 1}
		}
		if itap > 1 {
			if !nested {
				nested = true
				r.warn("Nested tables are shown as plain paragraphs inside the outer table's cells.")
			}
			if pi.innerTtp {
				continue
			}
			ts.cell.Blocks = append(ts.cell.Blocks, para)
			continue
		}
		if pi.ttp {
			if ts.cell != nil && len(ts.cell.Blocks) > 0 {
				ts.cells = append(ts.cells, ts.cell)
			}
			ts.rows = append(ts.rows, rowTap{&model.TableRow{Cells: ts.cells}, g})
			ts.cells, ts.cell = nil, nil
			continue
		}
		ts.cell.Blocks = append(ts.cell.Blocks, para)
		if c == 0x07 {
			ts.cells = append(ts.cells, ts.cell)
			ts.cell = nil
		}
	}
	flush()
	if len(r.doc.Body) == 0 {
		r.doc.Body = []model.Block{&model.Paragraph{}}
	}
}

// paragraph builds the paragraph with text [start, mark) and mark at mark.
func (r *reader) paragraph(start, mark, istd int, pi *papInfo) *model.Paragraph {
	p := &model.Paragraph{Style: r.styleID(istd), Props: pi.props}
	if s := r.doc.Styles.Find(p.Style); s == nil || s.Type != model.ParagraphStyle || s.Default {
		p.Style = ""
	}
	p.Props.List = pi.listRef()
	if p.Props.List != nil && p.Props.List.NumID != 0 && r.doc.Lists[p.Props.List.NumID] == nil {
		p.Props.List = nil
	}
	mc := r.chpAt(mark)
	p.Mark, _ = r.runProps(mc, p.Style)
	p.Runs = r.runs(start, mark, p.Style)
	return p
}

// runs builds the runs of CPs [start, end).
func (r *reader) runs(start, end int, paraStyle string) []model.Run {
	var out []model.Run
	var buf []uint16
	var cur model.Run
	flushText := func() {
		if len(buf) > 0 {
			cur.Kind = model.RunText
			cur.Text = string(utf16.Decode(buf))
			out = append(out, cur)
			buf = buf[:0]
		}
	}
	add := func(run model.Run) {
		flushText()
		out = append(out, run)
	}
	for cp := start; cp < end; cp++ {
		if r.skip[cp] {
			continue
		}
		if e, ok := r.unitEnd[cp]; ok && e < end {
			add(r.fieldRun(cp, e, paraStyle))
			cp = e
			continue
		}
		c := r.text[cp]
		chp := r.chpAt(cp)
		if chp.rmDel {
			r.warn("Tracked changes are shown accepted (deleted text is hidden).")
			continue
		}
		props, cs := r.runProps(chp, paraStyle)
		run := model.Run{Props: props, Style: cs}
		switch {
		case c == 0x09:
			run.Kind = model.RunTab
			add(run)
		case c == 0x0B || c == 0x0E:
			run.Kind = model.RunBreak
			add(run)
		case c == 0x0C:
			run.Kind = model.RunPageBreak
			add(run)
		case c == 0x01 && chp.spec:
			if chp.fData {
				continue
			}
			if o := r.pictureRun(cp, cp, run); o != nil {
				add(*o)
			}
		case c == 0x08 && chp.spec:
			r.warn("Floating pictures and drawings are shown as placeholders and are not kept when saving.")
			add(r.simpleOpaque(run, "shape", "[shape]", cp))
		case c == 0x02 && chp.spec:
			n, ok := r.footnoteRefs[cp]
			if !ok {
				n = r.endnoteRefs[cp]
			}
			add(r.simpleOpaque(run, "note", strconv.Itoa(n), cp))
		case c == 0x05 && chp.spec:
			continue // comment anchor
		case c < 0x20 && c != 0x1E && c != 0x1F:
			continue
		default:
			switch c {
			case 0x1E:
				c = 0x2011
			case 0x1F:
				c = 0x00AD
			}
			if len(buf) > 0 && (cur.Props != props || cur.Style != cs) {
				flushText()
			}
			if len(buf) == 0 {
				if n := len(out); n > 0 && out[n-1].Kind == model.RunText && out[n-1].Props == props && out[n-1].Style == cs {
					cur = out[n-1]
					out = out[:n-1]
					buf = utf16.Encode([]rune(cur.Text))
				} else {
					cur = run
				}
			}
			buf = append(buf, c)
		}
	}
	flushText()
	return out
}

// segs collects the opaque segments of CPs [b, e].
func (r *reader) segs(b, e int, blob *opaqueBlob) {
	fontIdx := map[string]int{}
	for i, f := range blob.Fonts {
		fontIdx[f] = i
	}
	for cp := b; cp <= e; cp++ {
		chp := r.chpAt(cp)
		c := r.text[cp]
		if c >= 0x13 && c <= 0x15 {
			fld, ok := r.fldAt[cp]
			if !ok {
				fld = [2]byte{byte(c), 0}
				if c == 0x15 {
					fld[1] = 0x40
				}
			}
			blob.Flds = append(blob.Flds, fld)
		}
		chpx := r.portableChpx(chp.grpprl, blob, fontIdx)
		var pic []byte
		if c == 0x01 && chp.spec && chp.hasPic && !chp.fData {
			pic = r.picData(chp.picLoc)
		}
		n := len(blob.Segs)
		if pic == nil && n > 0 && blob.Segs[n-1].Pic == nil && string(blob.Segs[n-1].Chpx) == string(chpx) {
			blob.Segs[n-1].Text = append(blob.Segs[n-1].Text, c)
			continue
		}
		blob.Segs = append(blob.Segs, opaqueSeg{Text: []uint16{c}, Chpx: chpx, Pic: pic})
	}
}

// portableChpx removes file-specific sprms and makes font sprms index
// blob.Fonts.
func (r *reader) portableChpx(g []byte, blob *opaqueBlob, fontIdx map[string]int) []byte {
	var out grpprl
	eachSprm(g, func(op uint16, a []byte) {
		switch op {
		case sprmCIstd, sprmCPicLocation, 0xCA31, 0xCA57, 0xCA89, sprmCFOle2:
			return
		case sprmCRgFtc0, sprmCRgFtc1, sprmCRgFtc2, sprmCFtcBi, sprmCFtcDefault:
			f := r.font(u16(a))
			i, ok := fontIdx[f]
			if !ok {
				i = len(blob.Fonts)
				blob.Fonts = append(blob.Fonts, f)
				fontIdx[f] = i
			}
			out.w(op, uint16(i))
			return
		}
		out.op(op)
		if op>>13 == 6 {
			out = append(out, byte(len(a)))
		}
		out = append(out, a...)
	})
	return out
}

// picData returns the PICFAndOfficeArtData at offset loc of the Data stream.
func (r *reader) picData(loc uint32) []byte {
	if int(loc)+4 > len(r.data) {
		return nil
	}
	lcb := int(u32(r.data[loc:]))
	if lcb < 0x44 || int(loc)+lcb > len(r.data) {
		return nil
	}
	return append([]byte(nil), r.data[loc:int(loc)+lcb]...)
}

// picSize returns a picture's displayed size in points from its PICF.
func picSize(pic []byte) (w, h float64) {
	if len(pic) < 0x44 {
		return 0, 0
	}
	dxa, dya := float64(u16(pic[28:])), float64(u16(pic[30:]))
	mx, my := float64(u16(pic[32:])), float64(u16(pic[34:]))
	if mx == 0 {
		mx = 1000
	}
	if my == 0 {
		my = 1000
	}
	return dxa * mx / 1000 / 20, dya * my / 1000 / 20
}

func (r *reader) opaqueRun(base model.Run, text string, blob *opaqueBlob) model.Run {
	data, _ := json.Marshal(blob)
	base.Kind = model.RunOpaque
	base.Text = text
	base.Opaque = &model.Opaque{Format: opaqueFormat, Data: data}
	for _, s := range blob.Segs {
		if s.Pic != nil {
			base.Opaque.Width, base.Opaque.Height = picSize(s.Pic)
			break
		}
	}
	return base
}

func (r *reader) simpleOpaque(run model.Run, kind, text string, cp int) model.Run {
	blob := &opaqueBlob{Kind: kind}
	r.segs(cp, cp, blob)
	return r.opaqueRun(run, text, blob)
}

// pictureRun makes the opaque run of an inline picture at cp.
func (r *reader) pictureRun(b, e int, run model.Run) *model.Run {
	blob := &opaqueBlob{Kind: "picture"}
	r.segs(b, e, blob)
	if len(blob.Segs) == 0 || blob.Segs[0].Pic == nil {
		r.warn("Some pictures could not be read and are left out.")
		return nil
	}
	o := r.opaqueRun(run, "[picture]", blob)
	return &o
}

// fieldRun makes the opaque run of the field [b, e]; its text is the
// field result.
func (r *reader) fieldRun(b, e int, paraStyle string) model.Run {
	var res []uint16
	depth, inResult := 0, []bool{}
	firstRes := -1
	ole := false
	for cp := b; cp <= e; cp++ {
		c := r.text[cp]
		switch c {
		case 0x13:
			depth++
			inResult = append(inResult, false)
			continue
		case 0x14:
			if depth > 0 {
				inResult[depth-1] = true
			}
			continue
		case 0x15:
			if depth > 0 {
				depth--
				inResult = inResult[:depth]
			}
			continue
		}
		all := depth > 0
		for _, v := range inResult {
			all = all && v
		}
		if !all {
			continue
		}
		if firstRes < 0 {
			firstRes = cp
		}
		if chp := r.chpAt(cp); c == 0x01 && chp.spec && chp.ole2 {
			ole = true
		}
		if c >= 0x20 || c == 0x09 {
			res = append(res, c)
		}
	}
	fmtCp := b
	if firstRes >= 0 {
		fmtCp = firstRes
	}
	props, cs := r.runProps(r.chpAt(fmtCp), paraStyle)
	run := model.Run{Props: props, Style: cs}
	if ole {
		// An embedded object: keep only its picture.
		for cp := b; cp <= e; cp++ {
			if chp := r.chpAt(cp); r.text[cp] == 0x01 && chp.spec && chp.ole2 && chp.hasPic {
				if o := r.pictureRun(cp, cp, run); o != nil {
					r.warn("Embedded objects are shown as their pictures only.")
					return *o
				}
			}
		}
	}
	blob := &opaqueBlob{Kind: "field"}
	r.segs(b, e, blob)
	text := string(utf16.Decode(res))
	o := r.opaqueRun(run, text, blob)
	o.Opaque.Field = r.fieldInstr(b, e)
	return o
}

// fieldInstr returns the instruction of the field [b, e]: its own code
// characters (nested fields contribute their results).
func (r *reader) fieldInstr(b, e int) string {
	var code []uint16
	depth, inCode := 0, []bool{}
	for cp := b; cp <= e; cp++ {
		switch c := r.text[cp]; c {
		case 0x13:
			depth++
			inCode = append(inCode, true)
		case 0x14:
			if depth > 0 {
				inCode[depth-1] = false
			}
		case 0x15:
			if depth > 0 {
				depth--
				inCode = inCode[:depth]
			}
		default:
			// Top-level code, or the result of a field nested in the code.
			if depth >= 1 && inCode[0] && (depth == 1 || !inCode[depth-1]) && (c >= 0x20 || c == 0x09) {
				code = append(code, c)
			}
		}
	}
	return strings.TrimSpace(string(utf16.Decode(code)))
}

// finishTable applies each row's TAP and computes the grid.
func (r *reader) finishTable(ts *tableState) {
	t := ts.table
	type rowGeom struct {
		centers []int
	}
	var geoms []rowGeom
	borders := false
	for ri, rt := range ts.rows {
		row := rt.row
		var centers []int
		var tcs [][]byte
		var shd []string
		eachSprm(rt.tap, func(op uint16, a []byte) {
			switch op {
			case sprmTDefTable:
				if len(a) < 1 {
					return
				}
				n := int(a[0])
				if 1+2*(n+1) > len(a) {
					return
				}
				centers = make([]int, n+1)
				for i := range centers {
					centers[i] = int(i16(a[1+2*i:]))
				}
				rest := a[1+2*(n+1):]
				tcs = nil
				for i := 0; i < n && len(rest) >= 20; i++ {
					tcs = append(tcs, rest[:20])
					rest = rest[20:]
				}
			case sprmTDyaRowHeight:
				row.Height = twips(abs(int(i16(a))))
			case sprmTTableHeader:
				row.Header = u8(a) != 0
			case sprmTJc90:
				t.Align = tableAlign(u16(a))
			case sprmTJc:
				t.Align = tableAlign(uint16(u8(a)))
			case sprmTDefTableShd, sprmTDefTableShd2nd, sprmTDefTableShd3rd,
				sprmTDefTableShdRaw, sprmTDefTableShdRaw2, sprmTDefTableShdRaw3:
				off := 0
				switch op {
				case sprmTDefTableShd2nd, sprmTDefTableShdRaw2:
					off = 22
				case sprmTDefTableShd3rd, sprmTDefTableShdRaw3:
					off = 44
				}
				for i := 0; 10*i+10 <= len(a); i++ {
					for len(shd) <= off+i {
						shd = append(shd, "")
					}
					shd[off+i] = shdColor(a[10*i:])
				}
			case sprmTDefTableShd80:
				if shd == nil {
					for i := 0; 2*i+2 <= len(a); i++ {
						shd = append(shd, shd80Color(u16(a[2*i:])))
					}
				}
			case sprmTTableBorders80:
				for i := 0; i < 6 && 4*i+4 <= len(a); i++ {
					if bt := a[4*i+1]; bt != 0 && bt != 0xFF {
						borders = true
					}
				}
			case sprmTTableBorders:
				for i := 0; i < 6 && 8*i+8 <= len(a); i++ {
					if bt := a[8*i+5]; bt != 0 && bt != 0xFF {
						borders = true
					}
				}
			case sprmTVertMerge:
				if len(a) >= 2 {
					if i := int(a[0]); i < len(row.Cells) {
						row.Cells[i].VMerge = vmerge(a[1])
					}
				}
			case sprmTVertAlign:
				if len(a) >= 3 {
					for i := int(a[0]); i < int(a[1]) && i < len(row.Cells); i++ {
						row.Cells[i].VAlign = int8(a[2])
					}
				}
			}
		})
		if ri == 0 && len(centers) > 0 {
			t.Indent = twips(centers[0]) // the table's left edge
		}
		// Cell geometry and TC flags; horizontally merged cells fold into the first.
		var cells []*model.TableCell
		var merged []int // right boundary of each kept cell
		for i, c := range row.Cells {
			var tc []byte
			if i < len(tcs) {
				tc = tcs[i]
			}
			right := -1
			if i+1 < len(centers) {
				right = centers[i+1]
				c.Width = twips(centers[i+1] - centers[i])
			}
			if i < len(shd) {
				c.Shading = shd[i]
			}
			if tc != nil {
				grf := u16(tc)
				if v := grf >> 5 & 3; v != 0 {
					c.VMerge = vmerge(byte(v))
				}
				if v := int8(grf >> 7 & 3); v != 0 {
					c.VAlign = v
				}
				for k := 0; k < 4; k++ {
					if bt := tc[4+4*k+1]; bt != 0 && bt != 0xFF {
						borders = true
					}
				}
				if h := grf & 3; h >= 2 && len(cells) > 0 {
					prev := cells[len(cells)-1]
					prev.Width += c.Width
					if hasText(c.Blocks) {
						prev.Blocks = append(prev.Blocks, c.Blocks...)
					}
					merged[len(merged)-1] = right
					continue
				}
			}
			cells = append(cells, c)
			merged = append(merged, right)
		}
		row.Cells = cells
		g := rowGeom{}
		if len(centers) > 0 {
			g.centers = append([]int{centers[0]}, merged...)
		}
		geoms = append(geoms, g)
		t.Rows = append(t.Rows, row)
	}
	t.Borders = borders
	// Grid: every distinct cell boundary.
	set := map[int]bool{}
	for _, g := range geoms {
		for _, x := range g.centers {
			set[x] = true
		}
	}
	var grid []int
	for x := range set {
		grid = append(grid, x)
	}
	sort.Ints(grid)
	for i := 1; i < len(grid); i++ {
		t.Columns = append(t.Columns, twips(grid[i]-grid[i-1]))
	}
	for ri, row := range t.Rows {
		g := geoms[ri]
		for ci, c := range row.Cells {
			c.Span = 1
			if ci+1 < len(g.centers) {
				l, rt := g.centers[ci], g.centers[ci+1]
				n := 0
				for _, x := range grid {
					if x > l && x <= rt {
						n++
					}
				}
				if n > 1 {
					c.Span = n
				}
			}
			if len(c.Blocks) == 0 {
				c.Blocks = []model.Block{&model.Paragraph{}}
			}
		}
	}
}

func hasText(blocks []model.Block) bool {
	has := false
	model.WalkBlocks(blocks, func(p *model.Paragraph) bool {
		if len(p.Runs) > 0 {
			has = true
		}
		return !has
	})
	return has
}

func vmerge(v byte) int8 {
	switch v {
	case 3: // first cell of a vertical merge
		return 1
	case 1, 2: // merged with the cell above
		return 2
	}
	return 0
}

func tableAlign(jc uint16) model.Align {
	switch jc {
	case 1:
		return model.AlignCenter
	case 2:
		return model.AlignRight
	}
	return model.AlignUnset
}
