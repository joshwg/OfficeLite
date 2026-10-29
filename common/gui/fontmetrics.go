// WordMetrics: the GDI-style ascent and descent Word gives a face, read from
// its OS/2 and hhea tables.

package gui

import (
	"encoding/binary"
	"os"
	"sync"

	"gioui.org/text"
)

// Word's line metrics. Gio takes a face's ascent and descent from its hhea
// table and adds the line gap to the descent; Word (through GDI) uses the
// OS/2 usWinAscent and usWinDescent, with the part of the line gap they
// don't already cover (GDI's external leading) below. For one font both
// give the same line height (Calibri: 0.75 + 0.25 + 0.22 gap = 0.95 +
// 0.27 em), but a line mixing fonts is as tall as its tallest ascent plus
// its deepest descent, so the split matters: Calibri text with a Symbol
// bullet (ascent 1.005 em) is 0.05 em taller in Word, and 0.26 em with
// Gio's split.

type winMetrics struct {
	asc, desc float64 // ems
	ok        bool
}

var winMetricsCache sync.Map // FaceFile -> winMetrics

// WordMetrics returns the ascent and descent (in ems) Word uses for the
// face a glyph shaped by sh (a FontLibrary shaper) comes from; ok is false
// when the face's file isn't known (the Go fonts, other shapers).
func WordMetrics(sh *text.Shaper, id text.GlyphID) (asc, desc float64, ok bool) {
	files := faceFiles(sh)
	i := int(uint64(id) >> 48)
	if i >= len(files) || files[i].Path == "" {
		return 0, 0, false
	}
	ff := files[i]
	if v, hit := winMetricsCache.Load(ff); hit {
		m := v.(winMetrics)
		return m.asc, m.desc, m.ok
	}
	m := readWinMetrics(ff)
	winMetricsCache.Store(ff, m)
	return m.asc, m.desc, m.ok
}

// readWinMetrics reads the head, hhea and OS/2 tables of a font file (a
// face of a collection) and returns Word's ascent and descent for it; ok is
// false when a table is missing or unreadable.
func readWinMetrics(ff FaceFile) winMetrics {
	f, err := os.Open(ff.Path)
	if err != nil {
		return winMetrics{}
	}
	defer f.Close()
	be := binary.BigEndian
	read := func(off int64, n int) []byte {
		b := make([]byte, n)
		if _, err := f.ReadAt(b, off); err != nil {
			return nil
		}
		return b
	}
	dir := int64(0)
	if h := read(0, 12); h == nil {
		return winMetrics{}
	} else if string(h[:4]) == "ttcf" {
		o := read(12+4*int64(ff.Index), 4)
		if o == nil {
			return winMetrics{}
		}
		dir = int64(be.Uint32(o))
	}
	h := read(dir, 12)
	if h == nil {
		return winMetrics{}
	}
	n := int(be.Uint16(h[4:]))
	recs := read(dir+12, 16*n)
	if recs == nil {
		return winMetrics{}
	}
	table := func(tag string, size int) []byte {
		for k := 0; k < n; k++ {
			r := recs[16*k:]
			if string(r[:4]) == tag && int(be.Uint32(r[12:])) >= size {
				return read(int64(be.Uint32(r[8:])), size)
			}
		}
		return nil
	}
	head, hhea, os2 := table("head", 20), table("hhea", 10), table("OS/2", 78)
	if head == nil || hhea == nil || os2 == nil {
		return winMetrics{}
	}
	upem := float64(be.Uint16(head[18:]))
	if upem <= 0 {
		return winMetrics{}
	}
	hAsc, hDesc, gap := float64(int16(be.Uint16(hhea[4:]))), float64(int16(be.Uint16(hhea[6:]))), float64(int16(be.Uint16(hhea[8:])))
	wAsc, wDesc := float64(be.Uint16(os2[74:])), float64(be.Uint16(os2[76:]))
	if wAsc+wDesc <= 0 {
		return winMetrics{}
	}
	ext := max(0, gap-((wAsc+wDesc)-(hAsc-hDesc)))
	return winMetrics{asc: wAsc / upem, desc: (wDesc + ext) / upem, ok: true}
}
