package formula

import "testing"

func TestRoundDigits(t *testing.T) {
	for _, c := range []struct {
		x    float64
		d    int
		mode RoundMode
		want float64
	}{
		{1.005, 2, RoundHalf, 1.01},
		{-1.005, 2, RoundHalf, -1.01},
		{2.5, 0, RoundHalf, 3},
		{999.95, 1, RoundHalf, 1000},
		{1234.5, -2, RoundHalf, 1200},
		{0.001, 0, RoundUp, 1},
		{0.001, 0, RoundHalf, 0},
		{3.14159, 3, RoundDown, 3.141},
		{0.1 + 0.2, 20, RoundHalf, 0.3},
	} {
		if got := RoundDigits(c.x, c.d, c.mode); got != c.want {
			t.Errorf("RoundDigits(%v, %d, %d) = %v, want %v", c.x, c.d, c.mode, got, c.want)
		}
	}
	if got := Round15(1.3 / 0.2); got != 6.5 {
		t.Errorf("Round15(1.3/0.2) = %v", got)
	}
}
