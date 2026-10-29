// Package xlsfile reads Excel 97-2003 binary workbooks (.xls, BIFF8).
//
// Reference: [MS-XLS] Excel Binary File Format Structure.
package xlsfile

import (
	"encoding/binary"
	"math"

	"officelite/common/textenc"
)

// Record identifiers used by the reader.
const (
	recFormula       = 0x0006
	recExternSheet   = 0x0017
	recName          = 0x0018
	recExternName    = 0x0023
	recEOF           = 0x000A
	recDateMode      = 0x0022
	recFilePass      = 0x002F
	recFont          = 0x0031
	recContinue      = 0x003C
	recSelection     = 0x001D
	recPane          = 0x0041
	recDefColWidth   = 0x0055
	recStandardWidth = 0x0099
	recColInfo       = 0x007D
	recBoundSheet    = 0x0085
	recPalette       = 0x0092
	recMulRK         = 0x00BD
	recMulBlank      = 0x00BE
	recRString       = 0x00D6
	recSupBook       = 0x01AE
	recXF            = 0x00E0
	recMergedCells   = 0x00E5
	recSST           = 0x00FC
	recLabelSST      = 0x00FD
	recBlank         = 0x0201
	recNumber        = 0x0203
	recLabel         = 0x0204
	recBoolErr       = 0x0205
	recString        = 0x0207
	recRow           = 0x0208
	recDefaultRowHt  = 0x0225
	recWindow2       = 0x023E
	recArray         = 0x0221
	recTable         = 0x0236
	recRK            = 0x027E
	recShrFmla       = 0x04BC
	recFormat        = 0x041E
	recBOF           = 0x0809
	biff8Version     = 0x0600
	bofTypeWorksheet = 0x0010
	bofTypeChart     = 0x0020
)

// u16 reads the little-endian uint16 at off, or 0 past the end of b.
func u16(b []byte, off int) int {
	if off+2 > len(b) {
		return 0
	}
	return int(binary.LittleEndian.Uint16(b[off:]))
}

// u32 reads the little-endian uint32 at off, or 0 past the end of b.
func u32(b []byte, off int) uint32 {
	if off+4 > len(b) {
		return 0
	}
	return binary.LittleEndian.Uint32(b[off:])
}

// f64 reads the little-endian IEEE double at off, or 0 past the end of b.
func f64(b []byte, off int) float64 {
	if off+8 > len(b) {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(b[off:]))
}

// record is one BIFF record: a 2-byte id, 2-byte length, then the body.
type record struct {
	id   int
	body []byte
}

// records iterates over the records of stream starting at pos.
func records(stream []byte, pos int, yield func(record) bool) {
	for pos+4 <= len(stream) {
		id := u16(stream, pos)
		n := u16(stream, pos+2)
		end := min(pos+4+n, len(stream))
		if !yield(record{id, stream[pos+4 : end]}) {
			return
		}
		pos = end
	}
}

// rkValue decodes an RK number: a compressed float or integer, optionally
// divided by 100.
func rkValue(rk uint32) float64 {
	var v float64
	if rk&0x02 != 0 {
		v = float64(int32(rk) >> 2)
	} else {
		v = math.Float64frombits(uint64(rk&0xFFFFFFFC) << 32)
	}
	if rk&0x01 != 0 {
		v /= 100
	}
	return v
}

var errorTexts = map[byte]string{
	0x00: "#NULL!", 0x07: "#DIV/0!", 0x0F: "#VALUE!", 0x17: "#REF!",
	0x1D: "#NAME?", 0x24: "#NUM!", 0x2A: "#N/A", 0x2B: "#GETTING_DATA",
}

// unicodeString reads an XLUnicodeString (cchBytes = 2) or
// ShortXLUnicodeString (cchBytes = 1) at off. Rich-text and phonetic data
// that may follow the characters are skipped.
func unicodeString(b []byte, off, cchBytes int) (string, int) {
	if off+cchBytes+1 > len(b) {
		return "", len(b)
	}
	cch := int(b[off])
	if cchBytes == 2 {
		cch = u16(b, off)
	}
	flags := b[off+cchBytes]
	off += cchBytes + 1
	if flags&0x08 != 0 { // rich text: run count
		off += 2
	}
	if flags&0x04 != 0 { // phonetic: ext size
		off += 4
	}
	return chars(b, off, cch, flags&0x01 != 0)
}

// chars reads cch characters at off, 16-bit when wide and Latin-1
// otherwise, clipped to b, and returns them with the offset after them.
func chars(b []byte, off, cch int, wide bool) (string, int) {
	n := cch
	if wide {
		n *= 2
	}
	end := min(off+n, len(b))
	if off > end {
		return "", len(b)
	}
	if wide {
		return textenc.UTF16LE(b[off:end]), end
	}
	return textenc.Latin1(b[off:end]), end
}

// segments reads data that is split across a record and its CONTINUE
// records, as the shared string table is. A string's characters may be cut
// at a record boundary, in which case the continuation starts with a fresh
// option byte saying whether the rest is 8- or 16-bit.
type segments struct {
	segs [][]byte
	i    int // current segment
	pos  int // offset in current segment
}

// done reports whether every segment is used up, first advancing past
// any exhausted ones.
func (s *segments) done() bool {
	for s.i < len(s.segs) && s.pos >= len(s.segs[s.i]) {
		s.i++
		s.pos = 0
	}
	return s.i >= len(s.segs)
}

// byte reads the next byte across segment boundaries, or 0 when done.
func (s *segments) byte() byte {
	if s.done() {
		return 0
	}
	c := s.segs[s.i][s.pos]
	s.pos++
	return c
}

// u16 reads a little-endian uint16 across segment boundaries.
func (s *segments) u16() int { return int(s.byte()) | int(s.byte())<<8 }

// u32 reads a little-endian uint32 across segment boundaries.
func (s *segments) u32() int { return s.u16() | s.u16()<<16 }

// skip advances n bytes across segment boundaries, stopping when done.
func (s *segments) skip(n int) {
	for n > 0 && !s.done() {
		k := min(n, len(s.segs[s.i])-s.pos)
		s.pos += k
		n -= k
	}
}

// str reads an XLUnicodeRichExtendedString.
func (s *segments) str() string {
	cch := s.u16()
	flags := s.byte()
	runs, ext := 0, 0
	if flags&0x08 != 0 {
		runs = s.u16()
	}
	if flags&0x04 != 0 {
		ext = s.u32()
	}
	wide := flags&0x01 != 0
	out := make([]rune, 0, cch)
	for cch > 0 {
		if s.i >= len(s.segs) {
			break
		}
		if s.pos >= len(s.segs[s.i]) {
			// Characters continue in the next record, which restates the width.
			s.i++
			s.pos = 0
			if s.i >= len(s.segs) {
				break
			}
			wide = s.byte()&0x01 != 0
			continue
		}
		seg := s.segs[s.i]
		if wide {
			n := min(cch, (len(seg)-s.pos)/2)
			for k := 0; k < n; k++ {
				out = append(out, rune(u16(seg, s.pos+2*k)))
			}
			s.pos += 2 * n
			cch -= n
			if n == 0 { // odd trailing byte; should not happen
				s.pos = len(seg)
			}
		} else {
			n := min(cch, len(seg)-s.pos)
			for _, c := range seg[s.pos : s.pos+n] {
				out = append(out, rune(c))
			}
			s.pos += n
			cch -= n
		}
	}
	s.skip(4*runs + ext)
	return fixSurrogates(out)
}

// fixSurrogates joins UTF-16 surrogate pairs that were appended as runes.
func fixSurrogates(r []rune) string {
	for _, c := range r {
		if c >= 0xD800 && c <= 0xDFFF {
			b := make([]byte, 0, 2*len(r))
			for _, c := range r {
				b = binary.LittleEndian.AppendUint16(b, uint16(c))
			}
			return textenc.UTF16LE(b)
		}
	}
	return string(r)
}
