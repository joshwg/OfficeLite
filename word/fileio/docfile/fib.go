package docfile

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// FibRgFcLcb97 entries (index of each fc/lcb pair), [MS-DOC] 2.5.6.
const (
	fcStshfOrig   = 0
	fcStshf       = 1
	fcPlcffndRef  = 2
	fcPlcfandRef  = 4
	fcPlcfSed     = 6
	fcPlcfHdd     = 11
	fcPlcfBteChpx = 12
	fcPlcfBtePapx = 13
	fcSttbfFfn    = 15
	fcPlcfFldMom  = 16
	fcDop         = 31
	fcClx         = 33
	fcPlcSpaMom   = 40
	fcPlcfendRef  = 46
	fcDggInfo     = 50
	fcPlcftxbxTxt = 56
	fcPlfLst      = 73
	fcPlfLfo      = 74

	nFcLcb97 = 0x5D
)

// fibRgLw97 indexes.
const (
	lwCbMac      = 0
	lwCcpText    = 3
	lwCcpFtn     = 4
	lwCcpHdd     = 5
	lwCcpAtn     = 7
	lwCcpEdn     = 8
	lwCcpTxbx    = 9
	lwCcpHdrTxbx = 10
	nLw97        = 22
)

const (
	fibBaseSize = 32
	nFib97      = 0x00C1
	wIdent      = 0xA5EC
)

// fib is the parts of the File Information Block the reader uses.
type fib struct {
	nFib       uint16
	flags      uint16
	lw         [nLw97]int32
	fcLcb      [][2]uint32 // fc, lcb pairs
	whichTable bool        // true: 1Table
}

func (f *fib) has(i int) bool { return i < len(f.fcLcb) && f.fcLcb[i][1] > 0 }

// blob returns the table-stream bytes of fc/lcb pair i, or nil.
func (f *fib) blob(table []byte, i int) []byte {
	if !f.has(i) {
		return nil
	}
	fc, lcb := f.fcLcb[i][0], f.fcLcb[i][1]
	if uint64(fc)+uint64(lcb) > uint64(len(table)) {
		return nil
	}
	return table[fc : fc+lcb]
}

func parseFib(wd []byte) (*fib, error) {
	if len(wd) < fibBaseSize+2 {
		return nil, errors.New("doc: WordDocument stream too short")
	}
	if binary.LittleEndian.Uint16(wd) != wIdent {
		return nil, errors.New("doc: not a Word document (bad FIB signature)")
	}
	f := &fib{nFib: binary.LittleEndian.Uint16(wd[2:]), flags: binary.LittleEndian.Uint16(wd[10:])}
	if f.nFib < 0x00C0 {
		if f.nFib >= 0x0065 && f.nFib <= 0x0069 {
			return nil, errors.New("Word 6.0/95 documents are not supported; open the file in Word and save it as a Word 97-2003 or .docx document")
		}
		return nil, fmt.Errorf("doc: unsupported Word file version (nFib 0x%04X); only Word 97 and later are supported", f.nFib)
	}
	if f.flags&0x0100 != 0 {
		if f.flags&0x8000 != 0 {
			return nil, errors.New("this document is obfuscated (XOR protected) and cannot be opened")
		}
		return nil, errors.New("this document is password-protected (encrypted) and cannot be opened")
	}
	f.whichTable = f.flags&0x0200 != 0
	p := fibBaseSize
	csw := int(binary.LittleEndian.Uint16(wd[p:]))
	p += 2 + 2*csw
	if p+2 > len(wd) {
		return nil, errors.New("doc: truncated FIB")
	}
	cslw := int(binary.LittleEndian.Uint16(wd[p:]))
	p += 2
	for i := 0; i < cslw && i < nLw97; i++ {
		if p+4*i+4 > len(wd) {
			return nil, errors.New("doc: truncated FIB")
		}
		f.lw[i] = int32(binary.LittleEndian.Uint32(wd[p+4*i:]))
	}
	p += 4 * cslw
	if p+2 > len(wd) {
		return nil, errors.New("doc: truncated FIB")
	}
	n := int(binary.LittleEndian.Uint16(wd[p:]))
	p += 2
	if p+8*n > len(wd) {
		return nil, errors.New("doc: truncated FIB")
	}
	f.fcLcb = make([][2]uint32, n)
	for i := range f.fcLcb {
		f.fcLcb[i] = [2]uint32{binary.LittleEndian.Uint32(wd[p+8*i:]), binary.LittleEndian.Uint32(wd[p+8*i+4:])}
	}
	return f, nil
}
