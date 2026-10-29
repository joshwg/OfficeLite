package lookup

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
	"officelite/excel/formula/functions/mathtrig"
)

func setup(ctx *f.TestContext) {
	// VLOOKUP documentation example (approximate match).
	ctx.SetRange("A1", [][]any{
		{"Density", "Viscosity", "Temperature"},
		{.457, 3.55, 500}, {.525, 3.25, 400}, {.606, 2.93, 300},
		{.675, 2.75, 250}, {.746, 2.57, 200}, {.835, 2.38, 150},
		{.946, 2.17, 100}, {1.09, 1.95, 50}, {1.29, 1.71, 0},
	})
	// HLOOKUP documentation example.
	ctx.SetRange("E1", [][]any{{"Axles", "Bearings", "Bolts"}, {4, 4, 9}, {5, 7, 10}, {6, 8, 11}})
	// Unsorted people table.
	ctx.SetRange("I1", [][]any{
		{8389, "Dianne", "Sales"}, {4390, "Ned", "Ops"}, {8604, "Margo", "IT"},
		{8007, "Dan", "Sales"}, {5376, "Joy", "IT"},
	})
	// Sorted keys for binary searches.
	ctx.SetRange("M1", [][]any{{10, "a"}, {20, "b"}, {30, "c"}, {40, "d"}, {50, "e"}})
	// MATCH documentation example.
	ctx.SetRange("P1", [][]any{{"Bananas", 25}, {"Oranges", 38}, {"Apples", 40}, {"Pears", 41}})
	// LOOKUP documentation example.
	ctx.SetRange("S1", [][]any{{4.14, "red"}, {4.19, "orange"}, {5.17, "yellow"}, {5.77, "green"}, {6.39, "blue"}})
	ctx.Set("U5", 99)
	ctx.SetRange("X1", [][]any{{3}, {nil}, {1}, {"a"}})
}

func TestLookup(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	ftest.Run(t, r, setup, []ftest.Case{
		// VLOOKUP (Microsoft's examples).
		{"=VLOOKUP(1,A2:C10,2)", 2.17},
		{"=VLOOKUP(1,A2:C10,3,TRUE)", 100.0},
		{"=VLOOKUP(0.7,A2:C10,3,FALSE)", f.ErrNA},
		{"=VLOOKUP(0.1,A2:C10,2,TRUE)", f.ErrNA},
		{"=VLOOKUP(2,A2:C10,2,TRUE)", 1.71},
		{"=VLOOKUP(0.746,A2:C10,3,FALSE)", 200.0},
		{"=VLOOKUP(\"NED\",J1:K5,2,FALSE)", "Ops"},
		{"=VLOOKUP(\"dian*\",J1:K5,2,FALSE)", "Sales"},
		{"=VLOOKUP(\"?a?\",J1:K5,2,FALSE)", "Sales"},
		{"=VLOOKUP(4390,I1:K5,2,)", "Ned"}, // an empty range_lookup is FALSE
		{"=VLOOKUP(\"4390\",I1:K5,2,FALSE)", f.ErrNA},
		{"=VLOOKUP(\"Ned\",J1:K5,3,FALSE)", f.ErrRef},
		{"=VLOOKUP(\"Ned\",J1:K5,0,FALSE)", f.ErrValue},
		{"=VLOOKUP(#N/A,J1:K5,2,FALSE)", f.ErrNA},
		{"=VLOOKUP({8604;4390},I1:K5,2,FALSE)", ftest.Arr{{"Margo"}, {"Ned"}}},
		{"=VLOOKUP(8604,I1:K5,{2,3},FALSE)", ftest.Arr{{"Margo", "IT"}}},
		{"=VLOOKUP(8604,I:K,3,FALSE)", "IT"},
		{"=VLOOKUP(\"Ned\",CHOOSE({1,2},J1:J5,I1:I5),2,FALSE)", 4390.0},

		// HLOOKUP (Microsoft's examples).
		{"=HLOOKUP(\"Axles\",E1:G4,2,TRUE)", 4.0},
		{"=HLOOKUP(\"Bearings\",E1:G4,3,FALSE)", 7.0},
		{"=HLOOKUP(\"B\",E1:G4,3,TRUE)", 5.0},
		{"=HLOOKUP(\"Bolts\",E1:G4,4)", 11.0},
		{"=HLOOKUP(3,{1,2,3;\"a\",\"b\",\"c\";\"d\",\"e\",\"f\"},2,TRUE)", "c"},
		{"=HLOOKUP(\"Bolts\",E1:G4,5)", f.ErrRef},
		{"=HLOOKUP(\"A\",E1:G4,2)", f.ErrNA},

		// MATCH (Microsoft's examples and more).
		{"=MATCH(39,Q1:Q4,1)", 2.0},
		{"=MATCH(41,Q1:Q4,0)", 4.0},
		{"=MATCH(40,Q1:Q4,-1)", f.ErrNA},
		{"=MATCH(39,Q1:Q4)", 2.0},
		{"=MATCH(100,Q1:Q4)", 4.0},
		{"=MATCH(1,Q1:Q4)", f.ErrNA},
		{"=MATCH(2,{1,2,2,2,3})", 4.0},
		{"=MATCH(40,{41,40,38,25},-1)", 2.0},
		{"=MATCH(39,{41,40,38,25},-1)", 2.0},
		{"=MATCH(50,{41,40,38,25},-1)", f.ErrNA},
		{"=MATCH(\"b*\",P1:P4,0)", 1.0},
		{"=MATCH(\"APPLES\",P1:P4,0)", 3.0},
		{"=MATCH(\"*s\",P1:P4,0)", 1.0},
		{"=MATCH(\"Pear\",P1:P4,0)", f.ErrNA},
		{"=MATCH(\"Pears\",P1:Q4,0)", f.ErrNA}, // not a vector
		{"=MATCH(TRUE,{1,TRUE},0)", 2.0},
		{"=MATCH(1,{TRUE,\"1\",1},0)", 3.0},
		{"=MATCH(0,X1:X4,0)", f.ErrNA}, // blanks are not 0
		{"=MATCH(\"a~*\",{\"ab\",\"a*\"},0)", 2.0},
		{"=MATCH({39,41},Q1:Q4)", ftest.Arr{{2.0, 4.0}}},
		{"=MATCH(1/0,Q1:Q4,0)", f.ErrDiv0},
		{"=MATCH(30,M1:M5,)", 3.0}, // an empty match_type is 0
		{"=MATCH(\"c\",{\"a\",\"b\",\"d\"})", 2.0},

		// LOOKUP (Microsoft's examples).
		{"=LOOKUP(4.19,S1:S5,T1:T5)", "orange"},
		{"=LOOKUP(5.75,S1:S5,T1:T5)", "yellow"},
		{"=LOOKUP(7.66,S1:S5,T1:T5)", "blue"},
		{"=LOOKUP(0,S1:S5,T1:T5)", f.ErrNA},
		{"=LOOKUP(\"C\",{\"a\",\"b\",\"c\",\"d\";1,2,3,4})", 3.0},
		{"=LOOKUP(\"bump\",{\"a\",1;\"b\",2;\"c\",3})", 2.0},
		{"=LOOKUP(5.2,S1:T5)", "yellow"},
		{"=LOOKUP(5.2,S1:S5)", 5.17},
		{"=LOOKUP(6.39,S1:S5,U1:U2)", 99.0}, // the result reference is extended
		{"=LOOKUP(5.2,S1:S5,{1,2,3,4,5})", 3.0},
		{"=LOOKUP(2,1/(Q1:Q4>30),P1:P4)", "Pears"}, // last match idiom
		{"=LOOKUP({4.2,9},S1:S5,T1:T5)", ftest.Arr{{"orange", "blue"}}},

		// XLOOKUP.
		{"=XLOOKUP(8604,I1:I5,J1:J5)", "Margo"},
		{"=XLOOKUP(8604,I1:I5,J1:K5)", ftest.Arr{{"Margo", "IT"}}},
		{"=XLOOKUP(1,I1:I5,J1:J5)", f.ErrNA},
		{"=XLOOKUP(1,I1:I5,J1:J5,\"none\")", "none"},
		{"=XLOOKUP(\"dan\",J1:J5,I1:I5)", 8007.0},
		{"=XLOOKUP(\"d*\",J1:J5,I1:I5)", f.ErrNA}, // no wildcards in mode 0
		{"=XLOOKUP(\"d*\",J1:J5,I1:I5,,2)", 8389.0},
		{"=XLOOKUP(\"d*\",J1:J5,I1:I5,,2,-1)", 8007.0},
		{"=XLOOKUP(\"*a*\",J1:J5,K1:K5,,2)", "Sales"},
		{"=XLOOKUP(5000,I1:I5,J1:J5,,-1)", "Ned"},
		{"=XLOOKUP(5000,I1:I5,J1:J5,,1)", "Joy"},
		{"=XLOOKUP(9000,I1:I5,J1:J5,,1)", f.ErrNA},
		{"=XLOOKUP(8007,I1:I5,J1:J5,,-1)", "Dan"},
		{"=XLOOKUP(\"Sales\",K1:K5,J1:J5,,0,-1)", "Dan"},
		{"=XLOOKUP({8604;4390},I1:I5,J1:J5)", ftest.Arr{{"Margo"}, {"Ned"}}},
		{"=XLOOKUP({8604,1},I1:I5,J1:K5,\"x\")", ftest.Arr{{"Margo", "x"}}},
		{"=XLOOKUP(\"Ned\",{\"Dianne\",\"Ned\"},{1,2;3,4})", ftest.Arr{{2.0}, {4.0}}},
		{"=XLOOKUP(\"Bolts\",E1:G1,E2:G4)", ftest.Arr{{9.0}, {10.0}, {11.0}}},
		{"=XLOOKUP(1,{1,2},{1,2,3})", f.ErrValue},
		{"=XLOOKUP(1,{1,2;3,4},{1;2})", f.ErrValue},
		{"=XLOOKUP(1,I1:I5,J1:J4)", f.ErrValue},
		{"=XLOOKUP(8604,I:I,J:J)", "Margo"},
		{"=XLOOKUP(#N/A,I1:I5,J1:J5)", f.ErrNA},
		{"=XLOOKUP(1,I1:I5,J1:J5,,3)", f.ErrValue},
		{"=XLOOKUP(1,I1:I5,J1:J5,,0,3)", f.ErrValue},
		{"=XLOOKUP(\"a*\",N1:N5,M1:M5,,2,2)", f.ErrValue},
		{"=ROW(XLOOKUP(8604,I1:I5,J1:J5))", 3.0}, // the result is a reference
		{"=XLOOKUP(8389,I1:I5,J1:J5):J4", ftest.Arr{{"Dianne"}, {"Ned"}, {"Margo"}, {"Dan"}}},
		// Binary search modes.
		{"=XLOOKUP(30,M1:M5,N1:N5,,0,2)", "c"},
		{"=XLOOKUP(35,M1:M5,N1:N5,,0,2)", f.ErrNA},
		{"=XLOOKUP(35,M1:M5,N1:N5,,-1,2)", "c"},
		{"=XLOOKUP(35,M1:M5,N1:N5,,1,2)", "d"},
		{"=XLOOKUP(5,M1:M5,N1:N5,,-1,2)", f.ErrNA},
		{"=XLOOKUP(55,M1:M5,N1:N5,,1,2)", f.ErrNA},
		{"=XLOOKUP(40,{50;40;30;20;10},{\"e\";\"d\";\"c\";\"b\";\"a\"},,0,-2)", "d"},
		{"=XLOOKUP(35,{50;40;30;20;10},{\"e\";\"d\";\"c\";\"b\";\"a\"},,-1,-2)", "c"},
		{"=XLOOKUP(35,{50;40;30;20;10},{\"e\";\"d\";\"c\";\"b\";\"a\"},,1,-2)", "d"},
		{"=XLOOKUP(\"C\",N1:N5,M1:M5,,0,2)", 30.0},

		// XMATCH (Microsoft's examples and more).
		{"=XMATCH(4,{5,4,3,2,1})", 2.0},
		{"=XMATCH(4.5,{5,4,3,2,1},1)", 1.0},
		{"=XMATCH(4.5,{5,4,3,2,1},-1)", 2.0},
		{"=XMATCH(\"Gra?e\",{\"Apple\",\"Grape\",\"Pear\"},2)", 2.0},
		{"=XMATCH(\"Gra?e\",{\"Apple\",\"Grape\",\"Pear\"})", f.ErrNA},
		{"=XMATCH(3,{1,2,3,3},0,-1)", 4.0},
		{"=XMATCH(3,{1,2,3,3})", 3.0},
		{"=XMATCH({4,1},{5,4,3,2,1})", ftest.Arr{{2.0, 5.0}}},
		{"=XMATCH(40,M1:M5,0,2)", 4.0},
		{"=XMATCH(1,{1,2;3,4})", f.ErrValue},

		// INDEX.
		{"=INDEX(J1:K5,2,2)", "Ops"},
		{"=INDEX(J1:K5,2,1)", "Ned"},
		{"=INDEX({\"Apples\",\"Lemons\";\"Bananas\",\"Pears\"},2,2)", "Pears"},
		{"=INDEX({\"Apples\",\"Lemons\";\"Bananas\",\"Pears\"},1,2)", "Lemons"},
		{"=INDEX({1,2;3,4},0,2)", ftest.Arr{{2.0}, {4.0}}},
		{"=INDEX({1,2;3,4},2,0)", ftest.Arr{{3.0, 4.0}}},
		{"=INDEX({1,2;3,4},2)", ftest.Arr{{3.0, 4.0}}},
		{"=INDEX({1,2;3,4},0,0)", ftest.Arr{{1.0, 2.0}, {3.0, 4.0}}},
		{"=INDEX({1,2,3},2)", 2.0},
		{"=INDEX({1;2;3},2)", 2.0},
		{"=INDEX({1,2,3},1,3)", 3.0},
		{"=INDEX(7,1,1)", 7.0},
		{"=INDEX({1,2;3,4},3,1)", f.ErrRef},
		{"=INDEX({1,2;3,4},1,3)", f.ErrRef},
		{"=INDEX({1,2;3,4},-1,1)", f.ErrValue},
		{"=INDEX(J1:K5,1,1,2)", f.ErrRef},
		{"=INDEX(J1:K5,1,1,1)", "Dianne"},
		{"=INDEX(J1:K5,\"x\",1)", f.ErrValue},
		{"=INDEX(#N/A,1)", f.ErrNA},
		{"=INDEX(I1:K5,2,0)", ftest.Arr{{4390.0, "Ned", "Ops"}}},
		{"=INDEX(I1:K5,{1;3},2)", ftest.Arr{{"Dianne"}, {"Margo"}}},
		{"=INDEX(I1:K5,{1;3},{1,2})", ftest.Arr{{8389.0, "Dianne"}, {8604.0, "Margo"}}},
		{"=INDEX(I:I,3)", 8604.0},
		{"=INDEX(I1:I5,2):I4", ftest.Arr{{4390.0}, {8604.0}, {8007.0}}},
		{"=ROW(INDEX(I1:K5,3,2))", 3.0},
		{"=COLUMN(INDEX(I1:K5,3,2))", 10.0},
		{"=ROWS(INDEX(I1:K5,0,1))", 5.0},
		{"=COLUMNS(INDEX(I1:K5,2,0))", 3.0},
		{"=INDEX(I1:K5,MATCH(\"Joy\",J1:J5,0),3)", "IT"},

		// CHOOSE.
		{"=CHOOSE(2,\"1st\",\"2nd\",\"3rd\",\"Finished\")", "2nd"},
		{"=CHOOSE(4,\"1st\",\"2nd\",\"3rd\",\"Finished\")", "Finished"},
		{"=CHOOSE(5,\"a\",\"b\")", f.ErrValue},
		{"=CHOOSE(0,\"a\",\"b\")", f.ErrValue},
		{"=CHOOSE(2.9,\"a\",\"b\",\"c\")", "b"},
		{"=CHOOSE(\"x\",\"a\",\"b\")", f.ErrValue},
		{"=CHOOSE(#N/A,\"a\",\"b\")", f.ErrNA},
		{"=CHOOSE(2,1/0,\"b\")", "b"},
		{"=CHOOSE({1,2,1},\"a\",\"b\")", ftest.Arr{{"a", "b", "a"}}},
		{"=CHOOSE({1,2},Q1:Q2,P1:P2)", ftest.Arr{{25.0, "Bananas"}, {38.0, "Oranges"}}},
		{"=CHOOSE(2,J1:J2,I1:I2)", ftest.Arr{{8389.0}, {4390.0}}},
		{"=ROWS(CHOOSE(2,A1,I1:I5))", 5.0},
		{"=CHOOSE(1,I2:I3,J1):I5", ftest.Arr{{4390.0}, {8604.0}, {8007.0}, {5376.0}}},

		// ROW / COLUMN / ROWS / COLUMNS.
		{"=ROW()", 1000.0},
		{"=COLUMN()", 1.0},
		{"=ROW(C10)", 10.0},
		{"=ROW(C4:D6)", ftest.Arr{{4.0}, {5.0}, {6.0}}},
		{"=COLUMN(D1)", 4.0},
		{"=COLUMN(B1:D9)", ftest.Arr{{2.0, 3.0, 4.0}}},
		{"=ROW(Sheet2!B7)", 7.0},
		{"=ROW({1,2})", f.ErrValue},
		{"=ROW(#REF!)", f.ErrRef},
		{"=ROWS(ROW(A:A))", 262144.0},
		{"=ROWS(A:A)", 262144.0},
		{"=COLUMNS(1:1)", 2048.0},
		{"=ROWS(C4:D6)", 3.0},
		{"=COLUMNS(C4:D6)", 2.0},
		{"=ROWS({1,2,3;4,5,6})", 2.0},
		{"=COLUMNS({1,2,3;4,5,6})", 3.0},
		{"=ROWS(5)", 1.0},
		{"=ROWS(#REF!)", f.ErrRef},

		// ADDRESS (Microsoft's examples and more).
		{"=ADDRESS(2,3)", "$C$2"},
		{"=ADDRESS(2,3,2)", "C$2"},
		{"=ADDRESS(2,3,3)", "$C2"},
		{"=ADDRESS(2,3,4)", "C2"},
		{"=ADDRESS(2,3,2,FALSE)", "R2C[3]"},
		{"=ADDRESS(2,3,3,FALSE)", "R[2]C3"},
		{"=ADDRESS(2,3,4,FALSE)", "R[2]C[3]"},
		{"=ADDRESS(2,3,1,FALSE,\"[Book1]Sheet1\")", "'[Book1]Sheet1'!R2C3"},
		{"=ADDRESS(2,3,1,FALSE,\"EXCEL SHEET\")", "'EXCEL SHEET'!R2C3"},
		{"=ADDRESS(1,1,1,TRUE,\"Sheet2\")", "Sheet2!$A$1"},
		{"=ADDRESS(1,1,1,TRUE,\"Q1\")", "'Q1'!$A$1"},
		{"=ADDRESS(1,1,1,TRUE,\"2024\")", "'2024'!$A$1"},
		{"=ADDRESS(1,1,1,TRUE,\"TRUE\")", "'TRUE'!$A$1"},
		{"=ADDRESS(1,28)", "$AB$1"},
		{"=ADDRESS(1048576,16384)", "$XFD$1048576"},
		{"=ADDRESS(1,16385)", f.ErrValue},
		{"=ADDRESS(0,1)", f.ErrValue},
		{"=ADDRESS(1,1,5)", f.ErrValue},
		{"=ADDRESS(5,{1,2})", ftest.Arr{{"$A$5", "$B$5"}}},

		// TRANSPOSE.
		{"=TRANSPOSE({1,2,3})", ftest.Arr{{1.0}, {2.0}, {3.0}}},
		{"=TRANSPOSE({1,2;3,4})", ftest.Arr{{1.0, 3.0}, {2.0, 4.0}}},
		{"=TRANSPOSE(E1:G2)", ftest.Arr{{"Axles", 4.0}, {"Bearings", 4.0}, {"Bolts", 9.0}}},
		{"=TRANSPOSE(5)", 5.0},

		// FILTER.
		{"=FILTER(J1:J5,K1:K5=\"Sales\")", ftest.Arr{{"Dianne"}, {"Dan"}}},
		{"=FILTER(I1:K5,K1:K5=\"IT\")", ftest.Arr{{8604.0, "Margo", "IT"}, {5376.0, "Joy", "IT"}}},
		{"=FILTER(I1:I5,I1:I5>8000)", ftest.Arr{{8389.0}, {8604.0}, {8007.0}}},
		{"=FILTER(I1:I5,(K1:K5=\"IT\")*(I1:I5>8000))", 8604.0},
		{"=FILTER(I1:K5,K1:K5=\"HR\")", f.ErrCalc},
		{"=FILTER(I1:K5,K1:K5=\"HR\",\"none\")", "none"},
		{"=FILTER({1,2,3},{1,0,1})", ftest.Arr{{1.0, 3.0}}},
		{"=FILTER({1,2;3,4},{1,0})", ftest.Arr{{1.0}, {3.0}}},
		{"=FILTER({1,2;3,4},{TRUE;FALSE})", ftest.Arr{{1.0, 2.0}}},
		{"=FILTER({1,2,3},{1,0})", f.ErrValue},
		{"=FILTER({1;2},{1;#N/A})", f.ErrNA},
		{"=FILTER({1;2},{\"x\";1})", f.ErrValue},
		{"=FILTER(#REF!,{1})", f.ErrRef},

		// UNIQUE.
		{"=UNIQUE({1;2;1;3;2})", ftest.Arr{{1.0}, {2.0}, {3.0}}},
		{"=UNIQUE({1;2;1;3;2},,TRUE)", 3.0},
		{"=UNIQUE({1;2;1;3;4},FALSE,TRUE)", ftest.Arr{{2.0}, {3.0}, {4.0}}},
		{"=UNIQUE({\"a\";\"A\";\"b\"})", ftest.Arr{{"a"}, {"b"}}},
		{"=UNIQUE({1,1,2},TRUE)", ftest.Arr{{1.0, 2.0}}},
		{"=UNIQUE({1,1,2})", ftest.Arr{{1.0, 1.0, 2.0}}},
		{"=UNIQUE({1,2;1,2;1,3})", ftest.Arr{{1.0, 2.0}, {1.0, 3.0}}},
		{"=UNIQUE({1;1},,TRUE)", f.ErrCalc},
		{"=UNIQUE(K1:K5)", ftest.Arr{{"Sales"}, {"Ops"}, {"IT"}}},
		{"=UNIQUE({1;\"1\";TRUE;1})", ftest.Arr{{1.0}, {"1"}, {true}}},
		{"=UNIQUE({#N/A;#N/A;1})", ftest.Arr{{f.ErrNA}, {1.0}}},

		// SORT.
		{"=SORT({3;1;2})", ftest.Arr{{1.0}, {2.0}, {3.0}}},
		{"=SORT({3;1;2},1,-1)", ftest.Arr{{3.0}, {2.0}, {1.0}}},
		{"=SORT({\"b\";2;TRUE;\"A\";1})", ftest.Arr{{1.0}, {2.0}, {"A"}, {"b"}, {true}}},
		{"=SORT({#N/A;TRUE;\"x\";1})", ftest.Arr{{1.0}, {"x"}, {true}, {f.ErrNA}}},
		{"=SORT({#N/A;TRUE;\"x\";1},,-1)", ftest.Arr{{f.ErrNA}, {true}, {"x"}, {1.0}}},
		{"=SORT(X1:X4)", ftest.Arr{{1.0}, {3.0}, {"a"}, {nil}}},
		{"=SORT(X1:X4,1,-1)", ftest.Arr{{"a"}, {3.0}, {1.0}, {nil}}},
		{"=SORT(I1:I5)", ftest.Arr{{4390.0}, {5376.0}, {8007.0}, {8389.0}, {8604.0}}},
		{"=SORT(J1:J5)", ftest.Arr{{"Dan"}, {"Dianne"}, {"Joy"}, {"Margo"}, {"Ned"}}},
		{"=SORT(I1:K5,2)", ftest.Arr{
			{8007.0, "Dan", "Sales"}, {8389.0, "Dianne", "Sales"}, {5376.0, "Joy", "IT"},
			{8604.0, "Margo", "IT"}, {4390.0, "Ned", "Ops"}}},
		{"=INDEX(SORT(I1:K5,{3,1},{1,-1}),0,1)", ftest.Arr{{8604.0}, {5376.0}, {4390.0}, {8389.0}, {8007.0}}},
		{"=SORT({3,1,2},1,1,TRUE)", ftest.Arr{{1.0, 2.0, 3.0}}},
		{"=SORT({3,1;1,2},2,-1,TRUE)", ftest.Arr{{1.0, 3.0}, {2.0, 1.0}}},
		{"=SORT({1,\"b\";0,\"a\";1,\"a\"})", ftest.Arr{{0.0, "a"}, {1.0, "b"}, {1.0, "a"}}}, // stable
		{"=SORT({1;2},2)", f.ErrValue},
		{"=SORT({1;2},0)", f.ErrValue},
		{"=SORT({1;2},1,0)", f.ErrValue},
		{"=SORT({1,2;3,4},{1,2},{1,-1,1})", f.ErrValue},

		// SORTBY.
		{"=SORTBY(J1:J5,I1:I5)", ftest.Arr{{"Ned"}, {"Joy"}, {"Dan"}, {"Dianne"}, {"Margo"}}},
		{"=SORTBY(J1:J5,I1:I5,-1)", ftest.Arr{{"Margo"}, {"Dianne"}, {"Dan"}, {"Joy"}, {"Ned"}}},
		{"=SORTBY(J1:J5,K1:K5,1,I1:I5,-1)", ftest.Arr{{"Margo"}, {"Joy"}, {"Ned"}, {"Dianne"}, {"Dan"}}},
		{"=SORTBY(J1:J5,K1:K5,,I1:I5)", ftest.Arr{{"Joy"}, {"Margo"}, {"Ned"}, {"Dan"}, {"Dianne"}}},
		{"=SORTBY({\"a\",\"b\",\"c\"},{3,1,2})", ftest.Arr{{"b", "c", "a"}}},
		{"=SORTBY({1;2},{1,2})", f.ErrValue},
		{"=SORTBY(J1:J5,I1:I5,2)", f.ErrValue},
		{"=SORTBY({1,2;3,4},{2;1},1,{1,2})", f.ErrValue},
		{"=SORTBY({1,2;3,4},{2;1})", ftest.Arr{{3.0, 4.0}, {1.0, 2.0}}},
		{"=SORTBY({\"x\";\"y\";\"z\"},{1;#N/A;\"t\"})", ftest.Arr{{"x"}, {"z"}, {"y"}}},
	})
}

func TestOffsetIndirect(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	mathtrig.Register(r) // SUM
	ftest.Run(t, r, func(ctx *f.TestContext) {
		setup(ctx)
		ctx.SetRange("Sheet2!A1", [][]any{{"s2", 7}, {8, 9}})
		ctx.Set("A999", "above")
		ctx.Set("D1", "B3")
		ctx.Names["DENSITY"] = "Sheet1!$A$2:$A$10"
		ctx.Names["RATE"] = "Sheet2!$B$1"
		ctx.Names["CONST"] = "42"
	}, []ftest.Case{
		// OFFSET.
		{"=OFFSET(A1,1,1)", 3.55},
		{"=OFFSET(A2,0,0)", .457},
		{"=OFFSET(A1,,)", "Density"},
		{"=OFFSET(C3,-1,-2)", .457},
		{"=OFFSET(A1,1,0,2,2)", ftest.Arr{{.457, 3.55}, {.525, 3.25}}},
		{"=OFFSET(A2:B3,1,0)", ftest.Arr{{.525, 3.25}, {.606, 2.93}}},
		{"=SUM(OFFSET(C1,1,0,3,1))", 1200.0},
		{"=SUM(OFFSET(C1,1,0,3))", 1200.0},
		{"=ROWS(OFFSET(A1,0,0,4,3))", 4.0},
		{"=COLUMNS(OFFSET(A1:C2,0,0,,2))", 2.0},
		{"=ROW(OFFSET(A1,4,2))", 5.0},
		{"=SUM(A2:OFFSET(A2,1,1))", .457 + 3.55 + .525 + 3.25},
		{"=SUM(OFFSET(C4,0,0,-3,1))", 1200.0},  // a negative height extends up
		{"=SUM(OFFSET(C2,0,0,1,-3))", 504.007}, // a negative width extends left
		{"=OFFSET(A1,0,0,0,1)", f.ErrRef},
		{"=OFFSET(A1,-1,0)", f.ErrRef},
		{"=OFFSET(A1,0,-1)", f.ErrRef},
		{"=OFFSET(A2,0,0,-3)", f.ErrRef},
		{"=OFFSET(A1,262144,0)", f.ErrRef},
		{"=ROWS(OFFSET(A:A,0,1))", 262144.0},
		{"=OFFSET(A:A,1,0)", f.ErrRef},
		{"=OFFSET({1,2},0,0)", f.ErrValue},
		{"=OFFSET(5,0,0)", f.ErrValue},
		{"=OFFSET(A1,\"x\",0)", f.ErrValue},
		{"=OFFSET(#N/A,0,0)", f.ErrNA},
		{"=OFFSET(A1,1/0,0)", f.ErrDiv0},
		{"=OFFSET(A1,1.9,1.9)", 3.55}, // truncated
		{"=OFFSET(INDEX(A1:C10,2,1),1,2)", 400.0},
		{"=OFFSET(Sheet2!A1,1,1)", 9.0},
		{"=OFFSET(A1,{1;2},0)", ftest.Arr{{.457}, {.525}}},

		// INDIRECT, A1 style.
		{"=INDIRECT(\"B2\")", 3.55},
		{"=INDIRECT(\"$B$2\")", 3.55},
		{"=INDIRECT(D1)", 3.25},
		{"=INDIRECT(\"b\"&2)", 3.55},
		{"=INDIRECT(\"A2:B3\")", ftest.Arr{{.457, 3.55}, {.525, 3.25}}},
		{"=SUM(INDIRECT(\"C2:C4\"))", 1200.0},
		{"=INDIRECT(\"Sheet2!B1\")", 7.0},
		{"=INDIRECT(\"'Sheet2'!A1:B2\")", ftest.Arr{{"s2", 7.0}, {8.0, 9.0}}},
		{"=ROWS(INDIRECT(\"A:A\"))", 262144.0},
		{"=COLUMNS(INDIRECT(\"3:3\"))", 2048.0},
		{"=SUM(INDIRECT(\"C:C\"))", 1950.0},
		{"=SUM(INDIRECT(\"Density\"))", .457 + .525 + .606 + .675 + .746 + .835 + .946 + 1.09 + 1.29},
		{"=INDIRECT(\"Rate\")", 7.0},
		{"=INDIRECT(\"Const\")", f.ErrRef},
		{"=INDIRECT(\"Nope\")", f.ErrRef},
		{"=INDIRECT(\"\")", f.ErrRef},
		{"=INDIRECT(\"A1+1\")", f.ErrRef},
		{"=INDIRECT(\"=A1\")", f.ErrRef},
		{"=INDIRECT(\"Sheet9!A1\")", f.ErrRef},
		{"=INDIRECT(\"[Book2.xlsx]Sheet1!A1\")", f.ErrRef},
		{"=INDIRECT(\"'[Book2.xlsx]Sheet1'!A1\")", f.ErrRef},
		{"=INDIRECT(\"A1:Sheet2!B2\")", f.ErrRef},
		{"=INDIRECT(\"ZZZ1\")", f.ErrRef}, // beyond this program's columns
		{"=INDIRECT(\"R2C2\")", f.ErrRef},
		{"=INDIRECT(\"A2\",TRUE)", .457},
		{"=INDIRECT(5)", f.ErrRef},
		{"=INDIRECT(#N/A)", f.ErrNA},
		{"=OFFSET(INDIRECT(\"A1\"),1,2)", 500.0},
		{"=INDIRECT({\"A2\",\"B2\"})", ftest.Arr{{.457, 3.55}}},
		{"=LET(Rate,1,INDIRECT(\"Rate\"))", 7.0}, // names, not LET variables

		// INDIRECT, R1C1 style (the formula is in A1000).
		{"=INDIRECT(\"R2C2\",FALSE)", 3.55},
		{"=INDIRECT(\"r2c2\",FALSE)", 3.55},
		{"=INDIRECT(\"R[-1]C\",FALSE)", "above"},
		{"=INDIRECT(\"R[-998]C[1]\",FALSE)", 3.55},
		{"=INDIRECT(\"R2C[+2]\",FALSE)", 500.0},
		{"=INDIRECT(\"R2C1:R3C2\",FALSE)", ftest.Arr{{.457, 3.55}, {.525, 3.25}}},
		{"=SUM(INDIRECT(\"R2C3:R4C3\",0))", 1200.0},
		{"=SUM(INDIRECT(\"C3\",FALSE))", 1950.0},
		{"=ROWS(INDIRECT(\"C1:C3\",FALSE))", 262144.0},
		{"=COLUMNS(INDIRECT(\"C1:C3\",FALSE))", 3.0},
		{"=COLUMNS(INDIRECT(\"R2\",FALSE))", 2048.0},
		{"=ROW(INDIRECT(\"R\",FALSE))", 1000.0},
		{"=INDIRECT(\"Sheet2!R1C2\",FALSE)", 7.0},
		{"=INDIRECT(\"'Sheet2'!R2C1\",FALSE)", 8.0},
		{"=INDIRECT(\"Rate\",FALSE)", 7.0},
		{"=INDIRECT(\"B2\",FALSE)", f.ErrRef},
		{"=INDIRECT(\"R0C1\",FALSE)", f.ErrRef},
		{"=INDIRECT(\"R[-1000]C\",FALSE)", f.ErrRef},
		{"=INDIRECT(\"R1C1:R2\",FALSE)", f.ErrRef},
		{"=INDIRECT(\"R[1C1\",FALSE)", f.ErrRef},
		{"=INDIRECT(\"R1C1\",\"x\")", f.ErrValue},
	})
}
