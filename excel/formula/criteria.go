// criteria.go parses and matches the criteria of the *IF(S) and database
// functions, with Excel's wildcard matching.

package formula

import (
	"strings"
	"unicode/utf8"
)

// Criteria is a parsed condition as used by SUMIF(S), COUNTIF(S),
// AVERAGEIF(S) ...: 5, ">=5", "<>apple", "a*", "", "=" etc.
type Criteria struct {
	op      string // "=", "<>", "<", "<=", ">", ">="
	num     float64
	isNum   bool
	text    string // operand text (for text comparisons / wildcards)
	boolean *bool
	err     Error
	empty   bool // operand is "" (match blanks for "=", non-blanks for "<>")
}

// ParseCriteria interprets a criteria argument.
func ParseCriteria(v Value) Criteria {
	switch x := v.(type) {
	case Number:
		return Criteria{op: "=", num: float64(x), isNum: true}
	case Bool:
		b := bool(x)
		return Criteria{op: "=", boolean: &b}
	case Error:
		return Criteria{op: "=", err: x}
	case Blank, Missing:
		// Excel treats a blank criteria cell as "=0".
		return Criteria{op: "=", num: 0, isNum: true}
	}
	s, _ := Str(v)
	c := Criteria{op: "="}
	for _, op := range []string{">=", "<=", "<>", "=", ">", "<"} {
		if strings.HasPrefix(s, op) {
			c.op, s = op, s[len(op):]
			break
		}
	}
	c.text = s
	switch {
	case s == "":
		c.empty = true
	case strings.EqualFold(s, "TRUE"), strings.EqualFold(s, "FALSE"):
		b := strings.EqualFold(s, "TRUE")
		c.boolean = &b
	case strings.HasPrefix(s, "#"):
		if e, n := lexError(s); n == len(s) {
			c.err = Error(e)
		}
	}
	if f, ok := ParseNumber(s); ok && c.boolean == nil {
		c.num, c.isNum = f, true
	}
	return c
}

// ParseDatabaseCriteria interprets a condition cell of a database
// function's criteria range (DSUM, DCOUNT ...). It differs from
// ParseCriteria in two ways: a blank cell is no condition at all (ok is
// false; the cell matches every record), and text without a comparison
// operator matches text that begins with it, as in Advanced Filter: "Ap"
// is "Ap*" while "=Ap" is an exact match.
func ParseDatabaseCriteria(v Value) (c Criteria, ok bool) {
	switch x := v.(type) {
	case Blank, Missing:
		return Criteria{}, false
	case String:
		if x == "" {
			return Criteria{}, false
		}
	}
	c = ParseCriteria(v)
	if s, isText := v.(String); isText && c.op == "=" && !strings.HasPrefix(string(s), "=") &&
		!c.isNum && c.boolean == nil && c.err == "" && !c.empty {
		c.text += "*"
	}
	return c, true
}

// Match reports whether a cell value satisfies the criteria.
func (c Criteria) Match(v Value) bool {
	switch {
	case c.err != "":
		e, ok := v.(Error)
		eq := ok && e == c.err
		return eq == (c.op == "=") || (c.op == "<>" && !eq)
	case c.empty:
		blank := false
		switch x := v.(type) {
		case Blank, Missing:
			blank = true
		case String:
			blank = x == ""
		}
		switch c.op {
		case "=":
			return blank
		case "<>":
			return !blank
		}
		return false
	case c.boolean != nil:
		b, ok := v.(Bool)
		eq := ok && bool(b) == *c.boolean
		if c.op == "<>" {
			return !eq
		}
		if c.op == "=" {
			return eq
		}
		if !ok {
			return false
		}
		cmp, _ := Compare(b, Bool(*c.boolean))
		return compareOp(c.op, cmp)
	case c.isNum:
		var f float64
		switch x := v.(type) {
		case Number:
			f = float64(x)
		case String:
			// "=5" also matches the text "5".
			g, ok := ParseNumber(string(x))
			if !ok || (c.op != "=" && c.op != "<>") {
				return c.op == "<>"
			}
			f = g
		default:
			return c.op == "<>"
		}
		cmp := 0
		if f < c.num {
			cmp = -1
		} else if f > c.num {
			cmp = 1
		}
		return compareOp(c.op, cmp)
	}
	// Text criteria.
	s, isText := v.(String)
	switch c.op {
	case "=":
		return isText && WildcardMatch(c.text, string(s))
	case "<>":
		return !(isText && WildcardMatch(c.text, string(s)))
	}
	if !isText {
		return false
	}
	cmp := strings.Compare(strings.ToLower(string(s)), strings.ToLower(c.text))
	return compareOp(c.op, cmp)
}

// compareOp reports whether a three-way comparison result cmp satisfies the
// comparison operator op.
func compareOp(op string, cmp int) bool {
	switch op {
	case "=":
		return cmp == 0
	case "<>":
		return cmp != 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	}
	return false
}

// WildcardMatch matches s against an Excel wildcard pattern, ignoring case:
// '*' is any run of characters, '?' any one character, and '~' escapes the
// next character.
func WildcardMatch(pattern, s string) bool {
	p := []rune(strings.ToLower(pattern))
	t := []rune(strings.ToLower(s))
	var match func(i, j int) bool
	memo := map[[2]int]bool{}
	match = func(i, j int) bool {
		key := [2]int{i, j}
		if v, ok := memo[key]; ok {
			return v
		}
		var res bool
		switch {
		case i == len(p):
			res = j == len(t)
		case p[i] == '*':
			res = match(i+1, j) || (j < len(t) && match(i, j+1))
		case p[i] == '~' && i+1 < len(p):
			res = j < len(t) && t[j] == p[i+1] && match(i+2, j+1)
		case p[i] == '?':
			res = j < len(t) && match(i+1, j+1)
		default:
			res = j < len(t) && t[j] == p[i] && match(i+1, j+1)
		}
		memo[key] = res
		return res
	}
	return match(0, 0)
}

// HasWildcards reports whether pattern contains unescaped * or ?.
func HasWildcards(pattern string) bool {
	for i := 0; i < len(pattern); {
		r, n := utf8.DecodeRuneInString(pattern[i:])
		switch r {
		case '~':
			i += n
			if i < len(pattern) {
				_, m := utf8.DecodeRuneInString(pattern[i:])
				i += m
			}
			continue
		case '*', '?':
			return true
		}
		i += n
	}
	return false
}
