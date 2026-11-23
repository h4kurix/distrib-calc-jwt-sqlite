package store_test

import (
	"calc-service/internal/store"
	"calc-service/pkg/database"
	"path/filepath"
	"testing"
)

func setupDB(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	if err := database.InitDB(path); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestClaimAndCompleteTask(t *testing.T) {
	setupDB(t)
	user, err := store.CreateUser("user", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tasks := []*store.Task{
		{ID: "t1", Arg1: "2", Arg2: "3", Operator: "+", OperationTime: 1},
		{ID: "t2", Arg1: "task:t1", Arg2: "4", Operator: "*", OperationTime: 1},
	}
	expr, err := store.CreateExpressionWithTasks("expr-1", "2+3*4", user.ID, tasks)
	if err != nil {
		t.Fatalf("create expression: %v", err)
	}
	if expr.UserID != user.ID {
		t.Fatalf("user id = %q, want %q", expr.UserID, user.ID)
	}

	claimed, ok, err := store.ClaimNextTask()
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if claimed.ID != "t1" {
		t.Fatalf("claimed %q, want t1", claimed.ID)
	}
	if claimed.UserID != user.ID {
		t.Fatalf("claimed user id empty: %q", claimed.UserID)
	}

	second, ok, err := store.ClaimNextTask()
	if err != nil || ok || second != nil {
		t.Fatalf("second claim should be empty: %v %v", second, err)
	}

	if err := store.CompleteTask("t1", 5); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := store.CompleteTask("t1", 999); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}
	got, found, err := store.GetTask("t1")
	if err != nil || !found {
		t.Fatalf("get task: found=%v err=%v", found, err)
	}
	if got.Result != 5 || got.Status != "completed" {
		t.Fatalf("result=%v status=%q, want 5/completed", got.Result, got.Status)
	}

	next, ok, err := store.ClaimNextTask()
	if err != nil || !ok || next.ID != "t2" {
		t.Fatalf("next task: %v ok=%v err=%v", next, ok, err)
	}
}

func TestExpressionOwnershipAndErrors(t *testing.T) {
	setupDB(t)
	user, _ := store.CreateUser("owner", "password123")
	if _, err := store.CreateExpressionWithTasks("expr-2", "1+1", user.ID, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, found, err := store.GetExpression("expr-2")
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if got.UserID != user.ID {
		t.Fatalf("user id = %q", got.UserID)
	}
	if err := store.UpdateExpressionStatus("expr-2", "completed", 2, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, found, _ := store.GetExpression("missing"); found {
		t.Fatal("missing expression reported as found")
	}
}
