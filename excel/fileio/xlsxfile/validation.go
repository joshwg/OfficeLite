package xlsxfile

import (
	"encoding/xml"
	"strings"

	"github.com/xuri/excelize/v2"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// Data validation: <dataValidations> in the worksheet, and the
// <x14:dataValidations> Excel 2010+ puts in the sheet's extLst for rules
// whose formulas refer to other sheets.

var validTypes = []string{
	model.ValidAny: "none", model.ValidWhole: "whole", model.ValidDecimal: "decimal", model.ValidList: "list",
	model.ValidDate: "date", model.ValidTime: "time", model.ValidTextLength: "textLength", model.ValidCustom: "custom",
}

var validOps = []string{
	model.ValidBetween: "between", model.ValidNotBetween: "notBetween", model.ValidEqual: "equal",
	model.ValidNotEqual: "notEqual", model.ValidGreater: "greaterThan", model.ValidLess: "lessThan",
	model.ValidGreaterEqual: "greaterThanOrEqual", model.ValidLessEqual: "lessThanOrEqual",
}

var errorStyles = []string{model.ErrorStop: "stop", model.ErrorWarning: "warning", model.ErrorInformation: "information"}

// validationReader builds a sheet's rules from its elements as the
// worksheet is streamed.
type validationReader struct {
	cur  *model.Validation
	in   string // the element whose text is being read: formula1, formula2, sqref
	text strings.Builder
}

func index(names []string, s string) int {
	for i, n := range names {
		if n == s {
			return i
		}
	}
	return -1
}

// start handles a start element; false when it isn't part of a rule.
func (v *validationReader) start(t xml.StartElement) bool {
	if t.Name.Local == "dataValidation" {
		dv := &model.Validation{
			AllowBlank:  xmlBool(attr(t, "allowBlank")),
			NoDropDown:  xmlBool(attr(t, "showDropDown")),
			ShowInput:   xmlBool(attr(t, "showInputMessage")),
			ShowError:   xmlBool(attr(t, "showErrorMessage")),
			PromptTitle: attr(t, "promptTitle"), Prompt: attr(t, "prompt"),
			ErrorTitle: attr(t, "errorTitle"), Error: attr(t, "error"),
		}
		if i := index(validTypes, attr(t, "type")); i > 0 {
			dv.Type = model.ValidationType(i)
		}
		if i := index(validOps, attr(t, "operator")); i > 0 {
			dv.Op = model.ValidationOp(i)
		}
		if i := index(errorStyles, attr(t, "errorStyle")); i > 0 {
			dv.ErrorStyle = model.ErrorStyle(i)
		}
		dv.Ranges = parseSqref(attr(t, "sqref"))
		v.cur = dv
		return true
	}
	if v.cur == nil {
		return false
	}
	switch t.Name.Local {
	case "formula1", "formula2", "sqref":
		v.in = t.Name.Local
		v.text.Reset()
	}
	return true // xm:f and anything else inside the rule
}

// chars takes text inside a rule; false when there is no rule.
func (v *validationReader) chars(b []byte) bool {
	if v.cur == nil {
		return false
	}
	if v.in != "" {
		v.text.Write(b)
	}
	return true
}

// end handles an end element; false when it isn't part of a rule.
func (v *validationReader) end(t xml.EndElement, sh *model.Sheet) bool {
	if v.cur == nil {
		return false
	}
	switch t.Name.Local {
	case "formula1":
		v.cur.Formula1 = strings.TrimPrefix(strings.TrimSpace(v.text.String()), "=")
		v.in = ""
	case "formula2":
		v.cur.Formula2 = strings.TrimPrefix(strings.TrimSpace(v.text.String()), "=")
		v.in = ""
	case "sqref":
		v.cur.Ranges = parseSqref(v.text.String())
		v.in = ""
	case "dataValidation":
		if len(v.cur.Ranges) > 0 {
			sh.Validations = append(sh.Validations, *v.cur)
		}
		v.cur = nil
	}
	return true
}

// parseSqref reads a space-separated list of ranges, cut to the sheet's
// size limits.
func parseSqref(s string) []model.Range {
	var out []model.Range
	for _, part := range strings.Fields(s) {
		r, ok := parseRange(part)
		if !ok {
			row, col, ok := parseCellRef(part)
			if !ok {
				continue
			}
			r = model.Range{R1: row, C1: col, R2: row, C2: col}
		}
		r.R2, r.C2 = min(r.R2, model.MaxRows-1), min(r.C2, model.MaxCols-1)
		if r.R1 <= r.R2 && r.C1 <= r.C2 {
			out = append(out, r)
		}
	}
	return out
}

// sqref writes ranges as a space-separated list.
func sqref(rs []model.Range) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = model.CellName(r.R1, r.C1)
		if r.R1 != r.R2 || r.C1 != r.C2 {
			parts[i] += ":" + model.CellName(r.R2, r.C2)
		}
	}
	return strings.Join(parts, " ")
}

// writeValidations adds the sheet's rules. Like writeAnnotations, it runs
// before the sheet's StreamWriter is created.
func (x *writer) writeValidations(sh *model.Sheet, name string) error {
	for _, v := range sh.Validations {
		if len(v.Ranges) == 0 {
			continue
		}
		dv := &excelize.DataValidation{
			AllowBlank:       v.AllowBlank,
			ShowDropDown:     v.NoDropDown,
			ShowInputMessage: v.ShowInput,
			ShowErrorMessage: v.ShowError,
			Sqref:            sqref(v.Ranges),
			Type:             validTypes[v.Type],
		}
		switch v.Type {
		case model.ValidWhole, model.ValidDecimal, model.ValidDate, model.ValidTime, model.ValidTextLength:
			dv.Operator = validOps[v.Op]
		}
		str := func(s string) *string {
			if s == "" {
				return nil
			}
			return &s
		}
		dv.PromptTitle, dv.Prompt = str(v.PromptTitle), str(v.Prompt)
		dv.ErrorTitle, dv.Error = str(v.ErrorTitle), str(v.Error)
		if v.ErrorStyle != model.ErrorStop {
			dv.ErrorStyle = str(errorStyles[v.ErrorStyle])
		}
		// The formulas go in as raw XML.
		if v.Formula1 != "" {
			dv.Formula1 = xmlEscape(formula.AddFuturePrefixes(v.Formula1))
		}
		if v.Formula2 != "" && (v.Op == model.ValidBetween || v.Op == model.ValidNotBetween) {
			dv.Formula2 = xmlEscape(formula.AddFuturePrefixes(v.Formula2))
		}
		if err := x.f.AddDataValidation(name, dv); err != nil {
			return err
		}
	}
	return nil
}
