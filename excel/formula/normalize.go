package formula

import "strings"

// Normalize checks typed formula text (without '=') and writes it the way
// Excel stores it: function names, cell references and TRUE/FALSE in upper
// case, and missing closing parentheses added. Strings, sheet names and
// defined names are left as typed. It returns Parse's error for formulas
// that don't parse.
func Normalize(f string) (string, error) {
	if _, err := Parse(f); err != nil {
		// Excel closes parentheses left open at the end: =SUM(A1:A3
		open := 0
		for i := 0; i < len(f); i++ {
			switch f[i] {
			case '"', '\'':
				i = skipQuoted(f, i) - 1
			case '(':
				open++
			case ')':
				open--
			}
		}
		if open <= 0 {
			return f, err
		}
		g := f + strings.Repeat(")", open)
		if _, err2 := Parse(g); err2 != nil {
			return f, err
		}
		f = g
	}
	f = rewriteRefs(f, func(t *refTok) string {
		if t.name != "" {
			return t.text
		}
		return t.prefix + strings.ToUpper(t.text[len(t.prefix):])
	})
	var b strings.Builder
	b.Grow(len(f))
	for i := 0; i < len(f); {
		c := f[i]
		switch {
		case c == '"' || c == '\'':
			j := skipQuoted(f, i)
			b.WriteString(f[i:j])
			i = j
		case isIdentStart(c) && (i == 0 || !isIdentChar(f[i-1]) && f[i-1] != '$'):
			j := i + 1
			for j < len(f) && isIdentChar(f[j]) {
				j++
			}
			word := f[i:j]
			if up := strings.ToUpper(word); (j < len(f) && f[j] == '(') || up == "TRUE" || up == "FALSE" {
				word = up
			}
			b.WriteString(word)
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}
