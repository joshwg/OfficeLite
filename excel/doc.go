// Package excel holds ExcelLite's limits (limits.go): the sheet sizes the
// model holds, the caps the file readers and writers apply, the formula
// engine's bounds, and the editor's, renderer's and window's caps and
// tunables, in one place. ExcelLite itself lives in the packages below it:
// model, formula, calc, fileio, edit, render and ui. This package imports
// no other excel/ package, so every one of them can use it.
package excel
