package xlsfile

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"officelite/common/ole"
	"officelite/excel/model"
)

// brec encodes one BIFF record.
func brec(id int, body ...byte) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(id))
	b = binary.LittleEndian.AppendUint16(b, uint16(len(body)))
	return append(b, body...)
}

// compound wraps a Workbook stream in a compound file.
func compound(t *testing.T, stream []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := ole.Write(&b, []ole.Stream{{Name: "Workbook", Data: stream}}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// workbook builds a one-sheet BIFF8 stream with extra globals and sheet
// records.
func workbook(globals, sheet []byte) []byte {
	bof := func(kind int) []byte { return brec(recBOF, cat(u16b(biff8Version), u16b(kind))...) }
	g := cat(bof(5), globals)
	// BOUNDSHEET: position (patched below), visible worksheet, name "S".
	bs := brec(recBoundSheet, 0, 0, 0, 0, 0, 0, 1, 0, 'S')
	pos := len(g) + len(bs) + 4
	binary.LittleEndian.PutUint32(bs[4:], uint32(pos))
	return cat(g, bs, brec(recEOF), bof(bofTypeWorksheet), sheet, brec(recEOF))
}

// Truncated PALETTE, BOUNDSHEET and SHRFMLA records indexed past their
// bodies and panicked.
func TestTruncatedRecords(t *testing.T) {
	globals := cat(
		brec(recPalette, 56, 0, 1, 2, 3), // claims 56 colors, has one
		brec(recBoundSheet, 1, 2, 3),     // shorter than its fixed part
	)
	sheet := cat(
		brec(recFormula, make([]byte, 22)...),
		brec(recShrFmla, 0, 0, 0), // shorter than its range
	)
	wb, err := Read(compound(t, workbook(globals, sheet)))
	if err != nil {
		t.Fatal(err)
	}
	if len(wb.Sheets) != 1 || wb.Sheets[0].Name != "S" {
		t.Errorf("sheets = %v", wb.Sheets)
	}
}

// Merges written bottom-right first are normalized, and a saved view out
// of the sheet's limits is clamped (it used to reach the writers as is).
func TestReversedMergeAndView(t *testing.T) {
	merge := cat(u16b(1), u16b(5), u16b(2), u16b(3), u16b(1)) // rows 5..2, cols 3..1
	sheet := cat(
		brec(recMergedCells, merge...),
		brec(recWindow2, cat(u16b(0x08), u16b(0), u16b(0xFFFF))...),               // frozen, left col 65535
		brec(recPane, cat(u16b(0xFFFF), u16b(3), u16b(0), u16b(0), []byte{0})...), // 65535 frozen cols
		brec(recSelection, append([]byte{0}, cat(u16b(7), u16b(0xFFFF))...)...),
	)
	wb, err := Read(compound(t, workbook(nil, sheet)))
	if err != nil {
		t.Fatal(err)
	}
	sh := wb.Sheets[0]
	if want := (model.Range{R1: 2, C1: 1, R2: 5, C2: 3}); len(sh.Merges) != 1 || sh.Merges[0] != want {
		t.Errorf("merges = %v, want %v", sh.Merges, want)
	}
	if sh.LeftCol != model.MaxCols-1 || sh.FreezeCols != model.MaxCols-1 || sh.FreezeRows != 3 ||
		sh.ActiveRow != 7 || sh.ActiveCol != model.MaxCols-1 {
		t.Errorf("view = left %d, freeze %d×%d, active %d,%d", sh.LeftCol, sh.FreezeRows, sh.FreezeCols, sh.ActiveRow, sh.ActiveCol)
	}
}

// A chart embedded in a sheet (a nested chart substream) is reported, not
// silently dropped.
func TestEmbeddedChartWarning(t *testing.T) {
	bof := func(kind int) []byte { return brec(recBOF, cat(u16b(biff8Version), u16b(kind))...) }
	sheet := cat(bof(bofTypeChart), brec(0x1001, 0, 0), brec(recEOF))
	wb, err := Read(compound(t, workbook(nil, sheet)))
	if err != nil {
		t.Fatal(err)
	}
	if len(wb.Warnings) != 1 || !strings.Contains(wb.Warnings[0], "charts can't be read") {
		t.Errorf("warnings = %q", wb.Warnings)
	}
}
