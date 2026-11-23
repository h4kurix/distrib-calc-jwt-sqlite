package calculator

import (
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
