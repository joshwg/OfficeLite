package text

import (
	"strings"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

// Half-width voiced sound marks.
const (
	hwDakuten    = 'ﾞ' // ﾞ
	hwHandakuten = 'ﾟ' // ﾟ
)

// asc converts full-width (double-byte) characters to half-width: ＡＢＣ
// to ABC, the ideographic space to a space and katakana to half-width
// katakana, splitting voiced kana into base + sound mark (ガ to ｶﾞ).
func asc(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x30A0 && r <= 0x30FF {
			if d := []rune(norm.NFD.String(string(r))); len(d) == 2 {
				base := []rune(width.Narrow.String(string(d[0])))
				if len(base) == 1 && base[0] != d[0] {
					b.WriteRune(base[0])
					if d[1] == '゙' {
						b.WriteRune(hwDakuten)
					} else {
						b.WriteRune(hwHandakuten)
					}
					continue
				}
			}
		}
		b.WriteString(width.Narrow.String(string(r)))
	}
	return b.String()
}

// dbcs converts half-width characters to full-width (the reverse of ASC),
// combining half-width kana with a following sound mark (ｶﾞ to ガ).
func dbcs(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		w := []rune(width.Widen.String(string(rs[i])))
		if len(w) == 1 && rs[i] >= 0xFF66 && rs[i] <= 0xFF9D && i+1 < len(rs) &&
			(rs[i+1] == hwDakuten || rs[i+1] == hwHandakuten) {
			mark := '゙'
			if rs[i+1] == hwHandakuten {
				mark = '゚'
			}
			if c := []rune(norm.NFC.String(string([]rune{w[0], mark}))); len(c) == 1 {
				b.WriteRune(c[0])
				i++
				continue
			}
		}
		b.WriteString(string(w))
	}
	return b.String()
}
