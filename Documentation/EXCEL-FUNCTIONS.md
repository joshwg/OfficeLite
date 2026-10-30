# Excel Functions

This is a reference to the worksheet functions implemented by ExcelLite.
Functions are grouped by their usual Excel category. Each entry gives the
accepted syntax and a short description. Optional arguments appear in
brackets; ellipses indicate a repeated argument or criteria pair.

ExcelLite currently implements 433 worksheet functions, plus the built-in
`LET` and `LAMBDA` functions. The Microsoft links below lead to Microsoft's
[Excel functions by category](https://support.microsoft.com/en-us/excel/excel-functions-by-category)
reference, where each function name links to its full Microsoft description.
ExcelLite's results and supported options are not guaranteed to match every
Excel release in all cases.

## Database

`database` is a table with field names in its first row; `field` is a field
name or 1-based column number; `criteria` is a criteria range. Criteria in
the same row are ANDed; criteria in separate rows are ORed.

| Function | Syntax | Description |
|---|---|---|
| DAVERAGE | `DAVERAGE(database, field, criteria)` | Averages the matching records' values in a field. |
| DCOUNT | `DCOUNT(database, [field], criteria)` | Counts numeric values in a field among matching records; without `field`, counts matching records. |
| DCOUNTA | `DCOUNTA(database, [field], criteria)` | Counts nonblank values in a field among matching records; without `field`, counts matching records. |
| DGET | `DGET(database, field, criteria)` | Returns the field value from the single matching record. |
| DMAX | `DMAX(database, field, criteria)` | Returns the largest matching value in a field. |
| DMIN | `DMIN(database, field, criteria)` | Returns the smallest matching value in a field. |
| DPRODUCT | `DPRODUCT(database, field, criteria)` | Multiplies the matching values in a field. |
| DSTDEV | `DSTDEV(database, field, criteria)` | Estimates the sample standard deviation of matching values. |
| DSTDEVP | `DSTDEVP(database, field, criteria)` | Calculates the population standard deviation of matching values. |
| DSUM | `DSUM(database, field, criteria)` | Adds the matching values in a field. |
| DVAR | `DVAR(database, field, criteria)` | Estimates the sample variance of matching values. |
| DVARP | `DVARP(database, field, criteria)` | Calculates the population variance of matching values. |

## Date and time

Microsoft reference: [Date and time functions](https://support.microsoft.com/en-us/excel/date-and-time-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| DATE | `DATE(year, month, day)` | Creates a date serial from year, month and day values. |
| DATEDIF | `DATEDIF(start_date, end_date, unit)` | Returns the number of days, months or years between two dates (`"Y"`, `"M"`, `"D"`, `"YM"`, `"YD"` or `"MD"`). |
| DATEVALUE | `DATEVALUE(date_text)` | Converts recognized date text to a date serial. |
| DAY | `DAY(serial_number)` | Returns the day of the month, 1–31. |
| DAYS | `DAYS(end_date, start_date)` | Returns the number of days between two dates. |
| DAYS360 | `DAYS360(start_date, end_date, [method])` | Returns days between dates using a 360-day year. |
| EDATE | `EDATE(start_date, months)` | Returns a date the given number of months before or after a start date. |
| EOMONTH | `EOMONTH(start_date, months)` | Returns the last day of the month a given number of months away. |
| HOUR | `HOUR(serial_number)` | Returns the hour component, 0–23. |
| ISOWEEKNUM | `ISOWEEKNUM(date)` | Returns the ISO 8601 week number of a date. |
| MINUTE | `MINUTE(serial_number)` | Returns the minute component, 0–59. |
| MONTH | `MONTH(serial_number)` | Returns the month number, 1–12. |
| NETWORKDAYS | `NETWORKDAYS(start_date, end_date, [holidays])` | Counts workdays between dates, excluding weekends and optional holidays. |
| NETWORKDAYS.INTL | `NETWORKDAYS.INTL(start_date, end_date, [weekend], [holidays])` | Counts workdays using a configurable weekend and optional holidays. |
| NOW | `NOW()` | Returns the current date and time. |
| SECOND | `SECOND(serial_number)` | Returns the second component, 0–59. |
| TIME | `TIME(hour, minute, second)` | Converts hour, minute and second values to a time serial. |
| TIMEVALUE | `TIMEVALUE(time_text)` | Converts recognized time text to a time serial. |
| TODAY | `TODAY()` | Returns the current date. |
| WEEKDAY | `WEEKDAY(serial_number, [return_type])` | Returns the day-of-week number using the requested numbering system. |
| WEEKNUM | `WEEKNUM(serial_number, [return_type])` | Returns the week number of a date. |
| WORKDAY | `WORKDAY(start_date, days, [holidays])` | Returns a workday a given number of workdays before or after a date. |
| WORKDAY.INTL | `WORKDAY.INTL(start_date, days, [weekend], [holidays])` | Returns a workday using a configurable weekend and optional holidays. |
| YEAR | `YEAR(serial_number)` | Returns the year of a date. |
| YEARFRAC | `YEARFRAC(start_date, end_date, [basis])` | Returns the fraction of a year between two dates. |

## Engineering

Microsoft reference: [Engineering functions](https://support.microsoft.com/en-us/excel/engineering-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| BIN2DEC | `BIN2DEC(number)` | Converts a binary number to decimal. |
| BIN2HEX | `BIN2HEX(number, [places])` | Converts a binary number to hexadecimal. |
| BIN2OCT | `BIN2OCT(number, [places])` | Converts a binary number to octal. |
| CONVERT | `CONVERT(number, from_unit, to_unit)` | Converts a number between supported measurement units. |
| DEC2BIN | `DEC2BIN(number, [places])` | Converts a decimal number to binary. |
| DEC2HEX | `DEC2HEX(number, [places])` | Converts a decimal number to hexadecimal. |
| DEC2OCT | `DEC2OCT(number, [places])` | Converts a decimal number to octal. |
| HEX2BIN | `HEX2BIN(number, [places])` | Converts a hexadecimal number to binary. |
| HEX2DEC | `HEX2DEC(number)` | Converts a hexadecimal number to decimal. |
| HEX2OCT | `HEX2OCT(number, [places])` | Converts a hexadecimal number to octal. |
| OCT2BIN | `OCT2BIN(number, [places])` | Converts an octal number to binary. |
| OCT2DEC | `OCT2DEC(number)` | Converts an octal number to decimal. |
| OCT2HEX | `OCT2HEX(number, [places])` | Converts an octal number to hexadecimal. |

Base-conversion functions use Excel's 10-digit two's-complement convention
for negative binary, octal and hexadecimal values.

## Financial

Microsoft reference: [Financial functions](https://support.microsoft.com/en-us/excel/financial-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| CUMIPMT | `CUMIPMT(rate, nper, pv, start_period, end_period, type)` | Returns cumulative interest paid between two payment periods. |
| CUMPRINC | `CUMPRINC(rate, nper, pv, start_period, end_period, type)` | Returns cumulative principal paid between two payment periods. |
| DB | `DB(cost, salvage, life, period, [month])` | Calculates fixed-declining-balance depreciation. |
| DDB | `DDB(cost, salvage, life, period, [factor])` | Calculates double-declining or other declining-balance depreciation. |
| DOLLARDE | `DOLLARDE(fractional_dollar, fraction)` | Converts a fractional-dollar price to a decimal number. |
| DOLLARFR | `DOLLARFR(decimal_dollar, fraction)` | Converts a decimal-dollar price to a fractional-dollar number. |
| EFFECT | `EFFECT(nominal_rate, npery)` | Returns the effective annual interest rate. |
| FV | `FV(rate, nper, pmt, [pv], [type])` | Returns the future value of an investment with periodic payments and a constant rate. |
| FVSCHEDULE | `FVSCHEDULE(principal, schedule)` | Returns the future value after applying a series of compound rates. |
| IPMT | `IPMT(rate, per, nper, pv, [fv], [type])` | Returns the interest portion of a payment for a specified period. |
| IRR | `IRR(values, [guess])` | Estimates the internal rate of return for periodic cash flows. |
| ISPMT | `ISPMT(rate, per, nper, pv)` | Calculates interest paid during a specific period of an investment. |
| MIRR | `MIRR(values, finance_rate, reinvest_rate)` | Returns the modified internal rate of return for periodic cash flows. |
| NOMINAL | `NOMINAL(effect_rate, npery)` | Returns the nominal annual interest rate. |
| NPER | `NPER(rate, pmt, pv, [fv], [type])` | Returns the number of payment periods for an investment or loan. |
| NPV | `NPV(rate, value1, [value2], ...)` | Returns the net present value of periodic cash flows. |
| PDURATION | `PDURATION(rate, pv, fv)` | Returns the periods required for an investment to reach a target value. |
| PMT | `PMT(rate, nper, pv, [fv], [type])` | Returns the periodic payment for a loan or annuity. |
| PPMT | `PPMT(rate, per, nper, pv, [fv], [type])` | Returns the principal portion of a payment for a specified period. |
| PV | `PV(rate, nper, pmt, [fv], [type])` | Returns the present value of an investment or loan. |
| RATE | `RATE(nper, pmt, pv, [fv], [type], [guess])` | Estimates the interest rate per period for an annuity or loan. |
| RRI | `RRI(nper, pv, fv)` | Returns the equivalent interest rate for investment growth. |
| SLN | `SLN(cost, salvage, life)` | Calculates straight-line depreciation for one period. |
| SYD | `SYD(cost, salvage, life, per)` | Calculates sum-of-years'-digits depreciation for one period. |
| TBILLEQ | `TBILLEQ(settlement, maturity, discount)` | Returns a Treasury bill's bond-equivalent yield. |
| TBILLPRICE | `TBILLPRICE(settlement, maturity, discount)` | Returns the price per $100 face value of a Treasury bill. |
| TBILLYIELD | `TBILLYIELD(settlement, maturity, pr)` | Returns the yield of a Treasury bill. |
| VDB | `VDB(cost, salvage, life, start_period, end_period, [factor], [no_switch])` | Calculates declining-balance depreciation over a specified period range. |
| XIRR | `XIRR(values, dates, [guess])` | Estimates the internal rate of return for cash flows on irregular dates. |
| XNPV | `XNPV(rate, values, dates)` | Returns the net present value for cash flows on irregular dates. |

## Information

Microsoft reference: [Information functions](https://support.microsoft.com/en-us/excel/information-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| CELL | `CELL(info_type, [reference])` | Returns information about a cell, such as its address, format or contents. |
| ERROR.TYPE | `ERROR.TYPE(error_val)` | Returns the number corresponding to an Excel error value. |
| INFO | `INFO(type_text)` | Returns information about the current operating environment. |
| ISBLANK | `ISBLANK(value)` | Returns TRUE if the value refers to a blank cell. |
| ISERR | `ISERR(value)` | Returns TRUE for an error other than `#N/A`. |
| ISERROR | `ISERROR(value)` | Returns TRUE if the value is any error. |
| ISEVEN | `ISEVEN(number)` | Returns TRUE if the integer portion of a number is even. |
| ISFORMULA | `ISFORMULA(reference)` | Returns TRUE if the reference contains a formula. |
| ISLOGICAL | `ISLOGICAL(value)` | Returns TRUE if the value is TRUE or FALSE. |
| ISNA | `ISNA(value)` | Returns TRUE if the value is `#N/A`. |
| ISNONTEXT | `ISNONTEXT(value)` | Returns TRUE if the value is not text. |
| ISNUMBER | `ISNUMBER(value)` | Returns TRUE if the value is numeric. |
| ISODD | `ISODD(number)` | Returns TRUE if the integer portion of a number is odd. |
| ISOMITTED | `ISOMITTED(argument)` | Returns TRUE if a parameter to a LAMBDA function was omitted. |
| ISREF | `ISREF(value)` | Returns TRUE if the value is a reference. |
| ISTEXT | `ISTEXT(value)` | Returns TRUE if the value is text. |
| N | `N(value)` | Converts a value to a number, with nonnumeric values generally becoming zero. |
| NA | `NA()` | Returns the `#N/A` error value. |
| SHEET | `SHEET([value])` | Returns the sheet number of a reference or named sheet. |
| SHEETS | `SHEETS([reference])` | Returns the number of sheets in a reference or workbook. |
| STOCKHISTORY | `STOCKHISTORY(stock, start_date, [end_date], [interval], [headers], [property0], ...)` | Online stock-history data function; ExcelLite returns `#CONNECT!`. |
| TYPE | `TYPE(value)` | Returns a number identifying the value's data type. |

## Logical

Microsoft reference: [Logical functions](https://support.microsoft.com/en-us/excel/logical-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| AND | `AND(logical1, [logical2], ...)` | Returns TRUE if every argument is TRUE. |
| BYCOL | `BYCOL(array, lambda)` | Applies a LAMBDA to each column and returns the results as an array. |
| BYROW | `BYROW(array, lambda)` | Applies a LAMBDA to each row and returns the results as an array. |
| FALSE | `FALSE()` | Returns the logical value FALSE. |
| IF | `IF(logical_test, [value_if_true], [value_if_false])` | Returns one value when a condition is TRUE and another when it is FALSE. |
| IFERROR | `IFERROR(value, value_if_error)` | Returns a fallback value if the first expression returns an error. |
| IFNA | `IFNA(value, value_if_na)` | Returns a fallback value if the first expression returns `#N/A`. |
| IFS | `IFS(logical_test1, value_if_true1, [logical_test2, value_if_true2], ...)` | Checks conditions in order and returns the value for the first TRUE condition. |
| LAMBDA | `LAMBDA([parameter1, ...], calculation)` | Defines a reusable custom function from a calculation and optional parameters. |
| LET | `LET(name1, name_value1, [name2, name_value2], ..., calculation)` | Assigns names to values or calculations within a formula. |
| MAKEARRAY | `MAKEARRAY(rows, cols, lambda)` | Creates an array by calculating each element with a LAMBDA. |
| MAP | `MAP(array1, [array2], ..., lambda)` | Maps one or more arrays element-by-element through a LAMBDA. |
| NOT | `NOT(logical)` | Reverses TRUE to FALSE or FALSE to TRUE. |
| OR | `OR(logical1, [logical2], ...)` | Returns TRUE if any argument is TRUE. |
| REDUCE | `REDUCE([initial_value], array, lambda)` | Accumulates an array into a single value using a LAMBDA. |
| SCAN | `SCAN([initial_value], array, lambda)` | Returns the intermediate accumulated values from a LAMBDA scan. |
| SWITCH | `SWITCH(expression, value1, result1, [value2, result2], ..., [default])` | Matches an expression against values and returns the corresponding result. |
| TRUE | `TRUE()` | Returns the logical value TRUE. |
| XOR | `XOR(logical1, [logical2], ...)` | Returns TRUE when an odd number of arguments are TRUE. |

## Lookup and reference

Microsoft reference: [Lookup and reference functions](https://support.microsoft.com/en-us/excel/lookup-and-reference-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| ADDRESS | `ADDRESS(row_num, column_num, [abs_num], [a1], [sheet_text])` | Returns a cell address as text. |
| AREAS | `AREAS(reference)` | Returns the number of areas in a reference. |
| CHOOSE | `CHOOSE(index_num, value1, [value2], ...)` | Selects a value from a list by its index. |
| CHOOSECOLS | `CHOOSECOLS(array, col_num1, [col_num2], ...)` | Returns selected columns from an array. |
| CHOOSEROWS | `CHOOSEROWS(array, row_num1, [row_num2], ...)` | Returns selected rows from an array. |
| COLUMN | `COLUMN([reference])` | Returns the column number of a reference. |
| COLUMNS | `COLUMNS(array)` | Returns the number of columns in an array or reference. |
| DROP | `DROP(array, rows, [columns])` | Removes a specified number of rows or columns from an array's edge. |
| EXPAND | `EXPAND(array, rows, [columns], [pad_with])` | Expands an array to the requested dimensions, optionally padding it. |
| FILTER | `FILTER(array, include, [if_empty])` | Returns rows or columns whose corresponding include values are TRUE. |
| FORMULATEXT | `FORMULATEXT(reference)` | Returns a cell's formula as text. |
| GETPIVOTDATA | `GETPIVOTDATA(data_field, pivot_table, [field1, item1], ...)` | Extracts summarized data from a PivotTable. |
| GROUPBY | `GROUPBY(row_fields, values, function, [field_headers], [total_depth], [sort_order], [filter_array], [field_relationship])` | Groups data by row fields and aggregates the associated values. |
| HLOOKUP | `HLOOKUP(lookup_value, table_array, row_index_num, [range_lookup])` | Searches the first row of a table and returns a value from a specified row. |
| HSTACK | `HSTACK(array1, [array2], ...)` | Appends arrays horizontally. |
| HYPERLINK | `HYPERLINK(link_location, [friendly_name])` | Creates a clickable link in a cell. |
| INDEX | `INDEX(array, row_num, [column_num], [area_num])` | Returns a value or reference at a row and column intersection. |
| INDIRECT | `INDIRECT(ref_text, [a1])` | Converts reference text into a reference. |
| LOOKUP | `LOOKUP(lookup_value, lookup_vector, [result_vector])` | Searches a vector and returns a corresponding value. |
| MATCH | `MATCH(lookup_value, lookup_array, [match_type])` | Returns the relative position of a matching value in an array. |
| OFFSET | `OFFSET(reference, rows, cols, [height], [width])` | Returns a reference offset from a starting reference. |
| PIVOTBY | `PIVOTBY(row_fields, col_fields, values, function, [field_headers], [row_total_depth], [row_sort_order], [col_total_depth], [col_sort_order], [filter_array], [relative_to])` | Groups and summarizes values by row and column fields. |
| ROW | `ROW([reference])` | Returns the row number of a reference. |
| ROWS | `ROWS(array)` | Returns the number of rows in an array or reference. |
| SORT | `SORT(array, [sort_index], [sort_order], [by_col])` | Sorts the rows or columns of an array. |
| SORTBY | `SORTBY(array, by_array1, [sort_order1], ...)` | Sorts an array by corresponding values in one or more arrays. |
| TAKE | `TAKE(array, rows, [columns])` | Returns a specified number of rows or columns from an array's edge. |
| TOCOL | `TOCOL(array, [ignore], [scan_by_column])` | Returns an array as a single column. |
| TOROW | `TOROW(array, [ignore], [scan_by_column])` | Returns an array as a single row. |
| TRANSPOSE | `TRANSPOSE(array)` | Switches an array's rows and columns. |
| TRIMRANGE | `TRIMRANGE(range, [trim_rows], [trim_cols])` | Removes blank outer rows or columns from a range. |
| UNIQUE | `UNIQUE(array, [by_col], [exactly_once])` | Returns the distinct rows or columns in an array. |
| VLOOKUP | `VLOOKUP(lookup_value, table_array, col_index_num, [range_lookup])` | Searches the first column of a table and returns a value from a specified column. |
| VSTACK | `VSTACK(array1, [array2], ...)` | Appends arrays vertically. |
| WRAPCOLS | `WRAPCOLS(vector, wrap_count, [pad_with])` | Wraps a vector into columns of a specified height. |
| WRAPROWS | `WRAPROWS(vector, wrap_count, [pad_with])` | Wraps a vector into rows of a specified width. |
| XLOOKUP | `XLOOKUP(lookup_value, lookup_array, return_array, [if_not_found], [match_mode], [search_mode])` | Searches an array and returns the corresponding value from another array. |
| XMATCH | `XMATCH(lookup_value, lookup_array, [match_mode], [search_mode])` | Returns the relative position of an item in an array. |

## Math and trigonometry

Microsoft reference: [Excel functions by category](https://support.microsoft.com/en-us/excel/excel-functions-by-category).

| Function | Syntax | Description |
|---|---|---|
| ABS | `ABS(number)` | Returns the absolute value of a number. |
| ACOS | `ACOS(number)` | Returns the arccosine of a number, in radians. |
| ACOSH | `ACOSH(number)` | Returns the inverse hyperbolic cosine. |
| ACOT | `ACOT(number)` | Returns the arccotangent of a number, in radians. |
| ACOTH | `ACOTH(number)` | Returns the inverse hyperbolic cotangent. |
| AGGREGATE | `AGGREGATE(function_num, options, ref1, [ref2], ...)` | Applies an aggregate function with options to ignore hidden rows, errors or nested subtotals. |
| ARABIC | `ARABIC(text)` | Converts a Roman numeral to an Arabic number. |
| ASIN | `ASIN(number)` | Returns the arcsine of a number, in radians. |
| ASINH | `ASINH(number)` | Returns the inverse hyperbolic sine. |
| ATAN | `ATAN(number)` | Returns the arctangent of a number, in radians. |
| ATAN2 | `ATAN2(x_num, y_num)` | Returns the arctangent from x- and y-coordinates. |
| ATANH | `ATANH(number)` | Returns the inverse hyperbolic tangent. |
| BASE | `BASE(number, radix, [min_length])` | Converts a number to text in a specified base. |
| CEILING | `CEILING(number, significance)` | Rounds a number up to a multiple of significance. |
| CEILING.MATH | `CEILING.MATH(number, [significance], [mode])` | Rounds a number up, with options for negative values. |
| CEILING.PRECISE | `CEILING.PRECISE(number, [significance])` | Rounds a number up to a multiple of significance. |
| COMBIN | `COMBIN(number, number_chosen)` | Returns the number of combinations for a set size. |
| COMBINA | `COMBINA(number, number_chosen)` | Returns combinations with repetitions. |
| COS | `COS(number)` | Returns the cosine of an angle in radians. |
| COSH | `COSH(number)` | Returns the hyperbolic cosine. |
| COT | `COT(number)` | Returns the cotangent of an angle in radians. |
| COTH | `COTH(number)` | Returns the hyperbolic cotangent. |
| CSC | `CSC(number)` | Returns the cosecant of an angle in radians. |
| CSCH | `CSCH(number)` | Returns the hyperbolic cosecant. |
| DECIMAL | `DECIMAL(text, radix)` | Converts a number represented as text in a base to decimal. |
| DEGREES | `DEGREES(angle)` | Converts radians to degrees. |
| EVEN | `EVEN(number)` | Rounds a number away from zero to the next even integer. |
| EXP | `EXP(number)` | Returns e raised to a number's power. |
| FACT | `FACT(number)` | Returns the factorial of a number. |
| FACTDOUBLE | `FACTDOUBLE(number)` | Returns the double factorial of a number. |
| FLOOR | `FLOOR(number, significance)` | Rounds a number down toward zero to a multiple of significance. |
| FLOOR.MATH | `FLOOR.MATH(number, [significance], [mode])` | Rounds a number down, with options for negative values. |
| FLOOR.PRECISE | `FLOOR.PRECISE(number, [significance])` | Rounds a number down to a multiple of significance. |
| GCD | `GCD(number1, [number2], ...)` | Returns the greatest common divisor of integers. |
| INT | `INT(number)` | Rounds a number down to the nearest integer. |
| ISO.CEILING | `ISO.CEILING(number, [significance])` | Rounds a number up to the nearest integer or significance multiple. |
| LCM | `LCM(number1, [number2], ...)` | Returns the least common multiple of integers. |
| LN | `LN(number)` | Returns the natural logarithm of a number. |
| LOG | `LOG(number, [base])` | Returns the logarithm of a number for a specified base. |
| LOG10 | `LOG10(number)` | Returns the base-10 logarithm. |
| MDETERM | `MDETERM(array)` | Returns the determinant of a matrix. |
| MINVERSE | `MINVERSE(array)` | Returns the inverse of a square matrix. |
| MMULT | `MMULT(array1, array2)` | Returns the matrix product of two arrays. |
| MOD | `MOD(number, divisor)` | Returns the remainder after division. |
| MROUND | `MROUND(number, multiple)` | Rounds a number to the nearest multiple. |
| MULTINOMIAL | `MULTINOMIAL(number1, [number2], ...)` | Returns the multinomial of a set of numbers. |
| MUNIT | `MUNIT(dimension)` | Returns an identity matrix of the requested dimension. |
| ODD | `ODD(number)` | Rounds a number away from zero to the next odd integer. |
| PERCENTOF | `PERCENTOF(data_subset, data_all)` | Returns the proportion that one value or array contributes to another. |
| PI | `PI()` | Returns the value of pi. |
| POWER | `POWER(number, power)` | Raises a number to a power. |
| PRODUCT | `PRODUCT(number1, [number2], ...)` | Multiplies its arguments. |
| QUOTIENT | `QUOTIENT(numerator, denominator)` | Returns the integer portion of a division. |
| RADIANS | `RADIANS(angle)` | Converts degrees to radians. |
| RAND | `RAND()` | Returns a random number greater than or equal to zero and less than one. |
| RANDARRAY | `RANDARRAY([rows], [columns], [min], [max], [whole_number])` | Returns an array of random numbers. |
| RANDBETWEEN | `RANDBETWEEN(bottom, top)` | Returns a random integer between two bounds. |
| ROMAN | `ROMAN(number, [form])` | Converts an Arabic number to Roman numeral text. |
| ROUND | `ROUND(number, num_digits)` | Rounds a number to a specified number of digits. |
| ROUNDDOWN | `ROUNDDOWN(number, num_digits)` | Rounds a number toward zero. |
| ROUNDUP | `ROUNDUP(number, num_digits)` | Rounds a number away from zero. |
| SEC | `SEC(number)` | Returns the secant of an angle in radians. |
| SECH | `SECH(number)` | Returns the hyperbolic secant. |
| SEQUENCE | `SEQUENCE(rows, [columns], [start], [step])` | Generates a sequential array of numbers. |
| SERIESSUM | `SERIESSUM(x, n, m, coefficients)` | Returns the sum of a power series. |
| SIGN | `SIGN(number)` | Returns -1, 0 or 1 according to a number's sign. |
| SIN | `SIN(number)` | Returns the sine of an angle in radians. |
| SINH | `SINH(number)` | Returns the hyperbolic sine. |
| SQRT | `SQRT(number)` | Returns the positive square root. |
| SQRTPI | `SQRTPI(number)` | Returns the square root of a number multiplied by pi. |
| SUBTOTAL | `SUBTOTAL(function_num, ref1, [ref2], ...)` | Calculates a subtotal, optionally excluding manually hidden rows. |
| SUM | `SUM(number1, [number2], ...)` | Adds its arguments. |
| SUMIF | `SUMIF(range, criteria, [sum_range])` | Adds values that meet one criterion. |
| SUMIFS | `SUMIFS(sum_range, criteria_range1, criteria1, ...)` | Adds values that meet multiple criteria. |
| SUMPRODUCT | `SUMPRODUCT(array1, [array2], ...)` | Multiplies corresponding array elements and sums the products. |
| SUMSQ | `SUMSQ(number1, [number2], ...)` | Returns the sum of the squares of its arguments. |
| SUMX2MY2 | `SUMX2MY2(array_x, array_y)` | Sums the differences between paired squares. |
| SUMX2PY2 | `SUMX2PY2(array_x, array_y)` | Sums the sums of paired squares. |
| SUMXMY2 | `SUMXMY2(array_x, array_y)` | Sums the squares of differences between paired values. |
| TAN | `TAN(number)` | Returns the tangent of an angle in radians. |
| TANH | `TANH(number)` | Returns the hyperbolic tangent. |
| TRUNC | `TRUNC(number, [num_digits])` | Truncates a number to an integer or specified precision. |

## Statistical

Microsoft reference: [Statistical functions](https://support.microsoft.com/en-us/excel/statistical-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| AVEDEV | `AVEDEV(number1, [number2], ...)` | Returns the average of absolute deviations from the mean. |
| AVERAGE | `AVERAGE(number1, [number2], ...)` | Returns the arithmetic mean of numeric values. |
| AVERAGEA | `AVERAGEA(value1, [value2], ...)` | Averages values, including text and logical values under Excel's A rules. |
| AVERAGEIF | `AVERAGEIF(range, criteria, [average_range])` | Averages values that meet one criterion. |
| AVERAGEIFS | `AVERAGEIFS(average_range, criteria_range1, criteria1, ...)` | Averages values that meet multiple criteria. |
| COUNT | `COUNT(value1, [value2], ...)` | Counts numeric values. |
| COUNTA | `COUNTA(value1, [value2], ...)` | Counts nonblank values. |
| COUNTBLANK | `COUNTBLANK(range)` | Counts blank cells in a range. |
| COUNTIF | `COUNTIF(range, criteria)` | Counts cells that meet one criterion. |
| COUNTIFS | `COUNTIFS(criteria_range1, criteria1, ...)` | Counts rows or cells that meet multiple criteria. |
| CORREL | `CORREL(array1, array2)` | Returns the correlation coefficient of two data sets. |
| COVAR | `COVAR(array1, array2)` | Returns population covariance. |
| COVARIANCE.P | `COVARIANCE.P(array1, array2)` | Returns population covariance. |
| COVARIANCE.S | `COVARIANCE.S(array1, array2)` | Returns sample covariance. |
| CRITBINOM | `CRITBINOM(trials, probability_s, alpha)` | Legacy name for the inverse binomial distribution. |
| DEVSQ | `DEVSQ(number1, [number2], ...)` | Returns the sum of squared deviations from the sample mean. |
| EXPON.DIST | `EXPON.DIST(x, lambda, cumulative)` | Returns the exponential distribution. |
| EXPONDIST | `EXPONDIST(x, lambda, cumulative)` | Legacy name for the exponential distribution. |
| F.DIST | `F.DIST(x, deg_freedom1, deg_freedom2, cumulative)` | Returns the left-tailed F probability distribution. |
| F.DIST.RT | `F.DIST.RT(x, deg_freedom1, deg_freedom2)` | Returns the right-tailed F probability distribution. |
| F.INV | `F.INV(probability, deg_freedom1, deg_freedom2)` | Returns the inverse left-tailed F distribution. |
| F.INV.RT | `F.INV.RT(probability, deg_freedom1, deg_freedom2)` | Returns the inverse right-tailed F distribution. |
| F.TEST | `F.TEST(array1, array2)` | Returns the result of an F-test for two data sets. |
| FDIST | `FDIST(x, deg_freedom1, deg_freedom2)` | Legacy name for the right-tailed F distribution. |
| FINV | `FINV(probability, deg_freedom1, deg_freedom2)` | Legacy name for the inverse right-tailed F distribution. |
| FISHER | `FISHER(x)` | Returns the Fisher transformation of a value. |
| FISHERINV | `FISHERINV(y)` | Returns the inverse Fisher transformation. |
| FORECAST | `FORECAST(x, known_y's, known_x's)` | Predicts a value using linear regression; compatibility name for FORECAST.LINEAR. |
| FORECAST.LINEAR | `FORECAST.LINEAR(x, known_y's, known_x's)` | Predicts a value using linear regression. |
| FREQUENCY | `FREQUENCY(data_array, bins_array)` | Returns a frequency distribution as a vertical array. |
| FTEST | `FTEST(array1, array2)` | Legacy name for F.TEST. |
| GAMMA | `GAMMA(number)` | Returns the gamma function value. |
| GAMMA.DIST | `GAMMA.DIST(x, alpha, beta, cumulative)` | Returns the gamma distribution. |
| GAMMA.INV | `GAMMA.INV(probability, alpha, beta)` | Returns the inverse gamma cumulative distribution. |
| GAMMADIST | `GAMMADIST(x, alpha, beta, cumulative)` | Legacy name for the gamma distribution. |
| GAMMAINV | `GAMMAINV(probability, alpha, beta)` | Legacy name for the inverse gamma distribution. |
| GAMMALN | `GAMMALN(number)` | Returns the natural logarithm of the gamma function. |
| GAMMALN.PRECISE | `GAMMALN.PRECISE(number)` | Returns the natural logarithm of the gamma function. |
| GAUSS | `GAUSS(z)` | Returns the standard normal probability between zero and z. |
| GEOMEAN | `GEOMEAN(number1, [number2], ...)` | Returns the geometric mean of positive values. |
| GROWTH | `GROWTH(known_y's, [known_x's], [new_x's], [const])` | Predicts exponential growth from existing data. |
| HARMEAN | `HARMEAN(number1, [number2], ...)` | Returns the harmonic mean of positive values. |
| HYPGEOM.DIST | `HYPGEOM.DIST(sample_s, number_sample, population_s, number_pop, cumulative)` | Returns the hypergeometric distribution. |
| HYPGEOMDIST | `HYPGEOMDIST(sample_s, number_sample, population_s, number_pop)` | Legacy name for the noncumulative hypergeometric distribution. |
| INTERCEPT | `INTERCEPT(known_y's, known_x's)` | Returns the intercept of a linear regression line. |
| KURT | `KURT(number1, [number2], ...)` | Returns the kurtosis of a data set. |
| LARGE | `LARGE(array, k)` | Returns the k-th largest value. |
| LINEST | `LINEST(known_y's, [known_x's], [const], [stats])` | Calculates a linear regression and optionally returns regression statistics. |
| LOGEST | `LOGEST(known_y's, [known_x's], [const], [stats])` | Calculates an exponential regression and optionally returns statistics. |
| LOGINV | `LOGINV(probability, mean, standard_dev)` | Legacy name for the inverse lognormal distribution. |
| LOGNORM.DIST | `LOGNORM.DIST(x, mean, standard_dev, cumulative)` | Returns the lognormal distribution. |
| LOGNORM.INV | `LOGNORM.INV(probability, mean, standard_dev)` | Returns the inverse lognormal distribution. |
| LOGNORMDIST | `LOGNORMDIST(x, mean, standard_dev)` | Legacy name for the cumulative lognormal distribution. |
| MAX | `MAX(number1, [number2], ...)` | Returns the largest numeric value. |
| MAXA | `MAXA(value1, [value2], ...)` | Returns the largest value, including text and logical values under Excel's A rules. |
| MAXIFS | `MAXIFS(max_range, criteria_range1, criteria1, ...)` | Returns the largest value that meets multiple criteria. |
| MEDIAN | `MEDIAN(number1, [number2], ...)` | Returns the median of numeric values. |
| MIN | `MIN(number1, [number2], ...)` | Returns the smallest numeric value. |
| MINA | `MINA(value1, [value2], ...)` | Returns the smallest value, including text and logical values under Excel's A rules. |
| MINIFS | `MINIFS(min_range, criteria_range1, criteria1, ...)` | Returns the smallest value that meets multiple criteria. |
| MODE | `MODE(number1, [number2], ...)` | Legacy name for the most frequently occurring value. |
| MODE.MULT | `MODE.MULT(number1, [number2], ...)` | Returns an array of the most frequently occurring values. |
| MODE.SNGL | `MODE.SNGL(number1, [number2], ...)` | Returns the most frequently occurring value. |
| NEGBINOM.DIST | `NEGBINOM.DIST(number_f, number_s, probability_s, cumulative)` | Returns the negative binomial distribution. |
| NEGBINOMDIST | `NEGBINOMDIST(number_f, number_s, probability_s)` | Legacy name for the noncumulative negative binomial distribution. |
| NORM.DIST | `NORM.DIST(x, mean, standard_dev, cumulative)` | Returns the normal distribution. |
| NORM.INV | `NORM.INV(probability, mean, standard_dev)` | Returns the inverse normal cumulative distribution. |
| NORM.S.DIST | `NORM.S.DIST(z, cumulative)` | Returns the standard normal distribution. |
| NORM.S.INV | `NORM.S.INV(probability)` | Returns the inverse standard normal cumulative distribution. |
| NORMDIST | `NORMDIST(x, mean, standard_dev, cumulative)` | Legacy name for the normal distribution. |
| NORMINV | `NORMINV(probability, mean, standard_dev)` | Legacy name for the inverse normal distribution. |
| NORMSDIST | `NORMSDIST(z)` | Legacy name for the standard normal cumulative distribution. |
| NORMSINV | `NORMSINV(probability)` | Legacy name for the inverse standard normal cumulative distribution. |
| PERCENTILE | `PERCENTILE(array, k)` | Legacy name for the inclusive k-th percentile. |
| PERCENTILE.EXC | `PERCENTILE.EXC(array, k)` | Returns the exclusive k-th percentile. |
| PERCENTILE.INC | `PERCENTILE.INC(array, k)` | Returns the inclusive k-th percentile. |
| PERCENTRANK | `PERCENTRANK(array, x, [significance])` | Legacy name for the inclusive rank of a value as a percentage. |
| PERCENTRANK.EXC | `PERCENTRANK.EXC(array, x, [significance])` | Returns the exclusive percentage rank of a value. |
| PERCENTRANK.INC | `PERCENTRANK.INC(array, x, [significance])` | Returns the inclusive percentage rank of a value. |
| PEARSON | `PEARSON(array1, array2)` | Returns the Pearson correlation coefficient. |
| PERMUT | `PERMUT(number, number_chosen)` | Returns the number of permutations without repetition. |
| PERMUTATIONA | `PERMUTATIONA(number, number_chosen)` | Returns the number of permutations with repetition. |
| PHI | `PHI(x)` | Returns the standard normal density for x. |
| POISSON | `POISSON(x, mean, cumulative)` | Legacy name for the Poisson distribution. |
| POISSON.DIST | `POISSON.DIST(x, mean, cumulative)` | Returns the Poisson distribution. |
| PROB | `PROB(x_range, prob_range, lower_limit, [upper_limit])` | Returns the probability that values fall within a specified range. |
| QUARTILE | `QUARTILE(array, quart)` | Legacy name for an inclusive quartile. |
| QUARTILE.EXC | `QUARTILE.EXC(array, quart)` | Returns an exclusive quartile. |
| QUARTILE.INC | `QUARTILE.INC(array, quart)` | Returns an inclusive quartile. |
| RANK | `RANK(number, ref, [order])` | Legacy name for ranking a number in a list. |
| RANK.AVG | `RANK.AVG(number, ref, [order])` | Returns a number's rank, averaging ties. |
| RANK.EQ | `RANK.EQ(number, ref, [order])` | Returns a number's rank, assigning tied values the same rank. |
| RSQ | `RSQ(known_y's, known_x's)` | Returns the square of the Pearson correlation coefficient. |
| SKEW | `SKEW(number1, [number2], ...)` | Estimates the skewness of a sample. |
| SKEW.P | `SKEW.P(number1, [number2], ...)` | Returns the population skewness. |
| SLOPE | `SLOPE(known_y's, known_x's)` | Returns the slope of a linear regression line. |
| SMALL | `SMALL(array, k)` | Returns the k-th smallest value. |
| STANDARDIZE | `STANDARDIZE(x, mean, standard_dev)` | Returns a normalized value from a distribution. |
| STDEV | `STDEV(number1, [number2], ...)` | Legacy name for the sample standard deviation. |
| STDEV.P | `STDEV.P(number1, [number2], ...)` | Calculates population standard deviation. |
| STDEV.S | `STDEV.S(number1, [number2], ...)` | Estimates sample standard deviation. |
| STDEVA | `STDEVA(value1, [value2], ...)` | Estimates sample standard deviation, including text and logical values. |
| STDEVP | `STDEVP(number1, [number2], ...)` | Legacy name for population standard deviation. |
| STDEVPA | `STDEVPA(value1, [value2], ...)` | Calculates population standard deviation, including text and logical values. |
| STEYX | `STEYX(known_y's, known_x's)` | Returns the standard error of the predicted y-value for regression. |
| T.DIST | `T.DIST(x, deg_freedom, cumulative)` | Returns the left-tailed Student's t distribution. |
| T.DIST.2T | `T.DIST.2T(x, deg_freedom)` | Returns the two-tailed Student's t distribution. |
| T.DIST.RT | `T.DIST.RT(x, deg_freedom)` | Returns the right-tailed Student's t distribution. |
| T.INV | `T.INV(probability, deg_freedom)` | Returns the inverse left-tailed Student's t distribution. |
| T.INV.2T | `T.INV.2T(probability, deg_freedom)` | Returns the inverse two-tailed Student's t distribution. |
| T.TEST | `T.TEST(array1, array2, tails, type)` | Returns the probability associated with a Student's t-test. |
| TDIST | `TDIST(x, deg_freedom, tails)` | Legacy name for a Student's t distribution. |
| TINV | `TINV(probability, deg_freedom)` | Legacy name for the inverse two-tailed Student's t distribution. |
| TREND | `TREND(known_y's, [known_x's], [new_x's], [const])` | Returns values along a linear trend. |
| TRIMMEAN | `TRIMMEAN(array, percent)` | Returns a mean excluding a percentage of extreme values. |
| TTEST | `TTEST(array1, array2, tails, type)` | Legacy name for T.TEST. |
| VAR | `VAR(number1, [number2], ...)` | Legacy name for sample variance. |
| VAR.P | `VAR.P(number1, [number2], ...)` | Calculates population variance. |
| VAR.S | `VAR.S(number1, [number2], ...)` | Estimates sample variance. |
| VARA | `VARA(value1, [value2], ...)` | Estimates sample variance, including text and logical values. |
| VARP | `VARP(number1, [number2], ...)` | Legacy name for population variance. |
| VARPA | `VARPA(value1, [value2], ...)` | Calculates population variance, including text and logical values. |
| WEIBULL | `WEIBULL(x, alpha, beta, cumulative)` | Legacy name for the Weibull distribution. |
| WEIBULL.DIST | `WEIBULL.DIST(x, alpha, beta, cumulative)` | Returns the Weibull distribution. |
| Z.TEST | `Z.TEST(array, x, [sigma])` | Returns the one-tailed probability value of a z-test. |
| ZTEST | `ZTEST(array, x, [sigma])` | Legacy name for Z.TEST. |

### Additional distribution and test functions

| Function | Syntax | Description |
|---|---|---|
| BETA.DIST | `BETA.DIST(x, alpha, beta, cumulative, [A], [B])` | Returns the beta probability distribution. |
| BETA.INV | `BETA.INV(probability, alpha, beta, [A], [B])` | Returns the inverse beta cumulative distribution. |
| BETADIST | `BETADIST(x, alpha, beta, [A], [B])` | Legacy name for the cumulative beta distribution. |
| BETAINV | `BETAINV(probability, alpha, beta, [A], [B])` | Legacy name for the inverse beta distribution. |
| BINOM.DIST | `BINOM.DIST(number_s, trials, probability_s, cumulative)` | Returns the binomial distribution. |
| BINOM.DIST.RANGE | `BINOM.DIST.RANGE(trials, probability_s, number_s, [number_s2])` | Returns the probability of successes within a specified range. |
| BINOM.INV | `BINOM.INV(trials, probability_s, alpha)` | Returns the smallest value whose cumulative binomial distribution meets alpha. |
| BINOMDIST | `BINOMDIST(number_s, trials, probability_s, cumulative)` | Legacy name for the binomial distribution. |
| CHIDIST | `CHIDIST(x, deg_freedom)` | Legacy name for the right-tailed chi-squared distribution. |
| CHIINV | `CHIINV(probability, deg_freedom)` | Legacy name for the inverse right-tailed chi-squared distribution. |
| CHISQ.DIST | `CHISQ.DIST(x, deg_freedom, cumulative)` | Returns the chi-squared distribution. |
| CHISQ.DIST.RT | `CHISQ.DIST.RT(x, deg_freedom)` | Returns the right-tailed chi-squared distribution. |
| CHISQ.INV | `CHISQ.INV(probability, deg_freedom)` | Returns the inverse left-tailed chi-squared distribution. |
| CHISQ.INV.RT | `CHISQ.INV.RT(probability, deg_freedom)` | Returns the inverse right-tailed chi-squared distribution. |
| CHISQ.TEST | `CHISQ.TEST(actual_range, expected_range)` | Returns the result of a chi-squared test. |
| CHITEST | `CHITEST(actual_range, expected_range)` | Legacy name for CHISQ.TEST. |
| CONFIDENCE | `CONFIDENCE(alpha, standard_dev, size)` | Legacy name for the normal confidence interval width. |
| CONFIDENCE.NORM | `CONFIDENCE.NORM(alpha, standard_dev, size)` | Returns the confidence interval width using a normal distribution. |
| CONFIDENCE.T | `CONFIDENCE.T(alpha, standard_dev, size)` | Returns the confidence interval width using a Student's t distribution. |

The statistical table includes older Excel function names for
compatibility. `FORECAST.ETS` and its related exponential-smoothing
functions, and several other Excel functions, are not implemented; see
[Not yet implemented](#not-yet-implemented).

## Text

Microsoft reference: [Text functions](https://support.microsoft.com/en-us/excel/text-functions-reference).

| Function | Syntax | Description |
|---|---|---|
| ARRAYTOTEXT | `ARRAYTOTEXT(array, [format])` | Returns an array's contents as text. |
| ASC | `ASC(text)` | Converts full-width characters to half-width characters. |
| BAHTTEXT | `BAHTTEXT(number)` | Converts a number to Thai text with a baht suffix. |
| CHAR | `CHAR(number)` | Returns the character for a numeric code. |
| CLEAN | `CLEAN(text)` | Removes nonprinting characters from text. |
| CODE | `CODE(text)` | Returns the numeric code of the first character. |
| CONCAT | `CONCAT(text1, [text2], ...)` | Joins text from ranges or strings without a delimiter. |
| CONCATENATE | `CONCATENATE(text1, [text2], ...)` | Joins text strings. |
| DBCS | `DBCS(text)` | Converts half-width characters to full-width characters. |
| DETECTLANGUAGE | `DETECTLANGUAGE(text)` | Online language-detection function; ExcelLite returns `#CONNECT!`. |
| DOLLAR | `DOLLAR(number, [decimals])` | Converts a number to currency-formatted text. |
| EXACT | `EXACT(text1, text2)` | Tests whether two text strings are identical, including case. |
| FIND | `FIND(find_text, within_text, [start_num])` | Finds case-sensitive text and returns its character position. |
| FINDB | `FINDB(find_text, within_text, [start_num])` | Byte-counting variant of FIND; behaves like FIND in ExcelLite. |
| FIXED | `FIXED(number, [decimals], [no_commas])` | Formats a number as text with fixed decimals. |
| LEFT | `LEFT(text, [num_chars])` | Returns characters from the start of text. |
| LEFTB | `LEFTB(text, [num_bytes])` | Byte-counting variant of LEFT; behaves like LEFT in ExcelLite. |
| LEN | `LEN(text)` | Returns the number of characters in text. |
| LENB | `LENB(text)` | Byte-counting variant of LEN; behaves like LEN in ExcelLite. |
| LOWER | `LOWER(text)` | Converts text to lowercase. |
| MID | `MID(text, start_num, num_chars)` | Returns characters from a specified position. |
| MIDB | `MIDB(text, start_num, num_bytes)` | Byte-counting variant of MID; behaves like MID in ExcelLite. |
| NUMBERVALUE | `NUMBERVALUE(text, [decimal_separator], [group_separator])` | Converts text to a number using specified separators. |
| PHONETIC | `PHONETIC(reference)` | Extracts phonetic characters from a reference. |
| PROPER | `PROPER(text)` | Capitalizes the first letter of each word. |
| REGEXEXTRACT | `REGEXEXTRACT(text, pattern, [return_mode], [case_sensitivity])` | Extracts text matching a regular expression. |
| REGEXREPLACE | `REGEXREPLACE(text, pattern, replacement, [occurrence], [case_sensitivity])` | Replaces text matching a regular expression. |
| REGEXTEST | `REGEXTEST(text, pattern, [case_sensitivity])` | Tests whether text matches a regular expression. |
| REPLACE | `REPLACE(old_text, start_num, num_chars, new_text)` | Replaces characters at a specified position. |
| REPLACEB | `REPLACEB(old_text, start_num, num_bytes, new_text)` | Byte-counting variant of REPLACE; behaves like REPLACE in ExcelLite. |
| REPT | `REPT(text, number_times)` | Repeats text a specified number of times. |
| RIGHT | `RIGHT(text, [num_chars])` | Returns characters from the end of text. |
| RIGHTB | `RIGHTB(text, [num_bytes])` | Byte-counting variant of RIGHT; behaves like RIGHT in ExcelLite. |
| SEARCH | `SEARCH(find_text, within_text, [start_num])` | Finds text without case sensitivity and supports wildcards. |
| SEARCHB | `SEARCHB(find_text, within_text, [start_num])` | Byte-counting variant of SEARCH; behaves like SEARCH in ExcelLite. |
| SUBSTITUTE | `SUBSTITUTE(text, old_text, new_text, [instance_num])` | Replaces matching text, optionally only a selected occurrence. |
| T | `T(value)` | Returns a value if it is text; otherwise returns empty text. |
| TEXT | `TEXT(value, format_text)` | Formats a value and returns the result as text. |
| TEXTAFTER | `TEXTAFTER(text, delimiter, [instance_num], [match_mode], [match_end], [if_not_found])` | Returns text after a delimiter. |
| TEXTBEFORE | `TEXTBEFORE(text, delimiter, [instance_num], [match_mode], [match_end], [if_not_found])` | Returns text before a delimiter. |
| TEXTJOIN | `TEXTJOIN(delimiter, ignore_empty, text1, [text2], ...)` | Joins text values using a delimiter, optionally skipping empty values. |
| TEXTSPLIT | `TEXTSPLIT(text, col_delimiter, [row_delimiter], [ignore_empty], [match_mode], [pad_with])` | Splits text into rows or columns using delimiters. |
| TRANSLATE | `TRANSLATE(text, [source_language], [target_language])` | Online translation function; ExcelLite returns `#CONNECT!`. |
| TRIM | `TRIM(text)` | Removes excess spaces, leaving single spaces between words. |
| UNICHAR | `UNICHAR(number)` | Returns the Unicode character for a code point. |
| UNICODE | `UNICODE(text)` | Returns the code point of the first character. |
| UPPER | `UPPER(text)` | Converts text to uppercase. |
| VALUE | `VALUE(text)` | Converts recognized numeric text to a number. |
| VALUETOTEXT | `VALUETOTEXT(value, [format])` | Converts a value to text. |

In the `B` variants (such as `LENB` and `LEFTB`), ExcelLite counts
characters like the corresponding non-`B` function. Regular expressions
use Go's RE2 syntax; PCRE-only constructs are unavailable.

## Not yet implemented

The function list above reflects the current calculation engine. Missing
functions include `FORECAST.ETS` and its relatives, exponential smoothing,
bond and coupon functions such as `PRICE`, `YIELD`, `COUPNCD`, `ACCRINT` and
`DURATION`, and engineering functions beyond base conversions and
`CONVERT`, such as `BESSELJ`, `ERF` and the `IM*` complex-number functions.
`STOCKHISTORY`, `TRANSLATE` and `DETECTLANGUAGE` require Microsoft's online
services and return `#CONNECT!` in ExcelLite. See
[ExcelLite limitations](./EXCEL.md#limitations-compared-with-excel) for
other formula limitations.
