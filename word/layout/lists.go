package layout

import (
	"strconv"
	"strings"

	"officelite/word/model"
)

// listLabels numbers the document's list paragraphs in order: each list
// (NumID) keeps a counter per level, and a paragraph at one level restarts
// the deeper levels.
func (e *Engine) listLabels(locs []model.ParaLoc) map[*model.Paragraph]string {
	out := map[*model.Paragraph]string{}
	counters := map[int]*[9]int{}
	for _, loc := range locs {
		p := loc.P
		if p.Props.List == nil && !e.styleHasList(p) {
			continue
		}
		ref := e.Doc.ResolvePara(p).List
		if ref == nil || ref.NumID == 0 {
			continue
		}
		list := e.Doc.Lists[ref.NumID]
		if list == nil || ref.Level < 0 || ref.Level >= len(list.Levels) {
			continue
		}
		c := counters[ref.NumID]
		if c == nil {
			c = new([9]int)
			counters[ref.NumID] = c
		}
		lvl := min(ref.Level, 8)
		if c[lvl] == 0 {
			c[lvl] = max(list.Levels[lvl].Start, 1) - 1
		}
		c[lvl]++
		for k := lvl + 1; k < 9; k++ {
			c[k] = 0
		}
		out[p] = formatLabel(list, lvl, c)
	}
	return out
}

// styleHasList reports whether p's style chain may put it in a list
// (cheap pre-check before resolving).
func (e *Engine) styleHasList(p *model.Paragraph) bool {
	seen := map[string]bool{}
	id := p.Style
	if id == "" {
		if st := e.Doc.Styles.Default(model.ParagraphStyle); st != nil {
			id = st.ID
		}
	}
	for st := e.Doc.Styles.Find(id); st != nil && !seen[st.ID]; st = e.Doc.Styles.Find(st.BasedOn) {
		seen[st.ID] = true
		if st.Para.List != nil {
			return true
		}
	}
	return e.Doc.Styles.DefaultPara.List != nil
}

// formatLabel expands a level's text ("%1.%2.") with the counters.
func formatLabel(list *model.List, lvl int, c *[9]int) string {
	lv := list.Levels[lvl]
	switch lv.Format {
	case "bullet":
		if lv.Text == "" {
			return "•"
		}
		return mapSymbols(lv.Text)
	case "none":
		return mapSymbols(stripPlaceholders(lv.Text))
	}
	text := lv.Text
	if text == "" {
		text = "%" + strconv.Itoa(lvl+1) + "."
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '%' && i+1 < len(text) && text[i+1] >= '1' && text[i+1] <= '9' {
			k := int(text[i+1] - '1')
			n := c[k]
			if n == 0 && k < len(list.Levels) {
				n = max(list.Levels[k].Start, 1)
			}
			f := "decimal"
			if k < len(list.Levels) {
				f = list.Levels[k].Format
			}
			b.WriteString(FormatNumber(n, f))
			i++
			continue
		}
		b.WriteByte(text[i])
	}
	return mapSymbols(b.String())
}

func stripPlaceholders(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+1 < len(s) && s[i+1] >= '1' && s[i+1] <= '9' {
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// FormatNumber formats a list counter in a Word number format.
func FormatNumber(n int, format string) string {
	switch format {
	case "lowerLetter", "upperLetter":
		if n <= 0 {
			return strconv.Itoa(n)
		}
		// Word repeats the letter: a..z, aa..zz, aaa...
		s := strings.Repeat(string(rune('a'+(n-1)%26)), (n-1)/26+1)
		if format == "upperLetter" {
			s = strings.ToUpper(s)
		}
		return s
	case "lowerRoman", "upperRoman":
		s := roman(n)
		if format == "lowerRoman" {
			s = strings.ToLower(s)
		}
		return s
	case "decimalZero":
		if n < 10 {
			return "0" + strconv.Itoa(n)
		}
	case "none":
		return ""
	}
	return strconv.Itoa(n)
}

func roman(n int) string {
	if n <= 0 || n >= 4000 {
		return strconv.Itoa(n)
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}
