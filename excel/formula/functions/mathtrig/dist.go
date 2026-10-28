package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

// The probability distributions: NORM.DIST, T.DIST, CHISQ.DIST, F.DIST,
// BINOM.DIST, GAMMA.DIST, BETA.DIST, ... with their inverses and the
// pre-2010 compatibility names. Degrees of freedom, counts and trials are
// truncated to integers, as Excel does.

func registerDist(r *f.Registry) {
	cum := func(i int) int { return 1 << i } // logical "cumulative" argument i
	r.Add(
		numSpec("NORM.DIST", 4, 4, cum(3), normDist),
		numSpec("NORMDIST", 4, 4, cum(3), normDist),
		numSpec("NORM.INV", 3, 3, 0, normInvSpec),
		numSpec("NORMINV", 3, 3, 0, normInvSpec),
		numSpec("NORM.S.DIST", 2, 2, cum(1), func(x []float64) f.Value { return normDist([]float64{x[0], 0, 1, x[1]}) }),
		numSpec("NORMSDIST", 1, 1, 0, func(x []float64) f.Value { return normDist([]float64{x[0], 0, 1, 1}) }),
		numSpec("NORM.S.INV", 1, 1, 0, func(x []float64) f.Value { return normInvSpec([]float64{x[0], 0, 1}) }),
		numSpec("NORMSINV", 1, 1, 0, func(x []float64) f.Value { return normInvSpec([]float64{x[0], 0, 1}) }),
		numSpec("PHI", 1, 1, 0, func(x []float64) f.Value { return f.NumberOrErr(normPDF(x[0])) }),
		numSpec("GAUSS", 1, 1, 0, func(x []float64) f.Value { return f.NumberOrErr(0.5 * math.Erf(x[0]/math.Sqrt2)) }),
		numSpec("STANDARDIZE", 3, 3, 0, func(x []float64) f.Value {
			if x[2] <= 0 {
				return f.ErrNum
			}
			return f.NumberOrErr((x[0] - x[1]) / x[2])
		}),
		numSpec("LOGNORM.DIST", 4, 4, cum(3), lognormDist),
		numSpec("LOGNORMDIST", 3, 3, 0, func(x []float64) f.Value { return lognormDist(append(x, 1)) }),
		numSpec("LOGNORM.INV", 3, 3, 0, lognormInv),
		numSpec("LOGINV", 3, 3, 0, lognormInv),
		numSpec("CONFIDENCE", 3, 3, 0, confidenceNorm),
		numSpec("CONFIDENCE.NORM", 3, 3, 0, confidenceNorm),
		numSpec("CONFIDENCE.T", 3, 3, 0, confidenceT),

		numSpec("T.DIST", 3, 3, cum(2), tDist),
		numSpec("T.DIST.2T", 2, 2, 0, func(x []float64) f.Value { return tDistTails(x[0], x[1], 2) }),
		numSpec("T.DIST.RT", 2, 2, 0, func(x []float64) f.Value {
			v := math.Trunc(x[1])
			if v < 1 {
				return f.ErrNum
			}
			return f.NumberOrErr(tUpper(x[0], v))
		}),
		numSpec("TDIST", 3, 3, 0, func(x []float64) f.Value { return tDistTails(x[0], x[1], math.Trunc(x[2])) }),
		numSpec("T.INV", 2, 2, 0, tInv),
		numSpec("T.INV.2T", 2, 2, 0, tInv2T),
		numSpec("TINV", 2, 2, 0, tInv2T),

		numSpec("CHISQ.DIST", 3, 3, cum(2), chisqDist),
		numSpec("CHISQ.DIST.RT", 2, 2, 0, chisqDistRT),
		numSpec("CHIDIST", 2, 2, 0, chisqDistRT),
		numSpec("CHISQ.INV", 2, 2, 0, func(x []float64) f.Value { return chisqInv(x[0], 1-x[0], x[1]) }),
		numSpec("CHISQ.INV.RT", 2, 2, 0, func(x []float64) f.Value { return chisqInv(1-x[0], x[0], x[1]) }),
		numSpec("CHIINV", 2, 2, 0, func(x []float64) f.Value { return chisqInv(1-x[0], x[0], x[1]) }),

		numSpec("F.DIST", 4, 4, cum(3), fDist),
		numSpec("F.DIST.RT", 3, 3, 0, fDistRT),
		numSpec("FDIST", 3, 3, 0, fDistRT),
		numSpec("F.INV", 3, 3, 0, func(x []float64) f.Value { return fInv(x[0], 1-x[0], x[1], x[2]) }),
		numSpec("F.INV.RT", 3, 3, 0, func(x []float64) f.Value { return fInv(1-x[0], x[0], x[1], x[2]) }),
		numSpec("FINV", 3, 3, 0, func(x []float64) f.Value { return fInv(1-x[0], x[0], x[1], x[2]) }),

		numSpec("GAMMA", 1, 1, 0, func(x []float64) f.Value {
			if x[0] <= 0 && x[0] == math.Trunc(x[0]) {
				return f.ErrNum
			}
			return f.NumberOrErr(math.Gamma(x[0]))
		}),
		numSpec("GAMMALN", 1, 1, 0, gammaLn),
		numSpec("GAMMALN.PRECISE", 1, 1, 0, gammaLn),
		numSpec("GAMMA.DIST", 4, 4, cum(3), gammaDist),
		numSpec("GAMMADIST", 4, 4, cum(3), gammaDist),
		numSpec("GAMMA.INV", 3, 3, 0, gammaInv),
		numSpec("GAMMAINV", 3, 3, 0, gammaInv),
		numSpec("EXPON.DIST", 3, 3, cum(2), exponDist),
		numSpec("EXPONDIST", 3, 3, cum(2), exponDist),
		numSpec("WEIBULL", 4, 4, cum(3), weibull),
		numSpec("WEIBULL.DIST", 4, 4, cum(3), weibull),
		numSpec("BETA.DIST", 4, 6, cum(3), betaDist),
		numSpec("BETADIST", 3, 5, 0, func(x []float64) f.Value {
			return betaDist([]float64{x[0], x[1], x[2], 1, x[3], x[4]})
		}),
		numSpec("BETA.INV", 3, 5, 0, betaInv),
		numSpec("BETAINV", 3, 5, 0, betaInv),

		numSpec("BINOM.DIST", 4, 4, cum(3), binomDist),
		numSpec("BINOMDIST", 4, 4, cum(3), binomDist),
		numSpec("BINOM.DIST.RANGE", 3, 4, 0, binomRange),
		numSpec("BINOM.INV", 3, 3, 0, binomInv),
		numSpec("CRITBINOM", 3, 3, 0, binomInv),
		numSpec("POISSON", 3, 3, cum(2), poisson),
		numSpec("POISSON.DIST", 3, 3, cum(2), poisson),
		numSpec("HYPGEOM.DIST", 5, 5, cum(4), hypgeom),
		numSpec("HYPGEOMDIST", 4, 4, 0, func(x []float64) f.Value { return hypgeom(append(x, 0)) }),
		numSpec("NEGBINOM.DIST", 4, 4, cum(3), negbinom),
		numSpec("NEGBINOMDIST", 3, 3, 0, func(x []float64) f.Value { return negbinom(append(x, 0)) }),
		numSpec("PERMUT", 2, 2, 0, func(x []float64) f.Value {
			n, k := math.Trunc(x[0]), math.Trunc(x[1])
			if n <= 0 || k < 0 || n < k {
				return f.ErrNum
			}
			p := 1.0
			for i := n - k + 1; i <= n; i++ {
				p *= i
			}
			return f.NumberOrErr(p)
		}),
		numSpec("PERMUTATIONA", 2, 2, 0, func(x []float64) f.Value {
			n, k := math.Trunc(x[0]), math.Trunc(x[1])
			if n < 0 || k < 0 {
				return f.ErrNum
			}
			return f.NumberOrErr(math.Pow(n, k))
		}),
		numSpec("FISHER", 1, 1, 0, func(x []float64) f.Value {
			if x[0] <= -1 || x[0] >= 1 {
				return f.ErrNum
			}
			return f.NumberOrErr(math.Atanh(x[0]))
		}),
		numSpec("FISHERINV", 1, 1, 0, func(x []float64) f.Value { return f.NumberOrErr(math.Tanh(x[0])) }),
	)
}

// numSpec builds a function of numeric arguments: min..max of them, those
// whose bit is set in logical are TRUE/FALSE arguments (passed as 1/0).
// Omitted optional arguments are passed as NaN.
func numSpec(name string, min, max, logical int, g func(x []float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: min, Max: max, Fn: func(e *f.Env, a []f.Value) f.Value {
		x := make([]float64, max)
		for i := range x {
			switch {
			case i >= len(a) || (i >= min && missing(a[i])):
				x[i] = math.NaN()
			case logical&(1<<i) != 0:
				b, err := f.Truth(a[i])
				if err != nil {
					return err
				}
				x[i] = b2f(b)
			default:
				v, err := f.Num(a[i])
				if err != nil {
					return err
				}
				x[i] = v
			}
		}
		return g(x)
	}}
}

// orDefault returns x, or def when the argument was omitted (NaN).
func orDefault(x, def float64) float64 {
	if math.IsNaN(x) {
		return def
	}
	return x
}

// prob reports whether p is a probability, 0 <= p <= 1.
func prob(p float64) bool { return p >= 0 && p <= 1 }

// NORM.DIST(x, mean, standard_dev, cumulative)
func normDist(x []float64) f.Value {
	if x[2] <= 0 {
		return f.ErrNum
	}
	z := (x[0] - x[1]) / x[2]
	if x[3] != 0 {
		return f.NumberOrErr(normCDF(z))
	}
	return f.NumberOrErr(normPDF(z) / x[2])
}

// NORM.INV(probability, mean, standard_dev)
func normInvSpec(x []float64) f.Value {
	if x[0] <= 0 || x[0] >= 1 || x[2] <= 0 {
		return f.ErrNum
	}
	return f.NumberOrErr(x[1] + x[2]*normInv(x[0]))
}

// LOGNORM.DIST(x, mean, standard_dev, cumulative)
func lognormDist(x []float64) f.Value {
	if x[0] <= 0 || x[2] <= 0 {
		return f.ErrNum
	}
	z := (math.Log(x[0]) - x[1]) / x[2]
	if x[3] != 0 {
		return f.NumberOrErr(normCDF(z))
	}
	return f.NumberOrErr(normPDF(z) / (x[0] * x[2]))
}

// LOGNORM.INV(probability, mean, standard_dev)
func lognormInv(x []float64) f.Value {
	if x[0] <= 0 || x[0] >= 1 || x[2] <= 0 {
		return f.ErrNum
	}
	return f.NumberOrErr(math.Exp(x[1] + x[2]*normInv(x[0])))
}

// CONFIDENCE.NORM(alpha, standard_dev, size)
func confidenceNorm(x []float64) f.Value {
	n := math.Trunc(x[2])
	if x[0] <= 0 || x[0] >= 1 || x[1] <= 0 || n < 1 {
		return f.ErrNum
	}
	return f.NumberOrErr(-normInv(x[0]/2) * x[1] / math.Sqrt(n))
}

// CONFIDENCE.T(alpha, standard_dev, size)
func confidenceT(x []float64) f.Value {
	n := math.Trunc(x[2])
	if x[0] <= 0 || x[0] >= 1 || x[1] <= 0 || n < 1 {
		return f.ErrNum
	}
	if n == 1 {
		return f.ErrDiv0
	}
	return f.NumberOrErr(tInvUpper(x[0]/2, n-1) * x[1] / math.Sqrt(n))
}

// T.DIST(x, deg_freedom, cumulative)
func tDist(x []float64) f.Value {
	v := math.Trunc(x[1])
	if v < 1 {
		return f.ErrNum
	}
	if x[2] != 0 {
		return f.NumberOrErr(1 - tUpper(x[0], v))
	}
	return f.NumberOrErr(tPDF(x[0], v))
}

// tDistTails is TDIST(x, deg_freedom, tails) and T.DIST.2T.
func tDistTails(t, df, tails float64) f.Value {
	v := math.Trunc(df)
	if t < 0 || v < 1 || (tails != 1 && tails != 2) {
		return f.ErrNum
	}
	return f.NumberOrErr(tails * tUpper(t, v))
}

// tInvUpper is the t with P(T > t) = q, for 0 < q <= 0.5.
func tInvUpper(q, v float64) float64 {
	if q >= 0.5 {
		return 0
	}
	upper := func(t float64) float64 { return tUpper(t, v) }
	lower := func(t float64) float64 { return 0.5 - tUpper(t, v) } // P(0 < T <= t)
	pdf := func(t float64) float64 { return tPDF(t, v) }
	return invert(lower, upper, pdf, 0.5-q, q, 0, math.Inf(1), -normInv(q))
}

// T.INV(probability, deg_freedom): the left-tailed inverse.
func tInv(x []float64) f.Value {
	p, v := x[0], math.Trunc(x[1])
	if p <= 0 || p > 1 || v < 1 {
		return f.ErrNum
	}
	if p < 0.5 {
		return f.NumberOrErr(-tInvUpper(p, v))
	}
	if p == 1 {
		return f.ErrNum
	}
	return f.NumberOrErr(tInvUpper(1-p, v))
}

// T.INV.2T(probability, deg_freedom), TINV
func tInv2T(x []float64) f.Value {
	p, v := x[0], math.Trunc(x[1])
	if p <= 0 || p > 1 || v < 1 {
		return f.ErrNum
	}
	return f.NumberOrErr(tInvUpper(p/2, v))
}

// CHISQ.DIST(x, deg_freedom, cumulative)
func chisqDist(x []float64) f.Value {
	v := math.Trunc(x[1])
	if x[0] < 0 || v < 1 || v > 1e10 {
		return f.ErrNum
	}
	if x[2] != 0 {
		p, _ := gammaPQ(v/2, x[0]/2)
		return f.NumberOrErr(p)
	}
	return f.NumberOrErr(gammaPDF(x[0], v/2, 2))
}

// CHISQ.DIST.RT(x, deg_freedom), CHIDIST
func chisqDistRT(x []float64) f.Value {
	v := math.Trunc(x[1])
	if x[0] < 0 || v < 1 || v > 1e10 {
		return f.ErrNum
	}
	_, q := gammaPQ(v/2, x[0]/2)
	return f.NumberOrErr(q)
}

// chisqInv is the x with lower tail p (upper tail q = 1-p).
func chisqInv(p, q, df float64) f.Value {
	v := math.Trunc(df)
	if !prob(p) || v < 1 || v > 1e10 {
		return f.ErrNum
	}
	return gammaInvPQ(p, q, v/2, 2)
}

// gammaInvPQ inverts the gamma distribution with shape a and scale b.
func gammaInvPQ(p, q, a, b float64) f.Value {
	switch {
	case p == 0:
		return f.Number(0)
	case q == 0:
		return f.ErrNum
	}
	lower := func(x float64) float64 { p, _ := gammaPQ(a, x/b); return p }
	upper := func(x float64) float64 { _, q := gammaPQ(a, x/b); return q }
	pdf := func(x float64) float64 { return gammaPDF(x, a, b) }
	return f.NumberOrErr(invert(lower, upper, pdf, p, q, 0, math.Inf(1), a*b))
}

// F.DIST(x, deg_freedom1, deg_freedom2, cumulative)
func fDist(x []float64) f.Value {
	d1, d2 := math.Trunc(x[1]), math.Trunc(x[2])
	if x[0] < 0 || d1 < 1 || d2 < 1 || d1 > 1e10 || d2 > 1e10 {
		return f.ErrNum
	}
	if x[3] != 0 {
		p, _ := fPQ(x[0], d1, d2)
		return f.NumberOrErr(p)
	}
	return f.NumberOrErr(fPDF(x[0], d1, d2))
}

// F.DIST.RT(x, deg_freedom1, deg_freedom2), FDIST
func fDistRT(x []float64) f.Value {
	d1, d2 := math.Trunc(x[1]), math.Trunc(x[2])
	if x[0] < 0 || d1 < 1 || d2 < 1 || d1 > 1e10 || d2 > 1e10 {
		return f.ErrNum
	}
	_, q := fPQ(x[0], d1, d2)
	return f.NumberOrErr(q)
}

// fInv is the F value with lower tail p (upper tail q = 1-p).
func fInv(p, q, df1, df2 float64) f.Value {
	d1, d2 := math.Trunc(df1), math.Trunc(df2)
	if !prob(p) || d1 < 1 || d2 < 1 || d1 > 1e10 || d2 > 1e10 {
		return f.ErrNum
	}
	switch {
	case p == 0:
		return f.Number(0)
	case q == 0:
		return f.ErrNum
	}
	lower := func(x float64) float64 { p, _ := fPQ(x, d1, d2); return p }
	upper := func(x float64) float64 { _, q := fPQ(x, d1, d2); return q }
	pdf := func(x float64) float64 { return fPDF(x, d1, d2) }
	return f.NumberOrErr(invert(lower, upper, pdf, p, q, 0, math.Inf(1), 1))
}

// GAMMALN(x), GAMMALN.PRECISE
func gammaLn(x []float64) f.Value {
	if x[0] <= 0 {
		return f.ErrNum
	}
	return f.NumberOrErr(lgamma(x[0]))
}

// GAMMA.DIST(x, alpha, beta, cumulative)
func gammaDist(x []float64) f.Value {
	if x[0] < 0 || x[1] <= 0 || x[2] <= 0 {
		return f.ErrNum
	}
	if x[3] != 0 {
		p, _ := gammaPQ(x[1], x[0]/x[2])
		return f.NumberOrErr(p)
	}
	return f.NumberOrErr(gammaPDF(x[0], x[1], x[2]))
}

// GAMMA.INV(probability, alpha, beta)
func gammaInv(x []float64) f.Value {
	if !prob(x[0]) || x[1] <= 0 || x[2] <= 0 {
		return f.ErrNum
	}
	return gammaInvPQ(x[0], 1-x[0], x[1], x[2])
}

// EXPON.DIST(x, lambda, cumulative)
func exponDist(x []float64) f.Value {
	if x[0] < 0 || x[1] <= 0 {
		return f.ErrNum
	}
	if x[2] != 0 {
		return f.NumberOrErr(-math.Expm1(-x[1] * x[0]))
	}
	return f.NumberOrErr(x[1] * math.Exp(-x[1]*x[0]))
}

// WEIBULL.DIST(x, alpha, beta, cumulative)
func weibull(x []float64) f.Value {
	if x[0] < 0 || x[1] <= 0 || x[2] <= 0 {
		return f.ErrNum
	}
	a, b := x[1], x[2]
	t := math.Pow(x[0]/b, a)
	if x[3] != 0 {
		return f.NumberOrErr(-math.Expm1(-t))
	}
	return f.NumberOrErr(a / b * math.Pow(x[0]/b, a-1) * math.Exp(-t))
}

// BETA.DIST(x, alpha, beta, cumulative, [A], [B])
func betaDist(x []float64) f.Value {
	a, b := x[1], x[2]
	lo, hi := orDefault(x[4], 0), orDefault(x[5], 1)
	if a <= 0 || b <= 0 || x[0] < lo || x[0] > hi || lo == hi {
		return f.ErrNum
	}
	t := (x[0] - lo) / (hi - lo)
	if x[3] != 0 {
		p, _ := betaPQ(t, (hi-x[0])/(hi-lo), a, b)
		return f.NumberOrErr(p)
	}
	return f.NumberOrErr(betaPDF(t, a, b) / (hi - lo))
}

// BETA.INV(probability, alpha, beta, [A], [B])
func betaInv(x []float64) f.Value {
	p, a, b := x[0], x[1], x[2]
	lo, hi := orDefault(x[3], 0), orDefault(x[4], 1)
	if p <= 0 || p > 1 || a <= 0 || b <= 0 || lo >= hi {
		return f.ErrNum
	}
	if p == 1 {
		return f.Number(hi)
	}
	lower := func(t float64) float64 { p, _ := betaPQ(t, 1-t, a, b); return p }
	upper := func(t float64) float64 { _, q := betaPQ(t, 1-t, a, b); return q }
	pdf := func(t float64) float64 { return betaPDF(t, a, b) }
	t := invert(lower, upper, pdf, p, 1-p, 0, 1, a/(a+b))
	return f.NumberOrErr(lo + t*(hi-lo))
}

// smallN bounds the counts for which the discrete distributions use exact
// binomial coefficients (products of up to smallN terms) rather than
// log-gamma functions.
const smallN = 1000

// binomPMF is the probability of k successes in n trials.
func binomPMF(k, n, p float64) float64 {
	if k < 0 || k > n {
		return 0
	}
	if n <= smallN {
		if v := combin(n, k) * math.Pow(p, k) * math.Pow(1-p, n-k); v > 0 && !math.IsInf(v, 0) {
			return v
		}
	}
	switch {
	case p == 0:
		return b2f(k == 0)
	case p == 1:
		return b2f(k == n)
	}
	return math.Exp(lgamma(n+1) - lgamma(k+1) - lgamma(n-k+1) + k*math.Log(p) + (n-k)*math.Log1p(-p))
}

// binomCDF is the probability of at most k successes in n trials.
func binomCDF(k, n, p float64) float64 {
	if k >= n {
		return 1
	}
	if n > smallN {
		_, q := betaPQ(p, 1-p, k+1, n-k)
		return q
	}
	s := 0.0
	for i := 0.0; i <= k; i++ {
		s += binomPMF(i, n, p)
	}
	return math.Min(s, 1)
}

// BINOM.DIST(number_s, trials, probability_s, cumulative)
func binomDist(x []float64) f.Value {
	k, n, p := math.Trunc(x[0]), math.Trunc(x[1]), x[2]
	if k < 0 || k > n || !prob(p) {
		return f.ErrNum
	}
	if x[3] != 0 {
		return f.NumberOrErr(binomCDF(k, n, p))
	}
	return f.NumberOrErr(binomPMF(k, n, p))
}

// BINOM.DIST.RANGE(trials, probability_s, number_s, [number_s2])
func binomRange(x []float64) f.Value {
	n, p, s := math.Trunc(x[0]), x[1], math.Trunc(x[2])
	s2 := math.Trunc(orDefault(x[3], s))
	if n < 0 || !prob(p) || s < 0 || s > n || s2 < s || s2 > n {
		return f.ErrNum
	}
	sum := 0.0
	for k := s; k <= s2; k++ {
		sum += binomPMF(k, n, p)
	}
	return f.NumberOrErr(math.Min(sum, 1))
}

// BINOM.INV(trials, probability_s, alpha), CRITBINOM: the smallest k
// whose cumulative probability is at least alpha.
func binomInv(x []float64) f.Value {
	n, p, alpha := math.Trunc(x[0]), x[1], x[2]
	if n < 0 || !prob(p) || !prob(alpha) {
		return f.ErrNum
	}
	s := 0.0
	for k := 0.0; k < n; k++ {
		s += binomPMF(k, n, p)
		if s >= alpha*(1-1e-15) {
			return f.Number(k)
		}
	}
	return f.Number(n)
}

// POISSON.DIST(x, mean, cumulative)
func poisson(x []float64) f.Value {
	k, m := math.Trunc(x[0]), x[1]
	if k < 0 || m < 0 {
		return f.ErrNum
	}
	if x[2] != 0 {
		_, q := gammaPQ(k+1, m)
		return f.NumberOrErr(q)
	}
	if m == 0 {
		return f.Number(b2f(k == 0))
	}
	return f.NumberOrErr(math.Exp(k*math.Log(m) - m - lgamma(k+1)))
}

// lnCombin is ln C(n, k).
func lnCombin(n, k float64) float64 { return lgamma(n+1) - lgamma(k+1) - lgamma(n-k+1) }

// HYPGEOM.DIST(sample_s, number_sample, population_s, number_pop, cumulative)
func hypgeom(x []float64) f.Value {
	k, n, succ, pop := math.Trunc(x[0]), math.Trunc(x[1]), math.Trunc(x[2]), math.Trunc(x[3])
	lo := math.Max(0, n-pop+succ)
	if k < lo || k > math.Min(n, succ) || n <= 0 || n > pop || succ <= 0 || succ > pop || pop <= 0 {
		return f.ErrNum
	}
	pmf := func(i float64) float64 {
		if pop <= smallN {
			if c := combin(succ, i) * combin(pop-succ, n-i) / combin(pop, n); c > 0 && !math.IsInf(c, 0) {
				return c
			}
		}
		return math.Exp(lnCombin(succ, i) + lnCombin(pop-succ, n-i) - lnCombin(pop, n))
	}
	if x[4] == 0 {
		return f.NumberOrErr(pmf(k))
	}
	s := 0.0
	for i := lo; i <= k; i++ {
		s += pmf(i)
	}
	return f.NumberOrErr(math.Min(s, 1))
}

// NEGBINOM.DIST(number_f, number_s, probability_s, cumulative): failures
// before the number_s-th success.
func negbinom(x []float64) f.Value {
	k, s, p := math.Trunc(x[0]), math.Trunc(x[1]), x[2]
	if !prob(p) || k < 0 || s < 1 {
		return f.ErrNum
	}
	if x[3] != 0 {
		v, _ := betaPQ(p, 1-p, s, k+1)
		return f.NumberOrErr(v)
	}
	if k+s <= smallN {
		if v := combin(k+s-1, s-1) * math.Pow(p, s) * math.Pow(1-p, k); v > 0 && !math.IsInf(v, 0) {
			return f.Number(v)
		}
	}
	if p == 0 || p == 1 {
		return f.Number(b2f(p == 1 && k == 0))
	}
	return f.NumberOrErr(math.Exp(lnCombin(k+s-1, s-1) + s*math.Log(p) + k*math.Log1p(-p)))
}
