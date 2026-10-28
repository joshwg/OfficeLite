package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

// Hypothesis tests (T.TEST, F.TEST, CHISQ.TEST, Z.TEST) and PROB.

func registerHypothesis(r *f.Registry) {
	r.Add(
		f.Spec{Name: "T.TEST", Min: 4, Max: 4, Refs: f.Only(0, 1), Arrays: f.Only(0, 1), Fn: tTest},
		f.Spec{Name: "TTEST", Min: 4, Max: 4, Refs: f.Only(0, 1), Arrays: f.Only(0, 1), Fn: tTest},
		f.Spec{Name: "F.TEST", Min: 2, Max: 2, Refs: f.All, Arrays: f.All, Fn: fTest},
		f.Spec{Name: "FTEST", Min: 2, Max: 2, Refs: f.All, Arrays: f.All, Fn: fTest},
		f.Spec{Name: "CHISQ.TEST", Min: 2, Max: 2, Arrays: f.All, Fn: chisqTest},
		f.Spec{Name: "CHITEST", Min: 2, Max: 2, Arrays: f.All, Fn: chisqTest},
		f.Spec{Name: "Z.TEST", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: zTest},
		f.Spec{Name: "ZTEST", Min: 2, Max: 3, Refs: f.Only(0), Arrays: f.Only(0), Fn: zTest},
		f.Spec{Name: "PROB", Min: 3, Max: 4, Refs: f.Only(0, 1), Arrays: f.Only(0, 1), Fn: probFn},
	)
}

// T.TEST(array1, array2, tails, type): the probability of Student's t
// test; type 1 paired, 2 two samples with equal variance, 3 unequal
// variance (Welch, with fractional degrees of freedom).
func tTest(e *f.Env, a []f.Value) f.Value {
	tails, err := f.Num(a[2])
	if err != nil {
		return err
	}
	typ, err := f.Num(a[3])
	if err != nil {
		return err
	}
	tails, typ = math.Trunc(tails), math.Trunc(typ)
	if (tails != 1 && tails != 2) || typ < 1 || typ > 3 {
		return f.ErrNum
	}
	var t, df float64
	if typ == 1 {
		xs, ys, err := pairs(e, a[0], a[1])
		if err != nil {
			return err
		}
		n := float64(len(xs))
		if n < 2 {
			return f.ErrDiv0
		}
		d := make([]float64, len(xs))
		for i := range d {
			d[i] = xs[i] - ys[i]
		}
		sd := math.Sqrt(devsq(d) / (n - 1))
		t, df = mean(d)/(sd/math.Sqrt(n)), n-1
	} else {
		xs, err := gather(e, a[:1], opts{k: kNum})
		if err != nil {
			return err
		}
		ys, err := gather(e, a[1:2], opts{k: kNum})
		if err != nil {
			return err
		}
		n1, n2 := float64(len(xs)), float64(len(ys))
		if n1 < 2 || n2 < 2 {
			return f.ErrDiv0
		}
		v1, v2 := devsq(xs)/(n1-1), devsq(ys)/(n2-1)
		diff := mean(xs) - mean(ys)
		if typ == 2 {
			df = n1 + n2 - 2
			sp := ((n1-1)*v1 + (n2-1)*v2) / df
			t = diff / math.Sqrt(sp*(1/n1+1/n2))
		} else {
			s1, s2 := v1/n1, v2/n2
			t = diff / math.Sqrt(s1+s2)
			df = (s1 + s2) * (s1 + s2) / (s1*s1/(n1-1) + s2*s2/(n2-1))
		}
	}
	if math.IsNaN(t) || math.IsInf(t, 0) {
		return f.ErrDiv0
	}
	return f.NumberOrErr(tails * tUpper(math.Abs(t), df))
}

// F.TEST(array1, array2): the two-tailed probability that the variances
// of the samples are not significantly different.
func fTest(e *f.Env, a []f.Value) f.Value {
	xs, err := gather(e, a[:1], opts{k: kNum})
	if err != nil {
		return err
	}
	ys, err := gather(e, a[1:2], opts{k: kNum})
	if err != nil {
		return err
	}
	n1, n2 := float64(len(xs)), float64(len(ys))
	if n1 < 2 || n2 < 2 {
		return f.ErrDiv0
	}
	v1, v2 := devsq(xs)/(n1-1), devsq(ys)/(n2-1)
	if v1 == 0 || v2 == 0 {
		return f.ErrDiv0
	}
	p, q := fPQ(v1/v2, n1-1, n2-1)
	return f.NumberOrErr(math.Min(1, 2*math.Min(p, q)))
}

// CHISQ.TEST(actual_range, expected_range): the probability of the χ²
// statistic with (rows-1)(columns-1) degrees of freedom (n-1 for a single
// row or column).
func chisqTest(e *f.Env, a []f.Value) f.Value {
	act, exp := e.ToArray(a[0]), e.ToArray(a[1])
	if act.Rows != exp.Rows || act.Cols != exp.Cols {
		return f.ErrNA
	}
	chi := 0.0
	for i, v := range act.V {
		if err, ok := v.(f.Error); ok {
			return err
		}
		if err, ok := exp.V[i].(f.Error); ok {
			return err
		}
		x, ok1 := v.(f.Number)
		m, ok2 := exp.V[i].(f.Number)
		if !ok1 || !ok2 {
			continue
		}
		if m == 0 {
			return f.ErrDiv0
		}
		if m < 0 {
			return f.ErrNum
		}
		chi += float64((x - m) * (x - m) / m)
	}
	var df float64
	switch {
	case act.Rows > 1 && act.Cols > 1:
		df = float64((act.Rows - 1) * (act.Cols - 1))
	default:
		df = float64(act.Rows*act.Cols - 1)
	}
	if df < 1 {
		return f.ErrNA
	}
	_, q := gammaPQ(df/2, chi/2)
	return f.NumberOrErr(q)
}

// Z.TEST(array, x, [sigma]): the one-tailed probability of the z test,
// 1 - Φ((mean - x)/(σ/√n)); σ defaults to the sample standard deviation.
func zTest(e *f.Env, a []f.Value) f.Value {
	xs, err := gather(e, a[:1], opts{k: kNum})
	if err != nil {
		return err
	}
	x, err := f.Num(a[1])
	if err != nil {
		return err
	}
	n := float64(len(xs))
	if n == 0 {
		return f.ErrNA
	}
	sigma := math.NaN()
	if len(a) > 2 && !missing(a[2]) {
		if sigma, err = f.Num(a[2]); err != nil {
			return err
		}
		if sigma <= 0 {
			return f.ErrNum
		}
	} else {
		if n < 2 {
			return f.ErrDiv0
		}
		sigma = math.Sqrt(devsq(xs) / (n - 1))
	}
	if sigma == 0 {
		return f.ErrDiv0
	}
	z := (mean(xs) - x) / (sigma / math.Sqrt(n))
	return f.NumberOrErr(normCDF(-z))
}

// PROB(x_range, prob_range, lower_limit, [upper_limit]): the probability
// that a value lies between the limits (equals lower_limit when there is
// no upper limit). The probabilities must lie in [0, 1] and sum to 1.
func probFn(e *f.Env, a []f.Value) f.Value {
	xs, ps, err := pairs(e, a[0], a[1])
	if err != nil {
		return err
	}
	lo, err := f.Num(a[2])
	if err != nil {
		return err
	}
	hi, err := optNum(a, 3, lo)
	if err != nil {
		return err
	}
	total, s := 0.0, 0.0
	for i, p := range ps {
		if p < 0 || p > 1 {
			return f.ErrNum
		}
		total += p
		if xs[i] >= lo && xs[i] <= hi {
			s += p
		}
	}
	if math.Abs(total-1) > 1e-12 {
		return f.ErrNum
	}
	return f.NumberOrErr(s)
}
