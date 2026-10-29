package iwork

import (
	"encoding/binary"
	"errors"
)

var errSnappy = errors.New("iwork: corrupt snappy data")

// snappyDecode decompresses one raw Snappy block (the format without the
// framing/CRC of Snappy streams): a varint uncompressed length, then
// literal and copy elements.
func snappyDecode(src []byte) ([]byte, error) {
	n, k := binary.Uvarint(src)
	// No element expands more than 64/3 times (a 3-byte copy of 64
	// bytes), so a longer claimed length is corrupt; checking it before
	// allocating keeps a few bad bytes from reserving a gigabyte.
	if k <= 0 || n > 22*uint64(len(src)) {
		return nil, errSnappy
	}
	src = src[k:]
	dst := make([]byte, 0, n)
	for len(src) > 0 {
		tag := src[0]
		var length, offset int
		switch tag & 3 {
		case 0: // literal
			length = int(tag >> 2)
			src = src[1:]
			if length >= 60 {
				nb := length - 59 // 1..4 length bytes follow
				if len(src) < nb {
					return nil, errSnappy
				}
				length = 0
				for i := nb - 1; i >= 0; i-- {
					length = length<<8 | int(src[i])
				}
				src = src[nb:]
			}
			length++
			if length <= 0 || length > len(src) {
				return nil, errSnappy
			}
			dst = append(dst, src[:length]...)
			src = src[length:]
			continue
		case 1: // copy with a 1-byte offset
			if len(src) < 2 {
				return nil, errSnappy
			}
			length = 4 + int(tag>>2&7)
			offset = int(tag&0xe0)<<3 | int(src[1])
			src = src[2:]
		case 2: // copy with a 2-byte offset
			if len(src) < 3 {
				return nil, errSnappy
			}
			length = 1 + int(tag>>2)
			offset = int(binary.LittleEndian.Uint16(src[1:]))
			src = src[3:]
		case 3: // copy with a 4-byte offset
			if len(src) < 5 {
				return nil, errSnappy
			}
			length = 1 + int(tag>>2)
			offset = int(binary.LittleEndian.Uint32(src[1:]))
			src = src[5:]
		}
		if offset <= 0 || offset > len(dst) {
			return nil, errSnappy
		}
		// Copies may overlap their own output (a run), so go byte by byte.
		start := len(dst) - offset
		for i := range length {
			dst = append(dst, dst[start+i])
		}
	}
	if uint64(len(dst)) != n {
		return nil, errSnappy
	}
	return dst, nil
}

// decodeIWA undoes an .iwa file's compression: a series of chunks, each a
// 0x00 byte, a 3-byte little-endian length and a raw Snappy block. A chunk
// that isn't valid Snappy is taken as stored uncompressed.
func decodeIWA(data []byte) ([]byte, error) {
	var out []byte
	for len(data) > 0 {
		if len(data) < 4 || data[0] != 0 {
			return nil, errors.New("iwork: bad .iwa chunk header")
		}
		n := int(data[1]) | int(data[2])<<8 | int(data[3])<<16
		data = data[4:]
		if n > len(data) {
			return nil, errors.New("iwork: truncated .iwa chunk")
		}
		b, err := snappyDecode(data[:n])
		if err != nil {
			b = data[:n]
		}
		out = append(out, b...)
		data = data[n:]
	}
	return out, nil
}
