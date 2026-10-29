package numfmt

import (
	"slices"
	"testing"
)

func TestSplitSections(t *testing.T) {
	for _, c := range []struct {
		code string
		want []string
	}{
		{"General", []string{"General"}},
		{"0.00;[Red]-0.00;0;@", []string{"0.00", "[Red]-0.00", "0", "@"}},
		{`#,##0_);(#,##0)`, []string{"#,##0_)", "(#,##0)"}},
		{`0" ;x";0`, []string{`0" ;x"`, "0"}},
		{`0\;0;-0`, []string{`0\;0`, "-0"}},
		{`0_;;0`, []string{"0_;", "0"}},
		{`0*;;-0`, []string{"0*;", "-0"}},
		{`[$-x;y]0;-0`, []string{"[$-x;y]0", "-0"}},
		{"0;;", []string{"0", "", ""}},
	} {
		if got := SplitSections(c.code); !slices.Equal(got, c.want) {
			t.Errorf("SplitSections(%q) = %q, want %q", c.code, got, c.want)
		}
	}
}
