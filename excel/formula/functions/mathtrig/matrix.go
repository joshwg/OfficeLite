package mathtrig

import (
	"math"

	f "officelite/excel/formula"
)

func registerMatrix(r *f.Registry) {
	r.Add(
		f.Spec{Name: "MMULT", Min: 2, Max: 2, Arrays: f.All, Fn: mmult},
		f.Spec{Name: "MDETERM", Min: 1, Max: 1, Arrays: f.All, Fn: mdeterm},
		f.Spec{Name: "MINVERSE", Min: 1, Max: 1, Arrays: f.All, Fn: minverse},
		f.Spec{Name: "MUNIT", Min: 1, Max: 1, Fn: munit},
	)
}

// matrix converts an argument to rows of numbers; blanks, text and
// booleans are #VALUE!, errors propagate.
func matrix(e *f.Env, v f.Value) ([][]float64, f.Value) {
	a := e.ToArray(v)
	m := make([][]float64, a.Rows)
	for r := range m {
		m[r] = make([]float64, a.Cols)
		for c := range m[r] {
			switch x := a.At(r, c).(type) {
			case f.Number:
				m[r][c] = float64(x)
			case f.Error:
				return nil, x
			default:
				return nil, f.ErrValue
			}
		}
	}
	return m, nil
}

func toArray(m [][]float64) f.Value {
	out := &f.Array{Rows: len(m), Cols: len(m[0])}
	for _, row := range m {
		for _, x := range row {
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return f.ErrNum
			}
			out.V = append(out.V, f.Number(x))
		}
	}
	return out
}

// MMULT(array1, array2): array1's column count must equal array2's row
// count.
func mmult(e *f.Env, a []f.Value) f.Value {
	x, err := matrix(e, a[0])
	if err != nil {
		return err
	}
	y, err := matrix(e, a[1])
	if err != nil {
		return err
	}
	if len(x[0]) != len(y) {
		return f.ErrValue
	}
	out := make([][]float64, len(x))
	for i := range x {
		out[i] = make([]float64, len(y[0]))
		for j := range y[0] {
			for k := range y {
				out[i][j] += x[i][k] * y[k][j]
			}
		}
	}
	return toArray(out)
}

func square(e *f.Env, v f.Value) ([][]float64, f.Value) {
	m, err := matrix(e, v)
	if err != nil {
		return nil, err
	}
	if len(m) != len(m[0]) {
		return nil, f.ErrValue
	}
	return m, nil
}

// MDETERM(array), by LU decomposition with partial pivoting.
func mdeterm(e *f.Env, a []f.Value) f.Value {
	m, err := square(e, a[0])
	if err != nil {
		return err
	}
	n := len(m)
	det := 1.0
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		if m[p][c] == 0 {
			return f.Number(0)
		}
		if p != c {
			m[p], m[c] = m[c], m[p]
			det = -det
		}
		det *= m[c][c]
		for r := c + 1; r < n; r++ {
			k := m[r][c] / m[c][c]
			for j := c; j < n; j++ {
				m[r][j] -= k * m[c][j]
			}
		}
	}
	return f.NumberOrErr(det)
}

// MINVERSE(array), by Gauss-Jordan elimination; a singular matrix is
// #NUM!.
func minverse(e *f.Env, a []f.Value) f.Value {
	m, err := square(e, a[0])
	if err != nil {
		return err
	}
	n := len(m)
	inv := identity(n)
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		if m[p][c] == 0 {
			return f.ErrNum
		}
		m[p], m[c] = m[c], m[p]
		inv[p], inv[c] = inv[c], inv[p]
		d := m[c][c]
		for j := 0; j < n; j++ {
			m[c][j] /= d
			inv[c][j] /= d
		}
		for r := 0; r < n; r++ {
			if r == c || m[r][c] == 0 {
				continue
			}
			k := m[r][c]
			for j := 0; j < n; j++ {
				m[r][j] -= k * m[c][j]
				inv[r][j] -= k * inv[c][j]
			}
		}
	}
	return toArray(inv)
}

func identity(n int) [][]float64 {
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		m[i][i] = 1
	}
	return m
}

// MUNIT(dimension): the identity matrix.
func munit(e *f.Env, a []f.Value) f.Value {
	x, err := f.Num(a[0])
	if err != nil {
		return err
	}
	n := int(math.Trunc(x))
	if x < 1 || n > maxCols {
		return f.ErrValue
	}
	return toArray(identity(n))
}
