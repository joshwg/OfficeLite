package formula

import (
	"math"
	"strconv"
	"strings"
)

// Value is a formula value: Number, String, Bool, Error, Blank, Missing,
// *Array, Ref or *Lambda.
type Value interface{ isValue() }

type (
	// Number is every numeric value, including dates (serial numbers).
	Number float64
	String string
	Bool   bool
	// Error is an Excel error value such as #DIV/0!.
	Error string
	// Blank is an empty cell.
	Blank struct{}
	// Missing is an omitted function argument, e.g. the second in IF(A1,,1).
	Missing struct{}
)

func (Number) isValue()  {}
func (String) isValue()  {}
func (Bool) isValue()    {}
func (Error) isValue()   {}
func (Blank) isValue()   {}
func (Missing) isValue() {}
func (*Array) isValue()  {}
func (Ref) isValue()     {}
func (*Lambda) isValue() {}

// Excel's error values.
const (
	ErrNull     Error = "#NULL!"
	ErrDiv0     Error = "#DIV/0!"
	ErrValue    Error = "#VALUE!"
	ErrRef      Error = "#REF!"
	ErrName     Error = "#NAME?"
	ErrNum      Error = "#NUM!"
	ErrNA       Error = "#N/A"
	ErrGetting  Error = "#GETTING_DATA"
	ErrSpill    Error = "#SPILL!"
	ErrConnect  Error = "#CONNECT!"
	ErrBlocked  Error = "#BLOCKED!"
	ErrUnknown  Error = "#UNKNOWN!"
	ErrField    Error = "#FIELD!"
	ErrCalc     Error = "#CALC!"
	ErrBusy     Error = "#BUSY!"
	ErrExternal Error = "#EXTERNAL!"
)

// ErrorCodes maps errors to their ERROR.TYPE numbers.
var ErrorCodes = map[Error]int{
	ErrNull: 1, ErrDiv0: 2, ErrValue: 3, ErrRef: 4, ErrName: 5, ErrNum: 6, ErrNA: 7,
	ErrGetting: 8, ErrSpill: 9, ErrConnect: 10, ErrBlocked: 11, ErrUnknown: 12,
	ErrField: 13, ErrCalc: 14,
}

// IsError reports whether v is an Error.
func IsError(v Value) bool {
	_, ok := v.(Error)
	return ok
}

// Array is a two-dimensional array value (array constants, range contents,
// dynamic-array results). Elements are scalars.
type Array struct {
	Rows, Cols int
	V          []Value // row-major
}

// NewArray returns a rows×cols array filled with Blank.
func NewArray(rows, cols int) *Array {
	a := &Array{Rows: rows, Cols: cols, V: make([]Value, rows*cols)}
	for i := range a.V {
		a.V[i] = Blank{}
	}
	return a
}

func (a *Array) At(r, c int) Value       { return a.V[r*a.Cols+c] }
func (a *Array) Set(r, c int, v Value)   { a.V[r*a.Cols+c] = v }
func (a *Array) Len() int                { return len(a.V) }
func (a *Array) Single() bool            { return a.Rows == 1 && a.Cols == 1 }
func (a *Array) Row(r int) []Value       { return a.V[r*a.Cols : (r+1)*a.Cols] }
func (a *Array) Shape() (rows, cols int) { return a.Rows, a.Cols }

// Broadcast returns the element at r, c treating a single row, column or
// cell as repeated, as Excel does when combining arrays of different
// shapes. Positions outside the array give #N/A.
func (a *Array) Broadcast(r, c int) Value {
	if a.Rows == 1 {
		r = 0
	}
	if a.Cols == 1 {
		c = 0
	}
	if r >= a.Rows || c >= a.Cols {
		return ErrNA
	}
	return a.At(r, c)
}

// Ref is a reference to a rectangle of cells on one sheet (zero-based,
// inclusive).
type Ref struct {
	Sheet          int
	R1, C1, R2, C2 int
}

func (r Ref) Single() bool { return r.R1 == r.R2 && r.C1 == r.C2 }
func (r Ref) Rows() int    { return r.R2 - r.R1 + 1 }
func (r Ref) Cols() int    { return r.C2 - r.C1 + 1 }
func (r Ref) Contains(row, col int) bool {
	return row >= r.R1 && row <= r.R2 && col >= r.C1 && col <= r.C2
}

// Lambda is a function value created by LAMBDA, or a worksheet function
// named without parentheses where a function is expected (Excel's
// "eta-reduced" lambda: BYROW(A1:B3,SUM), GROUPBY(A2:A9,B2:B9,AVERAGE)).
type Lambda struct {
	Params []string // upper-case
	Body   Node
	scope  *scope

	Func *Spec // the worksheet function, for an eta-reduced lambda
}

// Name is the function's name for an eta-reduced lambda ("SUM"), otherwise
// "LAMBDA".
func (l *Lambda) Name() string {
	if l.Func != nil {
		return l.Func.Name
	}
	return "LAMBDA"
}

// Accepts reports whether the lambda can be called with n arguments.
func (l *Lambda) Accepts(n int) bool {
	if l.Func != nil {
		return n >= l.Func.Min && (l.Func.Max == Variadic || n <= l.Func.Max)
	}
	return n <= len(l.Params)
}

// --- coercion -----------------------------------------------------------

// Num converts a scalar to a number the way Excel's arithmetic does:
// booleans become 1/0, blanks and missing arguments 0, numeric text
// (including dates, times, percentages and currency) its value. The second
// result is a non-nil Error when the conversion fails or v is an error.
func Num(v Value) (float64, Value) {
	switch v := v.(type) {
	case Number:
		return float64(v), nil
	case Bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case Blank, Missing:
		return 0, nil
	case String:
		if f, ok := ParseNumber(string(v)); ok {
			return f, nil
		}
		return 0, ErrValue
	case Error:
		return 0, v
	case *Array:
		if v.Len() > 0 {
			return Num(v.V[0])
		}
	}
	return 0, ErrValue
}

// Int is Num truncated toward zero (Excel truncates integer arguments).
func Int(v Value) (int, Value) {
	f, err := Num(v)
	if err != nil {
		return 0, err
	}
	if math.Abs(f) > 1<<53 {
		return 0, ErrNum
	}
	return int(f), nil
}

// Str converts a scalar to text: numbers as Excel's General format with up
// to 15 significant digits, booleans as TRUE/FALSE, blanks as "".
func Str(v Value) (string, Value) {
	switch v := v.(type) {
	case String:
		return string(v), nil
	case Number:
		return NumberText(float64(v)), nil
	case Bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case Blank, Missing:
		return "", nil
	case Error:
		return "", v
	case *Array:
		if v.Len() > 0 {
			return Str(v.V[0])
		}
	}
	return "", ErrValue
}

// Truth converts a scalar to a boolean: numbers are true when non-zero,
// "TRUE"/"FALSE" text is accepted, other text is #VALUE!.
func Truth(v Value) (bool, Value) {
	switch v := v.(type) {
	case Bool:
		return bool(v), nil
	case Number:
		return v != 0, nil
	case Blank, Missing:
		return false, nil
	case String:
		switch strings.ToUpper(string(v)) {
		case "TRUE":
			return true, nil
		case "FALSE":
			return false, nil
		}
		return false, ErrValue
	case Error:
		return false, v
	case *Array:
		if v.Len() > 0 {
			return Truth(v.V[0])
		}
	}
	return false, ErrValue
}

// NumberText formats a number as Excel does when converting it to text
// (e.g. =1/3&""): up to 15 significant digits, no trailing zeros,
// scientific notation for very large or small magnitudes.
func NumberText(f float64) string {
	if f == 0 {
		return "0"
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return string(ErrNum)
	}
	a := math.Abs(f)
	if a >= 1e15 || a < 1e-9 {
		s := strconv.FormatFloat(f, 'E', 14, 64)
		mant, exp, _ := strings.Cut(s, "E")
		mant = strings.TrimSuffix(strings.TrimRight(mant, "0"), ".")
		sign := exp[0]
		exp = strings.TrimLeft(exp[1:], "0")
		if len(exp) < 2 {
			exp = strings.Repeat("0", 2-len(exp)) + exp
		}
		return mant + "E" + string(sign) + exp
	}
	s := strconv.FormatFloat(f, 'G', 15, 64)
	if strings.ContainsAny(s, "Ee") {
		s = strconv.FormatFloat(f, 'f', -1, 64)
		// Round to 15 significant digits.
		v, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'G', 15, 64), 64)
		s = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return s
}

// ParseNumber parses text the way Excel coerces it to a number: plain
// numbers, thousands separators, a leading currency sign, a trailing
// percent, parentheses for negatives, and dates/times.
func ParseNumber(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, false
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil && !strings.ContainsAny(t, "xXpPnN_") {
		return f, true
	}
	neg := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg, t = true, t[1:len(t)-1]
	}
	pct := strings.HasSuffix(t, "%")
	t = strings.TrimSpace(strings.TrimSuffix(t, "%"))
	if strings.HasPrefix(t, "-") {
		neg, t = !neg, t[1:]
	} else if strings.HasPrefix(t, "+") {
		t = t[1:]
	}
	t = strings.TrimPrefix(t, "$")
	if strings.HasPrefix(t, "-") { // $-5
		neg, t = !neg, t[1:]
	}
	if strings.Contains(t, ",") {
		intPart, _, _ := strings.Cut(t, ".")
		groups := strings.Split(intPart, ",")
		for i, g := range groups {
			if (i == 0 && (g == "" || len(g) > 3)) || (i > 0 && len(g) != 3) {
				return parseDateTimeNumber(s)
			}
		}
		t = strings.ReplaceAll(t, ",", "")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil || strings.ContainsAny(t, "xXpPnN_") {
		return parseDateTimeNumber(s)
	}
	if pct {
		f /= 100
	}
	if neg {
		f = -f
	}
	return f, true
}

func parseDateTimeNumber(s string) (float64, bool) {
	if v, ok := ParseDateTime(s, false); ok {
		return v, true
	}
	return 0, false
}

// --- comparison ---------------------------------------------------------

// typeRank orders value types for comparison: numbers < text < booleans.
func typeRank(v Value) int {
	switch v.(type) {
	case Number, Blank, Missing:
		return 0
	case String:
		return 1
	case Bool:
		return 2
	}
	return 3
}

// Compare compares two scalars as Excel's comparison operators do: numbers
// sort before text, text before booleans; text compares case-insensitively.
// Numbers equal to 15 significant digits are equal (=0.1+0.2=0.3 is TRUE).
// A blank compares as 0 against numbers, "" against text and FALSE against
// booleans. Errors are returned as the second result.
func Compare(a, b Value) (int, Value) {
	if e, ok := a.(Error); ok {
		return 0, e
	}
	if e, ok := b.(Error); ok {
		return 0, e
	}
	a, b = blankAs(a, b), blankAs(b, a)
	ra, rb := typeRank(a), typeRank(b)
	if ra != rb {
		if ra < rb {
			return -1, nil
		}
		return 1, nil
	}
	switch av := a.(type) {
	case Number:
		bv := b.(Number)
		switch {
		case equal15(float64(av), float64(bv)):
			return 0, nil
		case av < bv:
			return -1, nil
		case av > bv:
			return 1, nil
		}
		return 0, nil
	case String:
		return strings.Compare(strings.ToLower(string(av)), strings.ToLower(string(b.(String)))), nil
	case Bool:
		bv := b.(Bool)
		switch {
		case av == bv:
			return 0, nil
		case !bool(av):
			return -1, nil
		}
		return 1, nil
	}
	return 0, nil
}

// equal15 reports whether a and b are the same number to 15 significant
// digits, the precision Excel compares at.
func equal15(a, b float64) bool {
	if a == b {
		return true
	}
	if math.Abs(a-b) > max(math.Abs(a), math.Abs(b))*1e-14 {
		return false // the usual case, without formatting
	}
	return Round15(a) == Round15(b)
}

// blankAs turns a blank into the zero value of other's type.
func blankAs(v, other Value) Value {
	switch v.(type) {
	case Blank, Missing:
		switch other.(type) {
		case String:
			return String("")
		case Bool:
			return Bool(false)
		}
		return Number(0)
	}
	return v
}

// Equal reports whether two scalars are equal under Compare.
func Equal(a, b Value) bool {
	c, err := Compare(a, b)
	return err == nil && c == 0
}
