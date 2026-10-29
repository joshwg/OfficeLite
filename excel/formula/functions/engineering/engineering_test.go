package engineering

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

// Expected values are Microsoft's documentation examples plus the edge
// cases its remarks describe.
func TestBases(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	ftest.Run(t, r, func(ctx *f.TestContext) {
		ctx.Set("A1", "1111111111")
		ctx.Set("A2", 1010)
		ctx.Set("A3", "=1/0")
	}, []ftest.Case{
		{"=BIN2DEC(1100100)", 100},
		{"=BIN2DEC(1111111111)", -1},
		{`=BIN2DEC("1000000000")`, -512},
		{`=BIN2DEC("0111111111")`, 511},
		{"=BIN2DEC(A1)", -1},
		{"=BIN2DEC(A2)", 10},
		{`=BIN2DEC("")`, 0},
		{"=BIN2DEC(12)", f.ErrNum},
		{"=BIN2DEC(10000000000)", f.ErrNum}, // 11 digits
		{"=BIN2DEC(-1)", f.ErrNum},
		{"=BIN2DEC(1.5)", f.ErrNum},
		{"=BIN2DEC(TRUE)", f.ErrValue},
		{"=BIN2DEC(A3)", f.ErrDiv0},
		{"=BIN2DEC({1,10,11})", ftest.Arr{{1, 2, 3}}},
		{"=BIN2HEX(11111011,4)", "00FB"},
		{"=BIN2HEX(1110)", "E"},
		{"=BIN2HEX(1111111111)", "FFFFFFFFFF"},
		{"=BIN2HEX(1110,0)", f.ErrNum},
		{"=BIN2OCT(1001,3)", "011"},
		{"=BIN2OCT(1100100)", "144"},
		{"=BIN2OCT(1111111111)", "7777777777"},
		{"=BIN2OCT(1111111111,2)", "7777777777"}, // places ignored when negative

		{"=DEC2BIN(9,4)", "1001"},
		{"=DEC2BIN(-100)", "1110011100"},
		{"=DEC2BIN(511)", "111111111"},
		{"=DEC2BIN(-512)", "1000000000"},
		{"=DEC2BIN(512)", f.ErrNum},
		{"=DEC2BIN(-513)", f.ErrNum},
		{"=DEC2BIN(9.9)", "1001"},
		{"=DEC2BIN(-1.9)", "1111111111"},
		{`=DEC2BIN("9")`, "1001"},
		{`=DEC2BIN("x")`, f.ErrValue},
		{"=DEC2BIN(TRUE)", f.ErrValue},
		{"=DEC2BIN(9,3)", f.ErrNum},
		{"=DEC2BIN(9,-1)", f.ErrNum},
		{"=DEC2BIN(9,11)", f.ErrNum},
		{"=DEC2BIN(9,10)", "0000001001"},
		{"=DEC2BIN(9,4.9)", "1001"},
		{`=DEC2BIN(9,"x")`, f.ErrValue},
		{"=DEC2BIN(9,)", "1001"},
		{"=DEC2BIN(0)", "0"},
		{"=DEC2HEX(100,4)", "0064"},
		{"=DEC2HEX(-54)", "FFFFFFFFCA"},
		{"=DEC2HEX(28)", "1C"},
		{"=DEC2HEX(64,1)", f.ErrNum},
		{"=DEC2HEX(549755813887)", "7FFFFFFFFF"},
		{"=DEC2HEX(-549755813888)", "8000000000"},
		{"=DEC2HEX(549755813888)", f.ErrNum},
		{"=DEC2OCT(58,3)", "072"},
		{"=DEC2OCT(-100)", "7777777634"},
		{"=DEC2OCT(536870911)", "3777777777"},
		{"=DEC2OCT(-536870912)", "4000000000"},
		{"=DEC2OCT(536870912)", f.ErrNum},

		{`=HEX2BIN("F",8)`, "00001111"},
		{`=HEX2BIN("B7")`, "10110111"},
		{`=HEX2BIN("FFFFFFFE00")`, "1000000000"},
		{`=HEX2BIN("200")`, f.ErrNum}, // 512 is out of DEC2BIN's range
		{`=HEX2BIN("FFFFFFFDFF")`, f.ErrNum},
		{`=HEX2DEC("A5")`, 165},
		{`=HEX2DEC("a5")`, 165},
		{`=HEX2DEC("FFFFFFFF5B")`, -165},
		{`=HEX2DEC("3DA408B9")`, 1034160313},
		{`=HEX2DEC("FFFFFFFFFF")`, -1},
		{`=HEX2DEC("8000000000")`, -549755813888},
		{`=HEX2DEC("1FFFFFFFFFF")`, f.ErrNum},
		{`=HEX2DEC("G")`, f.ErrNum},
		{"=HEX2DEC(99)", 153},
		{`=HEX2OCT("F",3)`, "017"},
		{`=HEX2OCT("3B4E")`, "35516"},
		{`=HEX2OCT("FFFFFFFF00")`, "7777777400"},
		{`=HEX2OCT("20000000")`, f.ErrNum},

		{"=OCT2BIN(3,3)", "011"},
		{"=OCT2BIN(7777777000)", "1000000000"},
		{"=OCT2BIN(1000)", f.ErrNum},
		{"=OCT2DEC(54)", 44},
		{"=OCT2DEC(7777777533)", -165},
		{`=OCT2DEC("7777777777")`, -1},
		{`=OCT2DEC("4000000000")`, -536870912},
		{"=OCT2DEC(8)", f.ErrNum},
		{"=OCT2HEX(100,4)", "0040"},
		{`=OCT2HEX("7777777533")`, "FFFFFFFF5B"},
		{`=OCT2HEX("3777777777")`, "1FFFFFFF"},
	})
}

func TestConvert(t *testing.T) {
	r := f.NewRegistry()
	Register(r)
	// rel compares to ~1e-12 relative.
	rel := func(v float64) ftest.Approx { return ftest.Approx{V: v, Tol: 1e-12 * max(1e-300, abs(v))} }
	ftest.Run(t, r, func(ctx *f.TestContext) {
		ctx.Set("A1", 6)
	}, []ftest.Case{
		// Microsoft's examples.
		{`=CONVERT(1,"lbm","kg")`, rel(0.45359237)},
		{`=CONVERT(68,"F","C")`, rel(20)},
		{`=CONVERT(2.5,"ft","sec")`, f.ErrNA},
		{`=CONVERT(CONVERT(100,"ft","m"),"ft","m")`, rel(9.290304)},
		{`=CONVERT(A1,"C","F")`, rel(42.8)},
		{`=CONVERT(A1,"tsp","tbs")`, rel(2)},
		{`=CONVERT(A1,"gal","l")`, rel(22.712470704)},
		{`=CONVERT(A1,"mi","km")`, rel(9.656064)},
		{`=CONVERT(A1,"km","mi")`, rel(6000 / 1609.344)},
		{`=CONVERT(A1,"in","ft")`, rel(0.5)},
		{`=CONVERT(A1,"cm","in")`, rel(2.3622047244094486)},

		// Each class.
		{`=CONVERT(1,"sg","lbm")`, rel(32.17404855643045)},
		{`=CONVERT(1,"stone","lbm")`, rel(14)},
		{`=CONVERT(1,"ton","kg")`, rel(907.18474)},
		{`=CONVERT(1,"LTON","brton")`, 1},
		{`=CONVERT(1,"uk_ton","lbm")`, rel(2240)},
		{`=CONVERT(7000,"grain","lbm")`, rel(1)},
		{`=CONVERT(16,"ozm","lbm")`, rel(1)},
		{`=CONVERT(1,"cwt","lbm")`, rel(100)},
		{`=CONVERT(1,"hweight","lcwt")`, 1},
		{`=CONVERT(1,"Nmi","m")`, 1852},
		{`=CONVERT(1,"yd","in")`, rel(36)},
		{`=CONVERT(1,"ell","in")`, rel(45)},
		{`=CONVERT(1,"ly","m")`, rel(9460730472580800)},
		{`=CONVERT(1,"pc","parsec")`, 1},
		{`=CONVERT(72,"Picapt","in")`, rel(1)},
		{`=CONVERT(6,"pica","in")`, rel(1)},
		{`=CONVERT(1,"Pica","Picapt")`, 1},
		{`=CONVERT(1,"survey_mi","m")`, rel(1609.3472186944373)},
		{`=CONVERT(10,"ang","nm")`, rel(1)},
		{`=CONVERT(1,"yr","day")`, rel(365.25)},
		{`=CONVERT(1,"d","hr")`, rel(24)},
		{`=CONVERT(1,"hr","mn")`, rel(60)},
		{`=CONVERT(1,"min","s")`, rel(60)},
		{`=CONVERT(1,"ms","sec")`, rel(0.001)},
		{`=CONVERT(1,"atm","mmHg")`, rel(760)},
		{`=CONVERT(1,"at","Pa")`, rel(101325)},
		{`=CONVERT(1,"Torr","mmHg")`, 1},
		{`=CONVERT(1,"psi","kPa")`, rel(6.894757293168361)},
		{`=CONVERT(1,"p","Pa")`, 1},
		{`=CONVERT(1,"lbf","N")`, rel(4.4482216152605)},
		{`=CONVERT(1,"N","dyn")`, rel(100000)},
		{`=CONVERT(1,"dy","dyn")`, 1},
		{`=CONVERT(1,"kpond","N")`, rel(9.80665)},
		{`=CONVERT(1,"cal","J")`, rel(4.1868)},
		{`=CONVERT(1,"c","J")`, rel(4.184)},
		{`=CONVERT(1,"J","e")`, rel(1e7)},
		{`=CONVERT(1,"Wh","J")`, rel(3600)},
		{`=CONVERT(1,"kWh","J")`, rel(3.6e6)},
		{`=CONVERT(1,"BTU","J")`, rel(1055.05585262)},
		{`=CONVERT(1,"btu","BTU")`, 1},
		{`=CONVERT(1,"HPh","Wh")`, rel(745.6998715822702)},
		{`=CONVERT(1,"flb","J")`, rel(1.3558179483314004)},
		{`=CONVERT(1,"eV","J")`, rel(1.602176487e-19)},
		{`=CONVERT(1,"HP","W")`, rel(745.6998715822702)},
		{`=CONVERT(1,"PS","W")`, rel(735.49875)},
		{`=CONVERT(1,"kW","w")`, rel(1000)},
		{`=CONVERT(1,"T","ga")`, rel(10000)},
		{`=CONVERT(1,"mga","T")`, rel(1e-7)},
		{`=CONVERT(1,"gal","qt")`, rel(4)},
		{`=CONVERT(1,"qt","pt")`, rel(2)},
		{`=CONVERT(1,"pt","cup")`, rel(2)},
		{`=CONVERT(1,"cup","oz")`, rel(8)},
		{`=CONVERT(1,"oz","tbs")`, rel(2)},
		{`=CONVERT(1,"tspm","ml")`, rel(5)},
		{`=CONVERT(1,"uk_gal","l")`, rel(4.54609)},
		{`=CONVERT(1,"uk_gal","uk_pt")`, rel(8)},
		{`=CONVERT(1,"uk_qt","uk_pt")`, rel(2)},
		{`=CONVERT(1,"us_pt","pt")`, 1},
		{`=CONVERT(1,"barrel","gal")`, rel(42)},
		{`=CONVERT(1,"bushel","l")`, rel(35.23907016688)},
		{`=CONVERT(1,"ft3","in^3")`, rel(1728)},
		{`=CONVERT(1,"m3","l")`, rel(1000)},
		{`=CONVERT(1,"GRT","ft3")`, rel(100)},
		{`=CONVERT(1,"MTON","ft^3")`, rel(40)},
		{`=CONVERT(1,"yd3","ft3")`, rel(27)},
		{`=CONVERT(1,"L","lt")`, 1},
		{`=CONVERT(1,"ha","m2")`, rel(10000)},
		{`=CONVERT(1,"ar","m^2")`, rel(100)},
		{`=CONVERT(1,"uk_acre","ft2")`, rel(43560)},
		{`=CONVERT(1,"us_acre","m2")`, rel(4046.872609874252)},
		{`=CONVERT(1,"Morgen","m2")`, rel(2500)},
		{`=CONVERT(1,"mi2","yd^2")`, rel(3097600)},
		{`=CONVERT(1,"Nmi2","m2")`, rel(1852 * 1852)},
		{`=CONVERT(1,"Pica2","Picapt^2")`, 1},
		{`=CONVERT(1,"byte","bit")`, 8},
		{`=CONVERT(1,"kbyte","byte")`, 1000},
		{`=CONVERT(1,"kibyte","byte")`, 1024},
		{`=CONVERT(1,"Gibit","Mibit")`, 1024},
		{`=CONVERT(1,"Mibyte","kbit")`, rel(8388.608)},
		{`=CONVERT(1,"kn","m/h")`, rel(1852)},
		{`=CONVERT(1,"admkn","kn")`, rel(6080 * 0.3048 / 1852)},
		{`=CONVERT(1,"mph","km/hr")`, rel(1.609344)},
		{`=CONVERT(1,"m/s","m/sec")`, 1},
		{`=CONVERT(36,"km/h","m/s")`, rel(10)},

		// Temperature.
		{`=CONVERT(100,"C","F")`, rel(212)},
		{`=CONVERT(32,"fah","cel")`, 0},
		{`=CONVERT(0,"C","K")`, rel(273.15)},
		{`=CONVERT(0,"kel","C")`, rel(-273.15)},
		{`=CONVERT(100,"K","Rank")`, rel(180)},
		{`=CONVERT(0,"F","Rank")`, rel(459.67)},
		{`=CONVERT(80,"Reau","C")`, rel(100)},
		{`=CONVERT(100,"C","Reau")`, rel(80)},
		{`=CONVERT(1000,"mK","K")`, rel(1)},
		{`=CONVERT(1,"kK","C")`, rel(726.85)},
		{`=CONVERT(0,"C","mK")`, rel(273150)},
		{`=CONVERT(1,"kC","C")`, f.ErrNA}, // only kelvin takes prefixes

		// Prefixes.
		{`=CONVERT(1,"km","m")`, 1000},
		{`=CONVERT(1,"mm","m")`, 0.001},
		{`=CONVERT(1,"dam","m")`, 10},
		{`=CONVERT(1,"em","m")`, 10},
		{`=CONVERT(1,"hm","m")`, 100},
		{`=CONVERT(1,"dm","m")`, 0.1},
		{`=CONVERT(1,"um","m")`, rel(1e-6)},
		{`=CONVERT(1,"Ym","m")`, rel(1e24)},
		{`=CONVERT(1,"ym","m")`, rel(1e-24)},
		{`=CONVERT(1,"Mg","kg")`, 1000},
		{`=CONVERT(1,"km2","m2")`, 1e6},
		{`=CONVERT(1,"km^2","m^2")`, 1e6},
		{`=CONVERT(1,"cm3","m3")`, rel(1e-6)},
		{`=CONVERT(1,"mL","cm3")`, rel(1)},
		{`=CONVERT(1,"kft","m")`, f.ErrNA}, // feet take no prefixes
		{`=CONVERT(1,"kim","m")`, f.ErrNA}, // binary prefixes only for bits and bytes
		{`=CONVERT(1,"Km","m")`, f.ErrNA},  // case-sensitive
		{`=CONVERT(1,"M","m")`, f.ErrNA},
		{`=CONVERT(1,"LBM","g")`, f.ErrNA},

		// Errors and coercion.
		{`=CONVERT(1,"m","kg")`, f.ErrNA},
		{`=CONVERT(1,"x","m")`, f.ErrNA},
		{`=CONVERT(1,"m","")`, f.ErrNA},
		{`=CONVERT("2","m","cm")`, 200},
		{`=CONVERT("x","m","cm")`, f.ErrValue},
		{`=CONVERT(TRUE,"m","cm")`, f.ErrValue},
		{`=CONVERT(1/0,"m","cm")`, f.ErrDiv0},
		{`=CONVERT(1,#N/A,"cm")`, f.ErrNA},
		{`=CONVERT({1,2},"m","cm")`, ftest.Arr{{100, 200}}},
		{`=CONVERT(-3,"m","m")`, -3},
	})
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
