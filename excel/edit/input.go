// Package edit changes a workbook the way Excel's editing commands do:
// typed input, clearing, copy and paste, inserting and deleting rows,
// columns and sheets. Every change is recorded for undo and redo, and the
// affected formulas are recalculated.
package edit

import (
	"regexp"
	"strconv"
	"strings"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// Input is typed cell content as Excel interprets it.
type Input struct {
	Kind    model.Kind
	Num     float64
	Text    string // the text of a text cell, or the error of an error cell
	Formula string // without '='; set for formulas (Kind is then Empty)
	// Format is the number format Excel gives a General cell for this
	// input: "0%" for 12%, "m/d/yyyy" for 1/15/2026 ...; "" for none.
	Format string
}

// FormulaError is returned by ParseInput for a formula that doesn't parse.
type FormulaError struct {
	Text string
	Err  error
}

func (e *FormulaError) Error() string {
	return "There's a problem with this formula: =" + e.Text
}

var (
	reNumber = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)
	// A number with any of: sign, parentheses, currency sign, thousands
	// separators, percent sign.
	reDecorated = regexp.MustCompile(`^(\()?([+-])?(\$)?([+-])?(\d{1,3}(,\d{3})+|\d+)?(\.\d*)?(%)?(\))?$`)
)

// ParseInput interprets text typed into a cell: a formula (=..., or
// +/- followed by a formula), a number (1,234.5, (12), 12%, $5.00, 1E3),
// a date or time, TRUE/FALSE, an error value, or text. A leading
// apostrophe makes the rest text.
func ParseInput(s string, date1904 bool) (Input, error) {
	if s == "" {
		return Input{}, nil
	}
	if s[0] == '\'' {
		return Input{Kind: model.KindText, Text: s[1:]}, nil
	}
	if s[0] == '=' && len(s) > 1 {
		f, err := formula.Normalize(s[1:])
		if err != nil {
			return Input{}, &FormulaError{Text: s[1:], Err: err}
		}
		return Input{Formula: f}, nil
	}
	t := strings.TrimSpace(s)
	if in, ok := parseNumber(t); ok {
		return in, nil
	}
	switch up := strings.ToUpper(t); up {
	case "TRUE":
		return Input{Kind: model.KindBool, Num: 1, Text: "TRUE"}, nil
	case "FALSE":
		return Input{Kind: model.KindBool, Text: "FALSE"}, nil
	default:
		if isErrorLiteral(up) {
			return Input{Kind: model.KindError, Text: up}, nil
		}
	}
	if v, ok := formula.ParseDateTime(t, date1904); ok {
		return Input{Kind: model.KindNumber, Num: v, Format: dateFormat(t)}, nil
	}
	// +A1 and -SUM(A1:A3) are formulas in Excel. Text such as "- item"
	// (a name, which Excel would also take as a formula) stays text.
	if (s[0] == '+' || s[0] == '-') && len(s) > 1 {
		if f, err := formula.Normalize(s); err == nil {
			n, _ := formula.Parse(f)
			if d := formula.Dependencies(n); len(d.Refs) > 0 || len(d.Funcs) > 0 {
				return Input{Formula: f}, nil
			}
		}
	}
	return Input{Kind: model.KindText, Text: s}, nil
}

func isErrorLiteral(s string) bool {
	switch formula.Error(s) {
	case formula.ErrNA, formula.ErrDiv0, formula.ErrValue, formula.ErrRef,
		formula.ErrName, formula.ErrNum, formula.ErrNull:
		return true
	}
	return false
}

// parseNumber reads plain and decorated numbers.
func parseNumber(t string) (Input, bool) {
	if reNumber.MatchString(t) {
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return Input{}, false
		}
		in := Input{Kind: model.KindNumber, Num: v}
		if strings.ContainsAny(t, "eE") {
			in.Format = "0.00E+00"
		}
		return in, true
	}
	m := reDecorated.FindStringSubmatch(t)
	if m == nil || (m[5] == "" && len(m[7]) < 2) {
		return Input{}, false
	}
	open, sign1, cur, sign2, intPart, frac, pct, closing := m[1], m[2], m[3], m[4], m[5], m[7], m[8], m[9]
	if (open == "") != (closing == "") || (sign1 != "" && sign2 != "") || (cur != "" && pct != "") {
		return Input{}, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(intPart, ",", "")+frac, 64)
	if err != nil {
		return Input{}, false
	}
	if open != "" {
		if sign1 != "" || sign2 != "" {
			return Input{}, false
		}
		v = -v
	}
	if sign1 == "-" || sign2 == "-" {
		v = -v
	}
	decimals := len(frac) > 1
	in := Input{Kind: model.KindNumber, Num: v}
	switch {
	case pct != "":
		in.Num /= 100
		in.Format = "0%"
		if decimals {
			in.Format = "0.00%"
		}
	case cur != "":
		in.Format = `"$"#,##0_);\("$"#,##0\)`
		if decimals {
			in.Format = `"$"#,##0.00_);\("$"#,##0.00\)`
		}
	case strings.Contains(intPart, ","):
		in.Format = "#,##0"
		if decimals {
			in.Format = "#,##0.00"
		}
	}
	return in, true
}

var (
	reTimeOnly = regexp.MustCompile(`(?i)^\d{1,2}(:\d{1,2})?(:\d{1,2}(\.\d+)?)?\s*(am|pm|a|p)?$`)
	reNumDate  = regexp.MustCompile(`^\d{1,4}[/-]\d{1,2}([/-]\d{1,4})?$`)
	reMonYear  = regexp.MustCompile(`(?i)^[a-z]+\.?[-\s/]+\d{4}$`)
)

// dateFormat is the format Excel gives a cell for typed date or time text.
func dateFormat(t string) string {
	timeFmt := func(s string) string {
		f := "h:mm"
		if strings.Count(s, ":") == 2 {
			f = "h:mm:ss"
		}
		if strings.ContainsAny(strings.ToLower(s), "ap") {
			f += " AM/PM"
		}
		return f
	}
	if reTimeOnly.MatchString(t) {
		return timeFmt(t)
	}
	date := t
	if i := strings.IndexByte(t, ':'); i > 0 {
		// Date and time: 1/15/2026 3:45 PM.
		if sp := strings.LastIndexAny(t[:i], " T"); sp > 0 {
			return "m/d/yyyy h:mm"
		}
	}
	switch {
	case reNumDate.MatchString(date):
		if strings.Count(date, "/")+strings.Count(date, "-") == 1 {
			return "d-mmm" // 1/15 shows as 15-Jan
		}
		return "m/d/yyyy"
	case reMonYear.MatchString(date):
		return "mmm-yy"
	}
	// Month names: 15-Jan-2026 or Jan 15, 2026 show as 15-Jan-26; without
	// a year, 15-Jan.
	if strings.ContainsAny(date, "0123456789") && hasYear(date) {
		return "d-mmm-yy"
	}
	return "d-mmm"
}

// hasYear reports whether date text with a month name has a year: a
// second group of digits.
func hasYear(s string) bool {
	groups := 0
	in := false
	for i := 0; i < len(s); i++ {
		d := s[i] >= '0' && s[i] <= '9'
		if d && !in {
			groups++
		}
		in = d
	}
	return groups >= 2
}
