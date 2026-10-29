// lexer.go splits formula text into tokens: numbers, strings, errors,
// references, names, function names, operators and punctuation.

package formula

import (
	"fmt"
	"strconv"
	"strings"
)

// tokKind classifies a lexer token.
type tokKind int

const (
	tEOF tokKind = iota
	tNumber
	tString
	tBool
	tError
	tRef    // cell, column range (A:B) or row range (1:3), optionally sheet-qualified
	tName   // identifier: defined name, LET/LAMBDA variable
	tFunc   // identifier immediately followed by '('
	tOp     // + - * / ^ & = <> < <= > >= : %
	tLParen // (
	tRParen // )
	tComma  // ,
	tSemi   // ; (row separator in array constants)
	tLBrace // {
	tRBrace // }
	tHash   // # (spill reference)
	tAt     // @ (implicit intersection)
)

// token is one lexer token; pos is its byte offset in the source.
type token struct {
	kind  tokKind
	text  string
	num   float64
	sheet string // for tRef and tName with a sheet prefix
	pos   int
}

// futurePrefixes are the prefixes Excel writes in files for functions and
// names added after 2007.
var futurePrefixes = []string{"_XLFN._XLWS.", "_XLFN.", "_XLWS.", "_XLPM.", "_XLUDF.", "_XLETA."}

// stripPrefixes removes any leading futurePrefixes from s, ignoring case.
func stripPrefixes(s string) string {
	for again := true; again; {
		again = false
		for _, p := range futurePrefixes {
			if len(s) > len(p) && strings.EqualFold(s[:len(p)], p) {
				s, again = s[len(p):], true
			}
		}
	}
	return s
}

// lex splits formula text (without '=') into tokens ending with tEOF.
func lex(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		start := i
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
			continue
		case c == '"':
			var b strings.Builder
			i++
			for {
				if i >= len(src) {
					return nil, fmt.Errorf("unterminated string")
				}
				if src[i] == '"' {
					if i+1 < len(src) && src[i+1] == '"' {
						b.WriteByte('"')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteByte(src[i])
				i++
			}
			toks = append(toks, token{kind: tString, text: b.String(), pos: start})
			continue
		case c == '#':
			if e, n := lexError(src[i:]); n > 0 {
				toks = append(toks, token{kind: tError, text: e, pos: start})
				i += n
				continue
			}
			toks = append(toks, token{kind: tHash, pos: start})
			i++
			continue
		case c == '\'':
			// Quoted sheet name: 'My Sheet'!A1
			j := i + 1
			var b strings.Builder
			for j < len(src) {
				if src[j] == '\'' {
					if j+1 < len(src) && src[j+1] == '\'' {
						b.WriteByte('\'')
						j += 2
						continue
					}
					break
				}
				b.WriteByte(src[j])
				j++
			}
			if j+1 >= len(src) || src[j] != '\'' || src[j+1] != '!' {
				return nil, fmt.Errorf("bad sheet reference at %d", i)
			}
			i = j + 2
			t, n, err := lexRefOrName(src[i:], b.String())
			if err != nil {
				return nil, err
			}
			t.pos = start
			toks = append(toks, t)
			i += n
			continue
		case c >= '0' && c <= '9' || c == '.':
			// A row range (1:3) or a number.
			if n := matchRowRange(src[i:]); n > 0 {
				toks = append(toks, token{kind: tRef, text: src[i : i+n], pos: start})
				i += n
				continue
			}
			n := scanNumber(src[i:])
			if n == 0 {
				return nil, fmt.Errorf("unexpected %q", c)
			}
			f, err := strconv.ParseFloat(src[i:i+n], 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", src[i:i+n])
			}
			toks = append(toks, token{kind: tNumber, num: f, text: src[i : i+n], pos: start})
			i += n
			continue
		case isIdentStart(c) || c == '$':
			t, n, err := lexRefOrName(src[i:], "")
			if err != nil {
				return nil, err
			}
			t.pos = start
			toks = append(toks, t)
			i += n
			continue
		}
		// Operators and punctuation.
		two := ""
		if i+1 < len(src) {
			two = src[i : i+2]
		}
		switch {
		case two == "<>" || two == "<=" || two == ">=":
			toks = append(toks, token{kind: tOp, text: two, pos: start})
			i += 2
		case strings.IndexByte("+-*/^&=<>:%", c) >= 0:
			toks = append(toks, token{kind: tOp, text: string(c), pos: start})
			i++
		case c == '(':
			toks = append(toks, token{kind: tLParen, pos: start})
			i++
		case c == ')':
			toks = append(toks, token{kind: tRParen, pos: start})
			i++
		case c == ',':
			toks = append(toks, token{kind: tComma, pos: start})
			i++
		case c == ';':
			toks = append(toks, token{kind: tSemi, pos: start})
			i++
		case c == '{':
			toks = append(toks, token{kind: tLBrace, pos: start})
			i++
		case c == '}':
			toks = append(toks, token{kind: tRBrace, pos: start})
			i++
		case c == '@':
			toks = append(toks, token{kind: tAt, pos: start})
			i++
		default:
			return nil, fmt.Errorf("unexpected %q at %d", c, i)
		}
	}
	return append(toks, token{kind: tEOF, pos: len(src)}), nil
}

// errorLiterals are the error constants a formula can contain.
var errorLiterals = []string{
	"#GETTING_DATA", "#DIV/0!", "#VALUE!", "#NAME?", "#NULL!", "#SPILL!",
	"#CALC!", "#REF!", "#NUM!", "#N/A", "#CONNECT!", "#BLOCKED!", "#UNKNOWN!",
	"#FIELD!", "#BUSY!", "#EXTERNAL!",
}

// lexError matches an error literal at the start of s, ignoring case, and
// returns its canonical text and length (0 if none).
func lexError(s string) (string, int) {
	for _, e := range errorLiterals {
		if len(s) >= len(e) && strings.EqualFold(s[:len(e)], e) {
			return e, len(e)
		}
	}
	return "", 0
}

// isIdentStart reports whether c can start a name: a letter, '_', '\' or a
// non-ASCII byte.
func isIdentStart(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '\\' || c >= 0x80
}

// isIdentChar reports whether c can continue a name: also digits, '.' and
// '?'.
func isIdentChar(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '.' || c == '?'
}

// scanNumber returns the length of the number literal (digits, fraction,
// exponent) at the start of s, or 0.
func scanNumber(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i == 0 || (i == 1 && s[0] == '.') {
		return 0
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		k := j
		for k < len(s) && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		if k > j {
			i = k
		}
	}
	return i
}

// maxLexCol is Excel's last column (XFD): longer letter runs are names.
const maxLexCol = 16384

// matchCol returns the length of a column part ($ and up to XFD) at the
// start of s, or 0.
func matchCol(s string) int {
	i := 0
	if i < len(s) && s[i] == '$' {
		i++
	}
	j := i
	col := 0
	for j < len(s) && j-i < 3 && (s[j] >= 'A' && s[j] <= 'Z' || s[j] >= 'a' && s[j] <= 'z') {
		col = col*26 + int(s[j]&^0x20-'A') + 1
		j++
	}
	if j == i || col > maxLexCol {
		return 0 // not a column: none, or past XFD (xml1, yes1 are names)
	}
	return j
}

// matchRow returns the length of a row part ($ and digits not starting with
// 0) at the start of s, or 0.
func matchRow(s string) int {
	i := 0
	if i < len(s) && s[i] == '$' {
		i++
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == i || s[i] == '0' {
		return 0
	}
	return j
}

// endsToken reports whether a match of length n is not followed by a name
// character or '('.
func endsToken(s string, n int) bool {
	return n >= len(s) || !isIdentChar(s[n]) && s[n] != '('
}

// matchCell matches A1, $A$1 (not followed by identifier characters).
func matchCell(s string) int {
	n := matchCol(s)
	if n == 0 {
		return 0
	}
	m := matchRow(s[n:])
	if m == 0 || !endsToken(s, n+m) {
		return 0
	}
	return n + m
}

// matchColRange matches A:B, $A:$C.
func matchColRange(s string) int {
	n := matchCol(s)
	if n == 0 || n >= len(s) || s[n] != ':' {
		return 0
	}
	m := matchCol(s[n+1:])
	if m == 0 || !endsToken(s, n+1+m) {
		return 0
	}
	return n + 1 + m
}

// matchRowRange matches 1:3, $1:$3.
func matchRowRange(s string) int {
	n := matchRow(s)
	if n == 0 || n >= len(s) || s[n] != ':' {
		return 0
	}
	m := matchRow(s[n+1:])
	if m == 0 || !endsToken(s, n+1+m) {
		return 0
	}
	return n + 1 + m
}

// lexRefOrName lexes a reference, function name, boolean or defined name at
// the start of s. sheet is a sheet prefix already consumed.
func lexRefOrName(s, sheet string) (token, int, error) {
	if sheet == "" {
		// An unquoted sheet name may look like a cell or column (S1, Day1,
		// XFE1), so test for the '!' before trying references.
		n := 0
		for n < len(s) && isIdentChar(s[n]) {
			n++
		}
		if n > 0 && n < len(s) && s[n] == '!' {
			t, m, err := lexRefOrName(s[n+1:], s[:n])
			return t, n + 1 + m, err
		}
	}
	if n := matchColRange(s); n > 0 {
		return token{kind: tRef, text: s[:n], sheet: sheet}, n, nil
	}
	if n := matchRowRange(s); n > 0 {
		return token{kind: tRef, text: s[:n], sheet: sheet}, n, nil
	}
	if n := matchCell(s); n > 0 {
		return token{kind: tRef, text: s[:n], sheet: sheet}, n, nil
	}
	if s != "" && s[0] == '$' {
		return token{}, 0, fmt.Errorf("bad reference %q", s)
	}
	n := 0
	for n < len(s) && isIdentChar(s[n]) {
		n++
	}
	if n == 0 {
		return token{}, 0, fmt.Errorf("expected a reference")
	}
	word := s[:n]
	// Unquoted sheet name: Sheet1!A1
	if n < len(s) && s[n] == '!' && sheet == "" {
		t, m, err := lexRefOrName(s[n+1:], word)
		return t, n + 1 + m, err
	}
	if n < len(s) && s[n] == '(' && sheet == "" {
		return token{kind: tFunc, text: strings.ToUpper(stripPrefixes(word))}, n, nil
	}
	switch up := strings.ToUpper(word); {
	case up == "TRUE" && sheet == "":
		return token{kind: tBool, text: up}, n, nil
	case up == "FALSE" && sheet == "":
		return token{kind: tBool, text: up}, n, nil
	}
	return token{kind: tName, text: stripPrefixes(word), sheet: sheet}, n, nil
}
