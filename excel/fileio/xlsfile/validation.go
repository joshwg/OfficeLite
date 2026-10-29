package xlsfile

import (
	"strings"

	"officelite/excel/model"
)

// Data validation: the DVAL record and a DV record per rule, after the
// sheet's hyperlinks.

const (
	recDVal = 0x01B2
	recDV   = 0x01BE
)

// DV flags.
const (
	dvStrLookup    = 1 << 7 // the List source is an explicit list (tStr, entries split by NUL)
	dvAllowBlank   = 1 << 8
	dvSuppressDrop = 1 << 9 // no in-cell drop-down
	dvShowInput    = 1 << 18
	dvShowError    = 1 << 19
)

// rawDV is a DV record read but not yet decoded: its formulas are
// decoded once the sheet's records are in.
type rawDV struct {
	v      model.Validation
	f1, f2 []byte
	lookup bool
}

// validationReader collects a sheet's DV records.
type validationReader struct {
	rules []rawDV
}

func (v *validationReader) record(r record) bool {
	switch r.id {
	case recDVal:
	case recDV:
		if dv, ok := parseDV(r.body); ok {
			v.rules = append(v.rules, dv)
		}
	default:
		return false
	}
	return true
}

// dvString reads one of DV's XLUnicodeStrings; Excel writes an empty
// one as a single NUL.
func dvString(b []byte, off int) (string, int) {
	if off+3 > len(b) {
		return "", len(b)
	}
	s, end := unicodeString(b, off, 2)
	if s == "\x00" {
		s = ""
	}
	return s, end
}

func parseDV(b []byte) (rawDV, bool) {
	if len(b) < 4 {
		return rawDV{}, false
	}
	flags := u32(b, 0)
	var dv rawDV
	v := &dv.v
	v.Type = model.ValidationType(flags & 0x0F)
	v.ErrorStyle = model.ErrorStyle(flags >> 4 & 0x07)
	v.Op = model.ValidationOp(flags >> 20 & 0x0F)
	v.AllowBlank = flags&dvAllowBlank != 0
	v.NoDropDown = flags&dvSuppressDrop != 0
	v.ShowInput = flags&dvShowInput != 0
	v.ShowError = flags&dvShowError != 0
	dv.lookup = flags&dvStrLookup != 0
	if v.Type > model.ValidCustom || v.ErrorStyle > model.ErrorInformation || v.Op > model.ValidLessEqual {
		return rawDV{}, false
	}
	off := 4
	v.PromptTitle, off = dvString(b, off)
	v.ErrorTitle, off = dvString(b, off)
	v.Prompt, off = dvString(b, off)
	v.Error, off = dvString(b, off)
	formula := func() []byte {
		if off+4 > len(b) {
			off = len(b)
			return nil
		}
		n := u16(b, off)
		off += 4
		end := min(off+n, len(b))
		f := b[off:end]
		off = end
		return f
	}
	dv.f1 = formula()
	dv.f2 = formula()
	if off+2 > len(b) {
		return rawDV{}, false
	}
	n := u16(b, off)
	off += 2
	for i := 0; i < n && off+8 <= len(b); i++ {
		r := model.Range{R1: u16(b, off), R2: u16(b, off+2), C1: u16(b, off+4), C2: u16(b, off+6)}
		off += 8
		if r.R1 <= r.R2 && r.C1 <= r.C2 {
			v.Ranges = append(v.Ranges, r)
		}
	}
	return dv, len(v.Ranges) > 0
}

// finishValidations decodes the formulas of the sheet's rules and adds
// them to sh.
func (p *parser) finishValidations(sh *model.Sheet, v *validationReader) {
	bad := 0
	for _, dv := range v.rules {
		rule := dv.v
		r0, c0 := rule.TopLeft()
		ok := true
		for i, rgce := range [][]byte{dv.f1, dv.f2} {
			if len(rgce) == 0 {
				continue
			}
			f, err := p.decodeFormula(rgce, nil, r0, c0, false)
			if err != nil {
				ok = false
				break
			}
			if i == 0 {
				if dv.lookup {
					f = strings.ReplaceAll(f, "\x00", ",")
				}
				rule.Formula1 = f
			} else {
				rule.Formula2 = f
			}
		}
		if !ok {
			bad++
			continue
		}
		sh.Validations = append(sh.Validations, rule)
	}
	if bad > 0 {
		p.warnf("%s: %d data validation rules could not be read", sh.Name, bad)
	}
}

// validations writes DVAL and the DV records of sh's rules.
func (x *writer) validations(b []byte, si int, sh *model.Sheet) []byte {
	var recs [][]byte
	for _, v := range sh.Validations {
		if body, ok := x.dvRecord(si, sh, v); ok {
			recs = append(recs, body)
		}
	}
	if len(recs) == 0 {
		return b
	}
	b = rec(b, recDVal, append(le(0), le32(0, 0, 0xFFFFFFFF, uint32(len(recs)))...))
	for _, body := range recs {
		b = rec(b, recDV, body)
	}
	return b
}

func (x *writer) dvRecord(si int, sh *model.Sheet, v model.Validation) ([]byte, bool) {
	var ranges []model.Range
	for _, r := range v.Ranges {
		if r.R1 < xlsMaxRows && r.C1 < xlsMaxCols {
			r.R2, r.C2 = min(r.R2, xlsMaxRows-1), min(r.C2, xlsMaxCols-1)
			ranges = append(ranges, r)
		}
	}
	if len(ranges) == 0 {
		x.issue("%s: a data validation rule lies beyond row 65,536 or column IV and will not be saved", sh.Name)
		return nil, false
	}
	flags := uint32(v.Type) | uint32(v.ErrorStyle)<<4 | uint32(v.Op)<<20
	if v.AllowBlank {
		flags |= dvAllowBlank
	}
	if v.NoDropDown {
		flags |= dvSuppressDrop
	}
	if v.ShowInput {
		flags |= dvShowInput
	}
	if v.ShowError {
		flags |= dvShowError
	}
	f1, f2 := v.Formula1, v.Formula2
	if v.Op != model.ValidBetween && v.Op != model.ValidNotBetween || v.Type == model.ValidList || v.Type == model.ValidCustom {
		f2 = ""
	}
	// A List source typed as text is an explicit list: a string with
	// the entries split by NUL.
	if v.Type == model.ValidList && len(f1) >= 2 && f1[0] == '"' && f1[len(f1)-1] == '"' {
		flags |= dvStrLookup
		f1 = strings.ReplaceAll(f1, ",", "\x00")
	}
	root := byte('V')
	if v.Type == model.ValidList {
		root = 'R'
	}
	var fs [2][]byte
	for i, f := range []string{f1, f2} {
		if f == "" {
			continue
		}
		rgce, _, err := compileFormulaRoot(x, f, si, false, root)
		if err != nil {
			x.issue("%s: the data validation formula %s cannot be saved in .xls", sh.Name, f)
			return nil, false
		}
		fs[i] = rgce
	}
	str := func(b []byte, s string, limit int) []byte {
		if s == "" {
			return append(b, 1, 0, 0, 0) // a single NUL, as Excel writes
		}
		return appendString(b, truncUTF16(s, limit))
	}
	body := le32(flags)
	body = str(body, v.PromptTitle, 32)
	body = str(body, v.ErrorTitle, 32)
	body = str(body, v.Prompt, 255)
	body = str(body, v.Error, 225)
	for _, f := range fs {
		body = append(body, le(len(f), 0)...)
		body = append(body, f...)
	}
	body = append(body, le(len(ranges))...)
	for _, r := range ranges {
		body = append(body, le(r.R1, r.R2, r.C1, r.C2)...)
	}
	return body, true
}
