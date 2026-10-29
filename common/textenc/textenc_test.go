package textenc

import "testing"

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
