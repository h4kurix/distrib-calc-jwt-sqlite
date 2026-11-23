package config

import "testing"

func TestNonNegativeIntDefault(t *testing.T) {
	const key = "TEST_OPERATION_MS"
	cases := []struct {
		value string
		want  int
	}{
		{"", 100},
		{"0", 0},
		{"5", 5},
		{"-1", 100},
		{"bad", 100},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(key, tc.value)
			if got := nonNegativeIntDefault(key, 100); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
