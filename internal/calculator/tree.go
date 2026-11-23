package calculator

import (
	"calc-service/internal/id"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
	"os"
	"strconv"
)

// Node — узел дерева выражения.
type Node struct {
	Value  string
	Left   *Node
	Right  *Node
	TaskID string
}

// operationTimeFallback используется, когда переменная окружения не задана.
var operationTimeFallback = map[string]int{"+": 100, "-": 100, "*": 200, "/": 300}

func generateTaskID() string { return id.New("task") }

func getNodeReference(n *Node) string {
	if n == nil {
		return ""
	}
	if isOperator(n.Value) {
		return "task:" + n.TaskID
	}
	return n.Value
}

func getOperationTime(op string) int {
	var envVar string
	switch op {
	case "+":
		envVar = os.Getenv("TIME_ADDITION_MS")
	case "-":
		envVar = os.Getenv("TIME_SUBTRACTION_MS")
	case "*":
		envVar = os.Getenv("TIME_MULTIPLICATIONS_MS")
	case "/":
		envVar = os.Getenv("TIME_DIVISIONS_MS")
	default:
		return 0
	}

	fallback := operationTimeFallback[op]
	t, err := strconv.Atoi(envVar)
	if err != nil {
		return fallback
	}
	if t < 0 || t > maxOperationMS {
		logger.Warn("operation time %d for %s is out of range, using %d", t, op, fallback)
		return fallback
	}
	return t
}

func createTasksFromTree(exprID string, node *Node) ([]*store.Task, error) {
	if node == nil {
		return nil, nil
	}
	var tasks []*store.Task
	if node.Left != nil {
		child, err := createTasksFromTree(exprID, node.Left)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, child...)
	}
	if node.Right != nil {
		child, err := createTasksFromTree(exprID, node.Right)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, child...)
	}
	if isOperator(node.Value) {
		node.TaskID = generateTaskID()
		tasks = append(tasks, &store.Task{
			ID:            node.TaskID,
			ExpressionID:  exprID,
			Arg1:          getNodeReference(node.Left),
			Arg2:          getNodeReference(node.Right),
			Operator:      node.Value,
			OperationTime: getOperationTime(node.Value),
		})
	}
	return tasks, nil
}
