package store

import (
	"calc-service/pkg/database"
	"calc-service/pkg/logger"
	"path/filepath"
	"testing"
	"time"
)

func TestGetExpressionScopesLookupToOwner(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "expressions.db"))
	logger.Init("error")

	if err := database.InitDB(); err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	defer database.CloseDB()

	createdAt := time.Now().UTC()
	_, err := database.GetDB().Exec(
		`INSERT INTO expressions (id, user_id, expression, status, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"expr-owned-by-a", "user-a", "2+2", "completed", createdAt,
	)
	if err != nil {
		t.Fatalf("insert expression: %v", err)
	}

	got, found := GetExpression("expr-owned-by-a", "user-a")
	if !found {
		t.Fatal("owner should be able to retrieve their expression")
	}
	if got.ID != "expr-owned-by-a" {
		t.Fatalf("unexpected expression ID: got %q", got.ID)
	}

	if _, found := GetExpression("expr-owned-by-a", "user-b"); found {
		t.Fatal("a different user must not be able to retrieve this expression")
	}
}
