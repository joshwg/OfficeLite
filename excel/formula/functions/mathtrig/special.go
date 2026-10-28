package mathtrig

import "math"

// Special functions behind the statistical distributions: the normal
// distribution and its inverse, the regularized incomplete gamma and beta
// functions, and a safeguarded Newton solver for the inverse distributions.

// normCDF is the standard normal distribution function Φ(z).
func normCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// normPDF is the standard normal density φ(z).
func normPDF(z float64) float64 { return math.Exp(-z*z/2) / math.Sqrt(2*math.Pi) }

// normInv is Φ⁻¹(p) for 0 < p < 1 (Wichura's algorithm AS 241, accurate
// to about 1e-16).
func normInv(p float64) float64 {
	q := p - 0.5
	if math.Abs(q) <= 0.425 {
		r := 0.180625 - q*q
		return q * (((((((r*2509.0809287301226727+33430.575583588128105)*r+67265.770927008700853)*r+
			45921.953931549871457)*r+13731.693765509461125)*r+1971.5909503065514427)*r+133.14166789178437745)*r +
			3.387132872796366608) /
			(((((((r*5226.495278852545925+28729.085735721942674)*r+39307.89580009271061)*r+
				21213.794301586595867)*r+5394.1960214247511077)*r+687.1870074920579083)*r+42.313330701600911252)*r + 1)
	}
	r := p
	if q > 0 {
		r = 1 - p
	}
	r = math.Sqrt(-math.Log(r))
	var v float64
	if r <= 5 {
		r -= 1.6
		v = (((((((r*7.7454501427834140764e-4+0.0227238449892691845833)*r+0.24178072517745061177)*r+
			1.27045825245236838258)*r+3.64784832476320460504)*r+5.7694972214606914055)*r+4.6303378461565452959)*r +
			1.42343711074968357734) /
			(((((((r*1.05075007164441684324e-9+5.475938084995344946e-4)*r+0.0151986665636164571966)*r+
				0.14810397642748007459)*r+0.68976733498510000455)*r+1.6763848301838038494)*r+2.05319162663775882187)*r + 1)
	} else {
		r -= 5
		v = (((((((r*2.01033439929228813265e-7+2.71155556874348757815e-5)*r+0.0012426609473880784386)*r+
			0.026532189526576123093)*r+0.29656057182850489123)*r+1.7848265399172913358)*r+5.4637849111641143699)*r +
			6.6579046435011037772) /
			(((((((r*2.04426310338993978564e-15+1.4215117583164458887e-7)*r+1.8463183175100546818e-5)*r+
				7.868691311456132591e-4)*r+0.0148753612908506148525)*r+0.13692988092273580531)*r+0.59983220655588793769)*r + 1)
	}
	if q < 0 {
		v = -v
	}
	return v
}

func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// lbeta is ln B(a, b).
func lbeta(a, b float64) float64 { return lgamma(a) + lgamma(b) - lgamma(a+b) }

const (
	cfTiny  = 1e-300
	cfEps   = 1e-16
	cfIters = 100000
)

// gammaPQ returns the regularized incomplete gamma functions P(a, x) and
// Q(a, x) = 1 - P(a, x), each computed directly where it is the smaller,
// so that tails keep their precision.
func gammaPQ(a, x float64) (p, q float64) {
	switch {
	case x <= 0:
		return 0, 1
	case math.IsInf(x, 1):
		return 1, 0
	}
	pre := math.Exp(a*math.Log(x) - x - lgamma(a))
	if x < a+1 {
		// Series: P = pre · Σ x^n / (a (a+1) ... (a+n)).
		ap, del, sum := a, 1/a, 1/a
		for range cfIters {
			ap++
			del *= x / ap
			sum += del
			if math.Abs(del) < math.Abs(sum)*cfEps {
				break
			}
		}
		p = pre * sum
		return p, 1 - p
	}
	// Continued fraction for Q (modified Lentz).
	b := x + 1 - a
	c, d := 1/cfTiny, 1/b
	h := d
	for i := 1; i < cfIters; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < cfTiny {
			d = cfTiny
		}
		c = b + an/c
		if math.Abs(c) < cfTiny {
			c = cfTiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < cfEps {
			break
		}
	}
	q = pre * h
	return 1 - q, q
}

// betaPQ returns the regularized incomplete beta function I_x(a, b) and
// its complement; y is 1-x, passed separately to keep its precision.
func betaPQ(x, y, a, b float64) (p, q float64) {
	switch {
	case x <= 0:
		return 0, 1
	case y <= 0:
		return 1, 0
	}
	pre := math.Exp(a*math.Log(x) + b*math.Log(y) - lbeta(a, b))
	if x < (a+1)/(a+b+2) {
		p = pre * betaCF(a, b, x) / a
		return p, 1 - p
	}
	q = pre * betaCF(b, a, y) / b
	return 1 - q, q
}

// betaCF evaluates the continued fraction of the incomplete beta function
// (modified Lentz).
func betaCF(a, b, x float64) float64 {
	qab, qap, qam := a+b, a+1, a-1
	c, d := 1.0, 1-qab*x/qap
	if math.Abs(d) < cfTiny {
		d = cfTiny
	}
	d = 1 / d
	h := d
	step := func(aa float64) float64 {
		d = 1 + aa*d
		if math.Abs(d) < cfTiny {
			d = cfTiny
		}
		c = 1 + aa/c
		if math.Abs(c) < cfTiny {
			c = cfTiny
		}
		d = 1 / d
		return d * c
	}
	for m := 1.0; m < cfIters; m++ {
		m2 := 2 * m
		h *= step(m * (b - m) * x / ((qam + m2) * (a + m2)))
		del := step(-(a + m) * (qab + m) * x / ((a + m2) * (qap + m2)))
		h *= del
		if math.Abs(del-1) < cfEps {
			break
		}
	}
	return h
}

// --- distributions (no argument checks) ------------------------------------

// gammaPDF is the density of the gamma distribution with shape a and
// scale b.
func gammaPDF(x, a, b float64) float64 {
	switch {
	case x < 0:
		return 0
	case x == 0:
		switch {
		case a < 1:
			return math.Inf(1)
		case a == 1:
			return 1 / b
		}
		return 0
	}
	return math.Exp((a-1)*math.Log(x) - x/b - lgamma(a) - a*math.Log(b))
}

// betaPDF is the density of the beta distribution on [0, 1].
func betaPDF(x, a, b float64) float64 {
	if x < 0 || x > 1 {
		return 0
	}
	v := math.Pow(x, a-1) * math.Pow(1-x, b-1) / math.Exp(lbeta(a, b))
	if v == 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		if (x == 0 && a < 1) || (x == 1 && b < 1) {
			return math.Inf(1)
		}
		if x == 0 || x == 1 {
			return v
		}
		v = math.Exp((a-1)*math.Log(x) + (b-1)*math.Log1p(-x) - lbeta(a, b))
	}
	return v
}

// tPDF is the density of Student's t distribution with v degrees of
// freedom.
func tPDF(t, v float64) float64 {
	return math.Exp(lgamma((v+1)/2)-lgamma(v/2)) / math.Sqrt(v*math.Pi) * math.Pow(1+t*t/v, -(v+1)/2)
}

// tUpper is P(T > t) for Student's t with v degrees of freedom.
func tUpper(t, v float64) float64 {
	tt := t * t
	_, q := betaPQ(tt/(v+tt), v/(v+tt), 0.5, v/2) // P(|T| > |t|)
	if t >= 0 {
		return q / 2
	}
	return 1 - q/2
}

// fPDF is the density of the F distribution.
func fPDF(x, d1, d2 float64) float64 {
	switch {
	case x < 0:
		return 0
	case x == 0:
		switch {
		case d1 < 2:
			return math.Inf(1)
		case d1 == 2:
			return 1
		}
		return 0
	}
	return math.Exp((d1/2)*math.Log(d1) + (d2/2)*math.Log(d2) + (d1/2-1)*math.Log(x) -
		((d1+d2)/2)*math.Log(d2+d1*x) - lbeta(d1/2, d2/2))
}

// fPQ returns the F distribution function and its upper tail.
func fPQ(x, d1, d2 float64) (p, q float64) {
	if x <= 0 {
		return 0, 1
	}
	return betaPQ(d1*x/(d1*x+d2), d2/(d1*x+d2), d1/2, d2/2)
}

// invert solves for x a distribution equation lower(x) = p, or
// upper(x) = q when q < p, so that upper-tail probabilities keep their
// precision (p + q is the total mass, normally 1). lower increases from 0 at lo to 1 at hi (which
// may be +Inf), pdf is its derivative and x0 a starting guess. The
// iteration is Newton's method kept inside a shrinking bracket.
func invert(lower, upper, pdf func(float64) float64, p, q, lo, hi, x0 float64) float64 {
	h := func(x float64) float64 { return lower(x) - p }
	if q < p {
		h = func(x float64) float64 { return q - upper(x) }
	}
	if math.IsInf(hi, 1) {
		hi = math.Max(2*x0, 1)
		for h(hi) < 0 {
			lo, hi = hi, hi*2
			if hi > 1e300 {
				return math.Inf(1)
			}
		}
	}
	x := x0
	if !(x > lo && x < hi) {
		x = lo + (hi-lo)/2
	}
	for range 2000 {
		v := h(x)
		if v == 0 {
			return x
		}
		if v < 0 {
			lo = x
		} else {
			hi = x
		}
		d := pdf(x)
		nx := x - v/d
		if !(d > 0) || math.IsInf(d, 0) || !(nx > lo && nx < hi) {
			nx = lo + (hi-lo)/2 // bisect
		}
		if math.Abs(nx-x) <= 1e-15*math.Abs(nx) || hi-lo <= 1e-15*math.Abs(hi) {
			return nx
		}
		x = nx
	}
	return x
}
