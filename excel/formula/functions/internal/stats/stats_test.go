package stats

import "testing"

func TestVariance(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	if s := Sum(xs); s != 40 {
		t.Errorf("Sum = %v", s)
	}
	if v, ok := Variance(xs, 0); !ok || v != 4 {
		t.Errorf("population variance = %v %v", v, ok)
	}
	if v, ok := Variance(xs, 1); !ok || v != 32.0/7 {
		t.Errorf("sample variance = %v %v", v, ok)
	}
	if _, ok := Variance([]float64{1}, 1); ok {
		t.Error("sample variance of one value")
	}
	if _, ok := Variance(nil, 0); ok {
		t.Error("population variance of nothing")
	}
}
