// Package textenc converts plain-text file content (CSV, TXT) to UTF-8.
package textenc

import (
	"bytes"
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
		b, err := charmap.Windows1252.NewDecoder().Bytes(data)
		return string(b), err
	}
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
