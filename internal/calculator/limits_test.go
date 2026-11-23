package calculator

import (
	"context"
	"maps"
	"testing"
)

func TestConfigureAllowsZeroOperationTime(t *testing.T) {
	previous := maps.Clone(operationTimes)
	t.Cleanup(func() { operationTimes = previous })

	Configure(Limits{
		AdditionMS:       0,
		SubtractionMS:    0,
		MultiplicationMS: 0,
		DivisionMS:       0,
	})
	for _, op := range []string{"+", "-", "*", "/"} {
		if got := getOperationTime(op); got != 0 {
			t.Fatalf("operation %s: got %d, want 0", op, got)
		}
	}
}

func TestProcessExpressionRequiresOperation(t *testing.T) {
	for _, expr := range []string{"1", "(123)", "((((1))))"} {
		if _, err := ProcessExpression(nil, context.Background(), expr, "user-1"); err == nil {
			t.Fatalf("expression %q accepted without operations", expr)
		}
	}
}
