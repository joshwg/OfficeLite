// Tests for the Windows-1252 and UTF-16 helpers.

package textenc

import "testing"

// TestWindows1252 checks Windows-1252 decoding (including the unassigned C1
// bytes), Decode's fallback to it, and the byte↔rune round trip.
func TestWindows1252(t *testing.T) {
	in := []byte{'A', 0x80, 0x81, 0x8D, 0x8F, 0x90, 0x9D, 0x93, 0xE9}
	want := "A€\u0081\u008D\u008F\u0090\u009D“é"
	if got := Windows1252(in); got != want {
		t.Errorf("Windows1252 = %q, want %q", got, want)
	}
	if got, _ := Decode(in); got != want {
		t.Errorf("Decode = %q, want %q", got, want)
	}
	for _, c := range in {
		if b, ok := Windows1252Byte(Windows1252Rune(c)); !ok || b != c {
			t.Errorf("round trip of %#x = %#x %v", c, b, ok)
		}
	}
	if _, ok := Windows1252Byte('漢'); ok {
		t.Error("漢 encoded")
	}
}

// TestUTF16 checks the UTF-16LE encode/decode round trip, NUL termination in
// UTF16Z, and its handling of an odd trailing byte.
func TestUTF16(t *testing.T) {
	s := "héllo 😀"
	b := AppendUTF16LE(nil, s)
	if got := UTF16LE(b); got != s {
		t.Errorf("UTF16LE = %q", got)
	}
	if got := UTF16Z(append(b, 0, 0, 'x', 0)); got != s {
		t.Errorf("UTF16Z = %q", got)
	}
	if got := UTF16Z(b[:3]); got != "h" {
		t.Errorf("UTF16Z odd length = %q", got)
	}
}

// TestDecodeBOMs checks Decode on each byte-order mark, a UTF-8 mark
// followed by invalid UTF-8 (read as Windows-1252, the mark dropped), and
// plain UTF-16 whose first character isn't taken for UTF-32.
func TestDecodeBOMs(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want string
	}{
		{"utf-8", []byte("\xEF\xBB\xBFh\xC3\xA9"), "hé"},
		{"utf-8 mark, invalid rest", []byte("\xEF\xBB\xBFcaf\xE9"), "café"},
		{"utf-16le", []byte{0xFF, 0xFE, 'h', 0, 0xE9, 0}, "hé"},
		{"utf-16be", []byte{0xFE, 0xFF, 0, 'h', 0, 0xE9}, "hé"},
		{"utf-32le", []byte{0xFF, 0xFE, 0, 0, 'h', 0, 0, 0, 0xE9, 0, 0, 0}, "hé"},
		{"utf-32be", []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'h', 0, 0, 0, 0xE9}, "hé"},
		{"utf-16le, 2 characters", []byte{0xFF, 0xFE, 'a', 0, 'b', 0}, "ab"},
		{"no mark, windows-1252", []byte("caf\xE9"), "café"},
		{"no mark, utf-8", []byte("h\xC3\xA9"), "hé"},
	} {
		if got, err := Decode(tt.data); err != nil || got != tt.want {
			t.Errorf("%s: Decode = %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}
}
