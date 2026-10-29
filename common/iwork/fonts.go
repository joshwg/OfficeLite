// Mapping of iWork PostScript font names to family names and bold/italic.

package iwork

import (
	"strings"
	"unicode"
)

// fontFamilies maps PostScript base names (the part before the style
// suffix) whose family name isn't just the words split apart.
var fontFamilies = map[string]string{
	"ArialMT":              "Arial",
	"TimesNewRomanPSMT":    "Times New Roman",
	"TimesNewRomanPS":      "Times New Roman",
	"CourierNewPSMT":       "Courier New",
	"CourierNewPS":         "Courier New",
	"BodoniSvtyTwoITCTT":   "Bodoni 72",
	"BodoniSvtyTwoOSITCTT": "Bodoni 72 Oldstyle",
	"PalatinoLinotype":     "Palatino Linotype",
	"Palatino":             "Palatino",
	"SFPro":                "SF Pro",
	"SFProText":            "SF Pro Text",
	"SFProDisplay":         "SF Pro Display",
	".SFNS":                "SF Pro",
	"AmericanTypewriter":   "American Typewriter",
	"GillSans":             "Gill Sans",
	"AvenirNext":           "Avenir Next",
	"AvenirNextCondensed":  "Avenir Next Condensed",
	"HelveticaNeue":        "Helvetica Neue",
	"HoeflerText":          "Hoefler Text",
	"Superclarendon":       "Superclarendon",
	"BigCaslon":            "Big Caslon",
	"Kailasa":              "Kailasa",
}

// FontFamily turns an iWork font name (a PostScript name such as
// "HelveticaNeue-Bold" or "Georgia-Italic") into a family name and the
// bold/italic the style suffix implies.
func FontFamily(ps string) (family string, bold, italic bool) {
	if ps == "" {
		return "", false, false
	}
	base, style, _ := strings.Cut(ps, "-")
	ls := strings.ToLower(style)
	bold = strings.Contains(ls, "bold") || strings.Contains(ls, "black") || strings.Contains(ls, "heavy")
	italic = strings.Contains(ls, "italic") || strings.Contains(ls, "oblique") || ls == "it" || strings.HasSuffix(ls, "it")
	if f, ok := fontFamilies[base]; ok {
		return f, bold, italic
	}
	if strings.Contains(ps, " ") {
		return ps, bold, italic // already a family name
	}
	base = strings.TrimSuffix(strings.TrimSuffix(base, "MT"), "PS")
	// Split CamelCase into words: "GillSans" -> "Gill Sans".
	var b strings.Builder
	rs := []rune(base)
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(rs[i-1]))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String(), bold, italic
}
