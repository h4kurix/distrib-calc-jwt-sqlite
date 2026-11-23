package store_test

import (
	"calc-service/internal/store"
	"calc-service/pkg/database"
	"path/filepath"
	"sync"
	"testing"
	"time"
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

func TestConcurrentFinalize(t *testing.T) {
	setupDB(t)
	user, _ := store.CreateUser("race", "password123")
	tasks := []*store.Task{
		{ID: "r1", Arg1: "2", Arg2: "3", Operator: "+", OperationTime: 1},
		{ID: "r2", Arg1: "task:r1", Arg2: "4", Operator: "*", OperationTime: 1},
	}
	if _, err := store.CreateExpressionWithTasks("expr-race", "2+3*4", user.ID, tasks); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 2; i++ {
		claimed, ok, err := store.ClaimNextTask()
		if err != nil || !ok {
			t.Fatalf("claim %d: ok=%v err=%v", i, ok, err)
		}
		if err := store.CompleteTask(claimed.ID, float64(i+2)); err != nil {
			t.Fatalf("complete: %v", err)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.FinalizeExpression("expr-race", user.ID, func([]*store.Task) (float64, error) {
				return 14, nil
			})
		}()
	}
	wg.Wait()

	expr, found, err := store.GetExpression("expr-race")
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if expr.Status != "completed" || expr.Result != 14 {
		t.Fatalf("status=%q result=%v, want completed/14", expr.Status, expr.Result)
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

// Зависшее выражение должно закрываться по таймауту, а не висеть вечно.
func TestExpireStaleExpressions(t *testing.T) {
	setupDB(t)
	user, err := store.CreateUser("owner", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := store.CreateExpressionWithTasks("expr-stale", "1+1", user.ID, nil); err != nil {
		t.Fatalf("create expression: %v", err)
	}
	if _, err := store.CreateExpressionWithTasks("expr-done", "2+2", user.ID, nil); err != nil {
		t.Fatalf("create done expression: %v", err)
	}
	if err := store.UpdateExpressionStatus("expr-done", "completed", 4, ""); err != nil {
		t.Fatalf("complete expression: %v", err)
	}

	n, err := store.ExpireStaleExpressions(-time.Minute)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d, want 1", n)
	}

	stale, found, err := store.GetExpression("expr-stale")
	if err != nil || !found {
		t.Fatalf("get stale: found=%v err=%v", found, err)
	}
	if stale.Status != "error" || stale.Error == "" {
		t.Fatalf("stale = %+v, want error with message", stale)
	}
	done, _, err := store.GetExpression("expr-done")
	if err != nil {
		t.Fatalf("get done: %v", err)
	}
	if done.Status != "completed" || done.Result != 4 {
		t.Fatalf("completed expression changed: %+v", done)
	}
}
