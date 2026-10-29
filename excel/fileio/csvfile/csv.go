// Package csvfile reads comma/tab/semicolon separated text as a workbook.
package csvfile

import (
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"officelite/common/textenc"
	"officelite/excel/formula"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// Read parses CSV data; name (the file name) gives the sheet its name.
func Read(name string, data []byte) (*model.Workbook, error) {
	text, err := textenc.Decode(data)
	if err != nil {
		return nil, err
	}

	// Excel honours a "sep=;" first line naming the delimiter.
	var delim rune
	first, rest, _ := strings.Cut(text, "\n")
	if f := strings.TrimRight(first, "\r"); strings.HasPrefix(strings.ToLower(f), "sep=") && utf8.RuneCountInString(f) == 5 {
		delim, _ = utf8.DecodeLastRuneInString(f)
		text = rest
	}
	if delim == 0 {
		delim = sniffDelimiter(text, strings.ToLower(filepath.Ext(name)))
	}

	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = true

	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	sheetName := strings.TrimSuffix(name, filepath.Ext(name))
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	sh := model.NewSheet(sheetName)
	wb.Sheets = append(wb.Sheets, sh)
	dropped := 0

	for row := 0; ; row++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		var pe *csv.ParseError
		if err != nil && !errors.As(err, &pe) {
			return nil, err
		}
		for col, field := range rec {
			if field == "" {
				continue
			}
			if !sh.Set(row, col, csvCell(field)) {
				dropped++
			}
		}
	}
	wb.WarnDropped(sh.Name, dropped)
	return wb, nil
}

// csvCell interprets a field the way Excel does when it opens a CSV: numbers
// become numbers (right-aligned, leading zeros dropped), TRUE/FALSE become
// booleans, everything else stays text.
func csvCell(s string) model.Cell {
	// A formula, as Excel treats "=A1*2" in a CSV; calc fills in its value.
	if len(s) > 1 && s[0] == '=' {
		return model.Cell{Formula: s[1:], Dynamic: true}
	}
	switch strings.ToUpper(s) {
	case "TRUE":
		return model.Cell{Text: "TRUE", Kind: model.KindBool, Num: 1}
	case "FALSE":
		return model.Cell{Text: "FALSE", Kind: model.KindBool}
	}
	t := strings.TrimSpace(s)
	if v, err := strconv.ParseFloat(t, 64); err == nil && !strings.ContainsAny(t, "xXpPnN_") {
		return model.Cell{Text: numfmt.General(v), Num: v, Kind: model.KindNumber}
	}
	// Decorated numbers keep their text: "1,234", "$5.00", "12%".
	if v, ok := parseDecoratedNumber(t); ok {
		return model.Cell{Text: t, Num: v, Kind: model.KindNumber}
	}
	// Dates and times become serial numbers, as Excel converts them.
	if v, ok := formula.ParseDateTime(t, false); ok {
		return model.Cell{Text: t, Num: v, Kind: model.KindNumber}
	}
	return model.Cell{Text: s, Kind: model.KindText}
}

func parseDecoratedNumber(t string) (float64, bool) {
	if t == "" || !strings.ContainsAny(t, "0123456789") {
		return 0, false
	}
	neg := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg, t = true, t[1:len(t)-1]
	}
	pct := strings.HasSuffix(t, "%")
	t = strings.TrimSuffix(t, "%")
	if strings.HasPrefix(t, "-") {
		if neg {
			return 0, false // (-100): negative twice
		}
		neg, t = true, t[1:]
	}
	t = strings.TrimPrefix(t, "$")
	if strings.Contains(t, ",") {
		// Thousands separators must be in groups of three.
		intPart, frac, _ := strings.Cut(t, ".")
		if strings.Contains(frac, ",") {
			return 0, false // 123.4,5
		}
		groups := strings.Split(intPart, ",")
		for i, g := range groups {
			if (i == 0 && (len(g) == 0 || len(g) > 3)) || (i > 0 && len(g) != 3) {
				return 0, false
			}
		}
		t = strings.ReplaceAll(t, ",", "")
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil || strings.ContainsAny(t, "eExXpPnN_") {
		return 0, false
	}
	if pct {
		v /= 100
	}
	if neg {
		v = -v
	}
	return v, true
}

// sniffDelimiter picks the separator that splits the first lines into the
// most consistent, non-trivial number of fields.
func sniffDelimiter(text, ext string) rune {
	if ext == ".tsv" || ext == ".tab" {
		return '\t'
	}
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
		if len(lines) == 20 {
			break
		}
	}
	best, bestScore := ',', 0
	for _, d := range []rune{',', '\t', ';', '|'} {
		counts := map[int]int{}
		for _, l := range lines {
			if n := countOutsideQuotes(l, d); n > 0 {
				counts[n]++
			}
		}
		// Score: number of lines that agree on the most common field count.
		for _, agree := range counts {
			if agree > bestScore {
				best, bestScore = d, agree
			}
		}
	}
	return best
}

func countOutsideQuotes(line string, d rune) int {
	n, quoted := 0, false
	for _, c := range line {
		switch {
		case c == '"':
			quoted = !quoted
		case c == d && !quoted:
			n++
		}
	}
	return n
}
