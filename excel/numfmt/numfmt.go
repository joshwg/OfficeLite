// Package numfmt renders numbers with Excel number-format codes.
package numfmt

import (
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"officelite/excel/formula"
)

// Formatter turns a number plus an Excel number format into display text.
// General (the common case) is done directly; everything else goes through a
// one-cell scratch excelize workbook, which carries a full implementation of
// Excel's format-code language (dates, currency, fractions, sections...).
type Formatter struct {
	date1904 bool
	scratch  *excelize.File
	styles   map[fmtKey]int
	failed   map[fmtKey]bool
}

// fmtKey identifies a number format by built-in id and custom code.
type fmtKey struct {
	id   int
	code string
}

// Built-in formats whose excelize rendering differs from what Excel shows in
// an en-US locale.
var localeFormats = map[int]string{
	5:  `"$"#,##0_);\("$"#,##0\)`,
	6:  `"$"#,##0_);[Red]\("$"#,##0\)`,
	7:  `"$"#,##0.00_);\("$"#,##0.00\)`,
	8:  `"$"#,##0.00_);[Red]\("$"#,##0.00\)`,
	14: "m/d/yyyy",
	22: "m/d/yyyy h:mm",
}

// New returns a formatter for a workbook using the 1900 or 1904 date
// system.
func New(date1904 bool) *Formatter {
	// The scratch workbook always uses the 1900 system (excelize renders
	// 1904 dates wrongly); 1904 serials are shifted instead, see Format.
	return &Formatter{date1904: date1904, scratch: excelize.NewFile(), styles: map[fmtKey]int{}, failed: map[fmtKey]bool{}}
}

// showsCalendarDate reports whether a format displays a calendar date (as
// opposed to a number, a time of day or elapsed time), i.e. whether the
// date system matters.
func showsCalendarDate(id int, code string) bool {
	if code == "" {
		return id >= 14 && id <= 17 || id == 22 || id >= 27 && id <= 36 || id >= 50 && id <= 58
	}
	var b strings.Builder
	for i := 0; i < len(code); i++ {
		switch c := code[i]; c {
		case '"': // quoted literal
			for i++; i < len(code) && code[i] != '"'; i++ {
			}
		case '\\', '_', '*': // escaped / padding character
			i++
		case '[': // [Red], [h], [$-409]
			for i++; i < len(code) && code[i] != ']'; i++ {
			}
		default:
			b.WriteByte(c | 0x20)
		}
	}
	s := b.String()
	if strings.ContainsAny(s, "yd") {
		return true
	}
	// m is a month unless it is minutes next to hours or seconds.
	return strings.Contains(s, "m") && !strings.ContainsAny(s, "hs")
}

// Close releases the formatter's scratch workbook.
func (nf *Formatter) Close() { _ = nf.scratch.Close() }

// IsGeneral reports whether a format id/code is plain General.
func IsGeneral(id int, code string) bool {
	if code == "" {
		return id == 0
	}
	return strings.EqualFold(code, "General")
}

// Format renders v with built-in format id, or with code when code is set.
func (nf *Formatter) Format(v float64, id int, code string) string {
	if nf.date1904 && showsCalendarDate(id, code) {
		v += formula.Offset1904
	}
	if code == "" {
		code = localeFormats[id]
	}
	if IsGeneral(id, code) {
		return General(v)
	}
	k := fmtKey{id, code}
	if nf.failed[k] {
		return General(v)
	}
	style, ok := nf.styles[k]
	if !ok {
		st := &excelize.Style{NumFmt: id}
		if code != "" {
			st.CustomNumFmt = &code
		}
		var err error
		if style, err = nf.scratch.NewStyle(st); err != nil {
			nf.failed[k] = true
			return General(v)
		}
		nf.styles[k] = style
	}
	const sh, ref = "Sheet1", "A1"
	if nf.scratch.SetCellFloat(sh, ref, v, -1, 64) != nil || nf.scratch.SetCellStyle(sh, ref, ref, style) != nil {
		return General(v)
	}
	s, err := nf.scratch.GetCellValue(sh, ref)
	if err != nil {
		return General(v)
	}
	return s
}

// General mimics Excel's General format in a standard-width column:
// at most 11 characters, switching to scientific notation for very large or
// very small magnitudes.
func General(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "#NUM!"
	}
	if v == 0 {
		return "0"
	}
	if v < 0 {
		return "-" + General(-v) // the sign does not count toward the width
	}
	const maxLen = 11
	if v >= 1e11 || v < 1e-9 {
		return formatScientific(v)
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if len(s) <= maxLen {
		return s
	}
	intLen := len(strconv.FormatFloat(math.Trunc(v), 'f', 0, 64))
	decimals := maxLen - intLen - 1
	if decimals <= 0 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	s = strconv.FormatFloat(v, 'f', decimals, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "0" {
		return formatScientific(v)
	}
	return s
}

// formatScientific renders v in General's scientific form: at most five
// decimals, trailing zeros trimmed (e.g. 1.23457E+11, 1E-10).
func formatScientific(v float64) string {
	s := strconv.FormatFloat(v, 'E', 5, 64) // e.g. 1.23450E+11
	mant, exp, _ := strings.Cut(s, "E")
	if strings.Contains(mant, ".") {
		mant = strings.TrimSuffix(strings.TrimRight(mant, "0"), ".")
	}
	return mant + "E" + exp
}
