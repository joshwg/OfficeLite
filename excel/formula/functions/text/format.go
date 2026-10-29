package text

import (
	"math"
	"strconv"
	"strings"
	"sync"

	f "officelite/excel/formula"
	"officelite/excel/numfmt"
)

// The formatter is expensive to create and not safe for concurrent use:
// keep one behind a mutex. It always uses the 1900 date system (a
// 1904-system formatter from numfmt renders dates wrongly); 1904 serials
// are shifted instead (see formatSections).
var (
	fmtMu     sync.Mutex
	formatter *numfmt.Formatter
)

func formatNumber(v float64, code string) string {
	fmtMu.Lock()
	defer fmtMu.Unlock()
	if formatter == nil {
		formatter = numfmt.New(false)
	}
	return formatter.Format(v, 0, code)
}

// isDateFormat reports whether a format section shows a calendar date
// (y, m, d, e outside quotes; E+ and E- are scientific notation), so a 1904 serial must be shifted. Elapsed
// time ([h], [m], [s]) is a duration and is not shifted.
func isDateFormat(sec string) bool {
	date := false
	for i := 0; i < len(sec); i++ {
		switch c := sec[i] | 0x20; {
		case sec[i] == '"':
			if j := strings.IndexByte(sec[i+1:], '"'); j >= 0 {
				i += j + 1
			} else {
				i = len(sec)
			}
		case sec[i] == '\\', sec[i] == '_', sec[i] == '*':
			i++
		case sec[i] == '[':
			if i+1 < len(sec) && strings.ContainsRune("hHmMsS", rune(sec[i+1])) {
				return false
			}
			if j := strings.IndexByte(sec[i:], ']'); j >= 0 {
				i += j
			}
		case c == 'e' && i+1 < len(sec) && (sec[i+1] == '+' || sec[i+1] == '-'):
			// scientific notation
		case c == 'y' || c == 'd' || c == 'm' || c == 'e':
			date = true
		}
	}
	return date
}

// parseNumber is f.ParseNumber honoring the workbook's date system for
// date text.
func parseNumber(e *f.Env, s string) (float64, bool) {
	v, ok := f.ParseNumber(s)
	if ok && e.Date1904() {
		if d, isDate := f.ParseDateTime(s, false); isDate && d == v {
			v, _ = f.ParseDateTime(s, true)
		}
	}
	return v, ok
}

// --- TEXT ----------------------------------------------------------------------

// TEXT(value, format_text)
func textFn(e *f.Env, a []f.Value) f.Value {
	code, err := f.Str(a[1])
	if err != nil {
		return err
	}
	var x float64
	switch v := a[0].(type) {
	case f.Error:
		return v
	case f.Bool:
		s, _ := f.Str(v) // booleans are not formatted
		return f.String(s)
	case f.String:
		n, ok := parseNumber(e, string(v))
		if !ok {
			return f.String(formatText(string(v), code))
		}
		x = n
	default:
		x, _ = f.Num(v)
	}
	if code == "" {
		return f.String("")
	}
	return result(formatSections(x, code, e.Date1904()))
}

// formatSections picks the section for x's sign itself (the renderer
// mishandles a zero section) and renders it. Conditional formats such as
// [>100] are left to the renderer.
func formatSections(x float64, code string, date1904 bool) string {
	secs := numfmt.SplitSections(code)
	conditional := false
	for _, s := range secs {
		if strings.HasPrefix(s, "[<") || strings.HasPrefix(s, "[>") || strings.HasPrefix(s, "[=") {
			conditional = true
		}
	}
	sec := code
	if len(secs) > 1 && !conditional {
		switch {
		case x < 0:
			sec, x = secs[1], -x // the negative section supplies its own sign
		case x == 0 && len(secs) > 2:
			sec = secs[2]
		default:
			sec = secs[0]
		}
		if sec == "" {
			return ""
		}
	}
	if strings.EqualFold(strings.TrimSpace(sec), "General") {
		// TEXT's General is not limited to a column width.
		return f.NumberText(x)
	}
	if date1904 && isDateFormat(sec) {
		x += f.Offset1904
	}
	return formatNumber(x, sec)
}

// formatText applies a format to text: the fourth section, or a lone
// section containing '@', with '@' standing for the text. Other formats
// leave text unchanged.
func formatText(s, code string) string {
	secs := numfmt.SplitSections(code)
	var sec string
	switch {
	case len(secs) >= 4:
		sec = secs[3]
	case len(secs) == 1 && strings.Contains(code, "@"):
		sec = code
	default:
		return s
	}
	var b strings.Builder
	for i := 0; i < len(sec); i++ {
		switch c := sec[i]; c {
		case '"':
			j := strings.IndexByte(sec[i+1:], '"')
			if j < 0 {
				j = len(sec) - i - 1
			}
			b.WriteString(sec[i+1 : i+1+j])
			i += j + 1
		case '\\':
			if i+1 < len(sec) {
				i++
				b.WriteByte(sec[i])
			}
		case '@':
			b.WriteString(s)
		case '_': // space the width of the next character
			i++
			b.WriteByte(' ')
		case '*': // fill: no column width here
			i++
		case '[':
			if j := strings.IndexByte(sec[i:], ']'); j >= 0 {
				i += j
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// --- FIXED / DOLLAR -------------------------------------------------------------------

// fixedText formats |x| rounded to d decimals, with thousands separators
// unless noCommas. neg reports whether the rounded value is negative.
func fixedText(x float64, d int, noCommas bool) (s string, neg bool) {
	r := f.RoundDigits(x, d, f.RoundHalf)
	s = strconv.FormatFloat(math.Abs(r), 'f', max(d, 0), 64)
	if !noCommas {
		intPart, frac, hasFrac := strings.Cut(s, ".")
		var b strings.Builder
		for i, c := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(c)
		}
		if hasFrac {
			b.WriteByte('.')
			b.WriteString(frac)
		}
		s = b.String()
	}
	return s, r < 0
}

// decimalsArg reads FIXED's and DOLLAR's decimals (at most 127).
func decimalsArg(v f.Value) (int, f.Value) {
	d, err := intArg(v, 2)
	if err != nil {
		return 0, err
	}
	if d > 127 {
		return 0, f.ErrValue
	}
	return max(d, -400), nil
}

// FIXED(number, [decimals], [no_commas])
func fixed(_ *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	d, err := decimalsArg(arg(a, 1))
	if err != nil {
		return err
	}
	noCommas, err := f.Truth(arg(a, 2))
	if err != nil {
		return err
	}
	s, neg := fixedText(x, d, noCommas)
	if neg {
		s = "-" + s
	}
	return f.String(s)
}

// DOLLAR(number, [decimals]) in en-US currency format: $1,234.57 and
// ($1,234.57) for negatives.
func dollar(_ *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	d, err := decimalsArg(arg(a, 1))
	if err != nil {
		return err
	}
	s, neg := fixedText(x, d, false)
	if neg {
		return f.String("($" + s + ")")
	}
	return f.String("$" + s)
}

// --- VALUE / NUMBERVALUE ----------------------------------------------------------------

// VALUE(text) converts number, date, time, percent or currency text.
func value(e *f.Env, a []f.Value) f.Value {
	switch v := a[0].(type) {
	case f.Number, f.Error:
		return v
	case f.Blank, f.Missing:
		return f.Number(0)
	case f.String:
		if x, ok := parseNumber(e, string(v)); ok {
			return f.Number(x)
		}
	}
	return f.ErrValue
}

// sepArg reads a NUMBERVALUE separator: its first character.
func sepArg(v f.Value, def rune) (rune, f.Value) {
	if _, ok := v.(f.Missing); ok {
		return def, nil
	}
	s, err := f.Str(v)
	if err != nil {
		return 0, err
	}
	if s == "" {
		return 0, f.ErrValue
	}
	return []rune(s)[0], nil
}

// NUMBERVALUE(text, [decimal_separator], [group_separator]) converts text
// with the given separators. Spaces are ignored anywhere, group
// separators may only appear before the decimal separator, and trailing
// percent signs each divide by 100.
func numberValue(e *f.Env, a []f.Value) f.Value {
	var s string
	switch v := a[0].(type) {
	case f.Number, f.Error:
		return v
	case f.Bool:
		return f.ErrValue
	default:
		s, _ = f.Str(v)
	}
	dec, err := sepArg(arg(a, 1), '.')
	if err != nil {
		return err
	}
	group, err := sepArg(arg(a, 2), ',')
	if err != nil {
		return err
	}
	if dec == group {
		return f.ErrValue
	}
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return f.Number(0)
	}
	pct := 0
	for strings.HasSuffix(s, "%") {
		s = s[:len(s)-1]
		pct++
	}
	var b strings.Builder
	seenDec := false
	for _, r := range s {
		switch r {
		case dec:
			if seenDec {
				return f.ErrValue
			}
			seenDec = true
			b.WriteByte('.')
		case group:
			if seenDec {
				return f.ErrValue
			}
		default:
			if r == '.' || r == ',' {
				return f.ErrValue // a separator of another convention
			}
			b.WriteRune(r)
		}
	}
	x, ok := parseNumber(e, b.String())
	if !ok {
		return f.ErrValue
	}
	for ; pct > 0; pct-- {
		x /= 100
	}
	return f.NumberOrErr(x)
}

// --- VALUETOTEXT / ARRAYTOTEXT ------------------------------------------------------------

// toTextFormat reads the format argument: false for concise (0), true for
// strict (1).
func toTextFormat(v f.Value) (bool, f.Value) {
	n, err := intArg(v, 0)
	if err != nil {
		return false, err
	}
	if n != 0 && n != 1 {
		return false, f.ErrValue
	}
	return n == 1, nil
}

// valueText renders a value as VALUETOTEXT does; strict quotes text the
// way a formula would write it. Errors become their text.
func valueText(v f.Value, strict bool) string {
	switch x := v.(type) {
	case f.String:
		if strict {
			return `"` + strings.ReplaceAll(string(x), `"`, `""`) + `"`
		}
		return string(x)
	case f.Error:
		return string(x)
	}
	s, _ := f.Str(v)
	return s
}

// VALUETOTEXT(value, [format])
func valueToText(_ *f.Env, a []f.Value) f.Value {
	strict, err := toTextFormat(arg(a, 1))
	if err != nil {
		return err
	}
	return result(valueText(a[0], strict))
}

// ARRAYTOTEXT(array, [format]): concise joins the values with ", ";
// strict writes an array constant such as {1,"a";TRUE,#N/A}.
func arrayToText(e *f.Env, a []f.Value) f.Value {
	strict, err := toTextFormat(arg(a, 1))
	if err != nil {
		return err
	}
	arr := e.ToArray(a[0])
	var b strings.Builder
	if strict {
		b.WriteByte('{')
	}
	for r := 0; r < arr.Rows; r++ {
		for c := 0; c < arr.Cols; c++ {
			switch {
			case r == 0 && c == 0:
			case !strict:
				b.WriteString(", ")
			case c == 0:
				b.WriteByte(';')
			default:
				b.WriteByte(',')
			}
			b.WriteString(valueText(arr.At(r, c), strict))
		}
	}
	if strict {
		b.WriteByte('}')
	}
	return result(b.String())
}
