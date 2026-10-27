package formula

import (
	"testing"

	"officelite/excel/model"
)

type adjustCase struct{ in, want string }

func checkAdjust(t *testing.T, what string, cases []adjustCase, fn func(string) string) {
	t.Helper()
	for _, c := range cases {
		got := fn(c.in)
		if got != c.want {
			t.Errorf("%s(%q) = %q, want %q", what, c.in, got, c.want)
		}
		if _, err := Parse(got); err != nil {
			t.Errorf("%s(%q) = %q does not parse: %v", what, c.in, got, err)
		}
	}
}

func TestAdjustRowsInsert(t *testing.T) {
	// Two rows inserted before row 5 (index 4) on Sheet1.
	checkAdjust(t, "insert rows", []adjustCase{
		{"A1+A4", "A1+A4"},
		{"A5+B10", "A7+B12"},
		{"$A$5*A$6", "$A$7*A$8"},
		{"SUM(A1:A10)", "SUM(A1:A12)"}, // spans the insertion: grows
		{"SUM(A5:B6)", "SUM(A7:B8)"},   // below: moves
		{"SUM(A1:A4)", "SUM(A1:A4)"},   // above: unchanged
		{"SUM(A:A)", "SUM(A:A)"},       // whole columns ignore row ops
		{"SUM(3:5)", "SUM(3:7)"},       // whole rows grow
		{"SUM($5:$6)", "SUM($7:$8)"},   // and keep their $ markers
		{"Sheet1!A5+sheet1!A5", "Sheet1!A7+sheet1!A7"},
		{"Sheet2!A5+A5", "Sheet2!A5+A7"}, // other sheet untouched
		{`"A5"&A5`, `"A5"&A7`},           // strings untouched
		{"LOG10(A5)+ATAN2(A5,1)", "LOG10(A7)+ATAN2(A7,1)"},
		{"ABC12X+A5", "ABC12X+A7"},
		{"_xlfn.XLOOKUP(A5,B:B,C:C)", "_xlfn.XLOOKUP(A7,B:B,C:C)"},
		{"A5#", "A7#"},
		{"TRUE+A5 + 1.5E3", "TRUE+A7 + 1.5E3"},
		{"IFERROR(A5,#N/A)+#REF!", "IFERROR(A7,#N/A)+#REF!"},
		{"A1048576", "#REF!"}, // pushed off the sheet
		{"SUM(A1:A1048576)", "SUM(A1:A1048576)"},
		{"Sheet1!A5:Sheet1!A6", "Sheet1!A7:A8"},
	}, func(f string) string { return AdjustRows(f, "Sheet1", "Sheet1", 4, 2) })

	// A formula on another sheet only follows qualified references.
	checkAdjust(t, "insert rows elsewhere", []adjustCase{
		{"A5+Sheet1!A5", "A5+Sheet1!A7"},
		{"'Sheet1'!A5:B5", "'Sheet1'!A7:B7"},
	}, func(f string) string { return AdjustRows(f, "Other", "Sheet1", 4, 2) })

	checkAdjust(t, "quoted sheet", []adjustCase{
		{"'My Sheet'!A5+'my sheet'!$B$9", "'My Sheet'!A6+'my sheet'!$B$10"},
		{"'It''s'!A5", "'It''s'!A5"},
		{"A5", "A5"}, // defined name: home "" so unqualified refs don't match
	}, func(f string) string { return AdjustRows(f, "", "My Sheet", 0, 1) })
}

func TestAdjustRowsDelete(t *testing.T) {
	// Rows 5:6 (indexes 4, 5) deleted.
	checkAdjust(t, "delete rows", []adjustCase{
		{"A4", "A4"},
		{"A5", "#REF!"},
		{"Sheet1!A6+1", "#REF!+1"},
		{"A7*$B$10", "A5*$B$8"},
		{"SUM(A1:A10)", "SUM(A1:A8)"}, // shrinks
		{"SUM(A5:A6)", "SUM(#REF!)"},  // wholly deleted
		{"SUM(A5:A8)", "SUM(A5:A6)"},  // start inside: clipped to the block start
		{"SUM(A2:A5)", "SUM(A2:A4)"},  // end inside: clipped above it
		{"SUM(B8:A3)", "SUM(B6:A3)"},  // reversed corners keep their order
		{"SUM(5:6)", "SUM(#REF!)"},
		{"SUM(4:9)", "SUM(4:7)"},
		{"SUM(C:D)", "SUM(C:D)"},
	}, func(f string) string { return AdjustRows(f, "Sheet1", "Sheet1", 4, -2) })
}

func TestAdjustCols(t *testing.T) {
	// One column inserted before C.
	checkAdjust(t, "insert cols", []adjustCase{
		{"B1+C1+$D$2", "B1+D1+$E$2"},
		{"SUM(A1:C3)", "SUM(A1:D3)"},
		{"SUM(B:C)", "SUM(B:D)"},
		{"SUM(1:2)", "SUM(1:2)"},
		{"XFD1", "#REF!"},
	}, func(f string) string { return AdjustCols(f, "Sheet1", "Sheet1", 2, 1) })

	// Column C deleted.
	checkAdjust(t, "delete cols", []adjustCase{
		{"B1+C1+D1", "B1+#REF!+C1"},
		{"SUM(A1:E1)", "SUM(A1:D1)"},
		{"SUM(C1:C9)", "SUM(#REF!)"},
		{"SUM($C:$E)", "SUM($C:$D)"},
		{"VLOOKUP(A1,Data!A:D,2,FALSE)", "VLOOKUP(A1,Data!A:D,2,FALSE)"},
	}, func(f string) string { return AdjustCols(f, "Sheet1", "Sheet1", 2, -1) })
}

func TestMoveRefs(t *testing.T) {
	src := model.Range{R1: 0, C1: 0, R2: 1, C2: 1} // A1:B2 moved to D11
	checkAdjust(t, "move", []adjustCase{
		{"A1+$B$2+C3", "D11+$E$12+C3"},
		{"SUM(A1:B2)", "SUM(D11:E12)"},
		{"SUM(A1:B3)", "SUM(A1:B3)"}, // only partly inside
		{"Sheet2!A1", "Sheet2!A1"},
		{"SUM(A:B)", "SUM(A:B)"},
	}, func(f string) string { return MoveRefs(f, "Sheet1", "Sheet1", src, 10, 3) })
	checkAdjust(t, "move off sheet", []adjustCase{{"A2", "#REF!"}},
		func(f string) string { return MoveRefs(f, "Sheet1", "Sheet1", src, -2, 0) })
}

func TestRenameSheetRefs(t *testing.T) {
	cases := []struct{ in, old, new, want string }{
		{"Sheet1!A1+A1", "Sheet1", "Data", "Data!A1+A1"},
		{"SHEET1!A1:B2", "Sheet1", "My Data", "'My Data'!A1:B2"},
		{"'Old Name'!$A$1", "old name", "It's", "'It''s'!$A$1"},
		{"Sheet1!A:A+Sheet1!2:3", "Sheet1", "2024", "'2024'!A:A+'2024'!2:3"},
		{"Sheet1!TaxRate*2", "Sheet1", "AB1", "'AB1'!TaxRate*2"},
		{"Sheet1!A1", "Sheet1", "R1C1", "'R1C1'!A1"},
		{"Sheet10!A1", "Sheet1", "X", "Sheet10!A1"},
		{`"Sheet1!A1"`, "Sheet1", "X", `"Sheet1!A1"`},
	}
	for _, c := range cases {
		got := RenameSheetRefs(c.in, c.old, c.new)
		if got != c.want {
			t.Errorf("RenameSheetRefs(%q, %q, %q) = %q, want %q", c.in, c.old, c.new, got, c.want)
		}
		if _, err := Parse(got); err != nil {
			t.Errorf("%q does not parse: %v", got, err)
		}
	}
}

func TestDeleteSheetRefs(t *testing.T) {
	checkAdjust(t, "delete sheet", []adjustCase{
		{"Data!A1+A1", "#REF!+A1"},
		{"SUM('data'!A1:B9)", "SUM(#REF!)"},
		{"Data!TaxRate", "#REF!"},
		{"Other!A1", "Other!A1"},
	}, func(f string) string { return DeleteSheetRefs(f, "Data") })
}

func TestQuoteSheet(t *testing.T) {
	for in, want := range map[string]string{
		"Sheet1": "Sheet1", "My Sheet": "'My Sheet'", "A1": "'A1'", "XFD9": "'XFD9'",
		"R": "'R'", "C12": "'C12'", "Data.2024": "Data.2024", "Q&A": "'Q&A'", "": "''",
	} {
		if got := QuoteSheet(in); got != want {
			t.Errorf("QuoteSheet(%q) = %q, want %q", in, got, want)
		}
	}
}
