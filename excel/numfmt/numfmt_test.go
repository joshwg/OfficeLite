package numfmt

import "testing"

func TestGeneral(t *testing.T) {
	for _, c := range []struct {
		v    float64
		want string
	}{
		{0, "0"},
		{42, "42"},
		{-1.5, "-1.5"},
		{1.0 / 3, "0.333333333"},
		{-1.0 / 3, "-0.333333333"},
		{123456.789012345, "123456.789"},
		{12345678901, "12345678901"},
		{123456789012, "1.23457E+11"},
		{0.0000000001, "1E-10"},
		{1e20, "1E+20"},
	} {
		if got := General(c.v); got != c.want {
			t.Errorf("General(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	nf := New(false)
	defer nf.Close()
	for _, c := range []struct {
		v    float64
		id   int
		code string
		want string
	}{
		{1234.5, 4, "", "1,234.50"},
		{0.256, 10, "", "25.60%"},
		{46310, 14, "", "10/15/2026"},
		{1234.5, 7, "", "$1,234.50 "},
		{-1234.5, 7, "", "($1,234.50)"},
		{46310, 0, "dd-mmm-yyyy", "15-Oct-2026"},
		{-5, 0, "0.0;(0.0)", "(5.0)"},
	} {
		if got := nf.Format(c.v, c.id, c.code); got != c.want {
			t.Errorf("Format(%v, %d, %q) = %q, want %q", c.v, c.id, c.code, got, c.want)
		}
	}
}

func TestFormat1904(t *testing.T) {
	nf := New(true)
	defer nf.Close()
	for _, c := range []struct {
		v    float64
		id   int
		code string
		want string
	}{
		{1, 0, "yyyy-mm-dd", "1904-01-02"},
		{0, 14, "", "1/1/1904"},
		{1.5, 0, "h:mm", "12:00"}, // time of day: no shift
		{1.5, 0, "[h]", "36"},     // elapsed hours: no shift
		{1.5, 0, "0.00", "1.50"},  // plain number: no shift
		{44000, 0, "mmm yyyy", "Jun 2024"},
	} {
		if got := nf.Format(c.v, c.id, c.code); got != c.want {
			t.Errorf("1904 Format(%v, %d, %q) = %q, want %q", c.v, c.id, c.code, got, c.want)
		}
	}
}
