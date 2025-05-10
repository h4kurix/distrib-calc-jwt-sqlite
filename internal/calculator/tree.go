package calculator

import (
	"calc-service/internal/store"
	"os"
	"strconv"

	"github.com/google/uuid"
)

// Синтаксический анализ: построение дерева выражения
type Node struct {
	Value    string
	Left     *Node
	Right    *Node
	TaskID   string
	Priority int
}

func generateTaskID() string {
	return "task-" + uuid.New().String()
}

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
	t, err := strconv.Atoi(envVar)
	if err != nil {
		switch op {
		case "+", "-":
			return 100
		case "*":
			return 200
		case "/":
			return 300
		default:
			return 0
		}
	}
	return t
}

func createTasksFromTree(exprID string, node *Node) ([]*store.Task, error) {
	var tasks []*store.Task
	if node == nil {
		return tasks, nil
	}
	// обход в пост-ордера
	if node.Left != nil {
		childTasks, err := createTasksFromTree(exprID, node.Left)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, childTasks...)
	}
	if node.Right != nil {
		childTasks, err := createTasksFromTree(exprID, node.Right)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, childTasks...)
	}
	if isOperator(node.Value) {
		taskID := generateTaskID()
		node.TaskID = taskID
		arg1 := getNodeReference(node.Left)
		arg2 := getNodeReference(node.Right)
		task := &store.Task{
			ID:            taskID,
			ExpressionID:  exprID,
			Arg1:          arg1,
			Arg2:          arg2,
			Operator:      node.Value,
			OperationTime: getOperationTime(node.Value),
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}
