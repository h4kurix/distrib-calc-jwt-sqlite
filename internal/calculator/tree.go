package calculator

import (
	"calc-service/internal/id"
	"calc-service/internal/store"
	"calc-service/pkg/logger"
)

// Node — узел дерева выражения.
type Node struct {
	Value  string
	Left   *Node
	Right  *Node
	TaskID string
}

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
	t, ok := operationTimes[op]
	if !ok {
		return 0
	}
	if t < 0 || t > maxOperationMS {
		fallback := defaultOperationTimes[op]
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
