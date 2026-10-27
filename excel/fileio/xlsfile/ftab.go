package xlsfile

import "strings"

// Built-in worksheet functions of BIFF8 ([MS-XLS] 2.5.198.17 Ftab), with
// their argument counts and token classes. Functions added later (Analysis
// ToolPak, Excel 2007+) have no id: they are called through id 255 with
// the function's name as the first operand.

// funcInfo describes one function id.
type funcInfo struct {
	id       int
	name     string
	min, max int
	ret      byte   // result class: 'V' value, 'R' reference, 'A' array
	args     string // class of each argument; the last one repeats
}

// fixed reports whether calls use tFunc (a fixed argument count) rather
// than tFuncVar.
func (f *funcInfo) fixed() bool { return f.min == f.max }

// argClass is the class argument i expects.
func (f *funcInfo) argClass(i int) byte {
	if f.args == "" {
		return 'V'
	}
	return f.args[min(i, len(f.args)-1)]
}

const (
	funcUser  = 255 // add-in / user-defined / future function call
	maxArgs   = 30  // BIFF8 limit on arguments of one call
	funcIF    = 1
	funcSUM   = 4
	funcCHOOS = 100
)

var funcTable = []funcInfo{
	{0, "COUNT", 0, 30, 'V', "R"},
	{1, "IF", 2, 3, 'V', "VR"},
	{2, "ISNA", 1, 1, 'V', "V"},
	{3, "ISERROR", 1, 1, 'V', "V"},
	{4, "SUM", 0, 30, 'V', "R"},
	{5, "AVERAGE", 1, 30, 'V', "R"},
	{6, "MIN", 1, 30, 'V', "R"},
	{7, "MAX", 1, 30, 'V', "R"},
	{8, "ROW", 0, 1, 'V', "R"},
	{9, "COLUMN", 0, 1, 'V', "R"},
	{10, "NA", 0, 0, 'V', ""},
	{11, "NPV", 2, 30, 'V', "VR"},
	{12, "STDEV", 1, 30, 'V', "R"},
	{13, "DOLLAR", 1, 2, 'V', "V"},
	{14, "FIXED", 1, 3, 'V', "V"},
	{15, "SIN", 1, 1, 'V', "V"},
	{16, "COS", 1, 1, 'V', "V"},
	{17, "TAN", 1, 1, 'V', "V"},
	{18, "ATAN", 1, 1, 'V', "V"},
	{19, "PI", 0, 0, 'V', ""},
	{20, "SQRT", 1, 1, 'V', "V"},
	{21, "EXP", 1, 1, 'V', "V"},
	{22, "LN", 1, 1, 'V', "V"},
	{23, "LOG10", 1, 1, 'V', "V"},
	{24, "ABS", 1, 1, 'V', "V"},
	{25, "INT", 1, 1, 'V', "V"},
	{26, "SIGN", 1, 1, 'V', "V"},
	{27, "ROUND", 2, 2, 'V', "V"},
	{28, "LOOKUP", 2, 3, 'V', "VR"},
	{29, "INDEX", 2, 4, 'R', "RV"},
	{30, "REPT", 2, 2, 'V', "V"},
	{31, "MID", 3, 3, 'V', "V"},
	{32, "LEN", 1, 1, 'V', "V"},
	{33, "VALUE", 1, 1, 'V', "V"},
	{34, "TRUE", 0, 0, 'V', ""},
	{35, "FALSE", 0, 0, 'V', ""},
	{36, "AND", 1, 30, 'V', "R"},
	{37, "OR", 1, 30, 'V', "R"},
	{38, "NOT", 1, 1, 'V', "V"},
	{39, "MOD", 2, 2, 'V', "V"},
	{40, "DCOUNT", 3, 3, 'V', "R"},
	{41, "DSUM", 3, 3, 'V', "R"},
	{42, "DAVERAGE", 3, 3, 'V', "R"},
	{43, "DMIN", 3, 3, 'V', "R"},
	{44, "DMAX", 3, 3, 'V', "R"},
	{45, "DSTDEV", 3, 3, 'V', "R"},
	{46, "VAR", 1, 30, 'V', "R"},
	{47, "DVAR", 3, 3, 'V', "R"},
	{48, "TEXT", 2, 2, 'V', "V"},
	{49, "LINEST", 1, 4, 'A', "RRV"},
	{50, "TREND", 1, 4, 'A', "RRRV"},
	{51, "LOGEST", 1, 4, 'A', "RRV"},
	{52, "GROWTH", 1, 4, 'A', "RRRV"},
	{56, "PV", 3, 5, 'V', "V"},
	{57, "FV", 3, 5, 'V', "V"},
	{58, "NPER", 3, 5, 'V', "V"},
	{59, "PMT", 3, 5, 'V', "V"},
	{60, "RATE", 3, 6, 'V', "V"},
	{61, "MIRR", 3, 3, 'V', "RV"},
	{62, "IRR", 1, 2, 'V', "RV"},
	{63, "RAND", 0, 0, 'V', ""},
	{64, "MATCH", 2, 3, 'V', "VRV"},
	{65, "DATE", 3, 3, 'V', "V"},
	{66, "TIME", 3, 3, 'V', "V"},
	{67, "DAY", 1, 1, 'V', "V"},
	{68, "MONTH", 1, 1, 'V', "V"},
	{69, "YEAR", 1, 1, 'V', "V"},
	{70, "WEEKDAY", 1, 2, 'V', "V"},
	{71, "HOUR", 1, 1, 'V', "V"},
	{72, "MINUTE", 1, 1, 'V', "V"},
	{73, "SECOND", 1, 1, 'V', "V"},
	{74, "NOW", 0, 0, 'V', ""},
	{75, "AREAS", 1, 1, 'V', "R"},
	{76, "ROWS", 1, 1, 'V', "R"},
	{77, "COLUMNS", 1, 1, 'V', "R"},
	{78, "OFFSET", 3, 5, 'R', "RV"},
	{82, "SEARCH", 2, 3, 'V', "V"},
	{83, "TRANSPOSE", 1, 1, 'A', "A"},
	{86, "TYPE", 1, 1, 'V', "V"},
	{97, "ATAN2", 2, 2, 'V', "V"},
	{98, "ASIN", 1, 1, 'V', "V"},
	{99, "ACOS", 1, 1, 'V', "V"},
	{100, "CHOOSE", 2, 30, 'V', "VR"},
	{101, "HLOOKUP", 3, 4, 'V', "VRRV"},
	{102, "VLOOKUP", 3, 4, 'V', "VRRV"},
	{105, "ISREF", 1, 1, 'V', "R"},
	{109, "LOG", 1, 2, 'V', "V"},
	{111, "CHAR", 1, 1, 'V', "V"},
	{112, "LOWER", 1, 1, 'V', "V"},
	{113, "UPPER", 1, 1, 'V', "V"},
	{114, "PROPER", 1, 1, 'V', "V"},
	{115, "LEFT", 1, 2, 'V', "V"},
	{116, "RIGHT", 1, 2, 'V', "V"},
	{117, "EXACT", 2, 2, 'V', "V"},
	{118, "TRIM", 1, 1, 'V', "V"},
	{119, "REPLACE", 4, 4, 'V', "V"},
	{120, "SUBSTITUTE", 3, 4, 'V', "V"},
	{121, "CODE", 1, 1, 'V', "V"},
	{124, "FIND", 2, 3, 'V', "V"},
	{125, "CELL", 1, 2, 'V', "VR"},
	{126, "ISERR", 1, 1, 'V', "V"},
	{127, "ISTEXT", 1, 1, 'V', "V"},
	{128, "ISNUMBER", 1, 1, 'V', "V"},
	{129, "ISBLANK", 1, 1, 'V', "V"},
	{130, "T", 1, 1, 'V', "R"},
	{131, "N", 1, 1, 'V', "R"},
	{140, "DATEVALUE", 1, 1, 'V', "V"},
	{141, "TIMEVALUE", 1, 1, 'V', "V"},
	{142, "SLN", 3, 3, 'V', "V"},
	{143, "SYD", 4, 4, 'V', "V"},
	{144, "DDB", 4, 5, 'V', "V"},
	{148, "INDIRECT", 1, 2, 'R', "V"},
	{162, "CLEAN", 1, 1, 'V', "V"},
	{163, "MDETERM", 1, 1, 'V', "A"},
	{164, "MINVERSE", 1, 1, 'A', "A"},
	{165, "MMULT", 2, 2, 'A', "A"},
	{167, "IPMT", 4, 6, 'V', "V"},
	{168, "PPMT", 4, 6, 'V', "V"},
	{169, "COUNTA", 0, 30, 'V', "R"},
	{183, "PRODUCT", 0, 30, 'V', "R"},
	{184, "FACT", 1, 1, 'V', "V"},
	{189, "DPRODUCT", 3, 3, 'V', "R"},
	{190, "ISNONTEXT", 1, 1, 'V', "V"},
	{193, "STDEVP", 1, 30, 'V', "R"},
	{194, "VARP", 1, 30, 'V', "R"},
	{195, "DSTDEVP", 3, 3, 'V', "R"},
	{196, "DVARP", 3, 3, 'V', "R"},
	{197, "TRUNC", 1, 2, 'V', "V"},
	{198, "ISLOGICAL", 1, 1, 'V', "V"},
	{199, "DCOUNTA", 3, 3, 'V', "R"},
	{204, "USDOLLAR", 1, 2, 'V', "V"},
	{205, "FINDB", 2, 3, 'V', "V"},
	{206, "SEARCHB", 2, 3, 'V', "V"},
	{207, "REPLACEB", 4, 4, 'V', "V"},
	{208, "LEFTB", 1, 2, 'V', "V"},
	{209, "RIGHTB", 1, 2, 'V', "V"},
	{210, "MIDB", 3, 3, 'V', "V"},
	{211, "LENB", 1, 1, 'V', "V"},
	{212, "ROUNDUP", 2, 2, 'V', "V"},
	{213, "ROUNDDOWN", 2, 2, 'V', "V"},
	{214, "ASC", 1, 1, 'V', "V"},
	{215, "DBCS", 1, 1, 'V', "V"},
	{216, "RANK", 2, 3, 'V', "VRV"},
	{219, "ADDRESS", 2, 5, 'V', "V"},
	{220, "DAYS360", 2, 3, 'V', "V"},
	{221, "TODAY", 0, 0, 'V', ""},
	{222, "VDB", 5, 7, 'V', "V"},
	{227, "MEDIAN", 1, 30, 'V', "R"},
	{228, "SUMPRODUCT", 1, 30, 'V', "A"},
	{229, "SINH", 1, 1, 'V', "V"},
	{230, "COSH", 1, 1, 'V', "V"},
	{231, "TANH", 1, 1, 'V', "V"},
	{232, "ASINH", 1, 1, 'V', "V"},
	{233, "ACOSH", 1, 1, 'V', "V"},
	{234, "ATANH", 1, 1, 'V', "V"},
	{235, "DGET", 3, 3, 'V', "R"},
	{244, "INFO", 1, 1, 'V', "V"},
	{247, "DB", 4, 5, 'V', "V"},
	{252, "FREQUENCY", 2, 2, 'A', "R"},
	{261, "ERROR.TYPE", 1, 1, 'V', "V"},
	{269, "AVEDEV", 1, 30, 'V', "R"},
	{270, "BETADIST", 3, 5, 'V', "V"},
	{271, "GAMMALN", 1, 1, 'V', "V"},
	{272, "BETAINV", 3, 5, 'V', "V"},
	{273, "BINOMDIST", 4, 4, 'V', "V"},
	{274, "CHIDIST", 2, 2, 'V', "V"},
	{275, "CHIINV", 2, 2, 'V', "V"},
	{276, "COMBIN", 2, 2, 'V', "V"},
	{277, "CONFIDENCE", 3, 3, 'V', "V"},
	{278, "CRITBINOM", 3, 3, 'V', "V"},
	{279, "EVEN", 1, 1, 'V', "V"},
	{280, "EXPONDIST", 3, 3, 'V', "V"},
	{281, "FDIST", 3, 3, 'V', "V"},
	{282, "FINV", 3, 3, 'V', "V"},
	{283, "FISHER", 1, 1, 'V', "V"},
	{284, "FISHERINV", 1, 1, 'V', "V"},
	{285, "FLOOR", 2, 2, 'V', "V"},
	{286, "GAMMADIST", 4, 4, 'V', "V"},
	{287, "GAMMAINV", 3, 3, 'V', "V"},
	{288, "CEILING", 2, 2, 'V', "V"},
	{289, "HYPGEOMDIST", 4, 4, 'V', "V"},
	{290, "LOGNORMDIST", 3, 3, 'V', "V"},
	{291, "LOGINV", 3, 3, 'V', "V"},
	{292, "NEGBINOMDIST", 3, 3, 'V', "V"},
	{293, "NORMDIST", 4, 4, 'V', "V"},
	{294, "NORMSDIST", 1, 1, 'V', "V"},
	{295, "NORMINV", 3, 3, 'V', "V"},
	{296, "NORMSINV", 1, 1, 'V', "V"},
	{297, "STANDARDIZE", 3, 3, 'V', "V"},
	{298, "ODD", 1, 1, 'V', "V"},
	{299, "PERMUT", 2, 2, 'V', "V"},
	{300, "POISSON", 3, 3, 'V', "V"},
	{301, "TDIST", 3, 3, 'V', "V"},
	{302, "WEIBULL", 4, 4, 'V', "V"},
	{303, "SUMXMY2", 2, 2, 'V', "A"},
	{304, "SUMX2MY2", 2, 2, 'V', "A"},
	{305, "SUMX2PY2", 2, 2, 'V', "A"},
	{306, "CHITEST", 2, 2, 'V', "A"},
	{307, "CORREL", 2, 2, 'V', "A"},
	{308, "COVAR", 2, 2, 'V', "A"},
	{309, "FORECAST", 3, 3, 'V', "VA"},
	{310, "FTEST", 2, 2, 'V', "A"},
	{311, "INTERCEPT", 2, 2, 'V', "A"},
	{312, "PEARSON", 2, 2, 'V', "A"},
	{313, "RSQ", 2, 2, 'V', "A"},
	{314, "STEYX", 2, 2, 'V', "A"},
	{315, "SLOPE", 2, 2, 'V', "A"},
	{316, "TTEST", 4, 4, 'V', "AAV"},
	{317, "PROB", 3, 4, 'V', "AAV"},
	{318, "DEVSQ", 1, 30, 'V', "R"},
	{319, "GEOMEAN", 1, 30, 'V', "R"},
	{320, "HARMEAN", 1, 30, 'V', "R"},
	{321, "SUMSQ", 0, 30, 'V', "R"},
	{322, "KURT", 1, 30, 'V', "R"},
	{323, "SKEW", 1, 30, 'V', "R"},
	{324, "ZTEST", 2, 3, 'V', "RV"},
	{325, "LARGE", 2, 2, 'V', "RV"},
	{326, "SMALL", 2, 2, 'V', "RV"},
	{327, "QUARTILE", 2, 2, 'V', "RV"},
	{328, "PERCENTILE", 2, 2, 'V', "RV"},
	{329, "PERCENTRANK", 2, 3, 'V', "RV"},
	{330, "MODE", 1, 30, 'V', "A"},
	{331, "TRIMMEAN", 2, 2, 'V', "RV"},
	{332, "TINV", 2, 2, 'V', "V"},
	{336, "CONCATENATE", 1, 30, 'V', "V"},
	{337, "POWER", 2, 2, 'V', "V"},
	{342, "RADIANS", 1, 1, 'V', "V"},
	{343, "DEGREES", 1, 1, 'V', "V"},
	{344, "SUBTOTAL", 2, 30, 'V', "VR"},
	{345, "SUMIF", 2, 3, 'V', "RVR"},
	{346, "COUNTIF", 2, 2, 'V', "RV"},
	{347, "COUNTBLANK", 1, 1, 'V', "R"},
	{350, "ISPMT", 4, 4, 'V', "V"},
	{351, "DATEDIF", 3, 3, 'V', "V"},
	{352, "DATESTRING", 1, 1, 'V', "V"},
	{353, "NUMBERSTRING", 2, 2, 'V', "V"},
	{354, "ROMAN", 1, 2, 'V', "V"},
	{358, "GETPIVOTDATA", 2, 30, 'V', "VRV"},
	{359, "HYPERLINK", 1, 2, 'V', "V"},
	{360, "PHONETIC", 1, 1, 'V', "R"},
	{361, "AVERAGEA", 1, 30, 'V', "R"},
	{362, "MAXA", 1, 30, 'V', "R"},
	{363, "MINA", 1, 30, 'V', "R"},
	{364, "STDEVPA", 1, 30, 'V', "R"},
	{365, "VARPA", 1, 30, 'V', "R"},
	{366, "STDEVA", 1, 30, 'V', "R"},
	{367, "VARA", 1, 30, 'V', "R"},
	{368, "BAHTTEXT", 1, 1, 'V', "V"},
	{369, "THAIDAYOFWEEK", 1, 1, 'V', "V"},
	{370, "THAIDIGIT", 1, 1, 'V', "V"},
	{371, "THAIMONTHOFYEAR", 1, 1, 'V', "V"},
	{372, "THAINUMSOUND", 1, 1, 'V', "V"},
	{373, "THAINUMSTRING", 1, 1, 'V', "V"},
	{374, "THAISTRINGLENGTH", 1, 1, 'V', "V"},
	{375, "ISTHAIDIGIT", 1, 1, 'V', "V"},
	{376, "ROUNDBAHTDOWN", 1, 1, 'V', "V"},
	{377, "ROUNDBAHTUP", 1, 1, 'V', "V"},
	{378, "THAIYEAR", 1, 1, 'V', "V"},
	{379, "RTD", 3, 30, 'V', "V"},
}

var (
	funcByID   = map[int]*funcInfo{}
	funcByName = map[string]*funcInfo{}
)

func init() {
	for i := range funcTable {
		f := &funcTable[i]
		funcByID[f.id] = f
		funcByName[f.name] = f
	}
}

// volatileFuncs are recalculated on every calculation; formulas using them
// start with tAttrVolatile.
var volatileFuncs = map[string]bool{
	"RAND": true, "NOW": true, "TODAY": true, "OFFSET": true, "INDIRECT": true,
	"CELL": true, "INFO": true, "RANDBETWEEN": true,
}

// addinFuncs are the Analysis ToolPak functions. BIFF8 calls them as
// add-in functions (EXTERNNAME records under the add-in SUPBOOK); every
// other function without an id is an Excel 2007+ function that Excel
// writes as a hidden "_xlfn." defined name.
var addinFuncs = func() map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(`
		ACCRINT ACCRINTM AMORDEGRC AMORLINC BESSELI BESSELJ BESSELK BESSELY BIN2DEC BIN2HEX BIN2OCT
		COMPLEX CONVERT COUPDAYBS COUPDAYS COUPDAYSNC COUPNCD COUPNUM COUPPCD CUMIPMT CUMPRINC
		DEC2BIN DEC2HEX DEC2OCT DELTA DISC DOLLARDE DOLLARFR DURATION EDATE EFFECT EOMONTH ERF ERFC
		FACTDOUBLE FVSCHEDULE GCD GESTEP HEX2BIN HEX2DEC HEX2OCT IMABS IMAGINARY IMARGUMENT
		IMCONJUGATE IMCOS IMDIV IMEXP IMLN IMLOG10 IMLOG2 IMPOWER IMPRODUCT IMREAL IMSIN IMSQRT
		IMSUB IMSUM INTRATE ISEVEN ISODD LCM MDURATION MROUND MULTINOMIAL NETWORKDAYS NOMINAL
		OCT2BIN OCT2DEC OCT2HEX ODDFPRICE ODDFYIELD ODDLPRICE ODDLYIELD PRICE PRICEDISC PRICEMAT
		QUOTIENT RANDBETWEEN RECEIVED SERIESSUM SQRTPI TBILLEQ TBILLPRICE TBILLYIELD WEEKNUM
		WORKDAY XIRR XNPV YEARFRAC YIELD YIELDDISC YIELDMAT`) {
		m[f] = true
	}
	return m
}()
