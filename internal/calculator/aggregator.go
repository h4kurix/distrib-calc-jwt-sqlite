package calculator

import (
	"calc-service/internal/store"
	"fmt"
	"strconv"
	"strings"
)

// AggregateResults вычисляет результат выражения по результатам задач.
func AggregateResults(tasks []*store.Task) (float64, error) {
	results := make(map[string]float64, len(tasks))
	for _, t := range tasks {
		if t.Status != "completed" && !t.Completed {
			return 0, fmt.Errorf("task %s is not completed", t.ID)
		}
		results[t.ID] = t.Result
	}

	root, err := getRootNode(tasks)
	if err != nil {
		return 0, err
	}
	return evaluateWithResults(root, results)
}

// getRootNode находит задачу, которая не является зависимостью другой задачи.
func getRootNode(tasks []*store.Task) (*Node, error) {
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks to aggregate")
	}

	nodes := make(map[string]*Node, len(tasks))
	referenced := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		nodes[t.ID] = &Node{Value: t.Operator, TaskID: t.ID}
	}
	for _, t := range tasks {
		n := nodes[t.ID]
		if strings.HasPrefix(t.Arg1, "task:") {
			id := strings.TrimPrefix(t.Arg1, "task:")
			referenced[id] = true
			n.Left = nodes[id]
		} else {
			n.Left = &Node{Value: t.Arg1}
		}
		if strings.HasPrefix(t.Arg2, "task:") {
			id := strings.TrimPrefix(t.Arg2, "task:")
			referenced[id] = true
			n.Right = nodes[id]
		} else {
			n.Right = &Node{Value: t.Arg2}
		}
	}

	root := nodes[tasks[len(tasks)-1].ID]
	for _, t := range tasks {
		if !referenced[t.ID] {
			root = nodes[t.ID]
			break
		}
	}
	if root == nil {
		return nil, fmt.Errorf("could not find root task")
	}
	return root, nil
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
