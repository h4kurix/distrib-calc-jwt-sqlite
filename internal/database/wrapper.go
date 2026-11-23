package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// DB владеет соединением с SQLite. Экземпляр создаётся при старте сервиса
// и передаётся в слой store явно.
type DB struct {
	conn *sql.DB
}

// Open открывает SQLite по пути из конфигурации и создаёт схему.
func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("DB_PATH is empty")
	}
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}

	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate", dbPath,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)

	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := createTables(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}
	return &DB{conn: conn}, nil
}

// Close закрывает соединение.
func (d *DB) Close() error { return d.conn.Close() }

// Conn возвращает соединение для запросов store.
func (d *DB) Conn() *sql.DB { return d.conn }

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

	// Миграции для уже существующих баз. Наличие колонки проверяется
	// через PRAGMA, а не по тексту ошибки ALTER TABLE.
	migrations := []struct {
		table  string
		column string
		stmt   string
	}{
		{"expressions", "error", `ALTER TABLE expressions ADD COLUMN error TEXT`},
		{"tasks", "status", `ALTER TABLE tasks ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'`},
		{"tasks", "attempts", `ALTER TABLE tasks ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`},
		{"tasks", "error", `ALTER TABLE tasks ADD COLUMN error TEXT`},
		{"tasks", "claimed_at", `ALTER TABLE tasks ADD COLUMN claimed_at TIMESTAMP`},
	}
	for _, m := range migrations {
		exists, err := columnExists(conn, m.table, m.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := conn.Exec(m.stmt); err != nil {
			return fmt.Errorf("add column %s.%s: %w", m.table, m.column, err)
		}
	}

	// Индексы создаются после миграций: часть из них использует новые колонки.
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_tasks_claimed_at ON tasks(claimed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_expressions_user ON expressions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_expressions_status ON expressions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
	}
	for _, stmt := range indexes {
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	// Приводим старые строки к новой модели.
	if _, err := conn.Exec(`UPDATE tasks SET status = 'completed' WHERE completed = 1 AND status = 'pending'`); err != nil {
		return err
	}
	return nil
}

func columnExists(conn *sql.DB, table, column string) (bool, error) {
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, fmt.Errorf("pragma table_info %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan table_info %s: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table_info %s: %w", table, err)
	}
	return false, nil
}

// Transaction выполняет fn в транзакции с откатом при ошибке или панике.
// При SQLITE_BUSY транзакция повторяется: fn не имеет побочных эффектов вне tx.
// Ожидание между попытками прерывается по ctx.
func (d *DB) Transaction(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	for attempt := 0; attempt < 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = d.runTransaction(ctx, fn)
		if err == nil || !isBusy(err) {
			return err
		}
		delay := time.Duration(attempt+1) * 20 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func (d *DB) runTransaction(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := d.conn.BeginTx(ctx, nil)
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
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}
