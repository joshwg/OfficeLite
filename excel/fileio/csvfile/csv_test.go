package csvfile

import (
	"testing"

	"officelite/excel/model"
)

func TestRead(t *testing.T) {
	cases := []struct {
		name, data string
		want       [][]string
	}{
		{"comma.csv", "a,b,c\n1,\"x,y\",3\n", [][]string{{"a", "b", "c"}, {"1", "x,y", "3"}}},
		{"semi.csv", "a;b;c\n1;2;3\n", [][]string{{"a", "b", "c"}, {"1", "2", "3"}}},
		{"tab.txt", "a\tb\n1\t2\n", [][]string{{"a", "b"}, {"1", "2"}}},
		{"sep.csv", "sep=|\na|b\n", [][]string{{"a", "b"}}},
		{"bom.csv", "\xEF\xBB\xBFhé,x\n", [][]string{{"hé", "x"}}},
		{"cp1252.csv", "caf\xE9,1\n", [][]string{{"café", "1"}}},
		{"multi.csv", "\"line1\nline2\",z\n", [][]string{{"line1\nline2", "z"}}},
	}
	for _, c := range cases {
		wb, err := Read(c.name, []byte(c.data))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		sh := wb.Sheets[0]
		for r, row := range c.want {
			for col, want := range row {
				if got := sh.Get(r, col).Text; got != want {
					t.Errorf("%s %s = %q, want %q", c.name, model.CellName(r, col), got, want)
				}
			}
		}
	}
}

func TestCellTyping(t *testing.T) {
	for _, c := range []struct {
		in   string
		text string
		kind model.Kind
		num  float64
	}{
		{"007", "7", model.KindNumber, 7},
		{"1.50", "1.5", model.KindNumber, 1.5},
		{"-2e3", "-2000", model.KindNumber, -2000},
		{"1,234", "1,234", model.KindNumber, 1234},
		{"$5.00", "$5.00", model.KindNumber, 5},
		{"12%", "12%", model.KindNumber, 0.12},
		{"(3)", "(3)", model.KindNumber, -3},
		{"true", "TRUE", model.KindBool, 1},
		{"1,23", "1,23", model.KindText, 0},
		{"NaN", "NaN", model.KindText, 0},
		{"Inf", "Inf", model.KindText, 0},
		{"abc", "abc", model.KindText, 0},
		{"=", "=", model.KindText, 0},
		{"1/15/2026", "1/15/2026", model.KindNumber, 46037},
		{"2026-01-15", "2026-01-15", model.KindNumber, 46037},
		{"13:30", "13:30", model.KindNumber, 0.5625},
	} {
		got := csvCell(c.in)
		if got.Text != c.text || got.Kind != c.kind || got.Num != c.num {
			t.Errorf("csvCell(%q) = %q kind %d num %v; want %q kind %d num %v",
				c.in, got.Text, got.Kind, got.Num, c.text, c.kind, c.num)
		}
	}
}

func TestFormulaCell(t *testing.T) {
	c := csvCell("=SUM(A1:A3)")
	if c.Formula != "SUM(A1:A3)" || !c.Dynamic || c.Kind != model.KindEmpty {
		t.Errorf("csvCell formula = %+v", c)
	}
}

func TestLimits(t *testing.T) {
	wide := make([]byte, 0, 3*(model.MaxCols+5))
	for i := 0; i < model.MaxCols+5; i++ {
		wide = append(wide, "1,"...)
	}
	wb, err := Read("wide.csv", wide)
	if err != nil {
		t.Fatal(err)
	}
	if got := wb.Sheets[0].UsedCols(); got != model.MaxCols {
		t.Errorf("used cols = %d, want %d", got, model.MaxCols)
	}
	if len(wb.Warnings) != 1 {
		t.Errorf("warnings = %v", wb.Warnings)
	}
}

// TestDecoratedNumbers checks which decorated text reads as a number.
func TestDecoratedNumbers(t *testing.T) {
	for in, want := range map[string]float64{"(100)": -100, "-$1,234.5": -1234.5, "12%": 0.12, "1,000": 1000} {
		if got, ok := parseDecoratedNumber(in); !ok || got != want {
			t.Errorf("parseDecoratedNumber(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"(-100)", "123.4,5", "1,23", ",5", "abc"} {
		if v, ok := parseDecoratedNumber(in); ok {
			t.Errorf("parseDecoratedNumber(%q) = %v, want text", in, v)
		}
	}
}
