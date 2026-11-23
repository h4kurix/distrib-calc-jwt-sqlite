package calculator

import (
	"math"
	"testing"
)

// Unary minus must bind to the operand, not to everything to its left.
func TestUnaryMinusBinding(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"2*-3", -6},
		{"10/-2", -5},
		{"2*-3+1", -5},
		{"-2*3", -6},
		{"--2", 2},
		{"3--2", 5},
		{"-2+-3", -5},
		{"2*(-3+1)", -4},
		{"2/(1-3)", -1},
	}
	for _, tc := range cases {
		if err := ValidateExpression(tc.expr); err != nil {
			t.Fatalf("ValidateExpression(%q): %v", tc.expr, err)
		}
		tokens, err := tokenize(tc.expr)
		if err != nil {
			t.Fatalf("tokenize %q: %v", tc.expr, err)
		}
		root, err := buildExpressionTree(tokens)
		if err != nil {
			t.Fatalf("tree %q: %v", tc.expr, err)
		}
		got := evalNode(t, root)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%q = %v, want %v", tc.expr, got, tc.want)
		}
	}
}
