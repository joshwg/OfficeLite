package numfmt

// SplitSections splits a format code at the semicolons between its
// sections (positive;negative;zero;text). Semicolons inside quotes or
// [brackets], escaped with a backslash, or following _ (space the width
// of the next character) or * (repeat the next character) don't count.
func SplitSections(code string) []string {
	var secs []string
	start := 0
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '"':
			for i++; i < len(code) && code[i] != '"'; i++ {
			}
		case '[':
			for i++; i < len(code) && code[i] != ']'; i++ {
			}
		case '\\', '_', '*':
			i++
		case ';':
			secs = append(secs, code[start:i])
			start = i + 1
		}
	}
	return append(secs, code[start:])
}
