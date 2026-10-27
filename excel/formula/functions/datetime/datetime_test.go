package datetime

import (
	"testing"

	f "officelite/excel/formula"
	"officelite/excel/formula/functions/internal/ftest"
)

func registry() *f.Registry {
	r := f.NewRegistry()
	Register(r)
	return r
}

// Expected values are Excel's results, most from Microsoft's documentation
// examples for each function.
func TestDateParts(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=DATE(2008,1,2)", 39449},
		{"=DATE(2008,14,2)", 39846}, // months roll over into the next year
		{"=DATE(2008,1,-1)", 39446},
		{"=DATE(2008,0,1)", 39417},
		{"=DATE(108,1,2)", 39449}, // years below 1900 are offset by 1900
		{"=DATE(2008.9,1.9,2.9)", 39449},
		{"=DATE(1900,2,29)", 60}, // Excel's fictitious leap day
		{"=DATE(1900,3,1)", 61},
		{"=DATE(1900,1,0)", 0},
		{"=DATE(1900,1,-1)", f.ErrNum},
		{"=DATE(-1,1,1)", f.ErrNum},
		{"=DATE(10000,1,1)", f.ErrNum},
		{"=DATE(9999,12,31)", 2958465},
		{"=DATE(2300,1,1)", 146099}, // past time.Duration's ~292-year range
		{"=YEAR(DATE(2300,6,15))", 2300},
		{"=DATE(9999,12,32)", f.ErrNum},
		{"=DATE(\"2008\",1,2)", 39449},
		{"=DATE(\"x\",1,2)", f.ErrValue},
		{"=DATE(#N/A,1,2)", f.ErrNA},

		{"=DATEVALUE(\"8/22/2011\")", 40777},
		{"=DATEVALUE(\"22-MAY-2011\")", 40685},
		{"=DATEVALUE(\"2011/02/23\")", 40597},
		{"=DATEVALUE(\"1/1/2026 10:00 PM\")", 46023},
		{"=DATEVALUE(\"2/29/1900\")", 60},
		{"=DATEVALUE(\"2/29/2023\")", f.ErrValue},
		{"=DATEVALUE(\"12/31/1899\")", f.ErrValue},
		{"=DATEVALUE(\"hello\")", f.ErrValue},
		{"=DATEVALUE(40777)", f.ErrValue}, // only text is accepted
		{"=DATEVALUE(TRUE)", f.ErrValue},

		{"=YEAR(DATE(2008,7,5))", 2008},
		{"=MONTH(DATE(2008,7,5))", 7},
		{"=DAY(DATE(2008,7,5))", 5},
		{"=DAY(\"15-Apr-11\")", 15},
		{"=MONTH(\"15-Apr-11\")", 4},
		{"=YEAR(\"2011-04-15\")", 2011},
		{"=DAY(45351.99)", 29},
		{"=YEAR(0)", 1900},
		{"=MONTH(0)", 1},
		{"=DAY(0)", 0}, // 1900-01-00
		{"=MONTH(60)", 2},
		{"=DAY(60)", 29},
		{"=DAY(61)", 1},
		{"=YEAR(TRUE)", 1900},
		{"=YEAR(2958465)", 9999},
		{"=YEAR(2958466)", f.ErrNum},
		{"=YEAR(-1)", f.ErrNum},
		{"=YEAR(\"x\")", f.ErrValue},
		{"=YEAR(#DIV/0!)", f.ErrDiv0},
		{"=YEAR({45351,0})", ftest.Arr{{2024, 1900}}},

		{"=DAYS(\"3/15/11\",\"2/1/11\")", 42},
		{"=DAYS(DATE(2021,12,31),DATE(2021,1,1))", 364},
		{"=DAYS(DATE(2021,1,1),DATE(2021,12,31))", -364},
		{"=DAYS(10.9,1.2)", 9},
		{"=DAYS(-1,1)", f.ErrNum},
		{"=DAYS(\"x\",1)", f.ErrValue},

		{"=NOW()", 46310.5625},
		{"=TODAY()", 46310},
	})
}

func TestDatedif(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=DATEDIF(\"1/1/2001\",\"1/1/2003\",\"Y\")", 2},
		{"=DATEDIF(\"6/1/2001\",\"8/15/2002\",\"D\")", 440},
		{"=DATEDIF(\"6/1/2001\",\"8/15/2002\",\"YD\")", 75},
		{"=DATEDIF(\"6/1/2001\",\"8/15/2002\",\"M\")", 14},
		{"=DATEDIF(\"6/1/2001\",\"8/15/2002\",\"ym\")", 2},
		{"=DATEDIF(\"6/1/2001\",\"8/15/2002\",\"MD\")", 14},
		{"=DATEDIF(\"6/20/2001\",\"8/15/2002\",\"MD\")", 26},
		{"=DATEDIF(\"6/20/2001\",\"8/15/2002\",\"YM\")", 1},
		{"=DATEDIF(\"9/20/2001\",\"8/15/2002\",\"YD\")", 329},
		{"=DATEDIF(DATE(2015,1,31),DATE(2015,3,1),\"MD\")", -2}, // Excel's MD quirk
		{"=DATEDIF(DATE(2023,1,31),DATE(2023,2,28),\"M\")", 0},
		{"=DATEDIF(DATE(2000,2,29),DATE(2001,2,28),\"Y\")", 0},
		{"=DATEDIF(DATE(2000,2,29),DATE(2001,3,1),\"Y\")", 1},
		{"=DATEDIF(DATE(2020,5,5),DATE(2020,5,5),\"D\")", 0},
		{"=DATEDIF(DATE(2020,5,5),DATE(2020,5,4),\"D\")", f.ErrNum},
		{"=DATEDIF(DATE(2020,5,5),DATE(2021,5,4),\"X\")", f.ErrNum},
		{"=DATEDIF(\"x\",DATE(2021,5,4),\"D\")", f.ErrValue},
	})
}

func TestDays360(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=DAYS360(\"1/30/2011\",\"2/1/2011\")", 1},
		{"=DAYS360(\"1/1/2011\",\"12/31/2011\")", 360},
		{"=DAYS360(\"1/1/2011\",\"2/1/2011\")", 30},
		{"=DAYS360(\"1/1/2011\",\"12/31/2011\",TRUE)", 359},
		{"=DAYS360(\"1/30/2011\",\"12/31/2011\")", 330},
		{"=DAYS360(DATE(2011,2,28),DATE(2011,3,31))", 30}, // start at end of February
		{"=DAYS360(DATE(2011,2,28),DATE(2011,3,31),TRUE)", 32},
		{"=DAYS360(DATE(2011,1,1),DATE(2011,2,28))", 57},
		{"=DAYS360(DATE(2012,3,31),DATE(2012,1,1))", -89},
		{"=DAYS360(DATE(2012,1,1),DATE(2012,3,31),\"x\")", f.ErrValue},
	})
}

func TestMonths(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=EDATE(DATE(2011,1,15),1)", 40589},
		{"=EDATE(DATE(2011,1,15),-1)", 40527},
		{"=EDATE(DATE(2011,1,15),2)", 40617},
		{"=EDATE(\"1/15/2011\",1.9)", 40589},
		{"=EDATE(DATE(2011,1,31),1)", 40602}, // clamped to February 28th
		{"=EDATE(DATE(2024,3,31),-1)", 45351},
		{"=EDATE(DATE(1900,1,31),1)", 60},
		{"=EDATE(DATE(1900,1,15),-1)", f.ErrNum},
		{"=EDATE(DATE(9999,12,1),1)", f.ErrNum},
		{"=EDATE(-1,1)", f.ErrNum},
		{"=EDATE(\"x\",1)", f.ErrValue},
		{"=EOMONTH(DATE(2011,1,1),1)", 40602},
		{"=EOMONTH(DATE(2011,1,1),-3)", 40482},
		{"=EOMONTH(DATE(2024,1,31),1)", 45351},
		{"=EOMONTH(DATE(1900,2,1),0)", 60},
		{"=EOMONTH(\"1/1/2011\",0)", 40574},
		{"=EOMONTH(DATE(9999,12,1),0)", 2958465},
		{"=EOMONTH(DATE(9999,12,1),1)", f.ErrNum},
	})
}

func TestTimes(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=HOUR(0.75)", 18},
		{"=HOUR(\"7/18/2011 7:45\")", 7},
		{"=HOUR(\"4/21/2012\")", 0},
		{"=HOUR(46310.5625)", 13},
		{"=MINUTE(\"12:45:00 PM\")", 45},
		{"=SECOND(\"4:48:18 PM\")", 18},
		{"=SECOND(\"4:48 PM\")", 0},
		{"=SECOND(0.5+29.6/86400)", 30}, // rounded to the nearest second
		{"=HOUR(0.99999999)", 0},
		{"=HOUR(-0.5)", f.ErrNum},
		{"=HOUR(\"x\")", f.ErrValue},
		{"=MINUTE({0.25,0.5104166666666666})", ftest.Arr{{0, 15}}},

		{"=TIME(12,0,0)", 0.5},
		{"=TIME(16,48,10)", ftest.Approx{V: 0.700115741, Tol: 1e-9}},
		{"=TIME(0,750,0)", ftest.Approx{V: 0.520833333, Tol: 1e-9}},
		{"=TIME(27,0,0)", 0.125},
		{"=TIME(1,-30,0)", ftest.Approx{V: 0.020833333, Tol: 1e-9}},
		{"=TIME(12.9,0,0)", 0.5},
		{"=TIME(-1,0,0)", f.ErrNum},
		{"=TIME(0,0,-1)", f.ErrNum},
		{"=TIME(32768,0,0)", f.ErrNum},
		{"=TIME(\"x\",0,0)", f.ErrValue},

		{"=TIMEVALUE(\"2:24 AM\")", 0.1},
		{"=TIMEVALUE(\"22-Aug-2011 6:35 AM\")", ftest.Approx{V: 0.274305556, Tol: 1e-9}},
		{"=TIMEVALUE(\"18:30\")", ftest.Approx{V: 0.770833333, Tol: 1e-9}},
		{"=TIMEVALUE(\"1/1/2026\")", 0},
		{"=TIMEVALUE(0.5)", f.ErrValue},
		{"=TIMEVALUE(\"noon\")", f.ErrValue},
	})
}

func TestWeeks(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=WEEKDAY(\"2/14/2008\")", 5}, // a Thursday
		{"=WEEKDAY(\"2/14/2008\",2)", 4},
		{"=WEEKDAY(\"2/14/2008\",3)", 3},
		{"=WEEKDAY(\"2/14/2008\",11)", 4},
		{"=WEEKDAY(\"2/14/2008\",14)", 1},
		{"=WEEKDAY(\"2/14/2008\",15)", 7},
		{"=WEEKDAY(\"2/14/2008\",17)", 5},
		{"=WEEKDAY(\"2/14/2008\",)", 5},
		{"=WEEKDAY(1)", 1}, // Excel's 1900-01-01 is a Sunday
		{"=WEEKDAY(0)", 7},
		{"=WEEKDAY(60)", 4},
		{"=WEEKDAY(61)", 5},
		{"=WEEKDAY(46310)", 5},
		{"=WEEKDAY(46310,4)", f.ErrNum},
		{"=WEEKDAY(46310,18)", f.ErrNum},
		{"=WEEKDAY(-1)", f.ErrNum},

		{"=WEEKNUM(\"3/9/2012\")", 10},
		{"=WEEKNUM(\"3/9/2012\",2)", 11},
		{"=WEEKNUM(\"3/9/2012\",21)", 10},
		{"=WEEKNUM(DATE(2026,1,1))", 1},
		{"=WEEKNUM(DATE(2026,1,3))", 1},
		{"=WEEKNUM(DATE(2026,1,4))", 2},
		{"=WEEKNUM(DATE(2026,1,4),2)", 1},
		{"=WEEKNUM(DATE(2026,1,5),2)", 2},
		{"=WEEKNUM(DATE(2026,1,2),15)", 2}, // weeks start on Friday
		{"=WEEKNUM(DATE(2026,12,31))", 53},
		{"=WEEKNUM(DATE(2021,1,1),21)", 53},
		{"=WEEKNUM(46310,3)", f.ErrNum},

		{"=ISOWEEKNUM(\"3/9/2012\")", 10},
		{"=ISOWEEKNUM(DATE(2021,1,1))", 53},
		{"=ISOWEEKNUM(DATE(2021,1,4))", 1},
		{"=ISOWEEKNUM(DATE(2024,12,30))", 1},
		{"=ISOWEEKNUM(DATE(2026,10,15))", 42},
		{"=ISOWEEKNUM(-1)", f.ErrNum},
	})
}

func TestWorkdays(t *testing.T) {
	setup := func(ctx *f.TestContext) {
		// NETWORKDAYS documentation example.
		ctx.SetRange("A1", [][]any{{41183}, {41334}, {41235}, {41247}, {41295}, {nil}})
		// WORKDAY documentation example.
		ctx.SetRange("B1", [][]any{{39722}, {39778}, {39786}, {39834}})
		ctx.SetRange("C1", [][]any{{"x"}, {true}, {"=1/0"}})
	}
	ftest.Run(t, registry(), setup, []ftest.Case{
		{"=NETWORKDAYS(A1,A2)", 110},
		{"=NETWORKDAYS(A1,A2,A3)", 109},
		{"=NETWORKDAYS(A1,A2,A3:A5)", 107},
		{"=NETWORKDAYS(A1,A2,A3:A6)", 107}, // blank cells are skipped
		{"=NETWORKDAYS(A1,A2,{41235,41235,41237})", 109},
		{"=NETWORKDAYS(A2,A1,A3:A5)", -107},
		{"=NETWORKDAYS(A1,A1)", 1},
		{"=NETWORKDAYS(DATE(2026,10,17),DATE(2026,10,18))", 0}, // a weekend
		{"=NETWORKDAYS(A1,A2,C1)", f.ErrValue},
		{"=NETWORKDAYS(A1,A2,C2)", f.ErrValue},
		{"=NETWORKDAYS(A1,A2,C3)", f.ErrDiv0},
		{"=NETWORKDAYS(A1,A2,-1)", f.ErrNum},
		{"=NETWORKDAYS(-1,A2)", f.ErrNum},
		{"=NETWORKDAYS({41183,41184},41189)", ftest.Arr{{5, 4}}},

		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31))", 22},
		{"=NETWORKDAYS.INTL(DATE(2006,2,28),DATE(2006,1,31))", -21},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,2,1),7,{\"2006/1/2\",\"2006/1/16\"})", 22},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,2,1),\"0010001\",{\"2006/1/2\",\"2006/1/16\"})", 20},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),11)", 26},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),\"0000000\")", 31},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),\"1111111\")", 0},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),\"000001\")", f.ErrValue},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),\"0000012\")", f.ErrValue},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),8)", f.ErrNum},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),0)", f.ErrNum},

		{"=WORKDAY(B1,151)", 39933},
		{"=WORKDAY(B1,151,B2:B4)", 39938},
		{"=WORKDAY(B1,151.9,B2:B4)", 39938},
		{"=WORKDAY(39938,-151,B2:B4)", 39722},
		{"=WORKDAY(DATE(2026,10,17),0)", 46312}, // a Saturday stays put
		{"=WORKDAY(DATE(2026,10,19),-1)", 46311},
		{"=WORKDAY(DATE(2026,10,16),1)", 46314},
		{"=WORKDAY(DATE(9999,12,31),1)", f.ErrNum},
		{"=WORKDAY(1,-5)", f.ErrNum},
		{"=WORKDAY(\"x\",1)", f.ErrValue},
		{"=WORKDAY(B1,1,C1)", f.ErrValue},

		{"=WORKDAY.INTL(DATE(2012,1,1),30,0)", f.ErrNum},
		{"=WORKDAY.INTL(DATE(2012,1,1),90,11)", 41013},
		{"=WORKDAY.INTL(DATE(2012,1,1),30,17)", 40944},
		{"=WORKDAY.INTL(DATE(2012,1,1),1,\"1111100\")", 40915},
		{"=WORKDAY.INTL(DATE(2012,1,1),1,\"1111111\")", f.ErrValue},
		{"=WORKDAY.INTL(B1,151,1,B2:B4)", 39938},
	})
}

func TestYearfrac(t *testing.T) {
	ftest.Run(t, registry(), nil, []ftest.Case{
		{"=YEARFRAC(\"1/1/2012\",\"7/30/2012\")", ftest.Approx{V: 0.58055556, Tol: 1e-8}},
		{"=YEARFRAC(\"1/1/2012\",\"7/30/2012\",1)", ftest.Approx{V: 0.57650273, Tol: 1e-8}},
		{"=YEARFRAC(\"1/1/2012\",\"7/30/2012\",2)", ftest.Approx{V: 0.58611111, Tol: 1e-8}},
		{"=YEARFRAC(\"1/1/2012\",\"7/30/2012\",3)", ftest.Approx{V: 0.57808219, Tol: 1e-8}},
		{"=YEARFRAC(\"1/1/2012\",\"7/30/2012\",4)", ftest.Approx{V: 0.58055556, Tol: 1e-8}},
		{"=YEARFRAC(\"7/30/2012\",\"1/1/2012\",1)", ftest.Approx{V: 0.57650273, Tol: 1e-8}},
		{"=YEARFRAC(DATE(2010,1,1),DATE(2012,7,30),1)", 2823.0 / 1096}, // average year length
		{"=YEARFRAC(DATE(2000,1,1),DATE(2001,1,1),1)", 1.0},            // leap start year
		{"=YEARFRAC(DATE(2023,7,1),DATE(2024,3,1),1)", 244.0 / 366},    // spans 2024-02-29
		{"=YEARFRAC(DATE(2023,7,1),DATE(2024,2,28),1)", 242.0 / 365},   // does not
		{"=YEARFRAC(DATE(2024,3,1),DATE(2025,2,1),1)", 337.0 / 365},    // leap year, after Feb
		{"=YEARFRAC(DATE(2023,1,31),DATE(2023,3,31),0)", 60.0 / 360},   // 31st -> 30th
		{"=YEARFRAC(DATE(2023,1,15),DATE(2023,3,31),0)", 76.0 / 360},   // end stays the 31st
		{"=YEARFRAC(DATE(2023,2,28),DATE(2024,2,29),0)", 1.0},          // February month ends
		{"=YEARFRAC(DATE(2023,2,28),DATE(2023,3,31),0)", 31.0 / 360},
		{"=YEARFRAC(DATE(2023,1,31),DATE(2023,3,31),4)", 60.0 / 360},
		{"=YEARFRAC(DATE(2023,1,15),DATE(2023,3,31),4)", 75.0 / 360},
		{"=YEARFRAC(DATE(2023,1,1),DATE(2023,1,1))", 0},
		{"=YEARFRAC(DATE(2023,1,1),DATE(2024,1,1),5)", f.ErrNum},
		{"=YEARFRAC(DATE(2023,1,1),DATE(2024,1,1),-1)", f.ErrNum},
		{"=YEARFRAC(-1,DATE(2024,1,1))", f.ErrNum},
		{"=YEARFRAC(\"x\",DATE(2024,1,1))", f.ErrValue},
	})
}

func TestDate1904(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) { ctx.Use1904 = true }, []ftest.Case{
		{"=DATE(1904,1,1)", 0},
		{"=DATE(1903,12,31)", f.ErrNum},
		{"=DATE(2026,10,15)", 44848},
		{"=DATE(9999,12,31)", 2957003},
		{"=DATE(9999,12,32)", f.ErrNum},
		{"=YEAR(0)", 1904},
		{"=DAY(0)", 1},
		{"=YEAR(2957004)", f.ErrNum},
		{"=WEEKDAY(0)", 6}, // 1904-01-01 was a Friday
		{"=DATEVALUE(\"1/2/1904\")", 1},
		{"=DATEVALUE(\"12/31/1903\")", f.ErrValue},
		{"=YEAR(\"10/15/2026\")", 2026},
		{"=DAY(\"1/2/1904\")", 2},
		{"=DAYS(\"10/15/2026\",\"1/1/1904\")", 44848},
		{"=TODAY()", 44848},
		{"=NOW()", 44848.5625},
		{"=EOMONTH(0,0)", 30},
		{"=EDATE(0,-1)", f.ErrNum},
		{"=ISOWEEKNUM(0)", 53},
		{"=WEEKNUM(DATE(2012,3,9))", 10},
		{"=WORKDAY.INTL(DATE(2012,1,1),30,17)", 39482},
		{"=NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31))", 22},
		{"=DATEDIF(DATE(2015,1,31),DATE(2015,3,1),\"MD\")", -2},
		{"=YEARFRAC(\"1/1/2012\",\"7/30/2012\",1)", ftest.Approx{V: 0.57650273, Tol: 1e-8}},
	})
}

func TestFormats(t *testing.T) {
	r := registry()
	want := map[string]string{
		"DATE": dateFormat, "DATEVALUE": dateFormat, "EDATE": dateFormat, "EOMONTH": dateFormat,
		"TODAY": dateFormat, "WORKDAY": dateFormat, "WORKDAY.INTL": dateFormat,
		"NOW": dateTimeFormat, "TIME": timeFormat, "TIMEVALUE": timeFormat,
		"YEAR": "", "DAYS": "", "NETWORKDAYS": "", "YEARFRAC": "",
	}
	for name, format := range want {
		if s := r.Lookup(name); s == nil || s.Format != format {
			t.Errorf("%s: format %q, want %q", name, s.Format, format)
		}
	}
	for _, name := range []string{"NOW", "TODAY"} {
		if !r.Lookup(name).Volatile {
			t.Errorf("%s is not volatile", name)
		}
	}
	if n := len(r.Names()); n != 25 {
		t.Errorf("registered %d functions, want 25", n)
	}
}
