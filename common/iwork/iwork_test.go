package iwork

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSnappyDecode(t *testing.T) {
	long := bytes.Repeat([]byte("z"), 70)
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"literal", []byte{5, 4 << 2, 'h', 'e', 'l', 'l', 'o'}, "hello"},
		// "abc", then a 1-byte-offset copy of 9 bytes from 3 back (an
		// overlapping run), then "X".
		{"copy1", []byte{13, 2 << 2, 'a', 'b', 'c', (9-4)<<2 | 1, 3, 0, 'X'}, "abcabcabcabcX"},
		// "ab", then a 2-byte-offset copy of 4 bytes from 2 back.
		{"copy2", []byte{6, 1 << 2, 'a', 'b', (4-1)<<2 | 2, 2, 0}, "ababab"},
		// "xy", then a 4-byte-offset copy of 2 bytes from 2 back.
		{"copy4", []byte{4, 1 << 2, 'x', 'y', (2-1)<<2 | 3, 2, 0, 0, 0}, "xyxy"},
		// A literal of 70 bytes needs a length byte (tag 60).
		{"long literal", append([]byte{70, 60 << 2, 70 - 1}, long...), string(long)},
	}
	for _, tt := range tests {
		got, err := snappyDecode(tt.in)
		if err != nil || string(got) != tt.want {
			t.Errorf("%s: got %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}
	for _, bad := range [][]byte{
		{3, 0, 'a'},              // literal runs past the end
		{4, 0, 'a', 1<<2 | 1, 5}, // copy from before the start
		{9, 1 << 2, 'a', 'b'},    // shorter than its stated length
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, // absurd length
	} {
		if _, err := snappyDecode(bad); err == nil {
			t.Errorf("snappyDecode(% x) succeeded", bad)
		}
	}
}

func TestDecodeIWAChunks(t *testing.T) {
	chunk := func(b []byte) []byte {
		return append([]byte{0, byte(len(b)), byte(len(b) >> 8), byte(len(b) >> 16)}, b...)
	}
	in := append(chunk([]byte{5, 4 << 2, 'h', 'e', 'l', 'l', 'o'}), chunk([]byte{3, 2 << 2, 'a', 'b', 'c'})...)
	got, err := decodeIWA(in)
	if err != nil || string(got) != "helloabc" {
		t.Fatalf("decodeIWA = %q, %v", got, err)
	}
	if _, err := decodeIWA([]byte{1, 2, 0, 0, 'x', 'y'}); err == nil {
		t.Error("a chunk not starting with 0 was accepted")
	}
	if _, err := decodeIWA([]byte{0, 9, 0, 0, 'x'}); err == nil {
		t.Error("a truncated chunk was accepted")
	}
}

func TestDecodeProtobuf(t *testing.T) {
	var b []byte
	b = append(b, 0x08, 0x96, 0x01)       // 1: 150
	b = append(b, 0x12, 2, 'h', 'i')      // 2: "hi"
	b = append(b, 0x1a, 2, 0x08, 5)       // 3: {1: 5}
	b = append(b, 0x22, 4, 1, 2, 0xac, 2) // 4: packed [1 2 300]
	b = append(b, 0x2d)                   // 5: float 1.5
	b = binary.LittleEndian.AppendUint32(b, math.Float32bits(1.5))
	b = append(b, 0x31) // 6: double 2.25
	b = binary.LittleEndian.AppendUint64(b, math.Float64bits(2.25))
	b = append(b, 0x3a, 2, 0x08, 7, 0x3a, 2, 0x08, 8)                               // 7: {1: 7}, {1: 8}
	b = append(b, 0x40, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01) // 8: -1
	b = append(b, 0x20, 9, 0x20, 10)                                                // 4 again, unpacked: 9, 10
	b = append(b, 0x08, 0x2a)                                                       // 1 again: 42 (last wins)
	m := Decode(b)
	if v := m.Uint(1); v != 42 {
		t.Errorf("Uint(1) = %d, want 42 (the last value)", v)
	}
	if s := m.Str(2); s != "hi" {
		t.Errorf("Str(2) = %q", s)
	}
	if v := m.Msg(3).Uint(1); v != 5 || m.Ref(3) != 5 {
		t.Errorf("Msg(3).Uint(1) = %d", v)
	}
	if v := m.Uints(4); len(v) != 5 || v[2] != 300 || v[4] != 10 {
		t.Errorf("Uints(4) = %v", v)
	}
	if v := m.Float(5); v != 1.5 {
		t.Errorf("Float(5) = %v", v)
	}
	if v := m.Double(6); v != 2.25 {
		t.Errorf("Double(6) = %v", v)
	}
	if v := m.Refs(7); len(v) != 2 || v[0] != 7 || v[1] != 8 {
		t.Errorf("Refs(7) = %v", v)
	}
	if v := m.Int(8); v != -1 {
		t.Errorf("Int(8) = %d", v)
	}
	if m.Has(9) || m.Msg(9) != nil || m.Str(9) != "" {
		t.Error("absent field 9 reported present")
	}
	// Truncated input keeps the fields before the damage.
	if m := Decode(append([]byte{0x08, 1}, 0x12, 50, 'x')); len(m) != 1 || m.Uint(1) != 1 {
		t.Errorf("truncated message decoded to %v", m)
	}
}

func TestDecimal128(t *testing.T) {
	enc := func(mant uint64, exp int, neg bool) []byte {
		b := make([]byte, 16)
		binary.LittleEndian.PutUint64(b, mant)
		e := exp + 6176
		b[14] = byte(e&0x7f) << 1
		b[15] = byte(e >> 7)
		if neg {
			b[15] |= 0x80
		}
		return b
	}
	for _, tt := range []struct {
		mant uint64
		exp  int
		neg  bool
		want float64
	}{{5, 0, false, 5}, {125, -1, true, -12.5}, {825, -4, false, 0.0825}, {3, 3, false, 3000}} {
		if got := decimal128(enc(tt.mant, tt.exp, tt.neg)); got != tt.want {
			t.Errorf("decimal128(%d e%d) = %v, want %v", tt.mant, tt.exp, got, tt.want)
		}
	}
}

func TestDecodeCell(t *testing.T) {
	// Version 5, a number with a decimal value, a cell style and a
	// number format.
	b := []byte{5, byte(CellNumber), 0, 0, 0, 0, 0, 0}
	b = binary.LittleEndian.AppendUint32(b, 0x1|0x20|0x2000)
	dec := make([]byte, 16)
	dec[0] = 42
	dec[14], dec[15] = byte(6176&0x7f)<<1, byte(6176>>7)
	b = append(b, dec...)
	b = binary.LittleEndian.AppendUint32(b, 3) // cell style key
	b = binary.LittleEndian.AppendUint32(b, 7) // number format key
	c, ok := decodeCell(b)
	if !ok || c.Type != CellNumber || c.Num != 42 || c.cellStyle != 3 || c.formats[FmtNumber] != 7 || c.str != -1 {
		t.Errorf("decodeCell = %+v %+v, %v", c, c.Cell, ok)
	}
	// A date: seconds since 2001.
	b = []byte{5, byte(CellDate), 0, 0, 0, 0, 0, 0}
	b = binary.LittleEndian.AppendUint32(b, 0x4)
	b = binary.LittleEndian.AppendUint64(b, math.Float64bits(86400))
	if c, ok := decodeCell(b); !ok || c.Seconds != 86400 {
		t.Errorf("date cell = %+v, %v", c.Cell, ok)
	}
	if _, ok := decodeCell([]byte{4, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); ok {
		t.Error("cell storage version 4 accepted")
	}
}

func TestExcelDateFormat(t *testing.T) {
	for in, want := range map[string]string{
		"M/d/yy":             "m/d/yy",
		"MMM d, y":           "mmm d, yyyy",
		"EEEE, MMMM d, yyyy": "dddd, mmmm d, yyyy",
		"h:mm a":             "h:mm AM/PM",
		"HH:mm:ss":           "hh:mm:ss",
		"d 'de' MMMM":        `d "de" mmmm`,
		"d":                  "d",
		"yyyy-MM-dd'T'HH:mm": `yyyy-mm-dd"T"hh:mm`,
	} {
		if got := ExcelDateFormat(in); got != want {
			t.Errorf("ExcelDateFormat(%q) = %q, want %q", in, got, want)
		}
	}
	d := time.Date(2026, 10, 28, 15, 4, 5, 0, time.UTC)
	for in, want := range map[string]string{
		"M/d/yy":      "10/28/26",
		"MMM d, y":    "Oct 28, 2026",
		"EEEE h:mm a": "Wednesday 3:04 PM",
		"dd/MM/yyyy":  "28/10/2026",
	} {
		if got := FormatDateTime(d, in); got != want {
			t.Errorf("FormatDateTime(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	for _, tt := range []struct {
		v         float64
		places    int
		thousands bool
		neg       uint64
		want      string
	}{
		{5500, 2, true, 0, "5,500.00"},
		{1234567.891, 1, true, 0, "1,234,567.9"},
		{-3.5, autoPlaces, false, 0, "-3.5"},
		{-3.5, 2, false, 2, "(3.50)"},
		{0.1 + 0.2, autoPlaces, false, 0, "0.3"},
		{55, autoPlaces, false, 0, "55"},
	} {
		if got := formatNumber(tt.v, tt.places, tt.thousands, tt.neg); got != tt.want {
			t.Errorf("formatNumber(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestFontFamily(t *testing.T) {
	for _, tt := range []struct {
		in           string
		family       string
		bold, italic bool
	}{
		{"HelveticaNeue-Bold", "Helvetica Neue", true, false},
		{"HelveticaNeue", "Helvetica Neue", false, false},
		{"Georgia-BoldItalic", "Georgia", true, true},
		{"TimesNewRomanPSMT", "Times New Roman", false, false},
		{"GillSans-Light", "Gill Sans", false, false},
		{"AvenirNext-Regular", "Avenir Next", false, false},
		{"Didot", "Didot", false, false},
	} {
		f, b, i := FontFamily(tt.in)
		if f != tt.family || b != tt.bold || i != tt.italic {
			t.Errorf("FontFamily(%q) = %q %v %v", tt.in, f, b, i)
		}
	}
}

func TestOpenErrors(t *testing.T) {
	mk := func(names ...string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			w, _ := zw.Create(n)
			w.Write([]byte("x"))
		}
		zw.Close()
		return buf.Bytes()
	}
	if _, err := Open(mk("index.xml.gz", "QuickLook/Thumbnail.jpg")); !errors.Is(err, ErrOldFormat) {
		t.Errorf("iWork '09 package: %v", err)
	}
	if !IsPackage(mk("index.xml.gz")) || IsPackage(mk("word/document.xml")) {
		t.Error("IsPackage misjudged a package")
	}
	if _, err := Open(mk("word/document.xml")); !errors.Is(err, ErrNotIWork) {
		t.Errorf("a .docx: %v", err)
	}
}

func TestOpenSamples(t *testing.T) {
	for name, app := range map[string]App{
		"Sample1.numbers": Numbers, "Sample2.numbers": Numbers, "Sample3.numbers": Numbers,
		"PagesSample1.pages": Pages, "PagesInvoice.pages": Pages, "PagesBrochure.pages": Pages,
		"Avery5371BusinessCardsWide.pages": Pages,
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "Samples", name))
		if err != nil {
			t.Skip(err)
		}
		d, err := Open(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if d.App != app {
			t.Errorf("%s: app %v, want %v", name, d.App, app)
		}
		if got := Detect(bytes.NewReader(data), int64(len(data))); got != app {
			t.Errorf("%s: Detect = %v", name, got)
		}
	}
}

func TestSampleTable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Samples", "PagesInvoice.pages"))
	if err != nil {
		t.Skip(err)
	}
	d, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	ids := d.OfType(TypeTableModel)
	if len(ids) != 1 {
		t.Fatalf("%d tables", len(ids))
	}
	tb, err := d.TableModel(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if tb.Name != "Details" || tb.Rows != 8 || tb.Cols != 4 || tb.HeaderRows != 1 || tb.FooterRows != 3 {
		t.Errorf("table %q %dx%d headers %d footers %d", tb.Name, tb.Rows, tb.Cols, tb.HeaderRows, tb.FooterRows)
	}
	for _, tt := range []struct {
		row, col int
		want     string
	}{{0, 0, "Description"}, {1, 1, "55"}, {1, 2, "$100.00"}, {1, 3, "$5,500.00"}, {6, 2, "8.25%"}, {7, 3, "$8,573.40"}} {
		c := tb.Cell(tt.row, tt.col)
		if c == nil {
			t.Errorf("cell %d,%d missing", tt.row, tt.col)
			continue
		}
		if got := d.Display(c); got != tt.want {
			t.Errorf("cell %d,%d = %q, want %q", tt.row, tt.col, got, tt.want)
		}
	}
	if c := tb.Cell(1, 3); c.Formula == nil {
		t.Error("cell D2's formula is missing")
	}
}

// Damaged objects must not crash the readers: every object of the samples
// is cut short in turn and everything that reads objects runs over it.
func TestDamagedObjects(t *testing.T) {
	for _, name := range []string{"Sample3.numbers", "PagesInvoice.pages"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "Samples", name))
		if err != nil {
			t.Skip(err)
		}
		for _, cut := range []int{2, 3, 5} {
			d, err := Open(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range d.Objects {
				var raw []byte
				for _, f := range o.Msg {
					raw = appendField(raw, f)
				}
				o.Msg = Decode(raw[:len(raw)*(cut-1)/cut])
			}
			for id, o := range d.Objects {
				d.Style(id)
				d.Paragraphs(id)
				d.ListLevels(id)
				d.Geometry(id)
				if tb, err := d.TableModel(id); err == nil {
					for _, c := range tb.Cells {
						d.Display(c)
						d.ExcelFormat(c.Format())
					}
				}
				_ = o
			}
		}
	}
}

// appendField re-encodes one field.
func appendField(b []byte, f Field) []byte {
	b = binary.AppendUvarint(b, uint64(f.Num<<3|f.Wire))
	switch f.Wire {
	case wireVarint:
		return binary.AppendUvarint(b, f.V)
	case wireFixed32:
		return binary.LittleEndian.AppendUint32(b, uint32(f.V))
	case wireFixed64:
		return binary.LittleEndian.AppendUint64(b, f.V)
	}
	b = binary.AppendUvarint(b, uint64(len(f.B)))
	return append(b, f.B...)
}

// A claimed length far beyond what the input could expand to must be
// rejected before anything is allocated for it.
func TestSnappyLengthBound(t *testing.T) {
	in := binary.AppendUvarint(nil, 1<<29)
	in = append(in, 0, 'a')
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := snappyDecode(in)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Error("an impossible length was accepted")
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Errorf("allocated %d bytes for a %d-byte block", n, len(in))
	}
}

// A rich-text payload whose storage reference points back at itself (or at
// another payload) overflowed the stack.
func TestParagraphsPayloadCycle(t *testing.T) {
	ref := func(id uint64) Message { // field 1 = a reference to id
		inner := binary.AppendUvarint([]byte{1<<3 | wireVarint}, id)
		return Message{{Num: 1, Wire: wireBytes, B: inner}}
	}
	d := &Document{Objects: map[uint64]*Object{
		1: {ID: 1, Type: TypeRichTextPayload, Msg: ref(1)},
		2: {ID: 2, Type: TypeRichTextPayload, Msg: ref(3)},
		3: {ID: 3, Type: TypeRichTextPayload, Msg: ref(2)},
	}}
	for id := uint64(1); id <= 3; id++ {
		if p := d.Paragraphs(id); p != nil {
			t.Errorf("Paragraphs(%d) = %v", id, p)
		}
		d.PlainText(id)
	}
}
