package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db *sql.DB
)

// InitDB initializes the database connection
func InitDB() error {
	// Check if DB_PATH environment variable is set
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbDir := "./data"
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %w", err)
		}
		dbPath = filepath.Join(dbDir, "calculator.db")
	}

	var err error

	// Use WAL journal mode and busy timeout
	dsn := fmt.Sprintf("file:%s?_busy_timeout=10000&_journal_mode=WAL", dbPath)
	db, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err = db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Optional: confirm journal mode (debug)
	var mode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err == nil {
		log.Printf("SQLite journal_mode: %s", mode)
	}

	if err = createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Printf("Database initialized successfully at %s", dbPath)
	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if db != nil {
		db.Close()
	}
}

// GetDB returns the database connection
func GetDB() *sql.DB {
	return db
}

// createTables creates necessary tables if they don't exist
func createTables() error {
	// Create users table
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            id TEXT PRIMARY KEY,
            username TEXT UNIQUE NOT NULL,
            password_hash TEXT NOT NULL,
            created_at TIMESTAMP NOT NULL
        )
    `)
	if err != nil {
		return err
	}

	// Updated expressions table with user_id
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS expressions (
            id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL,
            expression TEXT NOT NULL,
            status TEXT NOT NULL,
            result REAL,
            created_at TIMESTAMP NOT NULL,
            FOREIGN KEY (user_id) REFERENCES users(id)
        )
    `)
	if err != nil {
		return err
	}

	// Updated tasks table with agent tracking fields
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
				id TEXT PRIMARY KEY,
				expression_id TEXT NOT NULL,
				user_id TEXT NOT NULL,
				arg1 TEXT NOT NULL,
				arg2 TEXT NOT NULL,
				operator TEXT NOT NULL,
				operation_time INTEGER NOT NULL,
				result REAL,
				completed BOOLEAN NOT NULL DEFAULT FALSE,
				FOREIGN KEY (expression_id) REFERENCES expressions(id),
				FOREIGN KEY (user_id)       REFERENCES users(id)
			)
    `)
	return err
}

// Transaction executes a function within a database transaction
func Transaction(fn func(*sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
