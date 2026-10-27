package docfile

import (
	"encoding/binary"
	"sort"
	"unicode/utf16"

	"officelite/word/model"
)

// assignLists numbers the model's lists 1..n (the LFO indexes).
func (w *writer) assignLists() {
	var ids []int
	for id, l := range w.doc.Lists {
		if id > 0 && l != nil {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	w.listIDs = ids
	for i, id := range ids {
		w.ilfo[id] = i + 1
	}
}

// listTables encodes PlfLst followed by its LVLs, and PlfLfo.
func (w *writer) listTables() (lst, lfo []byte) {
	if len(w.listIDs) == 0 {
		return nil, nil
	}
	le := binary.LittleEndian
	lst = le.AppendUint16(nil, uint16(len(w.listIDs)))
	var lvls []byte
	for i, id := range w.listIDs {
		lsid := uint32(0x2B4C0000 + i*0x101)
		lst = le.AppendUint32(lst, lsid)
		lst = le.AppendUint32(lst, 0) // tplc
		for k := 0; k < 9; k++ {
			lst = le.AppendUint16(lst, istdNil)
		}
		l := w.doc.Lists[id]
		n := 9
		if len(l.Levels) == 1 {
			n = 1 // a simple (one-level) list
			lst = append(lst, 1, 0)
		} else {
			lst = append(lst, 0, 0)
		}
		for k := 0; k < n; k++ {
			lv := defaultLevel(k)
			if k < len(l.Levels) {
				lv = l.Levels[k]
			}
			lvls = append(lvls, w.lvl(lv)...)
		}
	}
	lst = append(lst, lvls...)

	lfo = le.AppendUint32(nil, uint32(len(w.listIDs)))
	for i := range w.listIDs {
		lfo = le.AppendUint32(lfo, uint32(0x2B4C0000+i*0x101))
		lfo = append(lfo, make([]byte, 12)...)
	}
	for range w.listIDs {
		lfo = le.AppendUint32(lfo, 0xFFFFFFFF) // LFOData.cp, no level overrides
	}
	return lst, lfo
}

func defaultLevel(k int) model.ListLevel {
	return model.ListLevel{Format: "decimal", Text: "%" + string(rune('1'+k)) + ".", Start: 1,
		Indent: float64(36 * (k + 1)), Hang: 18}
}

// lvl encodes one list level.
func (w *writer) lvl(lv model.ListLevel) []byte {
	le := binary.LittleEndian
	nfc := nfcFor(lv.Format)
	var xst []uint16
	var nums [9]byte
	n := 0
	if nfc == 23 {
		xst = utf16.Encode([]rune(lv.Text))
		if len(xst) == 0 {
			xst = []uint16{0xF0B7}
			if lv.Font == "" {
				lv.Font = "Symbol"
			}
		}
	} else {
		rs := []rune(lv.Text)
		for i := 0; i < len(rs); i++ {
			if rs[i] == '%' && i+1 < len(rs) && rs[i+1] >= '1' && rs[i+1] <= '9' {
				if n < 9 {
					nums[n] = byte(len(xst) + 1)
					n++
				}
				xst = append(xst, uint16(rs[i+1]-'1'))
				i++
				continue
			}
			xst = append(xst, utf16.Encode([]rune{rs[i]})...)
		}
	}
	var papx grpprl
	if lv.Indent != 0 || lv.Hang != 0 {
		left := uint16(int16(clampTw(lv.Indent, -31680, 31680)))
		first := uint16(int16(-clampTw(lv.Hang, -31680, 31680)))
		papx.w(sprmPDxaLeft80, left)
		papx.w(sprmPDxaLeft180, first)
		papx.w(sprmPDxaLeft, left)
		papx.w(sprmPDxaLeft1, first)
	}
	var chpx grpprl
	if lv.Font != "" {
		f := w.font(lv.Font)
		chpx.w(sprmCRgFtc0, f)
		chpx.w(sprmCRgFtc1, f)
		chpx.w(sprmCRgFtc2, f)
	}
	start := lv.Start
	if start == 0 && nfc != 23 {
		start = 1
	}
	b := le.AppendUint32(nil, uint32(int32(start)))
	b = append(b, nfc, 0) // nfc, jc = left
	b = append(b, nums[:]...)
	b = append(b, 0)          // ixchFollow: tab
	b = le.AppendUint32(b, 0) // dxaIndentSav
	b = le.AppendUint32(b, 0) // unused2
	b = append(b, byte(len(chpx)), byte(len(papx)), 0, 0)
	b = append(b, papx...)
	b = append(b, chpx...)
	b = le.AppendUint16(b, uint16(len(xst)))
	for _, c := range xst {
		b = le.AppendUint16(b, c)
	}
	return b
}
