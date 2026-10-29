// Tests for the paint helpers.

package gui

import (
	"testing"

	"gioui.org/op"

	"officelite/common/colors"
)

// TestDashedLineBadPattern checks that a dash pattern whose step doesn't
// advance draws nothing instead of looping forever.
func TestDashedLineBadPattern(t *testing.T) {
	var ops op.Ops
	DashedLine(&ops, 0, 0, 100, 0, 1, 2, -2, colors.Black)
	DashedLine(&ops, 0, 0, 0, 100, 1, 3, -5, colors.Black)
}
