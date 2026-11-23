package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

// InitDB открывает SQLite по пути из конфигурации и создаёт схему.
func InitDB(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("DB_PATH is empty")
	}
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
	}

	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate", dbPath,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ping database: %w", err)
	}
	if err := createTables(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("create tables: %w", err)
	}

	db = conn
	return nil
}

func CloseDB() error {
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	return err
}

func GetDB() *sql.DB { return db }

func createTables(conn *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS expressions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expression TEXT NOT NULL,
			status TEXT NOT NULL,
			result REAL,
			error TEXT,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			expression_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			arg1 TEXT NOT NULL,
			arg2 TEXT NOT NULL,
			operator TEXT NOT NULL,
			operation_time INTEGER NOT NULL,
			result REAL,
			completed BOOLEAN NOT NULL DEFAULT FALSE,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			error TEXT,
			claimed_at TIMESTAMP,
			FOREIGN KEY (expression_id) REFERENCES expressions(id),
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_expression ON tasks(expression_id)`,
	}
	for _, stmt := range statements {
		if _, err := conn.Exec(stmt); err != nil {
			return err
		}
	}

	// Миграции для уже существующих баз.
	migrations := []struct {
		stmt   string
		ignore string
	}{
		{`ALTER TABLE expressions ADD COLUMN error TEXT`, "duplicate column name"},
		{`ALTER TABLE tasks ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'`, "duplicate column name"},
		{`ALTER TABLE tasks ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`, "duplicate column name"},
		{`ALTER TABLE tasks ADD COLUMN error TEXT`, "duplicate column name"},
		{`ALTER TABLE tasks ADD COLUMN claimed_at TIMESTAMP`, "duplicate column name"},
	}
	for _, m := range migrations {
		_, err := conn.Exec(m.stmt)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), m.ignore) {
			return err
		}
	}

	// Приводим старые строки к новой модели.
	if _, err := conn.Exec(`UPDATE tasks SET status = 'completed' WHERE completed = 1 AND status = 'pending'`); err != nil {
		return err
	}
	return nil
}

// Transaction выполняет fn в транзакции с откатом при ошибке или панике.
// При SQLITE_BUSY транзакция повторяется: fn не имеет побочных эффектов вне tx.
func Transaction(fn func(*sql.Tx) error) (err error) {
	for attempt := 0; attempt < 5; attempt++ {
		err = runTransaction(fn)
		if err == nil || !isBusy(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return err
}

func runTransaction(fn func(*sql.Tx) error) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			err = fmt.Errorf("transaction panic: %v", p)
		}
	}()
	if err = fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func isBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}
