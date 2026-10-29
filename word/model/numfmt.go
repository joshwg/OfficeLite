package model

import (
	"strconv"
	"strings"
)

// FormatNumber formats n in a Word number format ("decimal",
// "lowerLetter", "upperRoman", "decimalZero"...), as list labels, page
// numbers and note references show it. Letters and roman numerals fall
// back to decimal outside the range they can show.
func FormatNumber(n int, format string) string {
	switch format {
	case "lowerLetter", "upperLetter":
		s := Alpha(n)
		if format == "upperLetter" {
			s = strings.ToUpper(s)
		}
		return s
	case "lowerRoman", "upperRoman":
		s := Roman(n)
		if format == "lowerRoman" {
			s = strings.ToLower(s)
		}
		return s
	case "decimalZero":
		if n >= 0 && n < 10 {
			return "0" + strconv.Itoa(n)
		}
	case "none":
		return ""
	}
	return strconv.Itoa(n)
}

// Roman is n in upper-case roman numerals (decimal unless 1..3999).
func Roman(n int) string {
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

// maxAlpha is the largest number Alpha spells in letters: z thirty times,
// Word's limit.
const maxAlpha = 780

// Alpha is Word's lower-case alphabetic numbering: a..z, aa..zz, aaa...
// (decimal outside 1..maxAlpha).
func Alpha(n int) string {
	if n <= 0 || n > maxAlpha {
		return strconv.Itoa(n)
	}
	return strings.Repeat(string(rune('a'+(n-1)%26)), (n-1)/26+1)
}
