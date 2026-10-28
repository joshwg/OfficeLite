package formula

import (
	"strings"
)

// This file serves the file writers: the formula tokens with their source
// positions, and the prefixes Excel requires in files for functions added
// after Excel 2007.

// TokenKind classifies a Token.
type TokenKind int

const (
	TokNumber TokenKind = iota + 1
	TokString           // Text is the unquoted value
	TokBool             // Text is TRUE or FALSE
	TokError            // Text is the error literal, e.g. #N/A
	TokRef              // Text is the reference as written ($ kept): A1, $A$1, A:B, 1:3
	TokName             // defined name or LET/LAMBDA variable (prefixes removed)
	TokFunc             // function name, upper case, prefixes removed; '(' follows
	TokOp               // + - * / ^ & = <> < <= > >= : %
	TokLParen
	TokRParen
	TokComma
	TokSemi // row separator in array constants
	TokLBrace
	TokRBrace
	TokHash // spill reference: A1#
	TokAt   // implicit intersection: @A1:A3
)

var tokKinds = map[tokKind]TokenKind{
	tNumber: TokNumber, tString: TokString, tBool: TokBool, tError: TokError,
	tRef: TokRef, tName: TokName, tFunc: TokFunc, tOp: TokOp,
	tLParen: TokLParen, tRParen: TokRParen, tComma: TokComma, tSemi: TokSemi,
	tLBrace: TokLBrace, tRBrace: TokRBrace, tHash: TokHash, tAt: TokAt,
}

// Token is one lexical token of a formula.
type Token struct {
	Kind  TokenKind
	Text  string
	Num   float64 // TokNumber
	Sheet string  // sheet prefix of a TokRef or TokName ("" if none)
	// Pos and End delimit the token in the source, sheet prefix included.
	Pos, End int
}

// Tokens splits formula text (without the leading '=') into tokens, the
// same way Parse does.
func Tokens(src string) ([]Token, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	out := make([]Token, 0, len(toks)-1)
	for i, t := range toks[:len(toks)-1] {
		end := toks[i+1].pos
		for end > t.pos && strings.IndexByte(" \t\r\n", src[end-1]) >= 0 {
			end--
		}
		out = append(out, Token{Kind: tokKinds[t.kind], Text: t.text, Num: t.num, Sheet: t.sheet, Pos: t.pos, End: end})
	}
	return out, nil
}

// futureFuncs are the functions Excel writes with a prefix in files
// because they are newer than the file formats (Excel 2010 and later).
// FILTER and SORT are also worksheet ("_xlws.") functions.
var futureFuncs = func() map[string]string {
	m := map[string]string{}
	for _, f := range strings.Fields(`
		ACOT ACOTH AGGREGATE ARABIC BASE BETA.DIST BETA.INV BINOM.DIST BINOM.DIST.RANGE BINOM.INV
		BITAND BITLSHIFT BITOR BITRSHIFT BITXOR CEILING.MATH CEILING.PRECISE CHISQ.DIST CHISQ.DIST.RT
		CHISQ.INV CHISQ.INV.RT CHISQ.TEST COMBINA CONCAT CONFIDENCE.NORM CONFIDENCE.T COT COTH
		COVARIANCE.P COVARIANCE.S CSC CSCH DAYS DECIMAL ECMA.CEILING ENCODEURL ERF.PRECISE ERFC.PRECISE
		EXPON.DIST F.DIST F.DIST.RT F.INV F.INV.RT F.TEST FILTERXML FLOOR.MATH FLOOR.PRECISE
		FORECAST.ETS FORECAST.ETS.CONFINT FORECAST.ETS.SEASONALITY FORECAST.ETS.STAT FORECAST.LINEAR
		FORMULATEXT GAMMA GAMMA.DIST GAMMA.INV GAMMALN.PRECISE GAUSS HYPGEOM.DIST IFNA IFS IMCOSH
		IMCOT IMCSC IMCSCH IMSEC IMSECH IMSINH IMTAN ISFORMULA ISO.CEILING ISOWEEKNUM LOGNORM.DIST
		LOGNORM.INV MAXIFS MINIFS MODE.MULT MODE.SNGL MUNIT NEGBINOM.DIST NETWORKDAYS.INTL NORM.DIST
		NORM.INV NORM.S.DIST NORM.S.INV NUMBERVALUE PDURATION PERCENTILE.EXC PERCENTILE.INC
		PERCENTRANK.EXC PERCENTRANK.INC PERMUTATIONA PHI POISSON.DIST QUARTILE.EXC QUARTILE.INC
		RANK.AVG RANK.EQ RRI SEC SECH SHEET SHEETS SKEW.P STDEV.P STDEV.S SWITCH T.DIST T.DIST.2T
		T.DIST.RT T.INV T.INV.2T T.TEST TEXTJOIN UNICHAR UNICODE VAR.P VAR.S WEBSERVICE WEIBULL.DIST
		WORKDAY.INTL XOR Z.TEST
		ANCHORARRAY ARRAYTOTEXT BYCOL BYROW CHOOSECOLS CHOOSEROWS DETECTLANGUAGE DROP EXPAND
		FIELDVALUE GROUPBY HSTACK IMAGE ISOMITTED LAMBDA LET MAKEARRAY MAP PERCENTOF PIVOTBY
		RANDARRAY REDUCE REGEXEXTRACT REGEXREPLACE REGEXTEST SCAN SEQUENCE SINGLE SORTBY
		STOCKHISTORY TAKE TEXTAFTER TEXTBEFORE TEXTSPLIT TOCOL TOROW TRANSLATE TRIMRANGE UNIQUE
		VALUETOTEXT VSTACK WRAPCOLS WRAPROWS XLOOKUP XMATCH`) {
		m[f] = "_xlfn."
	}
	m["FILTER"] = "_xlfn._xlws."
	m["SORT"] = "_xlfn._xlws."
	return m
}()

// classicFuncs are the worksheet functions older than Excel 2010 (those
// with a BIFF8 id, the Analysis ToolPak and the Excel 2007 additions).
// N and T are left out: as bare names they are more likely defined names.
var classicFuncs = strings.Fields(`
	ABS ACCRINT ACCRINTM ACOS ACOSH ADDRESS AMORDEGRC AMORLINC AND AREAS ASC ASIN ASINH ATAN
	ATAN2 ATANH AVEDEV AVERAGE AVERAGEA AVERAGEIF AVERAGEIFS BAHTTEXT BESSELI BESSELJ BESSELK
	BESSELY BETADIST BETAINV BIN2DEC BIN2HEX BIN2OCT BINOMDIST CEILING CELL CHAR CHIDIST CHIINV
	CHITEST CHOOSE CLEAN CODE COLUMN COLUMNS COMBIN COMPLEX CONCATENATE CONFIDENCE CONVERT
	CORREL COS COSH COUNT COUNTA COUNTBLANK COUNTIF COUNTIFS COUPDAYBS COUPDAYS COUPDAYSNC
	COUPNCD COUPNUM COUPPCD COVAR CRITBINOM CUMIPMT CUMPRINC DATE DATEDIF DATEVALUE DAVERAGE
	DAY DAYS360 DB DBCS DCOUNT DCOUNTA DDB DEC2BIN DEC2HEX DEC2OCT DEGREES DELTA DEVSQ DGET
	DISC DMAX DMIN DOLLAR DOLLARDE DOLLARFR DPRODUCT DSTDEV DSTDEVP DSUM DURATION DVAR DVARP
	EDATE EFFECT EOMONTH ERF ERFC ERROR.TYPE EVEN EXACT EXP EXPONDIST FACT FACTDOUBLE FDIST
	FIND FINDB FINV FISHER FISHERINV FIXED FLOOR FORECAST FREQUENCY FTEST FV FVSCHEDULE
	GAMMADIST GAMMAINV GAMMALN GCD GEOMEAN GESTEP GETPIVOTDATA GROWTH HARMEAN HEX2BIN HEX2DEC
	HEX2OCT HLOOKUP HOUR HYPERLINK HYPGEOMDIST IF IFERROR IMABS IMAGINARY IMARGUMENT
	IMCONJUGATE IMCOS IMDIV IMEXP IMLN IMLOG10 IMLOG2 IMPOWER IMPRODUCT IMREAL IMSIN IMSQRT
	IMSUB IMSUM INDEX INDIRECT INFO INT INTERCEPT INTRATE IPMT IRR ISBLANK ISERR ISERROR
	ISEVEN ISLOGICAL ISNA ISNONTEXT ISNUMBER ISODD ISPMT ISREF ISTEXT KURT LARGE LCM LEFT
	LEFTB LEN LENB LINEST LN LOG LOG10 LOGEST LOGINV LOGNORMDIST LOOKUP LOWER MATCH MAX MAXA
	MDETERM MDURATION MEDIAN MID MIDB MIN MINA MINUTE MINVERSE MIRR MMULT MOD MODE MONTH
	MROUND MULTINOMIAL NA NEGBINOMDIST NETWORKDAYS NOMINAL NORMDIST NORMINV NORMSDIST NORMSINV
	NOT NOW NPER NPV OCT2BIN OCT2DEC OCT2HEX ODD ODDFPRICE ODDFYIELD ODDLPRICE ODDLYIELD OFFSET
	OR PEARSON PERCENTILE PERCENTRANK PERMUT PHONETIC PI PMT POISSON POWER PPMT PRICE
	PRICEDISC PRICEMAT PROB PRODUCT PROPER PV QUARTILE QUOTIENT RADIANS RAND RANDBETWEEN RANK
	RATE RECEIVED REPLACE REPLACEB REPT RIGHT RIGHTB ROMAN ROUND ROUNDDOWN ROUNDUP ROW ROWS
	RSQ RTD SEARCH SEARCHB SECOND SERIESSUM SIGN SIN SINH SKEW SLN SLOPE SMALL SQRT SQRTPI
	STANDARDIZE STDEV STDEVA STDEVP STDEVPA STEYX SUBSTITUTE SUBTOTAL SUM SUMIF SUMIFS
	SUMPRODUCT SUMSQ SUMX2MY2 SUMX2PY2 SUMXMY2 SYD TAN TANH TBILLEQ TBILLPRICE TBILLYIELD TDIST
	TEXT TIME TIMEVALUE TINV TODAY TRANSPOSE TREND TRIM TRIMMEAN TRUNC TTEST TYPE UPPER
	USDOLLAR VALUE VAR VARA VARP VARPA VDB VLOOKUP WEEKDAY WEEKNUM WEIBULL WORKDAY XIRR XNPV
	YEAR YEARFRAC YIELD YIELDDISC YIELDMAT ZTEST`)

// lambdaTakers are the functions with function arguments.
var lambdaTakers = map[string]bool{
	"BYROW": true, "BYCOL": true, "MAP": true, "REDUCE": true, "SCAN": true, "MAKEARRAY": true,
	"GROUPBY": true, "PIVOTBY": true,
}

// etaFuncs are the function names that, written alone as a name (an
// eta-reduced lambda: GROUPBY(A2:A9,B2:B9,SUM)), Excel stores with the
// "_xleta." prefix.
var etaFuncs = func() map[string]bool {
	m := map[string]bool{}
	for f := range futureFuncs {
		m[f] = true
	}
	for _, f := range classicFuncs {
		m[f] = true
	}
	return m
}()

// FuturePrefix returns the prefix Excel writes in files before the
// function name (upper case, without prefixes): "_xlfn.", "_xlfn._xlws."
// or "" for functions every file format knows.
func FuturePrefix(name string) string { return futureFuncs[name] }

// AddFuturePrefixes adds the prefixes Excel requires in files to formula
// text typed without them: =XLOOKUP(...) is stored as _xlfn.XLOOKUP(...),
// LET and LAMBDA variables get "_xlpm." and function names used as values
// (eta-reduced lambdas, =BYROW(A1:B3,SUM)) "_xleta.". Prefixes already present are
// kept. Text that does not lex is returned unchanged.
func AddFuturePrefixes(f string) string {
	toks, err := Tokens(f)
	if err != nil {
		return f
	}
	// LET and LAMBDA variables: a name that is a whole argument followed by
	// a comma (the last argument is the body).
	params := map[string]bool{}
	type frame struct {
		fn   string
		arg  int
		toks int // tokens in the current argument
	}
	// Function names used as values: a name that is a whole argument of a
	// function taking a function (or of HSTACK/VSTACK inside GROUPBY or
	// PIVOTBY, which take vectors of functions). Elsewhere a bare name such
	// as Rate is taken to be a defined name.
	eta := map[int]bool{}
	var stack []frame
	endArg := func(i int) {
		n := len(stack)
		if n == 0 || stack[n-1].toks != 1 || toks[i-1].Kind != TokName || toks[i-1].Sheet != "" {
			return
		}
		fr := stack[n-1]
		switch {
		case fr.fn == "LAMBDA" || fr.fn == "LET" && fr.arg%2 == 0:
			if t := toks[i]; t.Kind == TokComma {
				params[strings.ToUpper(toks[i-1].Text)] = true
			}
		case lambdaTakers[fr.fn]:
			eta[i-1] = true
		case (fr.fn == "HSTACK" || fr.fn == "VSTACK") && n > 1 &&
			(stack[n-2].fn == "GROUPBY" || stack[n-2].fn == "PIVOTBY"):
			eta[i-1] = true
		}
	}
	for i, t := range toks {
		if t.Kind == TokComma || t.Kind == TokRParen {
			endArg(i)
		}
		if t.Kind == TokComma {
			if n := len(stack); n > 0 {
				stack[n-1].arg++
				stack[n-1].toks = 0
			}
			continue
		}
		afterFunc := i > 0 && toks[i-1].Kind == TokFunc
		if n := len(stack); n > 0 && !(t.Kind == TokLParen && afterFunc) {
			stack[n-1].toks++
		}
		switch t.Kind {
		case TokFunc:
			stack = append(stack, frame{fn: t.Text})
		case TokLParen:
			if !afterFunc {
				stack = append(stack, frame{})
			}
		case TokLBrace:
			stack = append(stack, frame{})
		case TokRParen, TokRBrace:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	var b strings.Builder
	last, changed := 0, false
	for i, t := range toks {
		raw := f[t.Pos:t.End]
		prefix := ""
		switch t.Kind {
		case TokFunc:
			if stripPrefixes(raw) == raw {
				prefix = futureFuncs[t.Text]
			}
		case TokName:
			up := strings.ToUpper(t.Text)
			switch {
			case t.Sheet != "" || stripPrefixes(raw) != raw:
			case params[up]:
				prefix = "_xlpm."
			case eta[i] && etaFuncs[up]:
				prefix = "_xleta."
			}
		}
		if prefix != "" {
			b.WriteString(f[last:t.Pos])
			b.WriteString(prefix)
			last, changed = t.Pos, true
		}
	}
	if !changed {
		return f
	}
	b.WriteString(f[last:])
	return b.String()
}
