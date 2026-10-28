// Package functions assembles the worksheet function library. Each category
// lives in its own subpackage with a Register(*formula.Registry) function:
//
//	logical/    IF, IFS, SWITCH, IFERROR, AND, ... MAP, REDUCE, BYROW, ...
//	database/   DSUM, DAVERAGE, DCOUNT, DGET, ... (criteria ranges)
//	datetime/   DATE, EDATE, NETWORKDAYS, YEARFRAC, ...
//	financial/  FV, NPV, PMT
//	info/       ISBLANK, ISNUMBER, CELL, TYPE, ...
//	lookup/     XLOOKUP, FILTER, UNIQUE, VLOOKUP, INDEX, MATCH, OFFSET, ...
//	            (and the array-shaping TAKE, DROP, VSTACK, TOCOL, ...,
//	            TRIMRANGE, GROUPBY and PIVOTBY)
//	mathtrig/   SUM, SUMIFS, ROUND, SUBTOTAL, AGGREGATE, SEQUENCE, ...
//	            (and the statistical functions: COUNT, COUNTIFS, AVERAGE,
//	            MEDIAN, STDEV, RANK, CORREL, LINEST, NORM.DIST, T.TEST, ...)
//	text/       LEFT, TEXT, TEXTBEFORE, TEXTSPLIT, REGEXEXTRACT, ...
//
// LET and LAMBDA are built into the evaluator (excel/formula), as are
// functions named as values (eta-reduced lambdas: BYROW(A1:B3,SUM)); a
// function taking a lambda calls either kind with e.CallLambda.
//
// # Conventions for function implementations
//
// Import the core as `f "officelite/excel/formula"` and register specs:
//
//	f.Spec{Name: "LEFT", Min: 1, Max: 2, Fn: left}
//
// Arguments. By default every argument is a scalar: the evaluator
// dereferences single cells and, when a range or array is passed where a
// scalar is expected, calls Fn once per element and returns an array
// ("lifting": =LEFT(A1:A3,1) gives three results). Fn then only ever sees
// f.Number, f.String, f.Bool, f.Error, f.Blank or f.Missing. To receive a
// whole range or array instead, list the argument in Spec.Arrays (it
// arrives as *f.Array); to receive the reference itself (for ROW, ISREF,
// SUMIF's ranges, or to iterate a huge range without materializing it),
// list it in Spec.Refs (it arrives as f.Ref when it was a reference,
// otherwise as a value). Helpers: f.All, f.From(k), f.Only(i, j).
//
// Omitted arguments arrive as f.Missing ("=ROUND(1.5,)" passes Missing as
// the second argument; so does a short argument list padded by the
// function itself). Treat Missing as the documented default.
//
// Coercion (Excel's rules): f.Num, f.Int (truncates), f.Str, f.Truth return
// (value, err) where err is a non-nil f.Error to return as-is:
//
//	n, err := f.Num(args[0])
//	if err != nil {
//		return err
//	}
//
// Errors in inputs propagate unless the function is documented otherwise.
// Return f.NumberOrErr(x) for computed floats (NaN/Inf become #NUM!).
//
// Iteration: e.Each(v, fn) visits every scalar of a Ref (trimmed to the
// used area), an *Array, or a scalar (direct == true). Excel aggregates
// count numbers in ranges but ignore text/booleans there, while scalars
// typed directly as arguments are coerced (SUM(A1:A3) ignores the text
// "3" in a cell, SUM("3") is 3).
//
// Arrays: f.NewArray(rows, cols), (*Array).At/Set/Row/Broadcast,
// e.ToArray(v) (deref + wrap scalars), e.Deref(v), e.Scalar(v). A function
// may return an *Array; the cell spills it.
//
// Other services: e.Now(), e.Date1904(), e.Rand(), e.CallLambda(fn,
// args...), e.Ctx (sheet names, formulas, used size), e.Reference (text
// to a reference, as INDIRECT reads it), f.Compare / f.Equal
// (Excel ordering: numbers < text < booleans, text case-insensitive),
// f.ParseCriteria(v).Match(x), f.WildcardMatch, f.ParseNumber,
// f.ParseDateTime, f.DateSerial, f.SerialToYMD, f.SerialToTime,
// f.TimeToSerial, f.NumberText (number to text like =1/3&"").
//
// Spec.Format suggests a number format for the result when the cell is
// General, e.g. "m/d/yyyy" for DATE so the serial number shows as a date.
// Spec.Volatile marks functions recalculated on every calculation.
//
// Tests: table-driven with officelite/excel/formula/functions/internal/ftest:
//
//	ftest.Run(t, reg, setup, []ftest.Case{{"=LEFT(\"abc\",2)", "ab"}, ...})
//
// TestContext has sheets Sheet1 and Sheet2, formulas evaluate at
// Sheet1!A1000 unless EvalAt is used, and the clock is fixed at
// 2026-10-15 13:30.
package functions
