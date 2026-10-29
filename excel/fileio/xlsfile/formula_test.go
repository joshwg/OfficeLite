package xlsfile

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"officelite/excel/model"
)

// testParser is a workbook context with three sheets, a defined name, an
// add-in functions and an external book.
func testParser() *parser {
	p := &parser{wb: &model.Workbook{}}
	for _, n := range []string{"Data", "My Sheet", "Other"} {
		p.sheets = append(p.sheets, boundSheet{name: n})
	}
	p.supbooks = []supbook{
		{kind: supSelf},
		{kind: supAddin, names: []string{"EDATE", "_xlfn.XLOOKUP", "BIN2DEC", "HEX2DEC", "OCT2DEC", "DEC2BIN",
			"DEC2HEX", "DEC2OCT", "BIN2HEX", "BIN2OCT", "HEX2BIN", "HEX2OCT", "OCT2BIN", "OCT2HEX", "CONVERT"}},
		{kind: supExternal, path: "book.xls", sheets: []string{"Ext"}, names: []string{"Rate"}, ext: 1},
	}
	p.xti = []xti{{0, 0, 0}, {0, 1, 1}, {0, 0, 2}, {1, -2, -2}, {2, 0, 0}, {0, -1, -1}}
	p.names = []definedName{{name: "TaxRate"}, {name: "_xlfn.IFERROR", flags: nameFunc | nameHidden}}
	return p
}

func u16b(v int) []byte { return binary.LittleEndian.AppendUint16(nil, uint16(v)) }

func cat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

func num(v float64) []byte {
	return binary.LittleEndian.AppendUint64([]byte{ptgNum}, math.Float64bits(v))
}

func TestDecodePtgs(t *testing.T) {
	ref := func(id byte, row, col int) []byte { return cat([]byte{id}, u16b(row), u16b(col)) }
	rel := 0xC000 // row and column relative
	for _, c := range []struct {
		name  string
		rgce  []byte
		extra []byte
		want  string
	}{
		{"int add", []byte{ptgInt, 1, 0, ptgInt, 2, 0, ptgAdd}, nil, "1+2"},
		{"operators", []byte{ptgInt, 1, 0, ptgInt, 2, 0, ptgSub, ptgInt, 3, 0, ptgMul, ptgInt, 4, 0, ptgPower, ptgUminus}, nil, "-(((1-2)*3)^4)"},
		{"compare", []byte{ptgInt, 1, 0, ptgInt, 2, 0, ptgNE, ptgBool, 1, ptgEQ}, nil, "1<>2=TRUE"},
		{"percent paren", []byte{ptgInt, 5, 0, ptgPercent, ptgParen, ptgUplus}, nil, "+(5%)"},
		{"concat str", cat([]byte{ptgStr, 2, 0, 'a', '"'}, []byte{ptgStr, 1, 1, 0xE5, 0x65, ptgConcat}), nil, `"a"""&"日"`},
		{"num err miss", cat(num(0.5), []byte{ptgErr, 0x2A, ptgMissArg, 0x42, 3, 1, 0}), nil, "IF(0.5,#N/A,)"},
		{"ref classes", cat(ref(0x24, 0, rel), ref(0x44, 1, 1), ref(0x64, 65535, 255|rel), []byte{0x42, 3, 4, 0}), nil, "SUM(A1,$B$2,IV65536)"},
		{"area whole", cat([]byte{ptgArea}, u16b(0), u16b(0xFFFF), u16b(0), u16b(1|0x4000), []byte{ptgArea}, u16b(2), u16b(4), u16b(0), u16b(255), []byte{ptgRange}), nil, "$A:B:$3:$5"},
		{"refN shared", cat(ref(ptgRefN, 0xFFFF, 0xC0FF), []byte{ptgAreaN}, u16b(0), u16b(2), u16b(rel), u16b(1|0x8000), []byte{ptgAdd}), nil, "A2+B3:$B5"},
		{"3d", cat([]byte{ptgRef3d}, u16b(1), u16b(0), u16b(0), []byte{ptgArea3d}, u16b(2), u16b(0), u16b(1), u16b(0), u16b(1), []byte{ptgAdd}), nil, "'My Sheet'!$A$1+Data:Other!$A$1:$B$2"},
		{"3d deleted", cat([]byte{ptgRef3d}, u16b(5), u16b(0), u16b(0), []byte{ptgRefErr3d}, u16b(0), u16b(0), u16b(0), []byte{ptgAreaErr}, make([]byte, 8), []byte{ptgUnion, ptgUnion, ptgParen}), nil, "(#REF!,(Data!#REF!,#REF!))"},
		{"external", cat([]byte{ptgRef3d}, u16b(4), u16b(0), u16b(0), []byte{ptgNameX}, u16b(4), u16b(1), u16b(0), []byte{ptgMul}), nil, "[1]Ext!$A$1*[1]!Rate"},
		{"name", cat([]byte{ptgName}, u16b(1), u16b(0)), nil, "TaxRate"},
		{"addin", cat([]byte{ptgNameX}, u16b(3), u16b(1), u16b(0), ref(0x24, 0, 0), []byte{ptgInt, 1, 0, 0x42, 3, 255, 0}), nil, "EDATE($A$1,1)"},
		{"future addin", cat([]byte{ptgNameX}, u16b(3), u16b(2), u16b(0), []byte{ptgInt, 1, 0, 0x42, 2, 255, 0}), nil, "_xlfn.XLOOKUP(1)"},
		{"future name", cat([]byte{ptgName}, u16b(2), u16b(0), []byte{ptgInt, 1, 0, ptgInt, 0, 0, 0x42, 3, 255, 0}), nil, "IFERROR(1,0)"},
		{"fixed func", cat(num(3.14159), []byte{ptgInt, 2, 0, 0x41, 27, 0}), nil, "ROUND(3.14159,2)"},
		{"attr sum", cat([]byte{0x25}, u16b(0), u16b(2), u16b(rel), u16b(rel), []byte{ptgAttr, attrSum, 0, 0}), nil, "SUM(A1:A3)"},
		{"attr if skip choose", cat([]byte{ptgInt, 1, 0, ptgAttr, attrChoose, 1, 0, 4, 0, 9, 0, ptgInt, 7, 0, ptgAttr, attrSkip, 3, 0, ptgInt, 8, 0, ptgAttr, attrSkip, 3, 0, 0x42, 3, 100, 0}), nil, "CHOOSE(1,7,8)"},
		{"attr space", cat([]byte{ptgInt, 1, 0, ptgAttr, attrSpace, 0, 1, ptgInt, 2, 0, ptgAttr, attrSpace, 0, 2, ptgAdd, ptgAttr, attrSpace, 2, 1, ptgAttr, attrSpace, 4, 1, ptgParen}), nil, " (1  + 2 )"},
		{"volatile", []byte{ptgAttr, attrVolatile, 0, 0, 0x41, 74, 0}, nil, "NOW()"},
		{"mem area", cat([]byte{ptgMemArea, 0, 0, 0, 0, 9, 0}, []byte{0x25}, u16b(0), u16b(1), u16b(0), u16b(0), []byte{0x42, 1, 4, 0}),
			cat(u16b(1), make([]byte, 8)), "SUM($A$1:$A$2)"},
		{"mem func", cat([]byte{ptgMemFunc, 4, 0, ptgInt, 1, 0, ptgInt, 2, 0, ptgIsect}), nil, "1 2"},
		{"array", []byte{0x60, 0, 0, 0, 0, 0, 0, 0}, cat([]byte{1, 1, 0}, []byte{0x01}, num(1.5)[1:], []byte{0x02, 1, 0, 0, 'x', 0x04, 1, 0, 0, 0, 0, 0, 0, 0, 0x10, 0x07, 0, 0, 0, 0, 0, 0, 0}), `{1.5,"x";TRUE,#DIV/0!}`},
	} {
		got, err := testParser().decodeFormula(c.rgce, c.extra, 2, 1, true)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestDecodeSpecial(t *testing.T) {
	p := testParser()
	var se errShared
	if _, err := p.decodeFormula([]byte{ptgExp, 4, 0, 2, 0}, nil, 9, 9, false); !errors.As(err, &se) || se != (errShared{4, 2}) {
		t.Errorf("tExp: %v", err)
	}
	if _, err := p.decodeFormula([]byte{ptgTbl, 4, 0, 2, 0}, nil, 9, 9, false); err != errTable {
		t.Errorf("tTbl: %v", err)
	}
	for _, bad := range [][]byte{{ptgAdd}, {0x41, 0xFF, 0x0F}, {ptgInt, 1}, {0x18, 0}, {ptgInt, 1, 0, ptgInt, 2, 0}} {
		if f, err := p.decodeFormula(bad, nil, 0, 0, false); err == nil {
			t.Errorf("% X decoded as %q", bad, f)
		}
	}
}

// fakeWriter is the encoder's context for TestEncodeDecode, matching
// testParser's sheets, names and links.
type fakeWriter struct{ p *parser }

func (f fakeWriter) sheetIndex(name string) int {
	for i, s := range f.p.sheets {
		if strings.EqualFold(s.name, name) {
			return i
		}
	}
	return -1
}

func (f fakeWriter) xtiIndex(first, last int) int {
	for i, x := range f.p.xti {
		if x == (xti{0, first, last}) {
			return i
		}
	}
	f.p.xti = append(f.p.xti, xti{0, first, last})
	return len(f.p.xti) - 1
}

func (f fakeWriter) nameIndex(name string, sheet int) int {
	if strings.EqualFold(name, "TaxRate") {
		return 1
	}
	return 0
}

func (f fakeWriter) funcNameRef(name string) (byte, []byte) {
	if i := slices.Index(f.p.supbooks[1].names, name); i >= 0 && name != "_xlfn.XLOOKUP" {
		return ptgNameX, cat(u16b(3), u16b(i+1), u16b(0)) // add-in functions
	}
	if name == "IFERROR" {
		return ptgName, cat(u16b(2), u16b(0))
	}
	return ptgNameX, cat(u16b(3), u16b(2), u16b(0)) // _xlfn.XLOOKUP
}

func TestEncodeDecode(t *testing.T) {
	for _, src := range []string{
		"1+2*3", "-(1-2)^2", "5%", `"a""b"&"ü日"`, "TRUE<>FALSE", "#N/A", "0.1+1E+20-1E-07+70000",
		"A1", "$A$1:B$2", "A:$C", "$1:3", "SUM(A1:A3,Other!B2,'My Sheet'!$C$3:D4)", "SUM(Data:Other!A1)",
		"IF(A1>0,,B1)", "VLOOKUP(2,A1:B3,2,FALSE)", "ROUND(PI(),2)", "NOW()+RAND()",
		"TaxRate*2", "EDATE(A1,1)", "IFERROR(1/0,0)", "_xlfn.XLOOKUP(1,A:A,B:B)",
		"{1,-2;\"x\",TRUE}", "SUM((A1,B2))", "SUM( A1, (B1 + 2))", "A1:INDEX(B1:B9,2)",
		"OFFSET(A1,1,2,3,4)", "SUM(OFFSET(A1,,,2))", `INDIRECT("A"&B1)`, `ROWS(INDIRECT("R1C1:R2C2",FALSE))`,
		"RANK(A1,A1:A3,1)", "TDIST(1.5,10,2)", "LINEST(A1:A3,B1:B3,TRUE,TRUE)", "BETADIST(0.5,2,3)",
		"PV(0.1,5,-100,,1)", "RATE(48,-200,8000,0,0,0.1)", "IRR(A1:A3,-0.1)", "MIRR(A1:A3,0.1,0.12)",
		"VDB(2400,300,10,0,0.875,1.5,TRUE)", "DB(1000000,100000,6,1,7)+ISPMT(0.1,1,3,8000)",
		`DSUM(A1:E7,"Profit",G1:H2)`, "DCOUNT(A1:E7,,G1:G3)+DCOUNTA(A:E,2,G:G)", `DGET(A1:E7,"Yield",G1:H2)`,
		"DPRODUCT(A1:E7,4,G1:H2)+DSTDEVP(A1:E7,4,G1:H2)+DVARP(A1:E7,4,G1:H2)",
		`BIN2DEC(1010)+HEX2DEC("FF")+OCT2DEC(17)`, `DEC2BIN(9,8)&DEC2HEX(-1)&DEC2OCT(8,3)`,
		`BIN2HEX(1110)&BIN2OCT(101)&HEX2BIN("F")&HEX2OCT("F")&OCT2BIN(7)&OCT2HEX(17)`, `CONVERT(68,"F","C")`,
	} {
		p := testParser()
		rgce, extra, err := compileFormula(fakeWriter{p}, src, 0, false)
		if err != nil {
			t.Errorf("compile %q: %v", src, err)
			continue
		}
		got, err := p.decodeFormula(rgce, extra, 0, 0, false)
		if err != nil || got != src {
			t.Errorf("%q -> % X -> %q, %v", src, rgce, got, err)
		}
	}
	for _, bad := range []string{"A70000", "IV1+IW1", "Nope!A1", "Undefined+1", "ROUND(1)", `"` + strings.Repeat("x", 256) + `"`,
		"#SPILL!", "[1]Sheet!A1", "LET(x,1,x)", "SUM(" + strings.Repeat("1,", 30) + "1)"} {
		if _, _, err := compileFormula(fakeWriter{testParser()}, bad, 0, false); err == nil {
			t.Errorf("compile %q succeeded", bad)
		}
	}
}

func TestEncodeClasses(t *testing.T) {
	for src, want := range map[string][]byte{
		"A1*2":           {0x44, 0, 0, 0, 0xC0, ptgInt, 2, 0, ptgMul},                // value class
		"SUM(A1)":        {0x24, 0, 0, 0, 0xC0, 0x42, 1, 4, 0},                       // reference argument
		"SUMPRODUCT(A1)": {0x64, 0, 0, 0, 0xC0, 0x42, 1, 228, 0},                     // array argument
		"SUM(A1*2)":      {0x64, 0, 0, 0, 0xC0, ptgInt, 2, 0, ptgMul, 0x42, 1, 4, 0}, // operators on arrays
		"NOW()":          {ptgAttr, attrVolatile, 0, 0, 0x41, 74, 0},
		// OFFSET is volatile and returns a reference: reference class inside SUM.
		"SUM(OFFSET(A1,1,1))": {ptgAttr, attrVolatile, 0, 0, 0x24, 0, 0, 0, 0xC0, ptgInt, 1, 0, ptgInt, 1, 0, 0x22, 3, 78, 0, 0x42, 1, 4, 0},
		`INDIRECT("A1")`:      {ptgAttr, attrVolatile, 0, 0, 0x17, 2, 0, 'A', '1', 0x42, 1, 148, 0},
	} {
		got, _, err := compileFormula(fakeWriter{testParser()}, src, 0, false)
		if err != nil || string(got) != string(want) {
			t.Errorf("%s = % X, %v; want % X", src, got, err, want)
		}
	}
}

func TestRKEncode(t *testing.T) {
	for _, v := range []float64{0, 1, -5, 536870911, 12.34, 0.5, 1e300, 1.0 / 3, -0.01, 123456.78} {
		rk, ok := rkEncode(v)
		if ok && rkValue(rk) != v {
			t.Errorf("rkEncode(%v) = %#x decodes to %v", v, rk, rkValue(rk))
		}
		if !ok && (v == 1 || v == 12.34 || v == 0.5) {
			t.Errorf("rkEncode(%v) not compressed", v)
		}
	}
}

func TestStringRecordsContinue(t *testing.T) {
	long := strings.Repeat("ab", 5000) + "日本" + strings.Repeat("c", 9000)
	strs := []string{"x", long, "", "ü"}
	b := stringRecords(nil, recSST, make([]byte, 8), strs, nil)
	var segs [][]byte
	records(b, 0, func(r record) bool {
		if len(r.body) > maxRecord {
			t.Errorf("record of %d bytes", len(r.body))
		}
		if len(segs) == 0 {
			segs = append(segs, r.body[8:])
		} else {
			segs = append(segs, r.body)
		}
		return true
	})
	s := &segments{segs: segs}
	for _, want := range strs {
		if got := s.str(); got != want {
			t.Errorf("string of %d chars read back as %d", len(want), len(got))
		}
	}
}
