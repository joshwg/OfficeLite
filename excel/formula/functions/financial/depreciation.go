package financial

import (
	"math"

	f "officelite/excel/formula"
)

// The depreciation functions: SLN, SYD, DB, DDB and VDB.

func registerDepreciation(r *f.Registry) {
	dep := func(s f.Spec) f.Spec { s.Format = currency; return s }
	r.Add(
		dep(numSpec("SLN", 3, 3, func(x []float64) f.Value { // SLN(cost, salvage, life)
			if x[2] == 0 {
				return f.ErrDiv0
			}
			return f.NumberOrErr((x[0] - x[1]) / x[2])
		})),
		dep(numSpec("SYD", 4, 4, func(x []float64) f.Value { // SYD(cost, salvage, life, per)
			cost, salvage, life, per := x[0], x[1], x[2], x[3]
			if life <= 0 || per <= 0 || per > life {
				return f.ErrNum
			}
			return f.NumberOrErr((cost - salvage) * (life - per + 1) * 2 / (life * (life + 1)))
		})),
		dep(f.Spec{Name: "DB", Min: 4, Max: 5, Fn: db}),
		dep(f.Spec{Name: "DDB", Min: 4, Max: 5, Fn: ddb}),
		dep(f.Spec{Name: "VDB", Min: 5, Max: 7, Fn: vdb}),
	)
}

// DB(cost, salvage, life, period, [month]): fixed-declining balance at a
// rate rounded to three decimals; the first year has month months (12,
// truncated) and the leftover months fall in period life+1.
func db(_ *f.Env, a []f.Value) f.Value {
	x, err := nums(a, 5)
	if err != nil {
		return err
	}
	cost, salvage, life, period, month := x[0], x[1], x[2], math.Trunc(x[3]), 12.0
	if given(a, 4) {
		month = math.Trunc(x[4])
	}
	if cost <= 0 || salvage < 0 || salvage > cost || life <= 0 || period <= 0 ||
		period > life+1 || month < 1 || month > 12 {
		return f.ErrNum
	}
	rate := math.Round((1-math.Pow(salvage/cost, 1/life))*1000) / 1000
	total := cost * rate * month / 12
	dep := total
	for p := 2.0; p <= period; p++ {
		if p > life {
			dep = (cost - total) * rate * (12 - month) / 12
		} else {
			dep = (cost - total) * rate
		}
		total += dep
	}
	return f.NumberOrErr(dep)
}

// ddbOf is the declining-balance depreciation of a (possibly fractional)
// period, never taking the book value below salvage.
func ddbOf(cost, salvage, life, period, factor float64) float64 {
	rate := factor / life
	var old float64
	if rate >= 1 {
		rate = 1
		if period == 1 {
			old = cost
		}
	} else {
		old = cost * math.Pow(1-rate, period-1)
	}
	next := cost * math.Pow(1-rate, period)
	d := old - next
	if next < salvage {
		d = old - salvage
	}
	return math.Max(d, 0)
}

// DDB(cost, salvage, life, period, [factor])
func ddb(_ *f.Env, a []f.Value) f.Value {
	x, err := nums(a, 5)
	if err != nil {
		return err
	}
	cost, salvage, life, period, factor := x[0], x[1], x[2], x[3], 2.0
	if given(a, 4) {
		factor = x[4]
	}
	if cost < 0 || salvage < 0 || salvage > cost || life <= 0 || factor <= 0 || period < 1 || period > life {
		return f.ErrNum
	}
	return f.NumberOrErr(ddbOf(cost, salvage, life, period, factor))
}

// interVDB is the declining-balance depreciation over the first periods
// periods (the last possibly partial) of an asset with life1 periods left,
// switching to straight-line once that is larger.
func interVDB(cost, salvage, life, life1, periods, factor float64) float64 {
	var vdb, sln float64
	end := math.Ceil(periods)
	left := cost - salvage
	switched := false
	for i := 1.0; i <= end; i++ {
		term := sln
		if !switched {
			d := ddbOf(cost, salvage, life, i, factor)
			sln = left / (life1 - (i - 1))
			if sln > d {
				term, switched = sln, true
			} else {
				term = d
				left -= d
			}
		}
		if i == end {
			term *= periods + 1 - end
		}
		vdb += term
	}
	return vdb
}

// VDB(cost, salvage, life, start_period, end_period, [factor],
// [no_switch]): declining-balance depreciation between two (possibly
// fractional) points in time, switching to straight-line when that is
// larger unless no_switch.
func vdb(_ *f.Env, a []f.Value) f.Value {
	x, err := nums(a, 7)
	if err != nil {
		return err
	}
	cost, salvage, life, start, end, factor := x[0], x[1], x[2], x[3], x[4], 2.0
	if given(a, 5) {
		factor = x[5]
	}
	noSwitch := x[6] != 0
	if cost < 0 || salvage < 0 || salvage > cost || life <= 0 || start < 0 || end < start || end > life || factor <= 0 {
		return f.ErrNum
	}
	iStart, iEnd := math.Floor(start), math.Ceil(end)
	var res float64
	if noSwitch {
		for i := iStart + 1; i <= iEnd; i++ {
			term := ddbOf(cost, salvage, life, i, factor)
			if i == iStart+1 {
				term *= math.Min(end, iStart+1) - start
			} else if i == iEnd {
				term *= end + 1 - iEnd
			}
			res += term
		}
		return f.NumberOrErr(res)
	}
	// Partial periods at either end are taken off whole ones.
	var part float64
	if start != iStart {
		left := cost - interVDB(cost, salvage, life, life, iStart, factor)
		part += (start - iStart) * interVDB(left, salvage, life, life-iStart, 1, factor)
	}
	if end != iEnd {
		left := cost - interVDB(cost, salvage, life, life, iEnd-1, factor)
		part += (iEnd - end) * interVDB(left, salvage, life, life-(iEnd-1), 1, factor)
	}
	left := cost - interVDB(cost, salvage, life, life, iStart, factor)
	res = interVDB(left, salvage, life, life-iStart, iEnd-iStart, factor) - part
	return f.NumberOrErr(res)
}
