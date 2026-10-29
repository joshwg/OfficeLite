// Package lockfile keeps two OfficeLite windows, or OfficeLite and Word
// or Excel, from editing the same file at once.
//
// Office marks an open document with a hidden "owner file" next to it,
// ~$name.docx, holding the user's name, and Word and Excel also keep the
// document itself open against writing. Check reads both before a file is
// opened. Doc tracks the file a window has open: a second open of it
// raises that window (in this process, or in another OfficeLite process
// over a localhost connection), and on a network share Doc writes an
// owner file of its own so that Office users see who has it.
package lockfile

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Format is the owner-file layout of an Office application.
type Format int

const (
	// Word writes 162 bytes: the name's length, the name in the ANSI
	// code page zero-padded to offset 54, the length again as 16 bits,
	// the name in UTF-16LE, zeros.
	Word Format = iota
	// Excel (and PowerPoint) write 165 bytes: as Word, but the ANSI name
	// is padded with spaces to offset 55, and the tail with UTF-16
	// spaces.
	Excel
)

const (
	maxName   = 52 // characters Office keeps of the user name
	wordSize  = 162
	excelSize = 165
)

// FormatOf is the owner-file format for a document, by its extension, and
// whether Office writes owner files for it at all.
func FormatOf(path string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".doc", ".docx", ".docm", ".dot", ".dotx", ".dotm", ".rtf":
		return Word, true
	case ".xls", ".xlsx", ".xlsm", ".xlsb", ".xlt", ".xltx", ".xltm":
		return Excel, true
	}
	return 0, false
}

// OwnerName is the owner file's name for a document file name. Excel puts
// "~$" before the whole name; Word replaces the first two characters of a
// name of eight or more characters (before the extension) and the first
// one of a seven-character name: "~$port2024.docx" is Report2024.docx's,
// "~$Memo.docx" Memo.docx's.
func OwnerName(name string) string {
	if f, ok := FormatOf(name); ok && f == Word {
		return wordOwnerName(name)
	}
	return "~$" + name
}

// wordOwnerName is Word's owner file name for a document file name.
func wordOwnerName(name string) string {
	ext := filepath.Ext(name)
	base := []rune(strings.TrimSuffix(name, ext))
	switch {
	case len(base) >= 8:
		base = base[2:]
	case len(base) == 7:
		base = base[1:]
	}
	return "~$" + string(base) + ext
}

// ownerPaths are the owner files that may belong to the document at path:
// Word's and Excel's names for it (a .txt may have been open in Word, a
// .csv in Excel), the one its own application writes first.
func ownerPaths(path string) []string {
	dir, name := filepath.Split(path)
	out := []string{filepath.Join(dir, OwnerName(name))}
	for _, n := range []string{wordOwnerName(name), "~$" + name} {
		if p := filepath.Join(dir, n); p != out[0] && (len(out) == 1 || p != out[1]) {
			out = append(out, p)
		}
	}
	return out
}

// Encode is an owner file naming user, in format f. Names longer than
// Office's 52 characters are cut; characters outside Windows-1252 are "?"
// in the ANSI copy.
func Encode(f Format, user string) []byte {
	u := utf16.Encode([]rune(user))
	if len(u) > maxName {
		u = u[:maxName]
		if utf16.IsSurrogate(rune(u[maxName-1])) && utf16.DecodeRune(rune(u[maxName-2]), rune(u[maxName-1])) == utf8.RuneError {
			u = u[:maxName-1] // half a surrogate pair
		}
	}
	size, uniAt, pad := wordSize, 54, byte(0)
	if f == Excel {
		size, uniAt, pad = excelSize, 55, ' '
	}
	b := make([]byte, size)
	b[0] = byte(len(u))
	for i, c := range u {
		ch := byte('?')
		if r := rune(c); r < 0x80 {
			ch = byte(r)
		} else if e, ok := charmap.Windows1252.EncodeRune(r); ok {
			ch = e
		}
		b[1+i] = ch
	}
	for i := 1 + len(u); i < uniAt; i++ {
		b[i] = pad
	}
	binary.LittleEndian.PutUint16(b[uniAt:], uint16(len(u)))
	i := uniAt + 2
	for _, c := range u {
		binary.LittleEndian.PutUint16(b[i:], c)
		i += 2
	}
	for ; f == Excel && i < size; i += 2 {
		b[i] = ' ' // and a zero high byte
	}
	return b
}

// Decode reads the user name from an owner file of either format,
// preferring the UTF-16 copy. It follows LibreOffice's reader: the UTF-16
// length is at offset 54 (Word), or at 55 when offset 54 holds Excel's
// space padding.
func Decode(b []byte) (string, bool) {
	if len(b) < wordSize || b[0] == 0 || b[0] > maxName {
		return "", false
	}
	n := int(b[0])
	at := 54
	if b[54] == ' ' && (b[53] != 0 || b[52] != 0) {
		at = 55
	}
	if m := int(binary.LittleEndian.Uint16(b[at:])); m > 0 && m <= maxName && at+2+2*m <= len(b) {
		u := make([]uint16, m)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[at+2+2*i:])
		}
		return string(utf16.Decode(u)), true
	}
	r := make([]rune, n)
	for i := range r {
		r[i] = charmap.Windows1252.DecodeByte(b[1+i])
	}
	return string(r), true
}
