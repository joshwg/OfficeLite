// limits.go holds every limit of the Excel side in one place: the sizes
// the in-memory model holds, the caps the file readers and writers apply
// (the formats' own limits and guards against damaged or hostile files),
// and the editor's, renderer's and UI's caps and tunables. Each name says
// which area owns it (Xlsx…, Xls…, CSV…, Chart…, UI…).

package excel

import "time"

// Sheet size limits. Anything beyond these is dropped on load with a warning.
const (
	// MaxRows is the most rows a sheet holds: 262,144 (256K), this app's
	// cap, a quarter of Excel's 1,048,576.
	MaxRows = 262144 // 256K
	// MaxCols is the most columns a sheet holds: 2,048 (2K), an eighth of
	// Excel's 16,384 (XFD).
	MaxCols = 2048 // 2K
)

// MaxOutlineLevel is the deepest outline level (Excel's 8 levels of
// buttons: 1 for the top, 7 of grouping).
const MaxOutlineLevel = 7

// Excel's own sheet size (A1:XFD1048576), larger than what this model
// holds: file readers check cell references against it to tell a valid
// reference past MaxRows/MaxCols (dropped with a warning) from a damaged one.
const (
	// ExcelMaxRows is the number of rows in an Excel 2007+ sheet.
	ExcelMaxRows = 1048576
	// ExcelMaxCols is the number of columns in an Excel 2007+ sheet (XFD).
	ExcelMaxCols = 16384
	// MaxSheetName is the most characters in a sheet name (Excel's rule);
	// readers cut longer names and the editor refuses them.
	MaxSheetName = 31
	// ExcelMaxColLetters is the most letters in a column name (XFD).
	ExcelMaxColLetters = 3
)

// Hyperlink fill caps shared by the .xlsx and .xls readers: a link over
// A1:XFD1048576 must not create a link on every cell of the sheet.
const (
	// MaxLinkCells bounds how many cells one hyperlink range
	// (<hyperlink ref="A1:Z9999"> or an HLINK record) fills.
	MaxLinkCells = 10000
	// MaxCommentReplies bounds how many replies of one comment thread
	// are read; the rest are dropped.
	MaxCommentReplies = 1000
	// MaxSheetLinkCells bounds how many cells all of a sheet's hyperlinks
	// fill together.
	MaxSheetLinkCells = 1 << 20
)

// .xlsx reader and writer limits.
const (
	// XlsxUnzipLimit caps how much excelize unpacks when it opens a
	// package for its styles: the parts' total uncompressed size, so a zip
	// bomb can't fill memory or the temporary directory (excelize's own
	// default is 16 GB). A larger workbook opens with unformatted values.
	XlsxUnzipLimit = 1 << 30
	// XlsxMaxPartSize caps the uncompressed size of each zip entry
	// (part) the .xlsx reader and writer read themselves: 1 GB. A larger
	// part is an error ("file is damaged or too large"), never cut short:
	// at most this many bytes plus one are read to tell. A required part
	// (workbook, relationships, shared strings, a worksheet) fails the
	// open; an optional one (theme, chart, comments, pivot table) is left
	// out with a warning.
	XlsxMaxPartSize = 1 << 30
	// XlsxMaxChartPartDepth is how many levels of a chart's relationships
	// (chart → its parts → theirs ...) the reader follows to keep the
	// parts the chart refers to.
	XlsxMaxChartPartDepth = 3
	// XlsxPatchBufSize is the size of the read and write buffers a
	// worksheet is streamed through when the writer patches it: 64 KiB.
	XlsxPatchBufSize = 64 << 10
)

// Chart cache caps for a chart's cached series (the .xlsx reader's
// cachePoints): a ptCount or idx of a million on every series of a few
// kilobytes of XML used to allocate gigabytes.
const (
	// ChartCacheMaxPoints is the most points one cached series holds.
	ChartCacheMaxPoints = 1 << 20
	// ChartCacheSlotsPerPoint is how many slots each <c:pt> present may
	// claim: the length trusts ptCount and the points' indexes only up to
	// this many slots per point.
	ChartCacheSlotsPerPoint = 64
)

// .xls (BIFF8) sheet, record and string limits: the format's own, which
// the writer clamps to.
const (
	// XlsMaxRows is the number of rows in an Excel 97-2003 sheet.
	XlsMaxRows = 65536
	// XlsMaxCols is the number of columns in an Excel 97-2003 sheet (IV).
	XlsMaxCols = 256
	// XlsMaxRecord is the largest record body; longer data goes in
	// CONTINUE records.
	XlsMaxRecord = 8224
	// XlsMaxCellStyles is the most cell XFs the writer emits, about what
	// Excel 97-2003 holds; cells past it use the default style.
	XlsMaxCellStyles = 4000
	// XlsMaxText is the most UTF-16 code units in a cell's text (or a
	// note's).
	XlsMaxText = 32767
	// XlsMaxShortString is the most characters a BIFF8 short string
	// holds: font names, number formats, hyperlink tooltips, AutoFilter
	// strings and formula text constants.
	XlsMaxShortString = 255
	// XlsMaxMergesPerRecord is the most ranges one MERGEDCELLS record
	// holds.
	XlsMaxMergesPerRecord = 1026
	// XlsMaxRowHeightTwips is the tallest row height a ROW record holds,
	// in twips (409.6 pt).
	XlsMaxRowHeightTwips = 8192
	// XlsDVMaxTitle is the most characters in a data validation rule's
	// input or error title.
	XlsDVMaxTitle = 32
	// XlsDVMaxPrompt is the most characters in a data validation rule's
	// input message.
	XlsDVMaxPrompt = 255
	// XlsDVMaxError is the most characters in a data validation rule's
	// error message.
	XlsDVMaxError = 225
)

// .xls formula limits and reader guards.
const (
	// XlsMaxFormulaBytes is the longest parsed expression Excel accepts.
	XlsMaxFormulaBytes = 1800
	// XlsMaxArgs is the BIFF8 limit on arguments of one call.
	XlsMaxArgs = 30
	// XlsMaxFormulaText caps a decoded formula's length, at twice the
	// 8,192 characters Excel allows: a hostile token stream (thousands of
	// 255-space tAttrSpace tokens joined by operators) otherwise grows
	// megabytes of text, copied again at every operator, and stalls the
	// reader for seconds per formula.
	XlsMaxFormulaText = 16384
	// XlsMaxHyperlinkString is the longest HyperlinkString character
	// count read; a larger one marks the HLINK record damaged.
	XlsMaxHyperlinkString = 1 << 16
)

// CSV reader and writer limits.
const (
	// CSVSniffLines is how many non-blank lines the reader looks at to
	// pick the separator: enough to see a consistent field count without
	// scanning a whole large file.
	CSVSniffLines = 20
	// CSVWriteBufSize is the size of the writer's output buffer: 64 KiB.
	CSVWriteBufSize = 64 << 10
)

// NumbersDenseTableCells is the largest Apple Numbers table (rows ×
// columns) whose every cell the reader visits; a bigger table visits only
// its stored cells, as empty ones keep no style.
const NumbersDenseTableCells = 1 << 20

// Editor (excel/edit) caps.
const (
	// UndoHistory is how many changes Undo can take back.
	UndoHistory = 100
	// UndoLabelMaxText is how many characters of typed text an Undo list
	// label ("Typing '…' in A1") quotes before it is cut with "…".
	UndoLabelMaxText = 20
	// MaxInvalidCircles is how many invalid cells Circle Invalid Data
	// marks, as in Excel.
	MaxInvalidCircles = 255
	// MaxNameDepth is how many defined names Go To follows when one name
	// refers to another; deeper chains (or a name referring to itself) are
	// a bad reference.
	MaxNameDepth = 4
	// FillMaxDigits is the most digits of a numbered text's suffix (Item1,
	// Week 10) that Fill continues as a series, so the number fits an int.
	FillMaxDigits = 9
)

// Formula engine (excel/formula) limits. References are bounded by Excel's
// grid (ExcelMaxRows × ExcelMaxCols), not this model's smaller one: a
// formula may name A1048576 or XFD1 even though no cell can be there.
const (
	// FormulaMaxText is the longest text (in UTF-16 code units) a cell can
	// hold; Excel's & gives #VALUE! for a longer result.
	FormulaMaxText = 32767
	// FormulaMaxArrayCells bounds the arrays functions build from a size
	// given as an argument (SEQUENCE, RANDARRAY, MAKEARRAY, EXPAND, WRAPROWS
	// ...): larger requests are an error rather than an allocation that
	// exhausts memory (16M elements is already ~400 MB). Check each
	// dimension against it before multiplying them, so the product can't
	// overflow.
	FormulaMaxArrayCells = 1 << 24
	// FormulaMaxParseDepth bounds the parser's recursion and the depth of
	// the tree it builds (which Eval recurses over). Excel caps formulas at
	// 8192 characters, so no real formula nests deeper; without a bound a
	// hostile file's "=((((...1", "=----...1" or "=1+1+...+1" could exhaust
	// the goroutine stack, which is a fatal error rather than a recoverable
	// panic.
	FormulaMaxParseDepth = 8192
	// FormulaMaxEvalDepth bounds nested evaluation (deep LAMBDA recursion):
	// past it a call is #CALC! rather than a stack overflow.
	FormulaMaxEvalDepth = 1000
	// FormulaSciFrom is the magnitude from which a number converted to text
	// (=1/3&"") is shown in scientific notation: 1e15 has 16 digits, more
	// than the 15 significant digits Excel keeps.
	FormulaSciFrom = 1e15
	// FormulaSciBelow is the magnitude below which a number converted to
	// text is shown in scientific notation rather than as a long run of
	// leading zeros.
	FormulaSciBelow = 1e-9
)

// Financial function (functions/financial) limits: the longest schedule
// stepped through period by period, and the stopping limits of the Newton
// iterations that solve for a rate.
const (
	// FinancialMaxPeriods bounds the periods the functions that step
	// through them one by one (CUMIPMT, CUMPRINC, DB, VDB) accept: more is
	// #NUM! rather than a calculation that hangs (or never ends, once p++
	// stops changing p past 2^53).
	FinancialMaxPeriods = 1e7
	// FinancialRateMaxIter is RATE's and IRR's Newton step limit, 20 as in
	// Excel: a guess that has not converged by then is #NUM!, and the user
	// supplies a better one.
	FinancialRateMaxIter = 20
	// FinancialRateTol is RATE's and IRR's convergence tolerance on the
	// rate step, 1e-7 (0.00001%) as Excel documents.
	FinancialRateTol = 1e-7
	// FinancialXIRRMaxIter is XIRR's Newton step limit, 100 as in Excel.
	FinancialXIRRMaxIter = 100
	// FinancialXIRRTol is XIRR's convergence tolerance on the rate step,
	// 1e-8 (0.000001%) as Excel documents.
	FinancialXIRRTol = 1e-8
)

// Math and trig function (functions/mathtrig) argument limits.
const (
	// MathTrigLimit is the magnitude from which Excel's SIN, COS, TAN (and
	// their reciprocals) return #NUM! because the argument has lost all
	// precision: 2²⁷.
	MathTrigLimit = 1 << 27
	// MathMaxFact is the largest n whose n! is finite in a float64 (171! is
	// about 1.2e309): FACT of more is +Inf, so #NUM!.
	MathMaxFact = 170
	// MathMaxFactDouble is FACTDOUBLE's largest argument: 301!! overflows a
	// float64, so more is #NUM!.
	MathMaxFactDouble = 300
	// MathRomanLimit is one past ROMAN's largest number (3999, MMMCMXCIX):
	// Roman numerals have no symbol for 5000, so 4000 and up are #VALUE!.
	MathRomanLimit = 4000
	// MathMaxNumeralLen is the longest numeral text ARABIC and DECIMAL
	// accept and the longest minimum length BASE pads to: Excel's
	// 255-character limit on these arguments.
	MathMaxNumeralLen = 255
	// MathMaxRoundDigits clamps the digits argument of ROUND, ROUNDUP,
	// ROUNDDOWN and TRUNC to ±400: beyond the ~±324 decimal exponents a
	// float64 spans more digits change nothing, and the clamp keeps the
	// float-to-int conversion defined.
	MathMaxRoundDigits = 400
)

// Statistical distribution (functions/mathtrig) limits: argument ranges,
// and the iteration caps and tolerances that stop the series, continued
// fractions and solvers behind the distributions.
const (
	// DistMaxDegFreedom is the largest degrees of freedom the chi-squared
	// and F distributions accept (Excel's 10^10): more is #NUM!.
	DistMaxDegFreedom = 1e10
	// DistProbSumTol is how far PROB's probabilities may sum from 1
	// (rounding in the user's figures) before the call is #NUM!.
	DistProbSumTol = 1e-12
	// DistSmallN bounds the counts for which the discrete distributions use
	// exact binomial coefficients (products of up to DistSmallN terms)
	// rather than log-gamma functions.
	DistSmallN = 1000
	// DistMaxTerms bounds the terms a discrete distribution sums one by
	// one, so a huge count can't hang the calculation.
	DistMaxTerms = 1e7
	// DistBinomBisectMaxIter caps BINOM.INV's bisection on the cumulative
	// distribution for large trial counts. Each step halves the bracket
	// [-1, n], and n is at most ~1.8e308 (2¹⁰²⁴), so it closes in about
	// 1,075 steps; 2100 leaves a margin and still guarantees termination.
	DistBinomBisectMaxIter = 2100
	// DistCFEps is the relative change of an incomplete gamma or beta
	// series sum or continued-fraction convergent below which it has
	// converged: about float64 precision.
	DistCFEps = 1e-16
	// DistCFMaxIter caps the terms of those series and continued fractions,
	// so an argument they converge slowly for can't hang the calculation.
	DistCFMaxIter = 100000
	// DistInvertMaxIter caps the safeguarded Newton steps of the inverse
	// distributions' solver; each step at least bisects the bracket, so this
	// is far more than any float64 bracket needs.
	DistInvertMaxIter = 2000
	// DistInvertTol is the relative step (or bracket width) below which the
	// inverse distributions' solver has converged: a few units in the last
	// place of a float64.
	DistInvertTol = 1e-15
	// DistInvertMaxBracket is how far the inverse distributions' solver
	// doubles an unbounded upper bracket looking for a sign change before it
	// reports +Inf.
	DistInvertMaxBracket = 1e300
)

// Text, engineering and date-time function limits.
const (
	// TextMaxJoinBytes is the UTF-8 length at which TEXTJOIN stops joining
	// early: four bytes per allowed UTF-16 unit is surely past
	// FormulaMaxText, so a huge join stops growing (the result is checked
	// exactly afterwards).
	TextMaxJoinBytes = 4 * FormulaMaxText
	// TextMaxIntArg clamps the integer arguments of the text functions to
	// ±10¹² rather than rejecting huge ones (LEFT("abc",1E10) is "abc"),
	// keeping the float-to-int conversion defined.
	TextMaxIntArg = 1e12
	// TextMaxDecimals is the most decimals FIXED and DOLLAR accept (Excel's
	// 127); more is #VALUE!.
	TextMaxDecimals = 127
	// TextMinDecimals is where FIXED's and DOLLAR's negative decimals are
	// clamped: rounding to more than 400 places left of the point always
	// gives 0.
	TextMinDecimals = -400
	// EngMaxDigits is the most digits a binary, octal or hex number has in
	// the base-conversion functions (BIN2DEC, DEC2HEX ...), both read and
	// as places: 10, Excel's limit.
	EngMaxDigits = 10
	// DateTimeMaxTimeArg is the largest hour, minute or second TIME accepts
	// (Excel's 32767); more is #NUM!.
	DateTimeMaxTimeArg = 32767
)

// Calculator (excel/calc) tunables of the dependency index.
const (
	// CalcWideCols is the width above which a range is kept in a sheet's
	// list of wide ranges instead of under each of its columns.
	CalcWideCols = 16
	// CalcMaxNameDepth is how deep the dependency index follows defined
	// names referring to names; a deeper (or circular) chain marks the
	// formula as always recalculated instead.
	CalcMaxNameDepth = 8
	// CalcCellScanLimit is the largest area (in cells) whose dependents or
	// formulas are found by visiting each cell; a larger one scans the
	// index instead.
	CalcCellScanLimit = 4096
	// CalcMaxStale is how many index entries of removed or replaced
	// formulas may pile up before the index is rebuilt (once they also
	// outnumber twice the live formulas).
	CalcMaxStale = 4096
)

// Number format (excel/numfmt) General-format limits.
const (
	// GeneralMaxLen is the most characters General shows for a positive
	// number (the sign doesn't count), as in an Excel cell of default
	// width: longer numbers lose decimals.
	GeneralMaxLen = 11
	// GeneralSciFrom is the magnitude from which General shows a number in
	// scientific notation: 1e11 has 12 digits, more than GeneralMaxLen.
	GeneralSciFrom = 1e11
	// GeneralSciBelow is the magnitude below which General shows a number
	// in scientific notation.
	GeneralSciBelow = 1e-9
)

// PivotMaxSourceHops is how many defined names a pivot table's source may
// pass through (a name referring to a name ...) before it resolves to a
// range; a deeper or circular chain is not a source.
const PivotMaxSourceHops = 4

// calccheck (excel/cmd/calccheck) report limits.
const (
	// CalcCheckMaxListed is how many mismatched cells the report lists
	// without -v.
	CalcCheckMaxListed = 20
	// CalcCheckTol is the relative difference within which a recalculated
	// number matches the saved one (Excel keeps 15 significant digits).
	CalcCheckTol = 1e-9
)

// Renderer (excel/render) caps.
const (
	// GridOverflowScan is how many columns beyond the viewport are checked
	// for text that overflows into view.
	GridOverflowScan = 32
	// ChartMaxDashes is how many dashes a dashed chart line is split into
	// at most; a longer line (a point far outside fixed axis bounds) is
	// drawn solid.
	ChartMaxDashes = 100000
	// ChartMaxTicks is the highest major tick index (ticks 0 to
	// ChartMaxTicks) a linear axis draws, so a tiny major unit can't stall
	// the frame.
	ChartMaxTicks = 1000
	// ChartMaxLogTicks is the most major ticks a logarithmic axis draws.
	ChartMaxLogTicks = 100
	// ChartMaxMinorTicks is the most minor-tick intervals an axis draws;
	// with more, minor ticks are left out.
	ChartMaxMinorTicks = 500
	// ChartOutlineMaxPoints is how many outline points a chart element's
	// selection shape collects before it stops adding primitives.
	ChartOutlineMaxPoints = 200
)

// UI (excel/ui) caps and tunables.
const (
	// UIDoubleClick is the longest time between the presses of a
	// double-click.
	UIDoubleClick = 500 * time.Millisecond
	// UIDropMaxBytes caps how much of a file dropped on the grid is read:
	// 64 MiB.
	UIDropMaxBytes = 64 << 20
	// UIGrowRowsMax bounds the automatic row height after a font or wrap
	// change: a selection spanning more than this many rows past its first
	// (R2−R1) keeps its heights, so formatting whole columns stays quick.
	UIGrowRowsMax = 2000
	// ExcelMaxRowHeightPt is the tallest row height Excel accepts, in
	// points.
	ExcelMaxRowHeightPt = 409
	// ExcelMaxColWidth is the widest column width Excel accepts, in
	// characters.
	ExcelMaxColWidth = 255
	// FilterTop10MaxItems is the most items a Top 10 AutoFilter keeps.
	FilterTop10MaxItems = 500
	// FilterTop10MaxPercent is the largest percent a Top 10 AutoFilter
	// keeps.
	FilterTop10MaxPercent = 100
)
