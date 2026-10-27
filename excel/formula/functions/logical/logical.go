// Package logical implements Excel's logical functions: IF, IFS, SWITCH,
// IFERROR, IFNA, AND, OR, XOR, NOT, TRUE, FALSE and the LAMBDA helpers MAP,
// REDUCE, SCAN, BYROW, BYCOL and MAKEARRAY. LET and LAMBDA themselves are
// built into the evaluator.
package logical

import (
	f "officelite/excel/formula"
)

// Register adds the logical functions to r.
func Register(r *f.Registry) {
	r.Add(
		f.Spec{Name: "IF", Min: 1, Max: 3, Lazy: ifFn},
		f.Spec{Name: "IFS", Min: 2, Max: f.Variadic, Lazy: ifsFn},
		f.Spec{Name: "SWITCH", Min: 3, Max: f.Variadic, Lazy: switchFn},
		f.Spec{Name: "IFERROR", Min: 2, Max: 2, Lazy: func(e *f.Env, a []f.Node) f.Value {
			return ifErr(e, a, func(err f.Error) bool { return true })
		}},
		f.Spec{Name: "IFNA", Min: 2, Max: 2, Lazy: func(e *f.Env, a []f.Node) f.Value {
			return ifErr(e, a, func(err f.Error) bool { return err == f.ErrNA })
		}},
		f.Spec{Name: "AND", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return combine(e, a, func(acc, b bool) bool { return acc && b }, true)
		}},
		f.Spec{Name: "OR", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return combine(e, a, func(acc, b bool) bool { return acc || b }, false)
		}},
		f.Spec{Name: "XOR", Min: 1, Max: f.Variadic, Refs: f.All, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return combine(e, a, func(acc, b bool) bool { return acc != b }, false)
		}},
		f.Spec{Name: "NOT", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			b, err := f.Truth(a[0])
			if err != nil {
				return err
			}
			return f.Bool(!b)
		}},
		f.Spec{Name: "TRUE", Min: 0, Max: 0, Fn: func(*f.Env, []f.Value) f.Value { return f.Bool(true) }},
		f.Spec{Name: "FALSE", Min: 0, Max: 0, Fn: func(*f.Env, []f.Value) f.Value { return f.Bool(false) }},
		f.Spec{Name: "MAP", Min: 2, Max: f.Variadic, Arrays: f.All, Fn: mapFn},
		f.Spec{Name: "REDUCE", Min: 3, Max: 3, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return reduce(e, a, false)
		}},
		f.Spec{Name: "SCAN", Min: 3, Max: 3, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return reduce(e, a, true)
		}},
		f.Spec{Name: "BYROW", Min: 2, Max: 2, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return byLine(e, a, true)
		}},
		f.Spec{Name: "BYCOL", Min: 2, Max: 2, Arrays: f.All, Fn: func(e *f.Env, a []f.Value) f.Value {
			return byLine(e, a, false)
		}},
		f.Spec{Name: "MAKEARRAY", Min: 3, Max: 3, Fn: makeArray, Arrays: f.Only(2)},
	)
}

// multi reports whether v (dereferenced) is an array with more than one
// element, i.e. whether a condition must be applied element-wise.
func multi(v f.Value) (*f.Array, bool) {
	a, ok := v.(*f.Array)
	return a, ok && !a.Single()
}

func scalar(v f.Value) f.Value {
	if a, ok := v.(*f.Array); ok && a.Single() {
		return a.V[0]
	}
	return v
}

// elementwise builds an array the size of the largest input by calling fn
// with each input's broadcast element.
func elementwise(inputs []f.Value, fn func(vals []f.Value) f.Value) f.Value {
	rows, cols := 1, 1
	for _, v := range inputs {
		if a, ok := v.(*f.Array); ok {
			rows, cols = max(rows, a.Rows), max(cols, a.Cols)
		}
	}
	out := &f.Array{Rows: rows, Cols: cols}
	vals := make([]f.Value, len(inputs))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			for i, v := range inputs {
				if a, ok := v.(*f.Array); ok {
					v = a.Broadcast(r, c)
				}
				vals[i] = v
			}
			out.V = append(out.V, scalar(fn(vals)))
		}
	}
	return out
}

func argOr(e *f.Env, args []f.Node, i int, def f.Value) f.Value {
	if i >= len(args) {
		return def
	}
	v := e.Eval(args[i])
	if _, ok := v.(f.Missing); ok {
		return f.Number(0) // IF(A1,,1): an empty argument is 0
	}
	return v
}

// IF(logical_test, [value_if_true], [value_if_false])
func ifFn(e *f.Env, args []f.Node) f.Value {
	cond := e.Deref(e.Eval(args[0]))
	if a, ok := multi(cond); ok {
		t := e.Deref(argOr(e, args, 1, f.Bool(true)))
		fv := e.Deref(argOr(e, args, 2, f.Bool(false)))
		return elementwise([]f.Value{a, t, fv}, func(v []f.Value) f.Value {
			b, err := f.Truth(v[0])
			if err != nil {
				return err
			}
			if b {
				return v[1]
			}
			return v[2]
		})
	}
	b, err := f.Truth(scalar(cond))
	if err != nil {
		return err
	}
	if b {
		return argOr(e, args, 1, f.Bool(true))
	}
	return argOr(e, args, 2, f.Bool(false))
}

// IFS(test1, value1, [test2, value2], ...)
func ifsFn(e *f.Env, args []f.Node) f.Value {
	if len(args)%2 != 0 {
		return f.ErrValue
	}
	// Scalar conditions are evaluated lazily, in order.
	conds := make([]f.Value, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		c := e.Deref(e.Eval(args[i]))
		if _, ok := multi(c); ok {
			return ifsArray(e, args, append(conds, c))
		}
		conds = append(conds, c)
		b, err := f.Truth(scalar(c))
		if err != nil {
			return err
		}
		if b {
			return argOr(e, args, i+1, f.Number(0))
		}
	}
	return f.ErrNA
}

func ifsArray(e *f.Env, args []f.Node, conds []f.Value) f.Value {
	for i := len(conds) * 2; i < len(args); i += 2 {
		conds = append(conds, e.Deref(e.Eval(args[i])))
	}
	inputs := append([]f.Value(nil), conds...)
	for i := 1; i < len(args); i += 2 {
		inputs = append(inputs, e.Deref(argOr(e, args, i, f.Number(0))))
	}
	n := len(conds)
	return elementwise(inputs, func(v []f.Value) f.Value {
		for i := 0; i < n; i++ {
			b, err := f.Truth(v[i])
			if err != nil {
				return err
			}
			if b {
				return v[n+i]
			}
		}
		return f.ErrNA
	})
}

// SWITCH(expression, value1, result1, [value2, result2], ..., [default])
func switchFn(e *f.Env, args []f.Node) f.Value {
	x := e.Deref(e.Eval(args[0]))
	pairs := (len(args) - 1) / 2
	hasDefault := (len(args)-1)%2 == 1
	if a, ok := multi(x); ok {
		inputs := []f.Value{a}
		for i := 1; i < len(args); i++ {
			inputs = append(inputs, e.Deref(argOr(e, args, i, f.Number(0))))
		}
		return elementwise(inputs, func(v []f.Value) f.Value {
			for p := 0; p < pairs; p++ {
				if f.Equal(v[0], v[1+2*p]) {
					return v[2+2*p]
				}
			}
			if hasDefault {
				return v[len(v)-1]
			}
			return f.ErrNA
		})
	}
	x = scalar(x)
	if err, ok := x.(f.Error); ok {
		return err
	}
	for p := 0; p < pairs; p++ {
		v := e.Scalar(e.Eval(args[1+2*p]))
		if f.Equal(x, v) {
			return argOr(e, args, 2+2*p, f.Number(0))
		}
	}
	if hasDefault {
		return argOr(e, args, len(args)-1, f.Number(0))
	}
	return f.ErrNA
}

// IFERROR / IFNA: replace (matching) errors with the alternative.
func ifErr(e *f.Env, args []f.Node, matches func(f.Error) bool) f.Value {
	raw := e.Eval(args[0])
	v := e.Deref(raw)
	if a, ok := multi(v); ok {
		var alt f.Value
		out := &f.Array{Rows: a.Rows, Cols: a.Cols, V: make([]f.Value, len(a.V))}
		for i, el := range a.V {
			if err, isErr := el.(f.Error); isErr && matches(err) {
				if alt == nil {
					alt = e.Scalar(argOr(e, args, 1, f.Number(0)))
				}
				el = alt
			}
			out.V[i] = el
		}
		return out
	}
	if err, ok := scalar(v).(f.Error); ok && matches(err) {
		return argOr(e, args, 1, f.Number(0))
	}
	return raw
}

// combine implements AND/OR/XOR. Booleans and numbers count; text and
// blanks inside ranges/arrays are ignored, text passed directly must be
// TRUE/FALSE. With no logical values at all the result is #VALUE!.
func combine(e *f.Env, args []f.Value, op func(acc, b bool) bool, start bool) f.Value {
	acc, seen := start, false
	var errv f.Value
	for _, a := range args {
		e.Each(a, func(v f.Value, direct bool) bool {
			switch x := v.(type) {
			case f.Error:
				errv = x
				return false
			case f.Bool, f.Number:
				b, _ := f.Truth(x)
				acc, seen = op(acc, b), true
			case f.String:
				if direct {
					b, err := f.Truth(x)
					if err != nil {
						errv = err
						return false
					}
					acc, seen = op(acc, b), true
				}
			}
			return true
		})
		if errv != nil {
			return errv
		}
	}
	if !seen {
		return f.ErrValue
	}
	return f.Bool(acc)
}

func lambdaArg(v f.Value) (*f.Lambda, f.Value) {
	if l, ok := v.(*f.Lambda); ok {
		return l, nil
	}
	if err, ok := v.(f.Error); ok {
		return nil, err
	}
	return nil, f.ErrValue
}

// lambdaResult reduces a lambda's result to a scalar; arrays are #CALC!.
func lambdaResult(e *f.Env, v f.Value) f.Value {
	v = e.Deref(v)
	if a, ok := v.(*f.Array); ok {
		if a.Single() {
			return a.V[0]
		}
		return f.ErrCalc
	}
	return v
}

// MAP(array1, [array2, ...], lambda)
func mapFn(e *f.Env, args []f.Value) f.Value {
	l, err := lambdaArg(args[len(args)-1])
	if err != nil {
		return err
	}
	arrays := make([]f.Value, len(args)-1)
	for i, a := range args[:len(args)-1] {
		arrays[i] = e.ToArray(a)
	}
	return elementwise(arrays, func(v []f.Value) f.Value {
		return lambdaResult(e, e.CallLambda(l, append([]f.Value(nil), v...)...))
	})
}

// REDUCE / SCAN([initial_value], array, lambda(accumulator, value))
func reduce(e *f.Env, args []f.Value, scan bool) f.Value {
	l, err := lambdaArg(args[2])
	if err != nil {
		return err
	}
	acc := args[0]
	if a, ok := acc.(*f.Array); ok {
		if !a.Single() {
			return f.ErrValue
		}
		acc = a.V[0]
	}
	arr := e.ToArray(args[1])
	out := &f.Array{Rows: arr.Rows, Cols: arr.Cols}
	for _, v := range arr.V {
		acc = e.CallLambda(l, acc, v)
		if scan {
			acc = lambdaResult(e, acc)
			out.V = append(out.V, acc)
		}
	}
	if scan {
		return out
	}
	return e.Deref(acc)
}

// BYROW / BYCOL(array, lambda(row_or_column))
func byLine(e *f.Env, args []f.Value, rows bool) f.Value {
	l, err := lambdaArg(args[1])
	if err != nil {
		return err
	}
	a := e.ToArray(args[0])
	var out *f.Array
	if rows {
		out = &f.Array{Rows: a.Rows, Cols: 1}
		for r := 0; r < a.Rows; r++ {
			line := &f.Array{Rows: 1, Cols: a.Cols, V: append([]f.Value(nil), a.Row(r)...)}
			out.V = append(out.V, lambdaResult(e, e.CallLambda(l, line)))
		}
		return out
	}
	out = &f.Array{Rows: 1, Cols: a.Cols}
	for c := 0; c < a.Cols; c++ {
		line := &f.Array{Rows: a.Rows, Cols: 1}
		for r := 0; r < a.Rows; r++ {
			line.V = append(line.V, a.At(r, c))
		}
		out.V = append(out.V, lambdaResult(e, e.CallLambda(l, line)))
	}
	return out
}

// MAKEARRAY(rows, cols, lambda(row, col))
func makeArray(e *f.Env, args []f.Value) f.Value {
	rows, err := f.Int(args[0])
	if err != nil {
		return err
	}
	cols, err := f.Int(args[1])
	if err != nil {
		return err
	}
	if rows < 1 || cols < 1 || rows*cols > 1<<24 {
		return f.ErrValue
	}
	l, err := lambdaArg(args[2])
	if err != nil {
		return err
	}
	out := &f.Array{Rows: rows, Cols: cols}
	for r := 1; r <= rows; r++ {
		for c := 1; c <= cols; c++ {
			out.V = append(out.V, lambdaResult(e, e.CallLambda(l, f.Number(r), f.Number(c))))
		}
	}
	return out
}
