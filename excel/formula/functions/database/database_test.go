package database

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
	"officelite/excel/formula/functions/logical"
	"officelite/excel/formula/functions/mathtrig"
)

func registry() *f.Registry {
	r := f.NewRegistry()
	Register(r)
	logical.Register(r) // AND and AVERAGE in computed criteria
	mathtrig.Register(r)
	return r
}

func near(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 1e-6} }

// dbData lays out Microsoft's documentation examples:
//
//	Sheet1!A1:F3   criteria on Tree and Height (DSUM, DAVERAGE, ... help)
//	Sheet1!A5:E11  the Tree / Height / Age / Yield / Profit database
//	Sheet2!A6:C10  the Type / Salesperson / Sales list of "Examples of
//	               complex criteria", with criteria ranges in rows 1-3
func dbData(ctx *f.TestContext) {
	ctx.SetRange("A1", [][]any{
		{"Tree", "Height", "Age", "Yield", "Profit", "Height"},
		{`="=Apple"`, ">10", nil, nil, nil, "<16"},
		{`="=Pear"`},
	})
	ctx.SetRange("A5", [][]any{
		{"Tree", "Height", "Age", "Yield", "Profit"},
		{"Apple", 18, 20, 14, 105},
		{"Pear", 12, 12, 10, 96},
		{"Cherry", 13, 14, 9, 105},
		{"Apple", 14, 15, 10, 75},
		{"Pear", 9, 8, 8, 76.8},
		{"Apple", 8, 9, 6, 45},
	})
	ctx.SetRange("Sheet2!A6", [][]any{
		{"Type", "Salesperson", "Sales"},
		{"Beverages", "Suyama", 5122},
		{"Meat", "Davolio", 450},
		{"produce", "Buchanan", 6328},
		{"Produce", "Davolio", 6544},
	})
	ctx.SetRange("Sheet2!E1", [][]any{
		// E          F              G            H        I            J
		{"Salesperson", "Salesperson", "Type", "Sales", "Type", "Salesperson",
			// K          L        M        N        O              P
			"Salesperson", "Sales", "Sales", "Sales", "Salesperson", "Salesperson",
			// Q     R                     S    T         U
			"Type", "Calculated Average", nil, "Nope", "Salesperson"},
		{`="=Davolio"`, `="=Davolio"`, `="=Produce"`, ">1000", `="=Produce"`, nil,
			`="=Davolio"`, ">3000", ">5000", "<8000", "Dav", f.String("=Dav"),
			`="=*ea*"`, "=C7>AVERAGE($C$7:$C$10)", `=AND(A7="Produce",C7>6400)`, "Apple", `="=Davolio"`},
		{nil, `="=Buchanan"`, nil, nil, nil, `="=Davolio"`,
			`="=Buchanan"`, ">1500", "<500"},
	})
}

func TestDatabase(t *testing.T) {
	ftest.Run(t, registry(), dbData, []ftest.Case{
		// Microsoft's examples.
		{`=DCOUNT(A5:E11,"Age",A1:F2)`, 1},
		{`=DCOUNTA(A5:E11,"Profit",A1:F2)`, 1},
		{`=DMAX(A5:E11,"Profit",A1:A3)`, 105},
		{`=DMIN(A5:E11,"Profit",A1:B2)`, 75},
		{`=DSUM(A5:E11,"Profit",A1:A2)`, 225},
		{`=DSUM(A5:E11,"Profit",A1:F2)`, 75},
		{`=DPRODUCT(A5:E11,"Yield",A1:F3)`, 800},
		{`=DAVERAGE(A5:E11,"Yield",A1:B2)`, 12},
		{`=DAVERAGE(A5:E11,3,A5:E11)`, 13},
		{`=DSTDEV(A5:E11,"Yield",A1:A3)`, near(2.966479395)},
		{`=DSTDEVP(A5:E11,"Yield",A1:A3)`, near(2.653299832)},
		{`=DVAR(A5:E11,"Yield",A1:A3)`, 8.8},
		{`=DVARP(A5:E11,"Yield",A1:A3)`, 7.04},
		{`=DGET(A5:E11,"Yield",A1:A3)`, f.ErrNum},
		{`=DGET(A5:E11,"Yield",A1:F2)`, 10},

		// Fields: labels ignore case, numbers count from 1, an omitted
		// field counts records.
		{`=DSUM(A5:E11,"profit",A1:A2)`, 225},
		{`=DSUM(A5:E11,5,A1:A2)`, 225},
		{`=DSUM(A5:E11,5.9,A1:A2)`, 225},
		{`=DSUM(A5:E11,"Price",A1:A2)`, f.ErrValue},
		{`=DSUM(A5:E11,0,A1:A2)`, f.ErrValue},
		{`=DSUM(A5:E11,6,A1:A2)`, f.ErrValue},
		{`=DSUM(A5:E11,,A1:A2)`, f.ErrValue},
		{`=DSUM(A5:E11,TRUE,A1:A2)`, f.ErrValue},
		{`=DSUM(A5:E11,#N/A,A1:A2)`, f.ErrNA},
		{`=DCOUNT(A5:E11,,A1:A2)`, 3},
		{`=DCOUNTA(A5:E11,,A1:A3)`, 5},
		{`=DCOUNT(A5:E11,"Tree",A1:A3)`, 0},
		{`=DCOUNTA(A5:E11,"Tree",A1:A3)`, 5},
		{`=DSUM(A5:E11,"Tree",A1:A3)`, 0},
		{`=DSUM(A:E,"Profit",A1:A2)`, 225},

		// Results with nothing (or too little) to aggregate.
		{`=DAVERAGE(A5:E11,"Yield",Sheet2!U1:U2)`, f.ErrDiv0},
		{`=DSTDEV(A5:E11,"Yield",A1:F2)`, f.ErrDiv0},
		{`=DVAR(A5:E11,"Yield",A1:F2)`, f.ErrDiv0},
		{`=DSTDEVP(A5:E11,"Yield",A1:F2)`, 0},
		{`=DVARP(A5:E11,"Yield",Sheet2!U1:U2)`, f.ErrDiv0},
		{`=DMAX(A5:E11,"Yield",Sheet2!U1:U2)`, 0},
		{`=DPRODUCT(A5:E11,"Yield",Sheet2!U1:U2)`, 0},
		{`=DGET(A5:E11,"Yield",Sheet2!U1:U2)`, f.ErrValue},

		// Criteria: only labels, blank conditions, a wholly blank row.
		{`=DSUM(A5:E11,"Profit",A1:F1)`, f.ErrValue},
		{`=DSUM(A5:E11,"Profit",C1:C2)`, 502.8},
		{`=DSUM(A5:E11,"Profit",A1:A4)`, 502.8},
		{`=DSUM(A5:E11,"Profit","Tree")`, f.ErrValue},
		{`=DSUM(A5:E11,"Profit",{"Tree";"Pear"})`, 172.8},
		{`=DSUM({"x","y";1,2;3,4},"y",{"x";">1"})`, 4},

		// "Examples of complex criteria".
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!E1:E2)`, 6994},  // Salesperson is Davolio
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!F1:F3)`, 13322}, // Davolio or Buchanan
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!G1:H2)`, 12872}, // Produce and over 1000
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!I1:J3)`, 13322}, // Produce or Davolio
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!K1:L3)`, 12872}, // (Davolio and >3000) or (Buchanan and >1500)
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!M1:N3)`, 18444}, // between 5000 and 8000, or under 500
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!O1:O2)`, 6994},  // begins with "Dav"
		{`=DCOUNTA(Sheet2!A6:C10,,Sheet2!P1:P2)`, 0},         // exactly "Dav"
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!Q1:Q2)`, 450},   // contains "ea"
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!R1:R2)`, 17994}, // above the average (computed)
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!S1:S2)`, 6544},  // computed, blank label
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!T1:T2)`, 0},     // a label that is no field
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!U1:U3)`, 18444}, // a blank row matches all
		{`=DSUM(Sheet2!A6:C10,"Sales",Sheet2!E:E)`, 18444},   // blank rows below
		{`=DGET(Sheet2!A6:C10,"Type",Sheet2!S1:S2)`, "Produce"},
	})
}

func TestDatabaseCriteriaText(t *testing.T) {
	db := `{"Name","Date";"Apple",46023;"apricot",46024;"Banana",46025;"Ap",46026}`
	ftest.Run(t, registry(), nil, []ftest.Case{
		{`=DCOUNT(` + db + `,"Date",{"Name";"Ap"})`, 3},
		{`=DCOUNT(` + db + `,"Date",{"Name";"=Ap"})`, 1},
		{`=DCOUNT(` + db + `,"Date",{"Name";"<>Ap"})`, 3},
		{`=DCOUNT(` + db + `,"Date",{"Name";"*an*"})`, 1},
		{`=DCOUNT(` + db + `,"Date",{"Name";"=a?"})`, 1},
		{`=DCOUNT(` + db + `,"Date",{"Name";">b"})`, 1},
		{`=DCOUNT(` + db + `,"Date",{"Date";"1/2/2026"})`, 1},
		{`=DCOUNT(` + db + `,"Date",{"Date";">=1/2/2026"})`, 3},
		{`=DCOUNT(` + db + `,"Date",{"Date";46025})`, 1},
		{`=DCOUNT(` + db + `,"Date",{"Date";""})`, 4},
	})
}
