// Field evaluation: a field instruction parsed into its type, arguments
// and switches, and the results of the fields WordLite computes (page
// numbers, dates and times, the file name, document statistics), with
// Word's \* format, \# numeric and \@ date-time picture switches.

package model

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"officelite/word"
)

// Field types WordLite computes, besides FieldPage and FieldNumPages.
const (
	FieldSectionPages = "SECTIONPAGES"
	FieldSection      = "SECTION"
	FieldDate         = "DATE"
	FieldTime         = "TIME"
	FieldFileName     = "FILENAME"
	FieldNumWords     = "NUMWORDS"
	FieldNumChars     = "NUMCHARS"
)

// FieldSwitch is one switch of a field instruction: its name (`\@`,
// `\*`, `\p`...) and argument ("" when it takes none).
type FieldSwitch struct {
	Name, Arg string
}

// FieldCode is a parsed field instruction.
type FieldCode struct {
	Name     string        // the field type, upper-cased ("PAGE")
	Args     []string      // the arguments that aren't switches, unquoted
	Switches []FieldSwitch // in the order given
}

// Switch returns the argument of the first switch called name, and
// whether there is one. Switch names are matched without regard to case
// (`\P` is `\p`).
func (c FieldCode) Switch(name string) (string, bool) {
	for _, s := range c.Switches {
		if strings.EqualFold(s.Name, name) {
			return s.Arg, true
		}
	}
	return "", false
}

// formats lists the arguments of the \* switches, in order.
func (c FieldCode) formats() []string {
	var out []string
	for _, s := range c.Switches {
		if s.Name == `\*` && s.Arg != "" {
			out = append(out, s.Arg)
		}
	}
	return out
}

// switchTakesArg reports whether a switch's next token is its argument
// even when it isn't quoted: the general formatting switches do.
func switchTakesArg(name string) bool {
	return name == `\@` || name == `\*` || name == `\#`
}

// ParseField splits a field instruction into its type, arguments and
// switches. Arguments may be quoted ("a b"); inside quotes, \" is a quote
// and \\ a backslash. A switch's argument is the token after it when the
// switch is \@, \* or \#, or when that token is quoted.
func ParseField(instr string) FieldCode {
	type token struct {
		text   string
		quoted bool
	}
	var toks []token
	for i := 0; i < len(instr); {
		c := instr[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '"':
			var b strings.Builder
			i++
			for i < len(instr) && instr[i] != '"' {
				if instr[i] == '\\' && i+1 < len(instr) && (instr[i+1] == '"' || instr[i+1] == '\\') {
					i++
				}
				b.WriteByte(instr[i])
				i++
			}
			i++ // the closing quote (or the end)
			toks = append(toks, token{b.String(), true})
		default:
			start := i
			for i < len(instr) && !strings.ContainsRune(" \t\r\n\"", rune(instr[i])) {
				i++
			}
			toks = append(toks, token{instr[start:i], false})
		}
	}
	var fc FieldCode
	if len(toks) == 0 {
		return fc
	}
	fc.Name = strings.ToUpper(toks[0].text)
	for i := 1; i < len(toks); i++ {
		t := toks[i]
		if t.quoted || !strings.HasPrefix(t.text, `\`) || len(t.text) < 2 {
			fc.Args = append(fc.Args, t.text)
			continue
		}
		sw := FieldSwitch{Name: t.text}
		if len(t.text) > 2 && strings.ContainsRune(`@*#!`, rune(t.text[1])) {
			// `\@"d MMM"` or `\*Upper`: the argument is run on.
			sw.Name, sw.Arg = t.text[:2], t.text[2:]
		} else if i+1 < len(toks) {
			if next := toks[i+1]; next.quoted || switchTakesArg(sw.Name) && !strings.HasPrefix(next.text, `\`) {
				sw.Arg = next.text
				i++
			}
		}
		fc.Switches = append(fc.Switches, sw)
	}
	return fc
}

// FieldContext is what field results are computed from.
type FieldContext struct {
	Page         int    // the page number the field is on (1-based, as numbered)
	PageFormat   string // the page's section's number format ("" = decimal; see FormatNumber)
	Pages        int    // the document's page count
	Section      int    // the field's section (1-based)
	SectionPages int    // the page count of the field's section
	Now          time.Time
	// FilePath is the document's file ("" when it hasn't been saved);
	// FileName falls back to Doc's name.
	FilePath string
	Doc      *Document // for FILENAME without a file and NUMWORDS / NUMCHARS; may be nil
}

// IsPageField reports whether a field's result depends on the page it's
// on, so that it is computed again whenever the document is paginated.
func IsPageField(name string) bool {
	switch name {
	case FieldPage, FieldNumPages, FieldSectionPages, FieldSection:
		return true
	}
	return false
}

// EvalField computes the result of the field instr in context c; ok is
// false for fields WordLite doesn't compute, which keep their last result.
func EvalField(instr string, c *FieldContext) (result string, ok bool) {
	fc := ParseField(instr)
	switch fc.Name {
	case FieldPage:
		format := c.PageFormat
		if hasNumberFormat(fc.formats()) {
			format = "" // a \* switch picks the format instead
		}
		return formatFieldNumber(fc, c.Page, format), true
	case FieldNumPages:
		return formatFieldNumber(fc, c.Pages, ""), true
	case FieldSectionPages:
		return formatFieldNumber(fc, c.SectionPages, ""), true
	case FieldSection:
		return formatFieldNumber(fc, c.Section, ""), true
	case FieldNumWords, FieldNumChars:
		if c.Doc == nil {
			return "", false
		}
		_, words, chars := c.Doc.Stats()
		n := words
		if fc.Name == FieldNumChars {
			n = chars
		}
		return formatFieldNumber(fc, n, ""), true
	case FieldDate, FieldTime:
		now := c.Now
		if now.IsZero() {
			now = time.Now()
		}
		pic, ok := fc.Switch(`\@`)
		if !ok {
			pic = "M/d/yyyy"
			if fc.Name == FieldTime {
				pic = "h:mm AM/PM"
			}
		}
		return applyTextFormats(fc, FormatDateTime(now, pic)), true
	case FieldFileName:
		name := ""
		switch {
		case c.FilePath != "":
			name = filepath.Base(c.FilePath)
			if _, full := fc.Switch(`\p`); full {
				name = c.FilePath
			}
		case c.Doc != nil:
			name = c.Doc.Name
		default:
			return "", false
		}
		return applyTextFormats(fc, name), true
	}
	return "", false
}

// hasNumberFormat reports whether one of a field's \* formats is a number
// format (as opposed to a text one like Upper, or MERGEFORMAT).
func hasNumberFormat(formats []string) bool {
	for _, f := range formats {
		switch strings.ToLower(f) {
		case "roman", "alphabetic", "arabic", "ordinal", "cardtext", "ordtext", "hex", "dollartext":
			return true
		}
	}
	return false
}

// formatFieldNumber formats a number field's value v: with a \#
// numeric picture, else its \* number format, else format (a section's
// page number format, "" for decimal); then any \* text formats.
func formatFieldNumber(fc FieldCode, v int, format string) string {
	s := ""
	if pic, ok := fc.Switch(`\#`); ok {
		s = FormatNumericPicture(float64(v), pic)
	} else {
		s = FormatNumber(v, format)
		if format == "" {
			s = strconv.Itoa(v)
		}
		for _, f := range fc.formats() {
			switch f {
			case "roman":
				s = FormatNumber(v, "lowerRoman")
			case "Roman", "ROMAN":
				s = FormatNumber(v, "upperRoman")
			case "alphabetic":
				s = FormatNumber(v, "lowerLetter")
			case "ALPHABETIC", "Alphabetic":
				s = FormatNumber(v, "upperLetter")
			case "Arabic", "arabic", "ARABIC":
				s = strconv.Itoa(v)
			case "Ordinal", "ordinal", "ORDINAL":
				s = strconv.Itoa(v) + ordinalSuffix(v)
			case "CardText", "cardtext", "Cardtext", "CARDTEXT":
				s = CardText(v)
			case "OrdText", "ordtext", "Ordtext", "ORDTEXT":
				s = OrdText(v)
			case "Hex", "hex", "HEX":
				s = strings.ToUpper(strconv.FormatInt(int64(v), 16))
			}
		}
	}
	return applyTextFormats(fc, s)
}

// applyTextFormats applies a field's \* Upper, Lower, Caps and FirstCap
// switches to its result; MERGEFORMAT and CHARFORMAT (which are about the
// result's formatting) and number formats leave it as it is.
func applyTextFormats(fc FieldCode, s string) string {
	for _, f := range fc.formats() {
		switch strings.ToLower(f) {
		case "upper":
			s = strings.ToUpper(s)
		case "lower":
			s = strings.ToLower(s)
		case "caps":
			s = titleCase(s)
		case "firstcap":
			if r, n := utf8.DecodeRuneInString(s); n > 0 {
				s = string(unicode.ToUpper(r)) + s[n:]
			}
		}
	}
	return s
}

// titleCase capitalizes the first letter of every word.
func titleCase(s string) string {
	out := []rune(s)
	start := true
	for i, r := range out {
		if unicode.IsSpace(r) {
			start = true
			continue
		}
		if start {
			out[i] = unicode.ToUpper(r)
		}
		start = false
	}
	return string(out)
}

// ordinalSuffix is the English ordinal suffix of n ("st" for 1, "th" for
// 11).
func ordinalSuffix(n int) string {
	if n < 0 {
		n = -n
	}
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

var (
	smallNumbers = [...]string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
		"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	tens = [...]string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
)

// CardText spells n out in words, as Word's \* CardText does ("one
// hundred twenty-three"); negative numbers and numbers of a million or
// more are written in digits.
func CardText(n int) string {
	if n < 0 || n >= word.MaxSpelled {
		return strconv.Itoa(n)
	}
	if n < 20 {
		return smallNumbers[n]
	}
	var parts []string
	if n >= 1000 {
		parts = append(parts, CardText(n/1000)+" thousand")
		n %= 1000
	}
	if n >= 100 {
		parts = append(parts, smallNumbers[n/100]+" hundred")
		n %= 100
	}
	switch {
	case n == 0:
	case n < 20:
		parts = append(parts, smallNumbers[n])
	case n%10 == 0:
		parts = append(parts, tens[n/10])
	default:
		parts = append(parts, tens[n/10]+"-"+smallNumbers[n%10])
	}
	return strings.Join(parts, " ")
}

// OrdText spells n out as an ordinal ("twenty-first"), as Word's
// \* OrdText does.
func OrdText(n int) string {
	s := CardText(n)
	if n < 0 || n >= word.MaxSpelled {
		return s + ordinalSuffix(n)
	}
	// Change the last word: one → first, twelve → twelfth, twenty → twentieth.
	cut := strings.LastIndexAny(s, " -") + 1
	last := s[cut:]
	irregular := map[string]string{"one": "first", "two": "second", "three": "third", "five": "fifth",
		"eight": "eighth", "nine": "ninth", "twelve": "twelfth"}
	switch {
	case irregular[last] != "":
		last = irregular[last]
	case strings.HasSuffix(last, "y"):
		last = strings.TrimSuffix(last, "y") + "ieth"
	default:
		last += "th"
	}
	return s[:cut] + last
}

// FormatNumericPicture formats v with a Word \# numeric picture: 0 is a
// digit always shown, # a digit shown when significant, ',' groups
// thousands, '.' places the decimals, other characters (and text in
// 'quotes') are copied. A picture with a ';' gives the format for
// negative numbers after it.
func FormatNumericPicture(v float64, pic string) string {
	pos, neg, hasNeg := pic, "", false
	if i := strings.IndexByte(pic, ';'); i >= 0 {
		pos, neg, hasNeg = pic[:i], pic[i+1:], true
		if j := strings.IndexByte(neg, ';'); j >= 0 {
			neg = neg[:j]
		}
	}
	if v < 0 && hasNeg {
		return formatPicture(-v, neg, false)
	}
	return formatPicture(v, pos, v < 0)
}

// formatPicture formats |v| with one section of a numeric picture,
// putting a minus sign before the first digit when minus is set.
func formatPicture(v float64, pic string, minus bool) string {
	if v < 0 {
		v = -v
	}
	// Find the digit placeholders around the decimal point.
	intDigits, fracDigits, minInt, minFrac := 0, 0, 0, 0
	group, seenPoint := false, false
	inQuote := false
	for _, r := range pic {
		switch {
		case r == '\'':
			inQuote = !inQuote
		case inQuote:
		case r == '.':
			seenPoint = true
		case r == ',' && !seenPoint:
			group = true
		case r == '0' || r == '#':
			if seenPoint {
				fracDigits++
				if r == '0' {
					minFrac = fracDigits
				}
			} else {
				intDigits++
				if r == '0' {
					minInt++
				}
			}
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return pic
	}
	num := strconv.FormatFloat(v, 'f', fracDigits, 64)
	intPart, fracPart, _ := strings.Cut(num, ".")
	if intPart == "0" && minInt == 0 {
		intPart = ""
	}
	for len(intPart) < minInt {
		intPart = "0" + intPart
	}
	for len(fracPart) > minFrac && strings.HasSuffix(fracPart, "0") {
		fracPart = fracPart[:len(fracPart)-1]
	}
	if group && len(intPart) > 3 {
		var b strings.Builder
		for i, d := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(d)
		}
		intPart = b.String()
	}
	digits := intPart
	if fracPart != "" {
		digits += "." + fracPart
	}
	// A minus goes before the picture's text ("-$12.00"), and not on a
	// value that rounds to zero ("0.00", not "-0.00").
	minus = minus && strings.ContainsAny(digits, "123456789")
	// Copy the picture's text around the digits: everything before its
	// first placeholder, then the digits, then everything after its last.
	var b strings.Builder
	placed := false
	inQuote = false
	for _, r := range pic {
		switch {
		case r == '\'':
			inQuote = !inQuote
		case inQuote:
			b.WriteRune(r)
		case r == '0' || r == '#' || r == '.' || r == ',':
			if !placed {
				b.WriteString(digits)
				placed = true
			}
		default:
			b.WriteRune(r)
		}
	}
	if minus {
		return "-" + b.String()
	}
	return b.String()
}

var (
	monthNames = [...]string{"January", "February", "March", "April", "May", "June", "July",
		"August", "September", "October", "November", "December"}
	dayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

// FormatDateTime formats t with a Word \@ date-time picture: d dd ddd
// dddd (day), M MM MMM MMMM (month), yy yyyy (year), h hh (12-hour) and
// H HH (24-hour), m mm (minutes), s ss (seconds), am/pm or AM/PM; text in
// 'quotes' and other characters are copied.
func FormatDateTime(t time.Time, pic string) string {
	var b strings.Builder
	rs := []rune(pic)
	run := func(i int) int { // how many times rs[i] repeats from i
		n := 1
		for i+n < len(rs) && rs[i+n] == rs[i] {
			n++
		}
		return n
	}
	pad := func(v, width int) string {
		s := strconv.Itoa(v)
		for len(s) < width {
			s = "0" + s
		}
		return s
	}
	hour12 := func() int {
		h := t.Hour() % 12
		if h == 0 {
			h = 12
		}
		return h
	}
	for i := 0; i < len(rs); {
		r := rs[i]
		if r == '\'' {
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				b.WriteRune(rs[j])
				j++
			}
			i = j + 1
			continue
		}
		if lower := strings.ToLower(string(rs[i:min(i+5, len(rs))])); lower == "am/pm" {
			ampm := "AM"
			if t.Hour() >= 12 {
				ampm = "PM"
			}
			if rs[i] == 'a' {
				ampm = strings.ToLower(ampm)
			}
			b.WriteString(ampm)
			i += 5
			continue
		}
		n := run(i)
		switch r {
		case 'd', 'D':
			switch {
			case n >= 4:
				b.WriteString(dayNames[t.Weekday()])
			case n == 3:
				b.WriteString(dayNames[t.Weekday()][:3])
			default:
				b.WriteString(pad(t.Day(), n))
			}
		case 'M':
			switch {
			case n >= 4:
				b.WriteString(monthNames[t.Month()-1])
			case n == 3:
				b.WriteString(monthNames[t.Month()-1][:3])
			default:
				b.WriteString(pad(int(t.Month()), n))
			}
		case 'y', 'Y':
			if n >= 3 {
				b.WriteString(pad(t.Year(), 4))
			} else {
				b.WriteString(pad(t.Year()%100, 2))
			}
		case 'h':
			b.WriteString(pad(hour12(), min(n, 2)))
		case 'H':
			b.WriteString(pad(t.Hour(), min(n, 2)))
		case 'm':
			b.WriteString(pad(t.Minute(), min(n, 2)))
		case 's', 'S':
			b.WriteString(pad(t.Second(), min(n, 2)))
		default:
			b.WriteString(string(rs[i : i+n]))
		}
		i += n
	}
	return b.String()
}

// isOneLineField reports whether r is a field kept as one run (its
// instruction in Opaque.Field, its last result in Text), as opposed to a
// note, a floating object or a part of a field spanning paragraphs.
func isOneLineField(r *Run) bool {
	o := r.Opaque
	return r.Kind == RunOpaque && o != nil && o.Field != "" && o.FieldChar == FieldNone &&
		o.Note == nil && !o.NoteMark && o.Float == nil && o.Picture == nil
}

// fieldUpdate is one field result UpdateFields changes.
type fieldUpdate struct {
	para, run int
	result    string
}

// UpdateFields computes again the fields the selection holds, or the
// field at the caret when nothing is selected, as Word's Update Field
// (F9) does: each field WordLite computes (EvalField) gets its new result
// as its text, in one undoable step; ctx gives the context of a field at
// a position (its page, the time, the file). Fields WordLite doesn't
// compute, and tables of contents (see UpdateTOC), are left alone. It
// returns how many fields it computed, how many of their results changed,
// and the types of the fields it left alone (each once, in order).
func (e *Editor) UpdateFields(ctx func(Pos) FieldContext) (computed, changed int, skipped []string) {
	s, t := e.Sel.Ordered()
	if s == t && t.Off > 0 {
		// The caret just after a field (or in it: a field is one position)
		// updates it.
		if p := e.locs[t.Para].P; p != nil {
			if i := p.RunAt(t.Off - 1); i >= 0 && isOneLineField(&p.Runs[i]) {
				s.Off = t.Off - 1
			}
		}
	}
	if s == t {
		if p := e.locs[t.Para].P; p != nil {
			if i := p.RunAt(t.Off); i >= 0 && isOneLineField(&p.Runs[i]) {
				t.Off++
			}
		}
	}
	var updates []fieldUpdate
	for pi := s.Para; pi <= t.Para && pi < len(e.locs); pi++ {
		p := e.locs[pi].P
		off := 0
		for ri := range p.Runs {
			r := &p.Runs[ri]
			n := RunLen(r)
			inside := (pi > s.Para || off >= s.Off) && (pi < t.Para || off < t.Off)
			if inside && isOneLineField(r) {
				c := ctx(Pos{pi, off})
				v, ok := EvalField(r.Opaque.Field, &c)
				switch {
				case !ok:
					if name := FieldName(r.Opaque.Field); !slices.Contains(skipped, name) {
						skipped = append(skipped, name)
					}
				case v != r.Text:
					updates = append(updates, fieldUpdate{pi, ri, v})
					computed++
				default:
					computed++
				}
			}
			off += n
		}
	}
	if len(updates) == 0 {
		return computed, 0, skipped
	}
	from, to := e.locs[updates[0].para].Top, e.locs[updates[len(updates)-1].para].Top
	defer e.Op("Update Field")()
	e.edit("format", from, to, false, func() {
		for _, u := range updates {
			r := &e.locs[u.para].P.Runs[u.run]
			o := *r.Opaque
			// The result changed: the original file's bytes for the field
			// (which hold the old result) can no longer be written back.
			o.Format, o.Data, o.Width, o.Height = "", nil, 0, 0
			r.Opaque, r.Text = &o, u.result
		}
	})
	return computed, len(updates), skipped
}
