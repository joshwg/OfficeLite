package formula

import "testing"

func TestNormalize(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"sum(a1:b2)", "SUM(A1:B2)"},
		{"a1+$b$2*sheet2!c3", "A1+$B$2*sheet2!C3"},
		{`if(a1="abc",true,false)`, `IF(A1="abc",TRUE,FALSE)`},
		{"'my sheet'!a1&TaxRate", "'my sheet'!A1&TaxRate"},
		{"sum(a:a)+sum(1:2)", "SUM(A:A)+SUM(1:2)"},
		{"let(x,1,x+1)", "LET(x,1,x+1)"},
		{"sum(a1:a3", "SUM(A1:A3)"},
		{"round(sum(a1,2", "ROUND(SUM(A1,2))"},
		{"log10(100)", "LOG10(100)"},
	} {
		got, err := Normalize(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"1+", "A1)", `"abc`, "SUM(1+)"} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%q): no error", bad)
		}
	}
}
