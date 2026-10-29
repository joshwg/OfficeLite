// Package textenc converts plain-text file content (CSV, TXT) to UTF-8,
// and has the UTF-16 and Windows-1252 helpers of the binary Office
// formats.
package textenc

import (
	"bytes"
	"encoding/binary"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// Decode converts data to UTF-8, honouring byte-order marks and falling back
// to Windows-1252 (what Office assumes on a US system) for invalid UTF-8.
func Decode(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(data[3:]), nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		b, err := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder().Bytes(data)
		return string(b), err
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		b, err := unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder().Bytes(data)
		return string(b), err
	case utf8.Valid(data):
		return string(data), nil
	default:
		return Windows1252(data), nil
	}
}

// Windows1252Rune maps a Windows-1252 byte to its character. The
// unassigned bytes 0x81, 0x8D, 0x8F, 0x90 and 0x9D map to the C1 controls
// U+0081 etc., as Windows' MultiByteToWideChar does.
func Windows1252Rune(b byte) rune {
	if b < 0x80 || b >= 0xA0 {
		return rune(b)
	}
	if r := charmap.Windows1252.DecodeByte(b); r != utf8.RuneError {
		return r
	}
	return rune(b)
}

// Windows1252Byte is the inverse of Windows1252Rune.
func Windows1252Byte(r rune) (byte, bool) {
	switch r {
	case 0x81, 0x8D, 0x8F, 0x90, 0x9D:
		return byte(r), true
	}
	return charmap.Windows1252.EncodeRune(r)
}

// Windows1252 decodes Windows-1252 text (see Windows1252Rune).
func Windows1252(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = Windows1252Rune(c)
	}
	return string(r)
}

// Latin1 decodes 8-bit "compressed" strings from binary Office formats, which
// store the low byte of each UTF-16 code unit (i.e. ISO-8859-1).
func Latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// UTF16LE decodes little-endian UTF-16 code units.
func UTF16LE(b []byte) string {
	s, _ := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder().Bytes(b)
	return string(s)
}

// UTF16Z decodes little-endian UTF-16 up to the first NUL.
func UTF16Z(b []byte) string {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

// AppendUTF16LE appends s as little-endian UTF-16 code units.
func AppendUTF16LE(b []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}
