package docfile

import (
	"encoding/binary"
	"fmt"

	"officelite/word/model"
)

// Floating shapes: each 0x08 character of the main text anchors a shape
// whose position and size are its FSPA in PlcSpaMom and whose look is
// the OfficeArt shape with the same id in the drawing of the main
// document (OfficeArtContent at fcDggInfo): its type, fill, outline and,
// for a picture, the BLIP in the drawing group's BStore. A text box's
// text is the story of the text box subdocument whose FTXBXS names the
// shape.

// shape is an OfficeArt shape: its type and properties.
type shape struct {
	typ   int
	flags uint32
	props map[uint16]uint32
	cplx  map[uint16][]byte
}

// record is an OfficeArt record header.
type record struct {
	ver, inst int
	typ       uint16
	data      []byte
}

// records splits b into OfficeArt records.
func records(b []byte) []record {
	var out []record
	for len(b) >= 8 {
		vi, typ, n := binary.LittleEndian.Uint16(b), binary.LittleEndian.Uint16(b[2:]), int(binary.LittleEndian.Uint32(b[4:]))
		if n < 0 || 8+n > len(b) {
			n = len(b) - 8
		}
		out = append(out, record{ver: int(vi & 0xF), inst: int(vi >> 4), typ: typ, data: b[8 : 8+n]})
		b = b[8+n:]
	}
	return out
}

// readShapes reads the floating shapes of the main document.
func (r *reader) readShapes() {
	r.shapes = map[uint32]*shape{}
	r.floats = map[int]*model.Floating{}
	r.txbx = map[uint32][2]int{}
	var blips []*model.Picture
	if dgg := r.fib.blob(r.table, fcDggInfo); len(dgg) > 8 {
		recs := records(dgg[:8+int(u32(dgg[4:]))])
		if len(recs) > 0 && recs[0].typ == 0xF000 {
			blips = r.bstore(recs[0].data)
		}
		rest := dgg[min(8+int(u32(dgg[4:])), len(dgg)):]
		for len(rest) > 9 {
			main := rest[0] == 0
			n := 8 + int(u32(rest[5:]))
			if n > len(rest)-1 {
				break
			}
			if main {
				for _, rec := range records(rest[1 : 1+n]) {
					if rec.typ == 0xF002 {
						r.collectShapes(rec.data)
					}
				}
			}
			rest = rest[1+n:]
		}
	}
	if plc := r.fib.blob(r.table, fcPlcftxbxTxt); len(plc) >= 30 && r.fib.lw[lwCcpTxbx] > 0 {
		n := (len(plc) - 4) / 26
		base := r.storyStart(lwCcpTxbx)
		for i := 0; i < n; i++ {
			ft := plc[4*(n+1)+22*i:]
			if u16(ft[8:]) != 0 { // a reusable (deleted) text box
				continue
			}
			if a, b, ok := r.story(plc[:4*(n+1)], i, base, true); ok {
				r.txbx[u32(ft[14:])] = [2]int{a, b}
			}
		}
	}
	plc := r.fib.blob(r.table, fcPlcSpaMom)
	if len(plc) < 30 {
		return
	}
	n := (len(plc) - 4) / 30
	for i := 0; i < n; i++ {
		cp := int(u32(plc[4*i:]))
		fspa := plc[4*(n+1)+26*i:]
		f := r.floating(fspa, blips)
		if f != nil {
			r.floats[cp] = f
		}
	}
}

// collectShapes finds the shape containers in a drawing container.
func (r *reader) collectShapes(b []byte) {
	for _, rec := range records(b) {
		switch rec.typ {
		case 0xF003: // group
			r.collectShapes(rec.data)
		case 0xF004: // shape
			sh := &shape{props: map[uint16]uint32{}, cplx: map[uint16][]byte{}}
			var spid uint32
			for _, k := range records(rec.data) {
				switch k.typ {
				case 0xF00A:
					if len(k.data) >= 8 {
						sh.typ, spid, sh.flags = k.inst, u32(k.data), u32(k.data[4:])
					}
				case 0xF00B, 0xF121, 0xF122:
					parseOPT(sh, k)
				}
			}
			if spid != 0 {
				r.shapes[spid] = sh
			}
		}
	}
}

// parseOPT reads a property table: k.inst properties of 6 bytes, then
// the complex properties' data in order.
func parseOPT(sh *shape, k record) {
	d := k.data
	n := k.inst
	if 6*n > len(d) {
		return
	}
	cplx := d[6*n:]
	for i := 0; i < n; i++ {
		id, v := u16(d[6*i:]), u32(d[6*i+2:])
		pid := id & 0x3FFF
		if id&0x8000 != 0 { // complex: v is the size of its data
			if int(v) <= len(cplx) {
				sh.cplx[pid] = cplx[:v]
				cplx = cplx[v:]
			}
			continue
		}
		sh.props[pid] = v
	}
}

// bstore reads the drawing group's pictures (BLIPs), in BStore order.
func (r *reader) bstore(dgg []byte) []*model.Picture {
	var out []*model.Picture
	for _, rec := range records(dgg) {
		if rec.typ != 0xF001 {
			continue
		}
		for _, fbse := range records(rec.data) {
			var pic *model.Picture
			if fbse.typ == 0xF007 && len(fbse.data) >= 36 {
				d := fbse.data
				size, off := int(u32(d[20:])), int(u32(d[28:]))
				name := int(d[33])
				var blip []byte
				switch {
				case len(d) > 36+name+8:
					blip = d[36+name:]
				case size > 0 && off >= 0 && off+size <= len(r.wd):
					blip = r.wd[off : off+size]
				}
				if rs := records(blip); len(rs) > 0 {
					pic = blipPicture(rs[0])
				}
			}
			out = append(out, pic)
		}
	}
	return out
}

// blipPicture decodes a bitmap BLIP record (JPEG, PNG, DIB, TIFF); nil
// for metafiles and other kinds.
func blipPicture(rec record) *model.Picture {
	ext := ""
	switch rec.typ {
	case 0xF01D, 0xF02A:
		ext = "jpeg"
	case 0xF01E:
		ext = "png"
	case 0xF01F:
		ext = "bmp"
	case 0xF029:
		ext = "tiff"
	default:
		return nil
	}
	skip := 17
	if rec.inst&1 == 1 {
		skip += 16 // a second UID
	}
	if len(rec.data) <= skip {
		return nil
	}
	data := rec.data[skip:]
	if ext == "bmp" && len(data) >= 40 {
		// A DIB: add the file header Go's decoder wants.
		hdr := make([]byte, 14)
		hdr[0], hdr[1] = 'B', 'M'
		binary.LittleEndian.PutUint32(hdr[2:], uint32(14+len(data)))
		colors := int(u32(data[32:]))
		bits := int(u16(data[14:]))
		if colors == 0 && bits <= 8 {
			colors = 1 << bits
		}
		binary.LittleEndian.PutUint32(hdr[10:], uint32(14+int(u32(data))+4*colors))
		data = append(hdr, data...)
	}
	return &model.Picture{Data: append([]byte(nil), data...), Ext: ext}
}

// shapeNames maps OfficeArt shape types to the model's preset names.
var shapeNames = map[int]string{
	1: "rect", 2: "roundRect", 3: "ellipse", 4: "diamond", 5: "triangle", 13: "rightArrow",
	20: "line", 32: "straightConnector1", 66: "leftArrow", 67: "downArrow", 68: "upArrow",
	75: "rect", 202: "rect",
}

// floating builds the floating object of an FSPA.
func (r *reader) floating(fspa []byte, blips []*model.Picture) *model.Floating {
	spid := u32(fspa)
	x0, y0, x1, y1 := int32(u32(fspa[4:])), int32(u32(fspa[8:])), int32(u32(fspa[12:])), int32(u32(fspa[16:]))
	fl := u16(fspa[20:])
	f := &model.Floating{X: twips(int(x0)), Y: twips(int(y0)), W: twips(int(x1 - x0)), H: twips(int(y1 - y0))}
	f.RelX = [...]model.FloatRel{model.RelMargin, model.RelPage, model.RelColumn, model.RelColumn}[fl>>1&3]
	f.RelY = [...]model.FloatRel{model.RelMargin, model.RelPage, model.RelParagraph, model.RelParagraph}[fl>>3&3]
	switch fl >> 5 & 0xF {
	case 1:
		f.Wrap = model.WrapTopBottom
	case 0, 2:
		f.Wrap = model.WrapSquare
	case 4, 5:
		f.Wrap = model.WrapTight
	default:
		if fl&(1<<14) != 0 {
			f.Wrap = model.WrapBehind
		}
	}
	sh := r.shapes[spid]
	if sh == nil {
		return nil
	}
	f.Shape = shapeNames[sh.typ]
	if f.Shape == "" {
		r.warn("Some shapes are drawn as rectangles.")
	}
	f.FlipH, f.FlipV = sh.flags&0x40 != 0, sh.flags&0x80 != 0
	prop := func(id uint16, def uint32) uint32 {
		if v, ok := sh.props[id]; ok {
			return v
		}
		return def
	}
	boolProp := func(id uint16, bit uint, def bool) bool {
		v, ok := sh.props[id]
		if !ok || v>>(bit+16)&1 == 0 {
			return def
		}
		return v>>bit&1 != 0
	}
	line := sh.typ != 75
	if boolProp(0x01BF, 4, sh.typ != 75 && sh.typ != 20 && sh.typ != 32) {
		f.Fill = artColor(prop(0x0181, 0xFFFFFF))
	}
	if boolProp(0x01FF, 3, line) {
		f.Line = artColor(prop(0x01C0, 0))
		f.LineWidth = float64(prop(0x01CB, 9525)) / 12700
	}
	if pib := int(prop(0x0104, 0)); pib > 0 && pib <= len(blips) {
		f.Picture = blips[pib-1]
		if f.Picture == nil {
			r.warn("Some floating pictures (metafiles) are shown as placeholders.")
			f.Fill = "F2F2F2"
			f.Line = "BFBFBF"
		}
	}
	emu := func(id uint16, def float64) float64 {
		if v, ok := sh.props[id]; ok {
			return float64(int32(v)) / 12700
		}
		return def
	}
	f.Inset = [4]float64{emu(0x0081, 7.2), emu(0x0082, 3.6), emu(0x0083, 7.2), emu(0x0084, 3.6)}
	if name, ok := sh.cplx[0x0380]; ok {
		f.Name = utf16z(name)
	}
	if st, ok := r.txbx[spid]; ok {
		f.Blocks = r.blocks(st[0], st[1], false)
		if len(f.Blocks) == 0 {
			f.Blocks = []model.Block{&model.Paragraph{}}
		}
	}
	return f
}

// artColor converts an OfficeArt color (red, green, blue, flags) to hex;
// scheme and system colors give black.
func artColor(v uint32) string {
	if v>>24 != 0 {
		return "000000"
	}
	return fmt.Sprintf("%02X%02X%02X", v&0xFF, v>>8&0xFF, v>>16&0xFF)
}

// utf16z decodes a zero-terminated UTF-16LE string.
func utf16z(b []byte) string {
	var u []rune
	for i := 0; i+1 < len(b); i += 2 {
		c := u16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, rune(c))
	}
	return string(u)
}

// floatAt returns the floating object anchored at CP cp, or nil.
func (r *reader) floatAt(cp int) *model.Floating { return r.floats[cp] }
