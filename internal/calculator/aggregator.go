package calculator

import (
	"calc-service/internal/store"
	"fmt"
	"strconv"
	"strings"
)

func AggregateResults(tasks []*store.Task) (float64, error) {
	results := make(map[string]float64)
	for _, task := range tasks {
		if !task.Completed {
			return 0, fmt.Errorf("task %s is not completed yet", task.ID)
		}
		results[task.ID] = task.Result
	}

	root, err := getRootNode(tasks)
	if err != nil {
		return 0, err
	}
	return evaluateWithResults(root, results)
}

func getRootNode(tasks []*store.Task) (*Node, error) {
	nodes := make(map[string]*Node)
	for _, t := range tasks {
		nodes[t.ID] = &Node{
			Value:  t.Operator,
			TaskID: t.ID,
		}
	}

	for _, t := range tasks {
		n := nodes[t.ID]
		if strings.HasPrefix(t.Arg1, "task:") {
			childID := strings.TrimPrefix(t.Arg1, "task:")
			n.Left = nodes[childID]
		} else {
			n.Left = &Node{Value: t.Arg1}
		}

		if strings.HasPrefix(t.Arg2, "task:") {
			childID := strings.TrimPrefix(t.Arg2, "task:")
			n.Right = nodes[childID]
		} else {
			n.Right = &Node{Value: t.Arg2}
		}
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks to aggregate")
	}
	return nodes[tasks[len(tasks)-1].ID], nil
}

func evaluateWithResults(n *Node, results map[string]float64) (float64, error) {
	if n == nil {
		return 0, fmt.Errorf("empty node")
	}
	if !isOperator(n.Value) {
		if val, ok := results[n.TaskID]; ok {
			return val, nil
		}
		return strconv.ParseFloat(n.Value, 64)
	}
	left, err := evaluateWithResults(n.Left, results)
	if err != nil {
		return 0, err
	}
	right, err := evaluateWithResults(n.Right, results)
	if err != nil {
		return 0, err
	}
	switch n.Value {
	case "+":
		return left + right, nil
	case "-":
		return left - right, nil
	case "*":
		return left * right, nil
	case "/":
		if right == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return left / right, nil
	default:
		return 0, fmt.Errorf("unknown operator %s", n.Value)
	}
}
