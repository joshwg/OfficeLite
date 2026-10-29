package calc

import (
	"testing"

	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

func checkTexts(t *testing.T, wb *model.Workbook, want map[string]string) {
	t.Helper()
	for ref, w := range want {
		if got := get(wb, ref).Text; got != w {
			t.Errorf("%s (%s) = %q, want %q", ref, get(wb, ref).Formula, got, w)
		}
	}
}

// Whole columns and rows include what dynamic arrays spill into them, even
// when those are calculated after the formula reading them.
func TestWholeColumnSeesLaterSpills(t *testing.T) {
	cells := map[string]model.Cell{
		"A1":  fx("SUM(B:B)"),
		"B1":  dyn("SEQUENCE(10)"),
		"D1":  fx("SUM(12:12)"),
		"D12": dyn("SEQUENCE(1,5)"),
		// Right of column B: not calculated early, so not circular.
		"F1": dyn("SEQUENCE(A1/55)"),
	}
	want := map[string]string{"A1": "55", "D1": "15", "F1": "1"}
	wb := book(cells)
	if res := Recalculate(wb, functions.Default(), Full, now); len(res.Circular) > 0 {
		t.Errorf("circular: %v", res.Circular)
	}
	checkTexts(t, wb, want)

	wb = book(cells)
	NewCalculator(wb, functions.Default()).Full(now)
	checkTexts(t, wb, want)
}

// Whole columns on different sheets combine element by element.
func TestWholeColumnsOnTwoSheets(t *testing.T) {
	wb := book(map[string]model.Cell{
		"B1": num(10), "B2": num(20), "B3": num(30), "B4": num(40), "B5": num(50),
		"Z10": num(1),
		"C1":  fx("SUMPRODUCT((Data!A:A>1)*B:B)"),
		"C2":  dyn("SUM(Data!A:A*B:B)"),
		"D1":  dyn("FILTER(B:B,Data!A:A>3)"),
		"E1":  dyn("SORTBY(B:B,Data!A:A,-1)"),
	})
	for i := 0; i < 5; i++ {
		wb.Sheets[1].Set(i, 0, num(float64(i+1)))
	}
	Recalculate(wb, functions.Default(), Full, now)
	checkTexts(t, wb, map[string]string{"C1": "140", "C2": "550", "D1": "40", "D2": "50", "E1": "50"})
}

// Ordinary (pre-dynamic-array) formulas intersect a range given where one
// value is expected; functions that take arrays still get arrays.
func TestLegacyFormulas(t *testing.T) {
	wb := book(map[string]model.Cell{
		"B1": num(-1), "B2": num(-2), "B3": num(3),
		"A2": fx("B1:B3*2"),
		"C2": fx("ABS(B1:B3)"),
		"D2": fx("SUM(B1:B3*2)"),
		"E2": fx("SUMPRODUCT(B1:B3*2)"),
		"F2": fx("SUM(B1:B3)"),
		"G2": fx("LOOKUP(2,1/(B1:B3>0),B1:B3)"),
		"H2": fx("INDEX(B1:B3*2,3)"),
		"I2": dyn("SUM(B1:B3*2)"),
		"J2": fx("B1:B3"),
		"K2": fx("IF(B1:B3<0,\"neg\",\"pos\")"),
		"L2": fx("SUMPRODUCT(ABS(B1:B3))"),
		"A5": fx("B1:B3*2"),
		"A6": fx("{7,8}*2"),
		"M2": fx("COUNTIF(B1:B3,\">0\")"),
		"N2": fx("@B1:B3"),
	})
	Recalculate(wb, functions.Default(), Full, now)
	checkTexts(t, wb, map[string]string{
		"A2": "-4", "C2": "2", "D2": "-4", "E2": "0", "F2": "0", "G2": "3", "H2": "6",
		"I2": "0", "J2": "-2", "K2": "neg", "L2": "6", "A5": "#VALUE!", "A6": "14",
		"M2": "1", "N2": "-2",
	})
}

func TestReferenceFunctions(t *testing.T) {
	wb := book(map[string]model.Cell{
		"A1": num(2),
		"A2": fx("A1*3"),
		"B1": fx("FORMULATEXT(A2)"),
		"B2": fx("FORMULATEXT(A1)"),
		"B3": fx("AREAS(A1:C9)"),
		"B4": fx("HYPERLINK(\"https://example.com\",\"Example\")"),
		"B5": fx("HYPERLINK(\"https://example.com\")"),
		"B6": fx("ISNA(FORMULATEXT(A1))"),
	})
	Recalculate(wb, functions.Default(), Full, now)
	checkTexts(t, wb, map[string]string{
		"B1": "=A1*3", "B2": "#N/A", "B3": "1", "B4": "Example", "B5": "https://example.com", "B6": "TRUE",
	})
}
