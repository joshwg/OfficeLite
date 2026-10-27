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

// FuturePrefix returns the prefix Excel writes in files before the
// function name (upper case, without prefixes): "_xlfn.", "_xlfn._xlws."
// or "" for functions every file format knows.
func FuturePrefix(name string) string { return futureFuncs[name] }

// AddFuturePrefixes adds the prefixes Excel requires in files to formula
// text typed without them: =XLOOKUP(...) is stored as _xlfn.XLOOKUP(...),
// and LET and LAMBDA variables get "_xlpm.". Prefixes already present are
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
	var stack []frame
	for i, t := range toks {
		if t.Kind == TokComma {
			if n := len(stack); n > 0 {
				fr := &stack[n-1]
				if fr.toks == 1 && toks[i-1].Kind == TokName && toks[i-1].Sheet == "" &&
					(fr.fn == "LAMBDA" || fr.fn == "LET" && fr.arg%2 == 0) {
					params[strings.ToUpper(toks[i-1].Text)] = true
				}
				fr.arg++
				fr.toks = 0
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
	for _, t := range toks {
		raw := f[t.Pos:t.End]
		prefix := ""
		switch t.Kind {
		case TokFunc:
			if stripPrefixes(raw) == raw {
				prefix = futureFuncs[t.Text]
			}
		case TokName:
			if t.Sheet == "" && stripPrefixes(raw) == raw && params[strings.ToUpper(t.Text)] {
				prefix = "_xlpm."
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
