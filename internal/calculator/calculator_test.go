package calculator

import (
	"calc-service/internal/store"
	"strconv"
	"strings"
	"testing"
)

func TestTokenizeRuneSafety(t *testing.T) {
	tokens, err := tokenize("١٢+٣")
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("got %d tokens, want 3", len(tokens))
	}
}

func TestValidateExpression(t *testing.T) {
	cases := []struct {
		expr    string
		wantErr bool
	}{
		{"2+2", false},
		{"-2+3", false},
		{"2*(3+4)", false},
		{"2..3", true},
		{"2+", false},
		{"2&3", true},
		{"(2+3", true},
		{"2+3)", true},
	}
	for _, tc := range cases {
		err := ValidateExpression(tc.expr)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateExpression(%q) = %v, wantErr %v", tc.expr, err, tc.wantErr)
		}
	}
}

func evalNode(t *testing.T, n *Node) float64 {
	t.Helper()
	if n == nil {
		t.Fatal("nil node")
	}
	if !isOperator(n.Value) {
		v, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			t.Fatalf("parse %q: %v", n.Value, err)
		}
		return v
	}
	left, right := evalNode(t, n.Left), evalNode(t, n.Right)
	switch n.Value {
	case "+":
		return left + right
	case "-":
		return left - right
	case "*":
		return left * right
	case "/":
		return left / right
	default:
		t.Fatalf("unknown operator %q", n.Value)
		return 0
	}
}

func TestBuildTree(t *testing.T) {
	cases := []struct {
		expr string
		want float64
	}{
		{"2+2", 4},
		{"-2+3", 1},
		{"2*(3+4)", 14},
		{"10-3*2", 4},
		{"-(2+3)", -5},
	}
	for _, tc := range cases {
		tokens, err := tokenize(tc.expr)
		if err != nil {
			t.Fatalf("tokenize %q: %v", tc.expr, err)
		}
		root, err := buildExpressionTree(tokens)
		if err != nil {
			t.Fatalf("tree %q: %v", tc.expr, err)
		}
		if got := evalNode(t, root); got != tc.want {
			t.Errorf("%q = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestAggregateResultsRoot(t *testing.T) {
	tasks := []*store.Task{
		{ID: "t1", Operator: "+", Arg1: "2", Arg2: "3", Status: "completed", Completed: true, Result: 5},
		{ID: "t2", Operator: "*", Arg1: "task:t1", Arg2: "4", Status: "completed", Completed: true, Result: 20},
	}
	tasks[0], tasks[1] = tasks[1], tasks[0]
	got, err := AggregateResults(tasks)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if got != 20 {
		t.Fatalf("got %v, want 20", got)
	}
}

func TestAggregateDivisionByZero(t *testing.T) {
	tasks := []*store.Task{
		{ID: "t1", Operator: "/", Arg1: "1", Arg2: "0", Status: "completed", Completed: true, Result: 0},
	}
	_, err := AggregateResults(tasks)
	if err == nil || !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("want division by zero, got %v", err)
	}
}
