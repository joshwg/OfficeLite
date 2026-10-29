// REGEXTEST, REGEXEXTRACT and REGEXREPLACE on Go's RE2 engine, with a
// compiled-pattern cache.

package text

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	f "officelite/excel/formula"
)

// The REGEX functions use Go's RE2 engine where Excel uses PCRE2. Syntax
// RE2 lacks (lookahead/lookbehind, backreferences, possessive quantifiers,
// recursion) is reported as an invalid pattern, #VALUE!.

var regexCache sync.Map // (?i)-prefixed or plain pattern -> *regexp.Regexp

// compile reads the pattern and case_sensitivity (0 sensitive, the
// default; 1 insensitive) arguments.
func compile(pattern, caseArg f.Value) (*regexp.Regexp, f.Value) {
	p, err := f.Str(pattern)
	if err != nil {
		return nil, err
	}
	mode, err := intArg(caseArg, 0)
	if err != nil {
		return nil, err
	}
	switch mode {
	case 0:
	case 1:
		p = "(?i)" + p
	default:
		return nil, f.ErrValue
	}
	if re, ok := regexCache.Load(p); ok {
		return re.(*regexp.Regexp), nil
	}
	re, cerr := regexp.Compile(p)
	if cerr != nil {
		return nil, f.ErrValue
	}
	regexCache.Store(p, re)
	return re, nil
}

// regexTest implements REGEXTEST(text, pattern, [case_sensitivity]).
func regexTest(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	re, err := compile(a[1], arg(a, 2))
	if err != nil {
		return err
	}
	return f.Bool(re.MatchString(s))
}

// regexExtract implements REGEXEXTRACT(text, pattern, [return_mode],
// [case_sensitivity]): return_mode 0 gives the first match, 1 all matches as a column, 2 the
// first match's capture groups as a row. No match is #N/A.
func regexExtract(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	mode, err := intArg(arg(a, 2), 0)
	if err != nil {
		return err
	}
	re, err := compile(a[1], arg(a, 3))
	if err != nil {
		return err
	}
	switch mode {
	case 0:
		loc := re.FindStringIndex(s)
		if loc == nil {
			return f.ErrNA
		}
		return f.String(s[loc[0]:loc[1]])
	case 1:
		all := re.FindAllString(s, -1)
		if all == nil {
			return f.ErrNA
		}
		out := f.NewArray(len(all), 1)
		for i, m := range all {
			out.V[i] = f.String(m)
		}
		return out
	case 2:
		loc := re.FindStringSubmatchIndex(s)
		if loc == nil {
			return f.ErrNA
		}
		if len(loc) == 2 { // no groups: the whole match
			return f.String(s[loc[0]:loc[1]])
		}
		out := f.NewArray(1, len(loc)/2-1)
		for g := 1; g < len(loc)/2; g++ {
			out.V[g-1] = f.String("")
			if loc[2*g] >= 0 {
				out.V[g-1] = f.String(s[loc[2*g]:loc[2*g+1]])
			}
		}
		return out
	}
	return f.ErrValue
}

// regexReplace implements REGEXREPLACE(text, pattern, replacement,
// [occurrence], [case_sensitivity]): occurrence 0 replaces every match, n the nth, -n the nth from the end.
// The replacement may refer to groups as $1, ${1}, ${name} and $0 for the
// whole match; $$ is a literal $.
func regexReplace(_ *f.Env, a []f.Value) f.Value {
	s, err := f.Str(a[0])
	if err != nil {
		return err
	}
	repl, err := f.Str(a[2])
	if err != nil {
		return err
	}
	occ, err := intArg(arg(a, 3), 0)
	if err != nil {
		return err
	}
	re, err := compile(a[1], arg(a, 4))
	if err != nil {
		return err
	}
	matches := re.FindAllStringSubmatchIndex(s, -1)
	switch {
	case occ > 0:
		if occ > len(matches) {
			return f.String(s)
		}
		matches = matches[occ-1 : occ]
	case occ < 0:
		if -occ > len(matches) {
			return f.String(s)
		}
		matches = matches[len(matches)+occ : len(matches)+occ+1]
	}
	var b strings.Builder
	prev := 0
	for _, m := range matches {
		b.WriteString(s[prev:m[0]])
		if !expand(&b, re, repl, s, m) {
			return f.ErrValue
		}
		prev = m[1]
	}
	b.WriteString(s[prev:])
	return result(b.String())
}

// expand writes the replacement for one match, PCRE2 style: $ followed by
// digits names a group number (all the digits), ${...} a number or name.
// A reference to a group that does not exist is an error.
func expand(b *strings.Builder, re *regexp.Regexp, repl, s string, m []int) bool {
	group := func(name string) bool {
		g, err := strconv.Atoi(name)
		if err != nil {
			g = re.SubexpIndex(name)
		}
		if g < 0 || g > re.NumSubexp() {
			return false
		}
		if m[2*g] >= 0 {
			b.WriteString(s[m[2*g]:m[2*g+1]])
		}
		return true
	}
	for i := 0; i < len(repl); i++ {
		c := repl[i]
		if c != '$' || i+1 == len(repl) {
			b.WriteByte(c)
			continue
		}
		switch n := repl[i+1]; {
		case n == '$':
			b.WriteByte('$')
			i++
		case n >= '0' && n <= '9':
			j := i + 1
			for j < len(repl) && repl[j] >= '0' && repl[j] <= '9' {
				j++
			}
			if !group(repl[i+1 : j]) {
				return false
			}
			i = j - 1
		case n == '{':
			end := strings.IndexByte(repl[i:], '}')
			if end < 0 {
				return false
			}
			if !group(repl[i+2 : i+end]) {
				return false
			}
			i += end
		default:
			b.WriteByte(c)
		}
	}
	return true
}
