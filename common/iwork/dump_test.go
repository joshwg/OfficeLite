// Diagnostic dumps of a document's raw objects and tables, run on demand via
// $IWORK_DUMP.

package iwork

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestDump prints every object of the document named by $IWORK_DUMP as
// raw protobuf fields, for working out the format:
//
//	IWORK_DUMP=Samples/Sample2.numbers go test ./common/iwork -run Dump -v
func TestDump(t *testing.T) {
	path := os.Getenv("IWORK_DUMP")
	if path == "" {
		t.Skip("set IWORK_DUMP to a document")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]uint64, 0, len(d.Objects))
	for id := range d.Objects {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var b strings.Builder
	for _, id := range ids {
		o := d.Objects[id]
		fmt.Fprintf(&b, "#%d type %d (%s)\n", id, o.Type, o.File)
		dumpMsg(&b, o.Msg, 1, 0)
	}
	fmt.Print(b.String())
}

// dumpMsg writes m's fields to b, one per line at the given indent, nesting
// into bytes fields that look like messages (up to depth 12) and showing
// others as strings or hex.
func dumpMsg(b *strings.Builder, m Message, indent, depth int) {
	pad := strings.Repeat("  ", indent)
	for _, f := range m {
		switch f.Wire {
		case wireBytes:
			sub := Decode(f.B)
			if depth < 12 && len(f.B) > 0 && looksLikeMessage(f.B, sub) {
				fmt.Fprintf(b, "%s%d {\n", pad, f.Num)
				dumpMsg(b, sub, indent+1, depth+1)
				fmt.Fprintf(b, "%s}\n", pad)
			} else if utf8.Valid(f.B) && !strings.ContainsFunc(string(f.B), func(r rune) bool { return r < 32 && r != '\n' && r != '\t' }) {
				fmt.Fprintf(b, "%s%d: %q\n", pad, f.Num, string(f.B))
			} else {
				fmt.Fprintf(b, "%s%d: bytes[%d] % x\n", pad, f.Num, len(f.B), f.B[:min(len(f.B), 64)])
			}
		case wireFixed32:
			fmt.Fprintf(b, "%s%d: f32 %v (0x%x)\n", pad, f.Num, Message{f}.Float(f.Num), f.V)
		case wireFixed64:
			fmt.Fprintf(b, "%s%d: f64 %v\n", pad, f.Num, Message{f}.Float(f.Num))
		default:
			fmt.Fprintf(b, "%s%d: %d\n", pad, f.Num, int64(f.V))
		}
	}
}

// looksLikeMessage guesses whether bytes are an embedded message: they
// decode completely into plausible field numbers.
func looksLikeMessage(raw []byte, m Message) bool {
	if len(m) == 0 {
		return false
	}
	n := 0
	for _, f := range m {
		if f.Num <= 0 || f.Num > 20000 {
			return false
		}
		n += protoSize(f)
	}
	return n == len(raw)
}

// protoSize returns the encoded size of f, key included.
func protoSize(f Field) int {
	key := uvarintLen(uint64(f.Num<<3 | f.Wire))
	switch f.Wire {
	case wireVarint:
		return key + uvarintLen(f.V)
	case wireFixed32:
		return key + 4
	case wireFixed64:
		return key + 8
	}
	return key + uvarintLen(uint64(len(f.B))) + len(f.B)
}

// uvarintLen returns the number of bytes v takes as a varint.
func uvarintLen(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}

// TestDumpTables prints the tables of $IWORK_DUMP cell by cell.
func TestDumpTables(t *testing.T) {
	path := os.Getenv("IWORK_DUMP")
	if path == "" {
		t.Skip("set IWORK_DUMP to a document")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range d.OfType(TypeTableModel) {
		tb, err := d.TableModel(id)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("table %d %q %dx%d headers %d/%d/%d merges %v warnings %v\n", id, tb.Name, tb.Rows, tb.Cols,
			tb.HeaderRows, tb.HeaderCols, tb.FooterRows, tb.Merges, tb.Warnings)
		for r := range tb.Rows {
			for c := range tb.Cols {
				cell := tb.Cell(r, c)
				if cell == nil {
					continue
				}
				fmt.Printf("  %d,%d type %d num %v sec %v text %q style %d/%d formula %v", r, c, cell.Type, cell.Num, cell.Seconds, cell.Text,
					cell.CellStyle, cell.TextStyle, cell.Formula != nil)
				if f := cell.Format(); f != nil {
					var b strings.Builder
					dumpMsg(&b, f, 0, 0)
					fmt.Printf(" fmt[%s]", strings.ReplaceAll(b.String(), "\n", " "))
				}
				fmt.Println()
			}
		}
	}
}
