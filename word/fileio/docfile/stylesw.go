package docfile

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"

	"officelite/word/model"
)

// wstyle is a style as written, with its istd.
type wstyle struct {
	s    *model.Style
	sti  int
	stk  int
	name string
}

// assignStyles gives every paragraph and character style an istd: 0 is
// the default paragraph style, 1-9 the headings, 10 Default Paragraph
// Font, 11 Table Normal, 12 No List, then the rest from 15.
func (w *writer) assignStyles() {
	st := &w.doc.Styles
	slots := make([]*wstyle, 15)
	if n := st.Default(model.ParagraphStyle); n != nil {
		slots[0] = &wstyle{s: n, sti: 0, stk: stkPara, name: "Normal"}
	} else {
		slots[0] = &wstyle{s: &model.Style{ID: "Normal", Name: "Normal", Type: model.ParagraphStyle, Default: true}, sti: 0, stk: stkPara, name: "Normal"}
	}
	used := map[*model.Style]bool{slots[0].s: true}
	for i := 1; i <= 9; i++ {
		if s := st.Find(fmt.Sprintf("Heading%d", i)); s != nil && s.Type == model.ParagraphStyle && !used[s] {
			slots[i] = &wstyle{s: s, sti: i, stk: stkPara, name: s.Name}
			used[s] = true
		}
	}
	if s := st.Default(model.CharacterStyle); s != nil {
		slots[10] = &wstyle{s: s, sti: 65, stk: stkChar, name: "Default Paragraph Font"}
		used[s] = true
	} else {
		slots[10] = &wstyle{s: &model.Style{ID: "DefaultParagraphFont", Type: model.CharacterStyle, Default: true, Hidden: true}, sti: 65, stk: stkChar, name: "Default Paragraph Font"}
	}
	slots[11] = &wstyle{s: &model.Style{ID: "TableNormal", Type: model.TableStyle, Default: true, Hidden: true}, sti: 105, stk: stkTable, name: "Table Normal"}
	slots[12] = &wstyle{s: &model.Style{ID: "NoList", Type: model.NumberingStyle, Default: true, Hidden: true}, sti: 107, stk: stkList, name: "No List"}
	if s := st.Default(model.TableStyle); s != nil {
		slots[11].s = s
		used[s] = true
	}
	if s := st.Default(model.NumberingStyle); s != nil {
		slots[12].s = s
		used[s] = true
	}
	takenSti := map[int]bool{}
	for _, sl := range slots {
		if sl != nil {
			takenSti[sl.sti] = true
		}
	}
	usedTable := map[string]bool{}
	var walkT func([]model.Block)
	walkT = func(bs []model.Block) {
		for _, b := range bs {
			if t, ok := b.(*model.Table); ok {
				usedTable[t.Style] = true
				for _, r := range t.Rows {
					for _, c := range r.Cells {
						walkT(c.Blocks)
					}
				}
			}
		}
	}
	walkT(w.doc.Body)
	dropped := false
	for _, s := range st.Styles {
		if used[s] {
			continue
		}
		if s.Type != model.ParagraphStyle && s.Type != model.CharacterStyle {
			dropped = dropped || (!s.Default && usedTable[s.ID])
			continue
		}
		sti := stiFor(s.ID)
		if sti != stiUser && (takenSti[sti] || sti <= 9 || sti == 65 || sti == 105 || sti == 107) {
			sti = stiUser
		}
		takenSti[sti] = true
		stk := stkPara
		if s.Type == model.CharacterStyle {
			stk = stkChar
		}
		slots = append(slots, &wstyle{s: s, sti: sti, stk: stk, name: s.Name})
		used[s] = true
	}
	if dropped {
		w.warn("Table styles are not saved in .doc files (each table's own borders and shading are kept).")
	}
	w.wstyles = slots
	for i, sl := range slots {
		if sl != nil {
			w.istds[sl.s.ID] = i
		}
	}
}

// styleProps returns what a style's UPX holds: its own formatting, with
// the document defaults folded into root styles (a .doc has no document
// defaults beyond 10pt and the standard font).
func (w *writer) styleProps(sl *wstyle) (model.ParaProps, model.RunProps) {
	s := sl.s
	para, run := s.Para, s.Run
	root := s.BasedOn == "" || w.doc.Styles.Find(s.BasedOn) == nil
	if root && sl.stk == stkPara {
		def := w.doc.Styles.DefaultRun
		if def.Font == "" {
			def.Font = "Calibri"
		}
		if def.Size == 0 {
			def.Size = 11
		}
		run = def.Merge(run)
		para = w.doc.Styles.DefaultPara.Merge(para)
	}
	return para, run
}

// stsh encodes the style sheet.
func (w *writer) stsh() []byte {
	le := binary.LittleEndian
	defFont := w.doc.Styles.DefaultRun.Font
	if defFont == "" {
		defFont = "Calibri"
	}
	ftc := w.font(defFont)
	var stds [][]byte
	for istd, sl := range w.wstyles {
		if sl == nil {
			stds = append(stds, nil)
			continue
		}
		stds = append(stds, w.std(istd, sl))
	}
	b := le.AppendUint16(nil, 18) // cbStshi
	b = le.AppendUint16(b, uint16(len(stds)))
	b = le.AppendUint16(b, 0x0A) // cbSTDBaseInFile
	b = le.AppendUint16(b, 1)    // fStdStylenamesWritten
	b = le.AppendUint16(b, 0x9D) // stiMaxWhenSaved
	b = le.AppendUint16(b, 15)   // istdMaxFixedWhenSaved
	b = le.AppendUint16(b, 0)    // nVerBuiltInNamesWhenSaved
	b = le.AppendUint16(b, ftc)
	b = le.AppendUint16(b, ftc)
	b = le.AppendUint16(b, ftc)
	for _, s := range stds {
		b = le.AppendUint16(b, uint16(len(s)))
		b = append(b, s...)
	}
	return b
}

func (w *writer) istdOf(id string, self int) uint16 {
	if id == "" {
		return istdNil
	}
	if i, ok := w.istds[id]; ok && i != self {
		return uint16(i)
	}
	return istdNil
}

// std encodes one style definition.
func (w *writer) std(istd int, sl *wstyle) []byte {
	le := binary.LittleEndian
	s := sl.s
	para, run := w.styleProps(sl)
	var upx [][]byte
	switch sl.stk {
	case stkPara:
		papx := le.AppendUint16(nil, uint16(istd))
		papx = append(papx, w.encPap(para)...)
		var c grpprl
		w.encRun(&c, run)
		upx = [][]byte{papx, c}
	case stkChar:
		var c grpprl
		w.encRun(&c, run)
		upx = [][]byte{c}
	case stkTable:
		t := []byte{0x17, 0xF6, 0x03, 0x00, 0x00, // sprmTWidthBefore 0
			0x34, 0xD6, 0x06, 0x00, 0x01, 0x0A, 0x03, 0x6C, 0x00, // left/right padding 108
			0x34, 0xD6, 0x06, 0x00, 0x01, 0x05, 0x03, 0x00, 0x00, // top/bottom padding 0
			0x61, 0xF6, 0x03, 0x00, 0x00} // sprmTWidthIndent 0
		upx = [][]byte{t, le.AppendUint16(nil, uint16(istd)), nil}
	case stkList:
		upx = [][]byte{le.AppendUint16(nil, uint16(istd))}
	}
	base := w.istdOf(s.BasedOn, istd)
	next := uint16(istd)
	if sl.stk == stkPara && s.Next != "" {
		if n, ok := w.istds[s.Next]; ok {
			if ns := w.doc.Styles.Find(s.Next); ns != nil && ns.Type == model.ParagraphStyle {
				next = uint16(n)
			}
		}
	}
	if sl.stk == stkChar && istd != 10 && base == istdNil {
		base = 10
	}
	name := sl.name
	if name == "" {
		name = s.Name
	}
	if name == "" {
		name = s.ID
	}
	u := utf16.Encode([]rune(name))
	if len(u) > 253 {
		u = u[:253]
	}
	out := make([]byte, 10)
	le.PutUint16(out[0:], uint16(sl.sti&0x0FFF))
	le.PutUint16(out[2:], uint16(sl.stk)|base<<4)
	le.PutUint16(out[4:], uint16(len(upx))|next<<4)
	var grf uint16
	if s.Hidden {
		grf |= 0x0100 // fSemiHidden
	}
	le.PutUint16(out[8:], grf)
	out = le.AppendUint16(out, uint16(len(u)))
	for _, c := range u {
		out = le.AppendUint16(out, c)
	}
	out = append(out, 0, 0)
	for _, x := range upx {
		out = le.AppendUint16(out, uint16(len(x)))
		out = append(out, x...)
		if len(x)%2 == 1 {
			out = append(out, 0)
		}
	}
	le.PutUint16(out[6:], uint16(len(out))) // bchUpe
	return out
}
