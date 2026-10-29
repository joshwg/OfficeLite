// Automatic value-axis scaling for charts: bounds, units and tick values.

package render

import "math"

// Value axis scaling as Excel picks it when the axis' bounds and units
// are automatic:
//
//   - The minimum is 0 when the data are all of one sign and the smallest
//     magnitude is less than 5/6 of the largest (the range is more than a
//     sixth of the maximum); otherwise the data's minimum less half the
//     range (with both signs: less 5% of the range).
//   - The maximum is the data's maximum plus 5% of the distance from the
//     minimum (negative data: mirrored).
//   - The major unit is the smallest of 1, 2, 5 × 10^n that splits the
//     range, rounded out to whole units, into at most maxSteps intervals
//     (10 for a chart of normal size; fewer when the labels wouldn't fit).
//   - Both bounds are then rounded out to multiples of the unit.
//
// So data 0..97 gets 0..120 by 20 and 3..8 gets 0..9 by 1, as in Excel.

// axisScale is a value axis' bounds and units.
type axisScale struct {
	Min, Max   float64
	Major      float64
	Minor      float64
	Log        float64 // the log base, 0 when linear
	Reversed   bool
	CrossAt    float64 // where the other axis crosses
	CrossAtMax bool    // it crosses at the maximum
	Percent    bool    // a 100% stacked axis
}

// scaleOpts are an axis' fixed settings (Has... false: automatic).
type scaleOpts struct {
	Min, Max, Major, Minor  float64
	HasMin, HasMax, HasUnit bool
	LogBase                 float64
	MaxSteps                int
}

// niceScale picks an axis' bounds and major unit for data dmin..dmax.
func niceScale(dmin, dmax float64, o scaleOpts) axisScale {
	if o.MaxSteps <= 0 {
		o.MaxSteps = 10
	}
	if o.LogBase > 1 {
		return logScale(dmin, dmax, o)
	}
	if math.IsInf(dmin, 0) || math.IsInf(dmax, 0) || math.IsNaN(dmin) || math.IsNaN(dmax) || dmin > dmax {
		dmin, dmax = 0, 1
	}
	if o.HasMin {
		dmin = min(dmin, o.Min)
	}
	if o.HasMax {
		dmax = max(dmax, o.Max)
	}
	if dmin == dmax {
		// One value: Excel shows 0..value (or value..0), or 0..1 for 0.
		switch {
		case dmax > 0:
			dmin = 0
		case dmax < 0:
			dmax = 0
		default:
			// Nothing but zeros: 0..1 by 0.1.
			if !o.HasMin && !o.HasMax && !o.HasUnit {
				return axisScale{Min: 0, Max: 1, Major: 0.1, Minor: 0.02}
			}
			dmax = 1
		}
	}

	if dmax <= 0 && dmin < 0 && !o.HasMin && !o.HasMax {
		// All negative: the mirror image of the positive case.
		m := niceScale(-dmax, -dmin, o)
		m.Min, m.Max = -m.Max, -m.Min
		return m
	}
	lo, hi := dmin, dmax
	switch {
	case o.HasMin:
		lo = o.Min
	case dmin >= 0 && (dmax-dmin)/dmax > 1.0/6:
		lo = 0
	case dmin >= 0:
		lo = dmin - (dmax-dmin)/2
	default:
		lo = dmin - 0.05*(dmax-dmin)
	}
	if !o.HasMax {
		hi = dmax + 0.05*(dmax-lo)
	} else {
		hi = o.Max
	}

	step := o.Major
	if !o.HasUnit || step <= 0 {
		step = niceStep(lo, hi, o.MaxSteps, o.HasMin, o.HasMax)
	}
	s := axisScale{Min: lo, Max: hi, Major: step}
	if !o.HasMin {
		s.Min = math.Floor(lo/step+1e-9) * step
	}
	if !o.HasMax {
		s.Max = math.Ceil(hi/step-1e-9) * step
	}
	if s.Max <= s.Min {
		s.Max = s.Min + step
	}
	s.Minor = o.Minor
	if s.Minor <= 0 {
		s.Minor = step / 5
	}
	return s
}

// niceStep is the smallest 1, 2 or 5 × 10^n that splits lo..hi (rounded
// out to multiples of it, unless fixed) into at most maxSteps intervals.
func niceStep(lo, hi float64, maxSteps int, fixedMin, fixedMax bool) float64 {
	span := hi - lo
	if span <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(span / float64(maxSteps)))
	for k := exp - 1; k < exp+3; k++ {
		p := math.Pow(10, k)
		for _, m := range []float64{1, 2, 5} {
			step := m * p
			a, b := lo, hi
			if !fixedMin {
				a = math.Floor(lo/step+1e-9) * step
			}
			if !fixedMax {
				b = math.Ceil(hi/step-1e-9) * step
			}
			if (b-a)/step <= float64(maxSteps)+1e-9 {
				return step
			}
		}
	}
	return math.Pow(10, exp+3)
}

// logScale bounds a logarithmic axis at powers of its base.
func logScale(dmin, dmax float64, o scaleOpts) axisScale {
	b := o.LogBase
	if dmin <= 0 {
		dmin = 1
	}
	if dmax <= dmin {
		dmax = dmin * b
	}
	s := axisScale{Log: b, Major: b}
	s.Min = math.Pow(b, math.Floor(math.Log(dmin)/math.Log(b)+1e-9))
	s.Max = math.Pow(b, math.Ceil(math.Log(dmax)/math.Log(b)-1e-9))
	if o.HasMin && o.Min > 0 {
		s.Min = o.Min
	}
	if o.HasMax && o.Max > s.Min {
		s.Max = o.Max
	}
	if s.Max <= s.Min {
		s.Max = s.Min * b
	}
	if o.HasUnit && o.Major > 1 {
		s.Major = o.Major
	}
	return s
}

// ticks lists the major tick values of s.
func (s *axisScale) ticks() []float64 {
	var out []float64
	if s.Log > 1 {
		for v := s.Min; v <= s.Max*(1+1e-9) && len(out) < 100; v *= s.Major {
			out = append(out, v)
		}
		return out
	}
	if s.Major <= 0 {
		return []float64{s.Min, s.Max}
	}
	n := int(math.Round((s.Max - s.Min) / s.Major))
	for i := 0; i <= n && i <= 1000; i++ {
		v := s.Min + float64(i)*s.Major
		if math.Abs(v) < s.Major*1e-9 {
			v = 0 // no -0 or 1e-17 labels
		}
		out = append(out, v)
	}
	return out
}

// minorTicks lists the minor tick values between the major ones.
func (s *axisScale) minorTicks() []float64 {
	if s.Log > 1 || s.Minor <= 0 || (s.Max-s.Min)/s.Minor > 500 {
		return nil
	}
	var out []float64
	n := int(math.Round((s.Max - s.Min) / s.Minor))
	for i := 0; i <= n; i++ {
		out = append(out, s.Min+float64(i)*s.Minor)
	}
	return out
}

// frac maps a value to 0..1 along the axis (0 at the minimum end, before
// reversing).
func (s *axisScale) frac(v float64) float64 {
	var f float64
	switch {
	case s.Log > 1:
		if v <= 0 {
			v = s.Min
		}
		f = (math.Log(v) - math.Log(s.Min)) / (math.Log(s.Max) - math.Log(s.Min))
	case s.Max == s.Min:
		return 0
	default:
		f = (v - s.Min) / (s.Max - s.Min)
	}
	if s.Reversed {
		f = 1 - f
	}
	return f
}
