package layout

import (
	"strconv"
	"strings"

	"officelite/word/model"
)

// listLabels numbers the document's list paragraphs in order. Lists
// sharing an abstract definition count together, as in Word; each keeps a
// counter per level, a paragraph at one level restarts the deeper levels,
// and a list with a start override restarts the level at its first
// paragraph there.
func (e *Engine) listLabels(locs []model.ParaLoc) map[*model.Paragraph]string {
	out := map[*model.Paragraph]string{}
	counters := map[string]*counter{}
	restarted := map[[2]int]bool{} // NumID and level already restarted
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
		key := "n" + strconv.Itoa(ref.NumID)
		if list.Abstract != "" {
			key = "a" + list.Abstract
		}
		c := counters[key]
		if c == nil {
			c = &counter{}
			counters[key] = c
		}
		lvl := min(ref.Level, 8)
		if lv := list.Levels[lvl]; lv.Restart && !restarted[[2]int{ref.NumID, lvl}] {
			restarted[[2]int{ref.NumID, lvl}] = true
			c.set[lvl] = false
		}
		if !c.set[lvl] {
			c.n[lvl], c.set[lvl] = list.Levels[lvl].Start-1, true
		}
		c.n[lvl]++
		for k := lvl + 1; k < 9; k++ {
			c.set[k] = false
		}
		out[p] = formatLabel(list, lvl, c)
	}
	return out
}

// counter is one list's numbers per level; set is false for a level not
// numbered yet (or restarted by a higher level).
type counter struct {
	n   [9]int
	set [9]bool
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

// formatLabel expands a level's text ("%1.%2.") with the counters. Symbol
// font characters are kept (see Engine.labelText).
func formatLabel(list *model.List, lvl int, c *counter) string {
	lv := list.Levels[lvl]
	switch lv.Format {
	case "bullet":
		if lv.Text == "" {
			return "•"
		}
		return lv.Text
	case "none":
		return stripPlaceholders(lv.Text)
	}
	text := lv.Text
	if text == "" {
		text = "%" + strconv.Itoa(lvl+1) + "."
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '%' && i+1 < len(text) && text[i+1] >= '1' && text[i+1] <= '9' {
			k := int(text[i+1] - '1')
			n := c.n[k]
			if !c.set[k] && k < len(list.Levels) {
				n = list.Levels[k].Start
			}
			f := "decimal"
			if k < len(list.Levels) {
				f = list.Levels[k].Format
			}
			b.WriteString(model.FormatNumber(n, f))
			i++
			continue
		}
		b.WriteByte(text[i])
	}
	return b.String()
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
