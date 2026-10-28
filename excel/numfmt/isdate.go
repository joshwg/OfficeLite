package numfmt

// IsDate reports whether a number format shows a calendar date, so that
// AutoFill steps the value by days.
func IsDate(id int, code string) bool { return showsCalendarDate(id, code) }
