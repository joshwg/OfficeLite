package formula

import "testing"

func TestAddFuturePrefixes(t *testing.T) {
	for in, want := range map[string]string{
		"SUM(A1:A3)":                         "SUM(A1:A3)",
		"XLOOKUP(1,A:A,B:B)":                 "_xlfn.XLOOKUP(1,A:A,B:B)",
		"_xlfn.XLOOKUP(1,A:A,B:B)":           "_xlfn.XLOOKUP(1,A:A,B:B)",
		"filter(A1:A9, A1:A9>2)":             "_xlfn._xlws.filter(A1:A9, A1:A9>2)",
		"IFERROR(1/0,\"XLOOKUP(\")":          "IFERROR(1/0,\"XLOOKUP(\")",
		"LET(x, 2, y, {1,2}, x*SUM(y))":      "_xlfn.LET(_xlpm.x, 2, _xlpm.y, {1,2}, _xlpm.x*SUM(_xlpm.y))",
		"MAP(A1:A3,LAMBDA(v,v+1))":           "_xlfn.MAP(A1:A3,_xlfn.LAMBDA(_xlpm.v,_xlpm.v+1))",
		"_xlfn.LET(_xlpm.x,1,_xlpm.x+Rate)":  "_xlfn.LET(_xlpm.x,1,_xlpm.x+Rate)",
		"CONCAT(A1,TEXTJOIN(\",\",1,B:B))":   "_xlfn.CONCAT(A1,_xlfn.TEXTJOIN(\",\",1,B:B))",
		"bad(":                               "bad(",
		"GROUPBY(A2:A9,B2:B9,SUM)":           "_xlfn.GROUPBY(A2:A9,B2:B9,_xleta.SUM)",
		"GROUPBY(A:A,B:B,HSTACK(MAX,VAR.S))": "_xlfn.GROUPBY(A:A,B:B,_xlfn.HSTACK(_xleta.MAX,_xleta.VAR.S))",
		"SUM(Rate,HSTACK(Rate))":             "SUM(Rate,_xlfn.HSTACK(Rate))",
		"_xlfn.BYROW(A1:B3,_xleta.SUM)":      "_xlfn.BYROW(A1:B3,_xleta.SUM)",
		"LET(sum,2,sum+Total)":               "_xlfn.LET(_xlpm.sum,2,_xlpm.sum+Total)",
	} {
		if got := AddFuturePrefixes(in); got != want {
			t.Errorf("AddFuturePrefixes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokensPositions(t *testing.T) {
	src := "SUM( 'My Sheet'!$A$1 , x )"
	toks, err := Tokens(src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tk := range toks {
		got = append(got, src[tk.Pos:tk.End])
	}
	want := []string{"SUM", "(", "'My Sheet'!$A$1", ",", "x", ")"}
	if len(got) != len(want) {
		t.Fatalf("tokens %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
	if toks[2].Kind != TokRef || toks[2].Sheet != "My Sheet" || toks[2].Text != "$A$1" {
		t.Errorf("ref token = %+v", toks[2])
	}
}
