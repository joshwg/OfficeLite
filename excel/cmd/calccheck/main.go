// Command calccheck recalculates every formula in workbooks and compares
// the results with the values Excel saved, to find calculation differences:
//
//	calccheck [-v] file.xlsx ...
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"officelite/excel/calc"
	"officelite/excel/fileio"
	"officelite/excel/formula"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

func main() {
	verbose := flag.Bool("v", false, "list every mismatch (default: first 20 per file)")
	flag.Parse()
	status := 0
	for _, path := range flag.Args() {
		if !check(path, *verbose) {
			status = 1
		}
	}
	os.Exit(status)
}

type saved struct {
	kind model.Kind
	num  float64
	text string
}

func check(path string, verbose bool) bool {
	wb, err := fileio.Open(path)
	if err != nil {
		fmt.Printf("%s: %v\n", path, err)
		return false
	}
	before := map[string]saved{}
	formulas := map[string]string{}
	for _, sh := range wb.Sheets {
		for r := 0; r < sh.UsedRows(); r++ {
			for c := 0; c < sh.RowLen(r); c++ {
				cell := sh.Get(r, c)
				if cell.Formula == "" || cell.Kind == model.KindEmpty {
					continue
				}
				ref := sh.Name + "!" + model.CellName(r, c)
				before[ref] = saved{cell.Kind, cell.Num, cell.Text}
				formulas[ref] = cell.Formula
			}
		}
	}
	start := time.Now()
	res := calc.Recalculate(wb, functions.Default(), calc.Full, time.Now())
	elapsed := time.Since(start)

	var bad []string
	unknown := map[string]int{}
	for _, sh := range wb.Sheets {
		for r := 0; r < sh.UsedRows(); r++ {
			for c := 0; c < sh.RowLen(r); c++ {
				ref := sh.Name + "!" + model.CellName(r, c)
				want, ok := before[ref]
				if !ok {
					continue
				}
				got := sh.Get(r, c)
				if same(want, got) {
					continue
				}
				if got.Kind == model.KindError && got.Text == string(formula.ErrName) {
					for _, fn := range unknownFunctions(formulas[ref]) {
						unknown[fn]++
					}
				}
				bad = append(bad, fmt.Sprintf("  %s =%s\n    Excel: %s   OfficeLite: %s", ref, formulas[ref], show(want), show(saved{got.Kind, got.Num, got.Text})))
			}
		}
	}
	fmt.Printf("%s: %d formulas, %d differ, %v", path, res.Calculated, len(bad), elapsed.Round(time.Millisecond))
	if len(res.Circular) > 0 {
		fmt.Printf(", circular: %v", res.Circular)
	}
	fmt.Println()
	if len(unknown) > 0 {
		names := make([]string, 0, len(unknown))
		for n := range unknown {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Printf("  functions not implemented: %v\n", names)
	}
	for i, b := range bad {
		if i == 20 && !verbose {
			fmt.Printf("  ... %d more (use -v)\n", len(bad)-20)
			break
		}
		fmt.Println(b)
	}
	return len(bad) == 0
}

// same compares a saved value with a recalculated cell: numbers within a
// relative 1e-9 (Excel keeps 15 significant digits), text exactly.
func same(want saved, got *model.Cell) bool {
	if want.kind != got.Kind {
		return false
	}
	if want.kind == model.KindNumber {
		return math.Abs(want.num-got.Num) <= 1e-9*math.Max(1, math.Abs(want.num))
	}
	return want.text == got.Text
}

func show(s saved) string {
	if s.kind == model.KindNumber {
		return fmt.Sprintf("%v (%q)", s.num, s.text)
	}
	return fmt.Sprintf("%q", s.text)
}

// unknownFunctions lists function names in a formula that the registry
// lacks. Text in "string literals" and 'sheet names' isn't scanned; a
// doubled quote inside one is an escaped quote and stays inside it.
func unknownFunctions(src string) []string {
	var out []string
	reg := functions.Default()
	var quote byte // the quote character of the literal being skipped
	for i := 0; i < len(src); i++ {
		if quote != 0 {
			if src[i] == quote {
				quote = 0 // a doubled quote reopens at the next character
			}
			continue
		}
		if src[i] == '"' || src[i] == '\'' {
			quote = src[i]
			continue
		}
		if src[i] != '(' {
			continue
		}
		j := i
		for j > 0 && (isNameChar(src[j-1])) {
			j--
		}
		if j < i {
			name := formula.FormulaName(src[j:i])
			if reg.Lookup(name) == nil && name != "LET" && name != "LAMBDA" {
				out = append(out, name)
			}
		}
	}
	return out
}

// isNameChar reports whether c can be part of a function name.
func isNameChar(c byte) bool {
	return c == '.' || c == '_' || c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}
