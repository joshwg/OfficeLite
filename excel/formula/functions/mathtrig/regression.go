package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

// Paired-data statistics and regression: CORREL, COVARIANCE, SLOPE,
// FORECAST, STEYX, ..., and the least-squares fits LINEST, LOGEST, TREND
// and GROWTH.

func registerRegression(r *f.Registry) {
	r.Add(
		pairSpec("CORREL", correl),
		pairSpec("PEARSON", correl),
		pairSpec("RSQ", func(y, x []float64) f.Value {
			v := correl(y, x)
			if n, ok := v.(f.Number); ok {
				return n * n
			}
			return v
		}),
		pairSpec("COVAR", covar(false)),
		pairSpec("COVARIANCE.P", covar(false)),
		pairSpec("COVARIANCE.S", covar(true)),
		pairSpec("SLOPE", func(y, x []float64) f.Value {
			b, _, err := simpleFit(y, x)
			if err != nil {
				return err
			}
			return f.NumberOrErr(b)
		}),
		pairSpec("INTERCEPT", func(y, x []float64) f.Value {
			_, a, err := simpleFit(y, x)
			if err != nil {
				return err
			}
			return f.NumberOrErr(a)
		}),
		pairSpec("STEYX", steyx),
		f.Spec{Name: "FORECAST", Min: 3, Max: 3, Refs: f.Only(1, 2), Arrays: f.Only(1, 2), Fn: forecast},
		f.Spec{Name: "FORECAST.LINEAR", Min: 3, Max: 3, Refs: f.Only(1, 2), Arrays: f.Only(1, 2), Fn: forecast},
		f.Spec{Name: "LINEST", Min: 1, Max: 4, Arrays: f.Only(0, 1), Fn: linest(false)},
		f.Spec{Name: "LOGEST", Min: 1, Max: 4, Arrays: f.Only(0, 1), Fn: linest(true)},
		f.Spec{Name: "TREND", Min: 1, Max: 4, Arrays: f.Only(0, 1, 2), Fn: trend(false)},
		f.Spec{Name: "GROWTH", Min: 1, Max: 4, Arrays: f.Only(0, 1, 2), Fn: trend(true)},
	)
}

// pairs collects the numbers of two ranges position by position, skipping
// positions where either value is not a number (text, logical, blank).
// Ranges with different numbers of cells are #N/A; errors propagate.
func pairs(e *f.Env, av, bv f.Value) (as, bs []float64, errv f.Value) {
	ga, err := newGrid(e, av)
	if err != nil {
		return nil, nil, err
	}
	gb, err := newGrid(e, bv)
	if err != nil {
		return nil, nil, err
	}
	if ga.rows*ga.cols != gb.rows*gb.cols {
		return nil, nil, f.ErrNA
	}
	add := func(x, y f.Value) f.Value {
		if e, ok := x.(f.Error); ok {
			return e
		}
		if e, ok := y.(f.Error); ok {
			return e
		}
		nx, ok1 := x.(f.Number)
		ny, ok2 := y.(f.Number)
		if ok1 && ok2 {
			as, bs = append(as, float64(nx)), append(bs, float64(ny))
		}
		return nil
	}
	if ga.sameShape(gb) {
		rows, cols := extent(ga, gb)
		for i := 0; i < rows; i++ {
			for j := 0; j < cols; j++ {
				if err := add(ga.at(i, j), gb.at(i, j)); err != nil {
					return nil, nil, err
				}
			}
		}
		return as, bs, nil
	}
	for k := 0; k < ga.rows*ga.cols; k++ {
		if err := add(ga.at(k/ga.cols, k%ga.cols), gb.at(k/gb.cols, k%gb.cols)); err != nil {
			return nil, nil, err
		}
	}
	return as, bs, nil
}

// pairSpec builds FN(array1, array2) over paired numbers.
func pairSpec(name string, g func(a, b []float64) f.Value) f.Spec {
	return f.Spec{Name: name, Min: 2, Max: 2, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
		as, bs, err := pairs(e, a[0], a[1])
		if err != nil {
			return err
		}
		return g(as, bs)
	}}
}

// comoments returns the sums of squared deviations of x and y and of
// their cross products.
func comoments(x, y []float64) (sxx, syy, sxy float64) {
	mx, my := mean(x), mean(y)
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	return
}

func correl(x, y []float64) f.Value {
	if len(x) == 0 {
		return f.ErrDiv0
	}
	sxx, syy, sxy := comoments(x, y)
	if sxx == 0 || syy == 0 {
		return f.ErrDiv0
	}
	return f.NumberOrErr(sxy / math.Sqrt(sxx*syy))
}

func covar(sample bool) func(x, y []float64) f.Value {
	return func(x, y []float64) f.Value {
		n := float64(len(x))
		if sample {
			n--
		}
		if n <= 0 {
			return f.ErrDiv0
		}
		_, _, sxy := comoments(x, y)
		return f.NumberOrErr(sxy / n)
	}
}

// simpleFit fits y = a + b·x by least squares.
func simpleFit(y, x []float64) (b, a float64, errv f.Value) {
	if len(x) == 0 {
		return 0, 0, f.ErrDiv0
	}
	sxx, _, sxy := comoments(x, y)
	if sxx == 0 {
		return 0, 0, f.ErrDiv0
	}
	b = sxy / sxx
	return b, mean(y) - b*mean(x), nil
}

// STEYX(known_y's, known_x's): the standard error of the predicted y.
func steyx(y, x []float64) f.Value {
	n := float64(len(x))
	if n < 3 {
		return f.ErrDiv0
	}
	sxx, syy, sxy := comoments(x, y)
	if sxx == 0 {
		return f.ErrDiv0
	}
	return f.NumberOrErr(math.Sqrt(math.Max(0, syy-sxy*sxy/sxx) / (n - 2)))
}

// FORECAST(x, known_y's, known_x's), FORECAST.LINEAR
func forecast(e *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	ys, xs, err := pairs(e, a[1], a[2])
	if err != nil {
		return err
	}
	b, c, err := simpleFit(ys, xs)
	if err != nil {
		return err
	}
	return f.NumberOrErr(c + b*x)
}

// --- LINEST and friends ---------------------------------------------------

// linData is the data of a least-squares fit: n observations of y and of
// k explanatory variables x[j].
type linData struct {
	y     []float64
	x     [][]float64 // x[j][i]
	byRow bool        // y is a row and each variable a row of known_x's
	yr    *f.Array    // y's shape
}

// numbers converts an array to numbers; anything else is #VALUE!.
func numbers(a *f.Array) ([]float64, f.Value) {
	out := make([]float64, len(a.V))
	for i, v := range a.V {
		switch x := v.(type) {
		case f.Number:
			out[i] = float64(x)
		case f.Error:
			return nil, x
		default:
			return nil, f.ErrValue
		}
	}
	return out, nil
}

// readLinData reads known_y's and known_x's (omitted: 1, 2, 3, ...).
// known_x's may have the shape of known_y's (one variable) or, when
// known_y's is a column (row), one column (row) per variable.
func readLinData(e *f.Env, yv, xv f.Value) (*linData, f.Value) {
	ya := e.ToArray(yv)
	y, err := numbers(ya)
	if err != nil {
		return nil, err
	}
	d := &linData{y: y, yr: ya}
	n := len(y)
	if xv == nil || missing(xv) {
		x := make([]float64, n)
		for i := range x {
			x[i] = float64(i + 1)
		}
		d.x = [][]float64{x}
		return d, nil
	}
	xa := e.ToArray(xv)
	xs, err := numbers(xa)
	if err != nil {
		return nil, err
	}
	switch {
	case xa.Rows == ya.Rows && xa.Cols == ya.Cols:
		d.x = [][]float64{xs}
	case ya.Cols == 1 && xa.Rows == ya.Rows:
		for j := 0; j < xa.Cols; j++ {
			col := make([]float64, n)
			for i := range col {
				col[i] = xs[i*xa.Cols+j]
			}
			d.x = append(d.x, col)
		}
	case ya.Rows == 1 && xa.Cols == ya.Cols:
		d.byRow = true
		for j := 0; j < xa.Rows; j++ {
			d.x = append(d.x, xs[j*n:(j+1)*n])
		}
	default:
		return nil, f.ErrRef
	}
	return d, nil
}

// fit is a least-squares solution.
type fit struct {
	b, se     []float64 // coefficients of x[j] and standard errors (0 for dropped variables)
	a, seA    float64   // intercept and its standard error
	r2, sey   float64
	fstat, df float64
	ssreg     float64
	ssresid   float64
}

// solve fits y = a + Σ b[j]·x[j] (a = 0 unless withConst), as Excel does:
// the data are centred when there is a constant, and variables that are
// collinear with earlier ones (or with the constant) are dropped, getting
// a zero coefficient and standard error.
func (d *linData) solve(withConst bool) (*fit, f.Value) {
	n, k := len(d.y), len(d.x)
	if n == 0 {
		return nil, f.ErrValue
	}
	my := 0.0
	mx := make([]float64, k)
	if withConst {
		my = mean(d.y)
		for j := range mx {
			mx[j] = mean(d.x[j])
		}
	}
	y := make([]float64, n)
	for i := range y {
		y[i] = d.y[i] - my
	}
	// Modified Gram-Schmidt on the (centred) columns: Q is n×p, R p×p
	// for the p kept variables.
	var q [][]float64
	var keep []int
	var rr [][]float64 // rr[c] is column c of R (length c+1)
	for j := 0; j < k; j++ {
		v := make([]float64, n)
		norm0 := 0.0
		for i := range v {
			v[i] = d.x[j][i] - mx[j]
			norm0 += d.x[j][i] * d.x[j][i]
		}
		col := make([]float64, len(q)+1)
		for c, qc := range q {
			dot := 0.0
			for i := range v {
				dot += qc[i] * v[i]
			}
			col[c] = dot
			for i := range v {
				v[i] -= dot * qc[i]
			}
		}
		nv := 0.0
		for _, t := range v {
			nv += t * t
		}
		nv = math.Sqrt(nv)
		if nv == 0 || nv <= 1e-13*math.Sqrt(norm0) {
			continue // collinear
		}
		for i := range v {
			v[i] /= nv
		}
		col[len(q)] = nv
		q = append(q, v)
		rr = append(rr, col)
		keep = append(keep, j)
	}
	p := len(q)
	// Qᵀy, then back substitution R·c = Qᵀy.
	qty := make([]float64, p)
	for c := range q {
		for i := range y {
			qty[c] += q[c][i] * y[i]
		}
	}
	coef := make([]float64, p)
	for c := p - 1; c >= 0; c-- {
		s := qty[c]
		for c2 := c + 1; c2 < p; c2++ {
			s -= rr[c2][c] * coef[c2]
		}
		coef[c] = s / rr[c][c]
	}
	// R⁻¹ (upper triangular) for the covariance (RᵀR)⁻¹ = R⁻¹R⁻ᵀ.
	rinv := make([][]float64, p) // rinv[r][c]
	for r := range rinv {
		rinv[r] = make([]float64, p)
	}
	for c := 0; c < p; c++ {
		rinv[c][c] = 1 / rr[c][c]
		for r := c - 1; r >= 0; r-- {
			s := 0.0
			for m := r + 1; m <= c; m++ {
				s += rr[m][r] * rinv[m][c]
			}
			rinv[r][c] = -s / rr[r][r]
		}
	}
	cov := func(r, c int) float64 { // ((RᵀR)⁻¹)[r][c]
		s := 0.0
		for m := max(r, c); m < p; m++ {
			s += rinv[r][m] * rinv[c][m]
		}
		return s
	}

	ft := &fit{b: make([]float64, k), se: make([]float64, k)}
	for c, j := range keep {
		ft.b[j] = coef[c]
	}
	ft.a = my
	for j := range ft.b {
		ft.a -= ft.b[j] * mx[j]
	}
	for i := 0; i < n; i++ {
		yh := ft.a
		for j := range ft.b {
			yh += ft.b[j] * d.x[j][i]
		}
		res := d.y[i] - yh
		ft.ssresid += res * res
		ft.ssreg += (yh - my) * (yh - my)
	}
	ft.df = float64(n - p)
	if withConst {
		ft.df--
	}
	ft.r2 = 1 // a constant y is fitted exactly
	if sst := ft.ssreg + ft.ssresid; sst > 0 {
		ft.r2 = ft.ssreg / sst
	}
	if ft.df > 0 {
		ft.sey = math.Sqrt(ft.ssresid / ft.df)
	} // else an exact fit with no spare degrees of freedom: sey stays 0
	ft.fstat = (ft.ssreg / float64(p)) / (ft.ssresid / ft.df)
	for c, j := range keep {
		ft.se[j] = ft.sey * math.Sqrt(cov(c, c))
	}
	if withConst {
		// var(a) = σ²(1/n + mxᵀ (XcᵀXc)⁻¹ mx) over the kept variables.
		v := 1 / float64(n)
		for c, j := range keep {
			for c2, j2 := range keep {
				v += mx[j] * cov(c, c2) * mx[j2]
			}
		}
		ft.seA = ft.sey * math.Sqrt(v)
	}
	return ft, nil
}

// logData takes the logarithm of y for LOGEST and GROWTH; y must be
// positive.
func (d *linData) logData() f.Value {
	ly := make([]float64, len(d.y))
	for i, v := range d.y {
		if v <= 0 {
			return f.ErrNum
		}
		ly[i] = math.Log(v)
	}
	d.y = ly
	return nil
}

func optTruth(a []f.Value, i int, def bool) (bool, f.Value) {
	if i >= len(a) || missing(a[i]) {
		return def, nil
	}
	return f.Truth(a[i])
}

func optArg(a []f.Value, i int) f.Value {
	if i >= len(a) {
		return nil
	}
	return a[i]
}

// num is a result cell: NaN and ±Inf are #NUM!.
func num(x float64) f.Value { return f.NumberOrErr(x) }

// linest is LINEST(known_y's, [known_x's], [const], [stats]) and LOGEST:
// one row m_k ... m_1, b; with stats, four more rows (standard errors;
// r², sey; F, df; ssreg, ssresid) padded with #N/A.
func linest(logest bool) func(e *f.Env, a []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		d, err := readLinData(e, a[0], optArg(a, 1))
		if err != nil {
			return err
		}
		withConst, err := optTruth(a, 2, true)
		if err != nil {
			return err
		}
		stats, err := optTruth(a, 3, false)
		if err != nil {
			return err
		}
		if logest {
			if err := d.logData(); err != nil {
				return err
			}
		}
		ft, err := d.solve(withConst)
		if err != nil {
			return err
		}
		k := len(d.x)
		rows := 1
		if stats {
			rows = 5
		}
		out := f.NewArray(rows, k+1)
		for i := range out.V {
			out.V[i] = f.ErrNA
		}
		coef := func(x float64) f.Value {
			if logest {
				return num(math.Exp(x))
			}
			return num(x)
		}
		for j := 0; j < k; j++ {
			out.Set(0, k-1-j, coef(ft.b[j]))
		}
		out.Set(0, k, coef(ft.a))
		if !stats {
			return out
		}
		for j := 0; j < k; j++ {
			out.Set(1, k-1-j, num(ft.se[j]))
		}
		if withConst {
			out.Set(1, k, num(ft.seA))
		}
		out.Set(2, 0, num(ft.r2))
		out.Set(2, 1, num(ft.sey))
		out.Set(3, 0, num(ft.fstat))
		out.Set(3, 1, num(ft.df))
		out.Set(4, 0, num(ft.ssreg))
		out.Set(4, 1, num(ft.ssresid))
		return out
	}
}

// trend is TREND(known_y's, [known_x's], [new_x's], [const]) and GROWTH:
// the fitted values at new_x's (default known_x's).
func trend(growth bool) func(e *f.Env, a []f.Value) f.Value {
	return func(e *f.Env, a []f.Value) f.Value {
		d, err := readLinData(e, a[0], optArg(a, 1))
		if err != nil {
			return err
		}
		withConst, err := optTruth(a, 3, true)
		if err != nil {
			return err
		}
		if growth {
			if err := d.logData(); err != nil {
				return err
			}
		}
		ft, err := d.solve(withConst)
		if err != nil {
			return err
		}
		k := len(d.x)
		// The points to predict: rows of a k-column matrix, or one
		// variable in any shape.
		var pts [][]float64
		var out *f.Array
		nv := optArg(a, 2)
		if nv == nil || missing(nv) {
			out = f.NewArray(d.yr.Rows, d.yr.Cols)
			if k > 1 && d.byRow {
				out = f.NewArray(1, len(d.y))
			} else if k > 1 {
				out = f.NewArray(len(d.y), 1)
			}
			for i := range d.y {
				pt := make([]float64, k)
				for j := range pt {
					pt[j] = d.x[j][i]
				}
				pts = append(pts, pt)
			}
		} else {
			na := e.ToArray(nv)
			xs, err := numbers(na)
			if err != nil {
				return err
			}
			switch {
			case k == 1:
				out = f.NewArray(na.Rows, na.Cols)
				for _, x := range xs {
					pts = append(pts, []float64{x})
				}
			case !d.byRow && na.Cols == k:
				out = f.NewArray(na.Rows, 1)
				for i := 0; i < na.Rows; i++ {
					pts = append(pts, xs[i*k:(i+1)*k])
				}
			case d.byRow && na.Rows == k:
				out = f.NewArray(1, na.Cols)
				for i := 0; i < na.Cols; i++ {
					pt := make([]float64, k)
					for j := range pt {
						pt[j] = xs[j*na.Cols+i]
					}
					pts = append(pts, pt)
				}
			default:
				return f.ErrRef
			}
		}
		for i, pt := range pts {
			v := ft.a
			for j, x := range pt {
				v += ft.b[j] * x
			}
			if growth {
				v = math.Exp(v)
			}
			out.V[i] = num(v)
		}
		return out
	}
}
