// Numbers cell formats: conversion to Excel format codes and display
// rendering of cell values and dates.

package iwork

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Numbers format types (TSK.FormatStructArchive format_type).
const (
	FormatBool           = 1
	FormatDecimal        = 256
	FormatCurrency       = 257
	FormatPercent        = 258
	FormatScientific     = 259
	FormatText           = 260
	FormatDate           = 261
	FormatFraction       = 262
	FormatCheckbox       = 263
	FormatRating         = 267
	FormatDuration       = 268
	FormatBase           = 269
	FormatCustomNumber   = 270
	FormatCustomText     = 271
	FormatCustomDate     = 272
	FormatCustomCurrency = 274
)

// autoPlaces is the decimal_places value meaning "as many as needed".
const autoPlaces = 253

// maxPlaces is the most decimals a format asks for; a larger decimal_places
// (damaged or hostile) is cut to it.
const maxPlaces = 15

// placesOf is the format's decimal places: autoPlaces or 0..maxPlaces.
func placesOf(f Message) int {
	if p := f.Uint(2); p != autoPlaces {
		return int(min(p, maxPlaces))
	}
	return autoPlaces
}

// Epoch is the origin of iWork dates: 2001-01-01 00:00 UTC.
var Epoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// ExcelEpochSerial is the Excel (1900 system) serial number of 2001-01-01.
const ExcelEpochSerial = 36892

// typeCustomFormatList is TSK.CustomFormatListArchive.
const typeCustomFormatList = 222

// resolveFormat replaces a custom format (one with a custom_uid) by its
// definition's default format from the document's custom format lists.
func (d *Document) resolveFormat(f Message) Message {
	uid := f.Msg(41)
	if uid == nil {
		return f
	}
	lo, hi := uid.Uint(1), uid.Uint(2)
	for _, id := range d.OfType(typeCustomFormatList) {
		m := d.Msg(id)
		uids, formats := m.Msgs(1), m.Msgs(2)
		for i, u := range uids {
			if u.Uint(1) == lo && u.Uint(2) == hi && i < len(formats) {
				if def := formats[i].Msg(3); def != nil {
					return def
				}
			}
		}
	}
	return f
}

var currencySymbols = map[string]string{
	"USD": "$", "CAD": "$", "AUD": "$", "NZD": "$", "HKD": "$", "SGD": "$", "MXN": "$",
	"EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥", "INR": "₹", "KRW": "₩", "RUB": "₽",
	"CHF": "CHF ", "SEK": "kr ", "NOK": "kr ", "DKK": "kr ", "BRL": "R$", "ZAR": "R ",
	"ILS": "₪", "TRY": "₺", "PLN": "zł ",
}

// currencySymbol returns the symbol (with any trailing space) for an ISO
// currency code: "$" when code is empty, else the code and a space when the
// symbol isn't known.
func currencySymbol(code string) string {
	if s, ok := currencySymbols[code]; ok {
		return s
	}
	if code == "" {
		return "$"
	}
	return code + " "
}

// ExcelFormat converts a cell's Numbers format to an Excel number format
// code; "" means General.
func (d *Document) ExcelFormat(f Message) string {
	if f == nil {
		return ""
	}
	f = d.resolveFormat(f)
	places := placesOf(f)
	digits := func(def int) string {
		p := places
		if p == autoPlaces {
			p = def
		}
		code := "0"
		if f.Bool(5) {
			code = "#,##0"
		}
		if p > 0 {
			code += "." + strings.Repeat("0", p)
		}
		return code
	}
	negative := func(pos string) string {
		switch f.Uint(4) {
		case 1:
			return pos + ";[Red]" + pos
		case 2:
			return pos + ";(" + pos + ")"
		case 3:
			return pos + ";[Red](" + pos + ")"
		}
		return pos
	}
	switch t := f.Uint(1); t {
	case FormatDecimal:
		if places == autoPlaces && !f.Bool(5) {
			return ""
		}
		return negative(digits(2))
	case FormatCurrency, FormatCustomCurrency:
		sym := currencySymbol(f.Str(3))
		if f.Bool(6) { // accounting style: symbol at the left edge
			return `_(` + quoteLiteral(sym) + `* ` + digits(2) + `_);_(` + quoteLiteral(sym) + `* (` + digits(2) + `);_(` + quoteLiteral(sym) + `* "-"??_);_(@_)`
		}
		return negative(quoteLiteral(sym) + digits(2))
	case FormatPercent:
		return negative(digits(0) + "%")
	case FormatScientific:
		p := places
		if p == autoPlaces {
			p = 2
		}
		code := "0"
		if p > 0 {
			code += "." + strings.Repeat("0", p)
		}
		return code + "E+00"
	case FormatFraction:
		switch f.Uint(11) {
		case 0xfffffffe:
			return "# ??/??"
		case 0xfffffffd:
			return "# ???/???"
		case 2:
			return "# ?/2"
		case 4:
			return "# ?/4"
		case 8:
			return "# ?/8"
		case 16:
			return "# ??/16"
		case 10:
			return "# ?/10"
		case 100:
			return "# ??/100"
		}
		return "# ?/?"
	case FormatDate, FormatCustomDate:
		pattern := f.Str(14)
		if t == FormatCustomDate || pattern == "" {
			pattern = f.Str(18)
		}
		return ExcelDateFormat(pattern)
	case FormatDuration:
		return durationFormat(f)
	case FormatCustomNumber:
		if s := f.Str(18); s != "" && !strings.ContainsAny(s, "\"'") {
			return s
		}
		return negative(digits(0))
	}
	return ""
}

// quoteLiteral quotes s as an Excel format-code literal ("$" stays bare).
func quoteLiteral(s string) string {
	if s == "$" {
		return "$"
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// durationFormat is the Excel elapsed-time code closest to a Numbers
// duration format.
func durationFormat(f Message) string {
	smallest := f.Uint(16) // 1 week, 2 day, 4 hour, 8 minute, 16 second, 32 ms
	switch {
	case smallest != 0 && smallest <= 4:
		return "[h]"
	case smallest == 8:
		return "[h]:mm"
	}
	return "[h]:mm:ss"
}

// ExcelDateFormat converts a Unicode (ICU) date pattern such as
// "MMM d, y h:mm a" to an Excel date format code.
func ExcelDateFormat(pattern string) string {
	var b strings.Builder
	rs := []rune(pattern)
	for i := 0; i < len(rs); {
		c := rs[i]
		if c == '\'' { // quoted literal; '' is a quote
			j := i + 1
			var lit strings.Builder
			for j < len(rs) {
				if rs[j] == '\'' {
					if j+1 < len(rs) && rs[j+1] == '\'' {
						lit.WriteRune('\'')
						j += 2
						continue
					}
					break
				}
				lit.WriteRune(rs[j])
				j++
			}
			if i+1 < len(rs) && rs[i+1] == '\'' {
				b.WriteString(`\'`)
				i += 2
				continue
			}
			if lit.Len() > 0 {
				b.WriteString(`"` + lit.String() + `"`)
			}
			i = j + 1
			continue
		}
		n := 1
		for i+n < len(rs) && rs[i+n] == c {
			n++
		}
		i += n
		switch c {
		case 'y', 'u', 'Y':
			if n == 2 {
				b.WriteString("yy")
			} else {
				b.WriteString("yyyy")
			}
		case 'M', 'L':
			b.WriteString(strings.Repeat("m", min(n, 5)))
		case 'd':
			b.WriteString(strings.Repeat("d", min(n, 2)))
		case 'E', 'c', 'e':
			if n >= 4 {
				b.WriteString("dddd")
			} else {
				b.WriteString("ddd")
			}
		case 'H', 'k', 'h', 'K':
			b.WriteString(strings.Repeat("h", min(n, 2)))
		case 'm':
			b.WriteString(strings.Repeat("m", min(n, 2)))
		case 's':
			b.WriteString(strings.Repeat("s", min(n, 2)))
		case 'S':
			b.WriteString("." + strings.Repeat("0", min(n, 3)))
		case 'a', 'b', 'B':
			b.WriteString("AM/PM")
		case 'G', 'z', 'Z', 'v', 'V', 'O', 'X', 'x', 'w', 'W', 'D', 'F', 'g', 'Q', 'q':
			// era, zone, week, quarter...: no Excel equivalent
		default:
			s := strings.Repeat(string(c), n)
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				s = `"` + s + `"`
			}
			b.WriteString(s)
		}
	}
	return strings.TrimSpace(b.String())
}

// Display renders a cell's value as Numbers shows it, for the cells of
// tables in Pages documents. It covers the common formats (decimal,
// currency, percent, scientific, dates, durations, booleans); others fall
// back to a plain rendering.
func (d *Document) Display(c *Cell) string {
	f := d.resolveFormat(c.Format())
	switch c.Type {
	case CellText, CellRichText:
		return c.Text
	case CellBool:
		if f.Uint(1) == FormatCheckbox {
			if c.Num != 0 {
				return "☑"
			}
			return "☐"
		}
		if c.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	case CellError:
		return "#ERROR"
	case CellDate:
		t := Epoch.Add(time.Duration(c.Seconds * float64(time.Second)))
		pattern := f.Str(14)
		if pattern == "" {
			pattern = f.Str(18)
		}
		if pattern == "" {
			pattern = "M/d/yy"
		}
		return FormatDateTime(t, pattern)
	case CellDuration:
		s := int64(math.Round(c.Num))
		sign := ""
		if s < 0 {
			sign, s = "-", -s
		}
		return fmt.Sprintf("%s%d:%02d:%02d", sign, s/3600, s/60%60, s%60)
	case CellNumber, CellCurrency:
	default:
		return ""
	}
	v := c.Num
	places := placesOf(f)
	if f == nil {
		places = autoPlaces
	}
	thousands := f.Bool(5)
	switch f.Uint(1) {
	case FormatPercent:
		return formatNumber(v*100, places, thousands, f.Uint(4)) + "%"
	case FormatScientific:
		if places == autoPlaces {
			places = 2
		}
		return strings.ToUpper(strconv.FormatFloat(v, 'e', places, 64))
	case FormatCurrency, FormatCustomCurrency:
		if places == autoPlaces {
			places = 2
		}
		s := formatNumber(math.Abs(v), places, thousands, 0)
		s = strings.TrimSpace(currencySymbol(f.Str(3))) + s
		if v < 0 {
			if f.Uint(4) >= 2 {
				return "(" + s + ")"
			}
			return "-" + s
		}
		return s
	case FormatRating:
		return stars(v)
	}
	if c.Type == CellCurrency {
		if places == autoPlaces {
			places = 2
		}
		s := "$" + formatNumber(math.Abs(v), places, true, 0)
		if v < 0 {
			s = "-" + s
		}
		return s
	}
	return formatNumber(v, places, thousands, f.Uint(4))
}

// stars is the display of a rating: v rounded to 0..5 stars (none for NaN).
func stars(v float64) string {
	if math.IsNaN(v) {
		return ""
	}
	return strings.Repeat("★", int(max(0, min(math.Round(v), 5))))
}

// formatNumber formats v with places decimals (autoPlaces: as many as
// needed) and optional thousands separators; neg is the negative style
// (2, 3: parentheses).
func formatNumber(v float64, places int, thousands bool, neg uint64) string {
	var s string
	if places == autoPlaces {
		s = strconv.FormatFloat(math.Abs(v), 'f', -1, 64)
		if len(s) > 15 { // binary noise: 0.1+0.2
			s = strconv.FormatFloat(math.Abs(v), 'g', 12, 64)
			if !strings.ContainsAny(s, "e") {
				s = strconv.FormatFloat(mustParse(s), 'f', -1, 64)
			}
		}
	} else {
		s = strconv.FormatFloat(math.Abs(v), 'f', places, 64)
	}
	if thousands && !strings.ContainsAny(s, "e") {
		intPart, frac, hasFrac := strings.Cut(s, ".")
		var b strings.Builder
		for i, ch := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(ch)
		}
		s = b.String()
		if hasFrac {
			s += "." + frac
		}
	}
	if v < 0 && strings.Trim(s, "0.,") != "" {
		if neg >= 2 {
			return "(" + s + ")"
		}
		return "-" + s
	}
	return s
}

// mustParse parses s as a float, 0 if it isn't one.
func mustParse(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// icuLiteral reads the quote at rs[i] of an ICU date pattern: a doubled
// quote is one quote, and '...' is literal text in which a doubled quote
// is also one quote. It returns the text and the index after it (an unclosed literal runs to the
// end).
func icuLiteral(rs []rune, i int) (string, int) {
	if i+1 < len(rs) && rs[i+1] == '\'' {
		return "'", i + 2
	}
	var lit strings.Builder
	j := i + 1
	for j < len(rs) {
		if rs[j] == '\'' {
			if j+1 < len(rs) && rs[j+1] == '\'' {
				lit.WriteRune('\'')
				j += 2
				continue
			}
			break
		}
		lit.WriteRune(rs[j])
		j++
	}
	return lit.String(), j + 1
}

// FormatDateTime renders t with a Unicode (ICU) date pattern.
func FormatDateTime(t time.Time, pattern string) string {
	var b strings.Builder
	rs := []rune(pattern)
	for i := 0; i < len(rs); {
		c := rs[i]
		if c == '\'' {
			var lit string
			lit, i = icuLiteral(rs, i)
			b.WriteString(lit)
			continue
		}
		n := 1
		for i+n < len(rs) && rs[i+n] == c {
			n++
		}
		i += n
		pad := func(v int) string { return fmt.Sprintf("%0*d", n, v) }
		switch c {
		case 'y', 'u', 'Y':
			if n == 2 {
				b.WriteString(fmt.Sprintf("%02d", t.Year()%100))
			} else {
				b.WriteString(strconv.Itoa(t.Year()))
			}
		case 'M', 'L':
			switch {
			case n >= 4:
				b.WriteString(t.Month().String())
			case n == 3:
				b.WriteString(t.Month().String()[:3])
			default:
				b.WriteString(pad(int(t.Month())))
			}
		case 'd':
			b.WriteString(pad(t.Day()))
		case 'E', 'c', 'e':
			if n >= 4 {
				b.WriteString(t.Weekday().String())
			} else {
				b.WriteString(t.Weekday().String()[:3])
			}
		case 'H', 'k':
			b.WriteString(pad(t.Hour()))
		case 'h', 'K':
			h := t.Hour() % 12
			if h == 0 && c == 'h' {
				h = 12
			}
			b.WriteString(pad(h))
		case 'm':
			b.WriteString(pad(t.Minute()))
		case 's':
			b.WriteString(pad(t.Second()))
		case 'a':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'G', 'z', 'Z', 'v', 'V', 'O', 'X', 'x', 'w', 'W', 'D', 'F', 'g', 'Q', 'q', 'S':
		default:
			b.WriteString(strings.Repeat(string(c), n))
		}
	}
	return strings.TrimSpace(b.String())
}
