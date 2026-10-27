package text

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

func setup(ctx *f.TestContext) {
	ctx.SetRange("A1", [][]any{
		{"Sale Price", 1234.567, "a"},
		{"Sweden", nil, ""},
		{"Tokyo", true, "b"},
	})
	ctx.Set("D1", f.ErrNA)
	ctx.Set("E1", "Ｅｘｃｅｌ")
}

func TestBasics(t *testing.T) {
	ftest.Run(t, registry(), setup, []ftest.Case{
		// LEN
		{`=LEN("Phoenix, AZ")`, 11},
		{`=LEN("")`, 0},
		{`=LEN(B2)`, 0},
		{`=LEN(12.5)`, 4},
		{`=LEN(TRUE)`, 4},
		{`=LEN("😀")`, 2}, // UTF-16 code units
		{`=LEN("é")`, 1},
		{`=LEN(D1)`, f.ErrNA},
		{`=LEN({"a","bb";"ccc",""})`, ftest.Arr{{1, 2}, {3, 0}}},
		{`=LEN(A1:A3)`, ftest.Arr{{10}, {6}, {5}}},
		{`=LENB("abc")`, 3},

		// LEFT / RIGHT / MID
		{`=LEFT("Sale Price",4)`, "Sale"},
		{`=LEFT("Sweden")`, "S"},
		{`=LEFT("abc",0)`, ""},
		{`=LEFT("abc",10)`, "abc"},
		{`=LEFT("abc",1E10)`, "abc"},
		{`=LEFT("abc",2.9)`, "ab"},
		{`=LEFT("abc",-1)`, f.ErrValue},
		{`=LEFT("abc","x")`, f.ErrValue},
		{`=LEFT(1234.5,3)`, "123"},
		{`=LEFT("abc",B2)`, ""}, // an empty cell is 0
		{`=LEFT({"ab","cd"},1)`, ftest.Arr{{"a", "c"}}},
		{`=LEFTB("abc",2)`, "ab"},
		{`=RIGHT("Sale Price",5)`, "Price"},
		{`=RIGHT("Stock Number")`, "r"},
		{`=RIGHT("abc",5)`, "abc"},
		{`=RIGHT("abc",-1)`, f.ErrValue},
		{`=RIGHTB("abc",2)`, "bc"},
		{`=MID("Fluid Flow",1,5)`, "Fluid"},
		{`=MID("Fluid Flow",7,20)`, "Flow"},
		{`=MID("Fluid Flow",20,5)`, ""},
		{`=MID("abc",0,1)`, f.ErrValue},
		{`=MID("abc",1,-1)`, f.ErrValue},
		{`=MID("abc",2,0)`, ""},
		{`=MIDB("abcdef",2,3)`, "bcd"},
		{`=MID("a😀b",2,2)`, "😀"},

		// REPLACE
		{`=REPLACE("abcdefghijk",6,5,"*")`, "abcde*k"},
		{`=REPLACE("2009",3,2,"10")`, "2010"},
		{`=REPLACE("123456",1,3,"@")`, "@456"},
		{`=REPLACE("abc",10,1,"x")`, "abcx"},
		{`=REPLACE("abc",2,0,"x")`, "axbc"},
		{`=REPLACE("abc",0,1,"x")`, f.ErrValue},
		{`=REPLACE("abc",1,-1,"x")`, f.ErrValue},
		{`=REPLACEB("abc",2,1,"X")`, "aXc"},

		// FIND / SEARCH
		{`=FIND("M","Miriam McGovern")`, 1},
		{`=FIND("m","Miriam McGovern")`, 6},
		{`=FIND("M","Miriam McGovern",3)`, 8},
		{`=FIND("z","abc")`, f.ErrValue},
		{`=FIND("a","abc",0)`, f.ErrValue},
		{`=FIND("a","abc",4)`, f.ErrValue},
		{`=FIND("","abc")`, 1},
		{`=FIND("","abc",3)`, 3},
		{`=FIND("?","a?c")`, 2}, // no wildcards in FIND
		{`=FIND("b","a😀b")`, 4},
		{`=FINDB("c","abc")`, 3},
		{`=SEARCH("e","Statements",6)`, 7},
		{`=SEARCH("margin","Profit Margin")`, 8},
		{`=SEARCH("MARGIN","profit margin")`, 8},
		{`=SEARCH("p?o","xxPROxx")`, 3},
		{`=SEARCH("b*d","abcde")`, 2},
		{`=SEARCH("*c","abcde")`, 1},
		{`=SEARCH("~?","what? yes")`, 5},
		{`=SEARCH("~*","a*b")`, 2},
		{`=SEARCH("x","abc")`, f.ErrValue},
		{`=SEARCH("a","abc",5)`, f.ErrValue},
		{`=SEARCHB("C","abc")`, 3},
		{`=SEARCH({"a","c"},"abc")`, ftest.Arr{{1, 3}}},

		// SUBSTITUTE
		{`=SUBSTITUTE("Sales Data","Sales","Cost")`, "Cost Data"},
		{`=SUBSTITUTE("Quarter 1, 2008","1","2",1)`, "Quarter 2, 2008"},
		{`=SUBSTITUTE("Quarter 1, 2011","1","2",3)`, "Quarter 1, 2012"},
		{`=SUBSTITUTE("aaa","a","b")`, "bbb"},
		{`=SUBSTITUTE("aaa","a","b",5)`, "aaa"},
		{`=SUBSTITUTE("aaa","a","b",0)`, f.ErrValue},
		{`=SUBSTITUTE("abc","","x")`, "abc"},
		{`=SUBSTITUTE("ABC","b","x")`, "ABC"}, // case-sensitive
		{`=SUBSTITUTE("aaaa","aa","b")`, "bb"},

		// REPT
		{`=REPT("*-",3)`, "*-*-*-"},
		{`=REPT("-",0)`, ""},
		{`=REPT("ab",2.9)`, "abab"},
		{`=REPT("x",-1)`, f.ErrValue},
		{`=LEN(REPT("x",32767))`, 32767},
		{`=REPT("x",32768)`, f.ErrValue},
		{`=REPT("ab",16384)`, f.ErrValue},

		// case, trimming
		{`=UPPER("total")`, "TOTAL"},
		{`=LOWER("E. E. Cummings")`, "e. e. cummings"},
		{`=UPPER("straße")`, "STRAßE"},
		{`=PROPER("this is a TITLE")`, "This Is A Title"},
		{`=PROPER("2-way street")`, "2-Way Street"},
		{`=PROPER("76BudGet")`, "76Budget"},
		{`=PROPER("it's o'neil")`, "It'S O'Neil"},
		{`=PROPER("élan VITAL")`, "Élan Vital"},
		{`=TRIM(" First Quarter   Earnings ")`, "First Quarter Earnings"},
		{"=TRIM(\"a  b\")", "a  b"}, // non-breaking spaces stay
		{`=CLEAN(CHAR(9)&"Monthly report"&CHAR(10))`, "Monthly report"},
		{`=CLEAN(CHAR(127)&"x")`, "\x7fx"},
		{`=EXACT("word","word")`, true},
		{`=EXACT("Word","word")`, false},
		{`=EXACT("w ord","word")`, false},
		{`=EXACT(1,"1")`, true},
		{`=T("Rainfall")`, "Rainfall"},
		{`=T(19)`, ""},
		{`=T(TRUE)`, ""},
		{`=T(B2)`, ""},
		{`=T(D1)`, f.ErrNA},

		// characters
		{`=CHAR(65)`, "A"},
		{`=CHAR(33)`, "!"},
		{`=CHAR(128)`, "€"},
		{`=CHAR(150)`, "–"},
		{`=CHAR(233)`, "é"},
		{`=CHAR(129)`, "\u0081"},
		{`=CHAR(65.9)`, "A"},
		{`=CHAR(0)`, f.ErrValue},
		{`=CHAR(256)`, f.ErrValue},
		{`=CODE("A")`, 65},
		{`=CODE("!")`, 33},
		{`=CODE("Alphabet")`, 65},
		{`=CODE("€")`, 128},
		{`=CODE("é")`, 233},
		{`=CODE("你")`, 63},
		{`=CODE("")`, f.ErrValue},
		{`=CODE(CHAR(141))`, 141},
		{`=CODE(5)`, 53},
		{`=UNICHAR(66)`, "B"},
		{`=UNICHAR(32)`, " "},
		{`=UNICHAR(8364)`, "€"},
		{`=UNICHAR(128512)`, "😀"},
		{`=UNICHAR(0)`, f.ErrValue},
		{`=UNICHAR(1114112)`, f.ErrValue},
		{`=UNICHAR(55296)`, f.ErrNA},
		{`=UNICODE(" ")`, 32},
		{`=UNICODE("B")`, 66},
		{`=UNICODE("😀x")`, 128512},
		{`=UNICODE("")`, f.ErrValue},
		{`=ASC(E1)`, "Excel"},
		{`=ASC("ＡＢＣ　１２３")`, "ABC 123"},
		{`=ASC("アイウガパ")`, "ｱｲｳｶﾞﾊﾟ"},
		{`=ASC("abc")`, "abc"},
		{`=DBCS("Excel")`, "Ｅｘｃｅｌ"},
		{`=DBCS("ｱｲｳｶﾞﾊﾟ")`, "アイウガパ"},
		{`=DBCS("A 1")`, "Ａ　１"},
	})
}

func TestJoin(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{"a1", "b1"}, {"a2", "b2"}, {"a4", "b4"}})
		ctx.SetRange("A5", [][]any{{"US Dollar", "Australian Dollar", nil, "Euro"}})
		ctx.Set("A7", f.ErrDiv0)
		ctx.Set("A8", 2.5)
	}, []ftest.Case{
		{`=CONCAT("The"," ","sun")`, "The sun"},
		{`=CONCAT(A1:B3)`, "a1b1a2b2a4b4"},
		{`=CONCAT(A1:A3,"|",B1:B3)`, "a1a2a4|b1b2b4"},
		{`=CONCAT({1,2;3,4})`, "1234"},
		{`=CONCAT(TRUE,A8,"")`, "TRUE2.5"},
		{`=CONCAT(A1,A7)`, f.ErrDiv0},
		{`=CONCAT(REPT("x",32767),"y")`, f.ErrValue},
		{`=CONCATENATE("Stream population for ","species"," is ",32,"/mile.")`, "Stream population for species is 32/mile."},
		{`=CONCATENATE(A1," ",B1)`, "a1 b1"},
		{`=CONCATENATE({"a","b"},"-",{"1";"2"})`, ftest.Arr{{"a-1", "b-1"}, {"a-2", "b-2"}}},
		{`=CONCATENATE(A7)`, f.ErrDiv0},
		{`=TEXTJOIN(", ",TRUE,A5:D5)`, "US Dollar, Australian Dollar, Euro"},
		{`=TEXTJOIN(", ",FALSE,A5:D5)`, "US Dollar, Australian Dollar, , Euro"},
		{`=TEXTJOIN("-",TRUE,"a","","b",A6)`, "a-b"},
		{`=TEXTJOIN({",",";"},TRUE,1,2,3,4)`, "1,2;3,4"},
		{`=TEXTJOIN("",TRUE,A1:B1,"x")`, "a1b1x"},
		{`=TEXTJOIN(",",TRUE,A7)`, f.ErrDiv0},
		{`=TEXTJOIN(",",TRUE,REPT("x",20000),REPT("y",20000))`, f.ErrValue},
		{`=TEXTJOIN(",",TRUE,{"",""})`, ""},
	})
}

func TestFormat(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.Set("A1", 1234.567)
		ctx.Set("A2", 46000)
		ctx.Set("A3", 0.285)
	}, []ftest.Case{
		// TEXT
		{`=TEXT(A1,"$#,##0.00")`, "$1,234.57"},
		{`=TEXT(A3,"0.0%")`, "28.5%"},
		{`=TEXT(A2,"dddd, mmmm d, yyyy")`, "Tuesday, December 9, 2025"},
		{`=TEXT(A2,"mm/dd/yy")`, "12/09/25"},
		{`=TEXT(A2,"YYYY-MM-DD")`, "2025-12-09"},
		{`=TEXT(0.75,"hh:mm:ss")`, "18:00:00"},
		{`=TEXT(46000.5,"h:mm AM/PM")`, "12:00 PM"},
		{`=TEXT(1234,"0000000")`, "0001234"},
		{`=TEXT(12200000,"0.00E+00")`, "1.22E+07"},
		{`=TEXT(1.5,"# ?/?")`, "1 1/2"},
		{`=TEXT(-5,"0;(0)")`, "(5)"},
		{`=TEXT(5,"0;(0)")`, "5"},
		{`=TEXT(0,"0;-0;""zero""")`, "zero"},
		{`=TEXT(-3,"0;-0;""zero""")`, "-3"},
		{`=TEXT(-3,"0;;0")`, ""},
		{`=TEXT(-1234.567,"#,##0")`, "-1,235"},
		{`=TEXT(1/3,"General")`, "0.333333333333333"},
		{`=TEXT(1234.5,"")`, ""},
		{`=TEXT("1234.5","0.0")`, "1234.5"},
		{`=TEXT("12/9/2025","yyyy")`, "2025"},
		{`=TEXT("abc","0.00")`, "abc"},
		{`=TEXT("abc","@")`, "abc"},
		{`=TEXT("abc","""<""@"">""")`, "<abc>"},
		{`=TEXT("abc","0;-0;0;""t:""@")`, "t:abc"},
		{`=TEXT(TRUE,"0")`, "TRUE"},
		{`=TEXT(B9,"0.00")`, "0.00"},
		{`=TEXT(#N/A,"0")`, f.ErrNA},
		{`=TEXT({1,2},"0.0")`, ftest.Arr{{"1.0", "2.0"}}},

		// FIXED
		{`=FIXED(1234.567,1)`, "1,234.6"},
		{`=FIXED(1234.567,-1)`, "1,230"},
		{`=FIXED(-1234.567,-1,TRUE)`, "-1230"},
		{`=FIXED(44.332)`, "44.33"},
		{`=FIXED(1234567.891,2,FALSE)`, "1,234,567.89"},
		{`=FIXED(1.005,2)`, "1.01"},
		{`=FIXED(2.5,0)`, "3"},
		{`=FIXED(-2.5,0)`, "-3"},
		{`=FIXED(-0.001,2)`, "0.00"},
		{`=FIXED(999.996,2)`, "1,000.00"},
		{`=FIXED(1234,-5)`, "0"},
		{`=FIXED(0.5,5)`, "0.50000"},
		{`=FIXED("12.345",1)`, "12.3"},
		{`=FIXED("x")`, f.ErrValue},
		{`=FIXED(1,128)`, f.ErrValue},

		// DOLLAR
		{`=DOLLAR(1234.567,2)`, "$1,234.57"},
		{`=DOLLAR(1234.567,-2)`, "$1,200"},
		{`=DOLLAR(-1234.567,-2)`, "($1,200)"},
		{`=DOLLAR(-0.123,4)`, "($0.1230)"},
		{`=DOLLAR(99.888)`, "$99.89"},
		{`=DOLLAR(-1234.567)`, "($1,234.57)"},
		{`=DOLLAR(0)`, "$0.00"},
		{`=DOLLAR(1,200)`, f.ErrValue},

		// VALUE
		{`=VALUE("$1,000")`, 1000},
		{`=VALUE("16:48:00")-VALUE("12:00:00")`, 0.2},
		{`=VALUE("1.5E3")`, 1500},
		{`=VALUE("50%")`, 0.5},
		{`=VALUE("(12)")`, -12},
		{`=VALUE(" 12 ")`, 12},
		{`=VALUE("1/1/2000")`, 36526},
		{`=VALUE(12)`, 12},
		{`=VALUE(B9)`, 0},
		{`=VALUE("")`, f.ErrValue},
		{`=VALUE("abc")`, f.ErrValue},
		{`=VALUE(TRUE)`, f.ErrValue},
		{`=VALUE({"1","x"})`, ftest.Arr{{1, f.ErrValue}}},

		// NUMBERVALUE
		{`=NUMBERVALUE("2.500,27",",",".")`, 2500.27},
		{`=NUMBERVALUE("3.5%")`, 0.035},
		{`=NUMBERVALUE("9%%")`, 0.0009},
		{`=NUMBERVALUE(" 3 000 ")`, 3000},
		{`=NUMBERVALUE("1,234.5")`, 1234.5},
		{`=NUMBERVALUE("1.234,5",",.",".,")`, 1234.5},
		{`=NUMBERVALUE("")`, 0},
		{`=NUMBERVALUE("1.2.3")`, f.ErrValue},
		{`=NUMBERVALUE("1.5,3")`, f.ErrValue},
		{`=NUMBERVALUE("1,5",",",",")`, f.ErrValue},
		{`=NUMBERVALUE("abc")`, f.ErrValue},
		{`=NUMBERVALUE(42)`, 42},
	})
}

func TestToText(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.SetRange("A1", [][]any{{true, f.ErrValue}, {1234.01234, "Seattle"}, {"Hello", "1,123"}})
	}, []ftest.Case{
		{`=VALUETOTEXT("Hello")`, "Hello"},
		{`=VALUETOTEXT("Hello",1)`, `"Hello"`},
		{`=VALUETOTEXT(1234.01234,1)`, "1234.01234"},
		{`=VALUETOTEXT(TRUE)`, "TRUE"},
		{`=VALUETOTEXT(#N/A)`, "#N/A"},
		{`=VALUETOTEXT("a""b",1)`, `"a""b"`},
		{`=VALUETOTEXT(1,2)`, f.ErrValue},
		{`=VALUETOTEXT(A1:A2)`, ftest.Arr{{"TRUE"}, {"1234.01234"}}},
		{`=ARRAYTOTEXT(A1:B3)`, "TRUE, #VALUE!, 1234.01234, Seattle, Hello, 1,123"},
		{`=ARRAYTOTEXT(A1:B3,1)`, `{TRUE,#VALUE!;1234.01234,"Seattle";"Hello","1,123"}`},
		{`=ARRAYTOTEXT({1,2;3,4},1)`, `{1,2;3,4}`},
		{`=ARRAYTOTEXT("x",1)`, `{"x"}`},
		{`=ARRAYTOTEXT(A1:B1,3)`, f.ErrValue},
	})
}

func TestSplit(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.Set("A1", "Little Red Riding Hood's red hood")
		ctx.Set("A2", "Marcus Aurelius")
		ctx.Set("A3", "Dakota Lennon Sanchez")
	}, []ftest.Case{
		// TEXTBEFORE
		{`=TEXTBEFORE(A1,"Red")`, "Little "},
		{`=TEXTBEFORE(A1,"red")`, "Little Red Riding Hood's "},
		{`=TEXTBEFORE(A1,"red",2)`, f.ErrNA},
		{`=TEXTBEFORE(A1,"red",2,1)`, "Little Red Riding Hood's "},
		{`=TEXTBEFORE(A1,"red",-2,1)`, "Little "},
		{`=TEXTBEFORE(A1,"red",-1)`, "Little Red Riding Hood's "},
		{`=TEXTBEFORE(A1,"blue")`, f.ErrNA},
		{`=TEXTBEFORE(A1,"blue",,,,"none")`, "none"},
		{`=TEXTBEFORE(A1,"blue",,,1)`, "Little Red Riding Hood's red hood"},
		{`=TEXTBEFORE(A1,"blue",-1,,1)`, ""},
		{`=TEXTBEFORE(A1,"red",0)`, f.ErrValue},
		{`=TEXTBEFORE(A1,"red",100)`, f.ErrValue},
		{`=TEXTBEFORE(A1,"red",1,2)`, f.ErrValue},
		{`=TEXTBEFORE(A2:A3," ")`, ftest.Arr{{"Marcus"}, {"Dakota"}}},
		{`=TEXTBEFORE("a-b_c",{"_","-"})`, "a"},
		{`=TEXTBEFORE("a-b_c",{"_","-"},2)`, "a-b"},
		{`=TEXTBEFORE("abc","")`, ""},
		{`=TEXTBEFORE("abc","",-1)`, "abc"},
		{`=TEXTBEFORE("a.b.c",".",-1)`, "a.b"},
		{`=TEXTBEFORE(#N/A,".")`, f.ErrNA},
		// TEXTAFTER
		{`=TEXTAFTER(A1,"Red")`, " Riding Hood's red hood"},
		{`=TEXTAFTER(A1,"basket")`, f.ErrNA},
		{`=TEXTAFTER(A1,"red",2)`, f.ErrNA},
		{`=TEXTAFTER(A1,"red",2,1)`, " hood"},
		{`=TEXTAFTER(A1,"red",-2)`, f.ErrNA},
		{`=TEXTAFTER(A1,"red",-2,1)`, " Riding Hood's red hood"},
		{`=TEXTAFTER(A1,"red",-1)`, " hood"},
		{`=TEXTAFTER(A1,"basket",,,,"x")`, "x"},
		{`=TEXTAFTER(A1,"basket",,,1)`, ""},
		{`=TEXTAFTER(A1,"basket",-1,,1)`, "Little Red Riding Hood's red hood"},
		{`=TEXTAFTER("a.b.c",".",-1)`, "c"},
		{`=TEXTAFTER("a-b",{"-","_"},2,,1)`, ""},
		{`=TEXTAFTER("abc","")`, "abc"},
		{`=TEXTAFTER("abc","",-1)`, ""},
		{`=TEXTAFTER(A2:A3," ",-1)`, ftest.Arr{{"Aurelius"}, {"Sanchez"}}},
		// TEXTSPLIT
		{`=TEXTSPLIT("Dakota Lennon Sanchez"," ")`, ftest.Arr{{"Dakota", "Lennon", "Sanchez"}}},
		{`=TEXTSPLIT("1,2,3;4,5,6",",",";")`, ftest.Arr{{"1", "2", "3"}, {"4", "5", "6"}}},
		{`=TEXTSPLIT("Do. Or do not. There is no try. -Anonymous",".")`,
			ftest.Arr{{"Do", " Or do not", " There is no try", " -Anonymous"}}},
		{`=TEXTSPLIT("Do. Or do not. There is no try. -Anonymous",{".","-"})`,
			ftest.Arr{{"Do", " Or do not", " There is no try", " ", "Anonymous"}}},
		{`=TEXTSPLIT("Do. Or do not. There is no try. -Anonymous",{".","-"},,TRUE)`,
			ftest.Arr{{"Do", " Or do not", " There is no try", " ", "Anonymous"}}},
		{`=TEXTSPLIT("Do. Or do not. There is no try. -Anonymous",,".")`,
			ftest.Arr{{"Do"}, {" Or do not"}, {" There is no try"}, {" -Anonymous"}}},
		{`=TEXTSPLIT("a,,b",",")`, ftest.Arr{{"a", "", "b"}}},
		{`=TEXTSPLIT("a,,b",",",,TRUE)`, ftest.Arr{{"a", "b"}}},
		{`=TEXTSPLIT("1,2,3;4,5",",",";")`, ftest.Arr{{"1", "2", "3"}, {"4", "5", f.ErrNA}}},
		{`=TEXTSPLIT("1,2,3;4,5",",",";",,,"-")`, ftest.Arr{{"1", "2", "3"}, {"4", "5", "-"}}},
		{`=TEXTSPLIT("aXbxc","x")`, ftest.Arr{{"aXb", "c"}}},
		{`=TEXTSPLIT("aXbxc","x",,,1)`, ftest.Arr{{"a", "b", "c"}}},
		{`=TEXTSPLIT("a;;b",",",";",TRUE)`, ftest.Arr{{"a"}, {"b"}}},
		{`=TEXTSPLIT("abc",",")`, "abc"},
		{`=TEXTSPLIT("",",")`, ""},
		{`=TEXTSPLIT(",",",",,TRUE)`, f.ErrCalc},
		{`=TEXTSPLIT("abc","")`, f.ErrValue},
		{`=TEXTSPLIT("abc",)`, f.ErrValue},
		{`=TEXTSPLIT({"a b","c d"}," ")`, ftest.Arr{{"a", "c"}}},
	})
}

func TestRegex(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.Set("A1", "Sonia Rees(378) 555-4195Angel Brown(878) 555-8622")
	}, []ftest.Case{
		{`=REGEXTEST("alpha","a")`, true},
		{`=REGEXTEST("alpha","^b")`, false},
		{`=REGEXTEST("Alpha","a.p")`, false},
		{`=REGEXTEST("ALPHA","a.p",1)`, true},
		{`=REGEXTEST("abc","[a-")`, f.ErrValue},
		{`=REGEXTEST("abc","(?=a)")`, f.ErrValue}, // lookahead is not RE2
		{`=REGEXTEST("abc","a",2)`, f.ErrValue},
		{`=REGEXTEST({"a1","b"},"\d")`, ftest.Arr{{true, false}}},
		{`=REGEXEXTRACT(A1,"[A-Z][a-z]+")`, "Sonia"},
		{`=REGEXEXTRACT(A1,"[A-Z][a-z]+ [A-Z][a-z]+",1)`, ftest.Arr{{"Sonia Rees"}, {"Angel Brown"}}},
		{`=REGEXEXTRACT(A1,"\((\d{3})\) (\d{3})-(\d{4})",2)`, ftest.Arr{{"378", "555", "4195"}}},
		{`=REGEXEXTRACT("abc","(x)?(b)",2)`, ftest.Arr{{"", "b"}}},
		{`=REGEXEXTRACT("abc","b",2)`, "b"},
		{`=REGEXEXTRACT("abc","z")`, f.ErrNA},
		{`=REGEXEXTRACT("abc","z",1)`, f.ErrNA},
		{`=REGEXEXTRACT("abc","z",2)`, f.ErrNA},
		{`=REGEXEXTRACT("abc","B",0,1)`, "b"},
		{`=REGEXEXTRACT("abc","B")`, f.ErrNA},
		{`=REGEXEXTRACT("abc","b",3)`, f.ErrValue},
		{`=REGEXREPLACE("Sonia Brown","[a-z]","*")`, "S**** B****"},
		{`=REGEXREPLACE("2024-10-15","(\d+)-(\d+)-(\d+)","$3/$2/$1")`, "15/10/2024"},
		{`=REGEXREPLACE("a1b2c3","\d","#",2)`, "a1b#c3"},
		{`=REGEXREPLACE("a1b2c3","\d","#",-1)`, "a1b2c#"},
		{`=REGEXREPLACE("a1b2c3","\d","#",4)`, "a1b2c3"},
		{`=REGEXREPLACE("ABC","b","x",0,1)`, "AxC"},
		{`=REGEXREPLACE("ABC","b","x")`, "ABC"},
		{`=REGEXREPLACE("ab","(a)","$1x")`, "axb"},
		{`=REGEXREPLACE("ab","(?P<first>a)","[${first}]$$")`, "[a]$b"},
		{`=REGEXREPLACE("ab","a","[$0]")`, "[a]b"},
		{`=REGEXREPLACE("ab","a","$2")`, f.ErrValue},
		{`=REGEXREPLACE("ab","(","x")`, f.ErrValue},
	})
}

func TestMisc(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) {
		ctx.Set("A1", "東京")
		ctx.Set("A2", 5)
	}, []ftest.Case{
		{`=BAHTTEXT(1234)`, "หนึ่งพันสองร้อยสามสิบสี่บาทถ้วน"},
		{`=BAHTTEXT(87.5)`, "แปดสิบเจ็ดบาทห้าสิบสตางค์"},
		{`=BAHTTEXT(0)`, "ศูนย์บาทถ้วน"},
		{`=BAHTTEXT(0.25)`, "ยี่สิบห้าสตางค์"},
		{`=BAHTTEXT(11)`, "สิบเอ็ดบาทถ้วน"},
		{`=BAHTTEXT(21)`, "ยี่สิบเอ็ดบาทถ้วน"},
		{`=BAHTTEXT(1)`, "หนึ่งบาทถ้วน"},
		{`=BAHTTEXT(-5)`, "ลบห้าบาทถ้วน"},
		{`=BAHTTEXT(1000000)`, "หนึ่งล้านบาทถ้วน"},
		{`=BAHTTEXT(21000000)`, "ยี่สิบเอ็ดล้านบาทถ้วน"},
		{`=BAHTTEXT(1500000.01)`, "หนึ่งล้านห้าแสนบาทหนึ่งสตางค์"},
		{`=BAHTTEXT("x")`, f.ErrValue},
		{`=PHONETIC(A1)`, "東京"},
		{`=PHONETIC(A1:A2)`, "東京"},
		{`=PHONETIC(A2)`, ""},
		{`=PHONETIC(B1)`, ""},
		{`=DETECTLANGUAGE("Bonjour")`, f.ErrConnect},
		{`=TRANSLATE("Hello","en","fr")`, f.ErrConnect},
		{`=TRANSLATE(#N/A)`, f.ErrNA},
	})
}

func TestDate1904(t *testing.T) {
	ftest.Run(t, registry(), func(ctx *f.TestContext) { ctx.Use1904 = true }, []ftest.Case{
		{`=VALUE("1/2/1904")`, 1},
		{`=VALUE("12:00")`, 0.5},
		{`=TEXT(1,"yyyy-mm-dd")`, "1904-01-02"},
	})
}

func TestDateFormatDetection(t *testing.T) {
	for code, want := range map[string]bool{
		"yyyy-mm-dd": true, "h:mm": true, "[h]:mm": false, "0.00": false,
		`"day "0`: false, `0\d`: false, "0.00E+00": false, "d-mmm": true,
	} {
		if got := isDateFormat(code); got != want {
			t.Errorf("isDateFormat(%q) = %v, want %v", code, got, want)
		}
	}
}
