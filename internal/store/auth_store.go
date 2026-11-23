package store

import (
	"calc-service/internal/id"
	"calc-service/pkg/database"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var ErrUsernameExists = errors.New("username already exists")

// User представляет пользователя.
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateUser создаёт нового пользователя.
func CreateUser(ctx context.Context, username, password string) (*User, error) {
	if username == "" || len(username) > 64 {
		return nil, fmt.Errorf("username must be 1-64 characters")
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	exists, err := UsernameExists(ctx, username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUsernameExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		ID:        id.New("user"),
		Username:  username,
		Password:  string(hash),
		CreatedAt: time.Now(),
	}
	_, err = database.GetDB().ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		user.ID, user.Username, user.Password, user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return user, nil
}

func UsernameExists(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := database.GetDB().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE username = ? LIMIT 1)`, username,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check username: %w", err)
	}
	return exists, nil
}

// GetUserByUsername возвращает пользователя. Ошибка БД отделена от «не найдено».
func GetUserByUsername(ctx context.Context, username string) (*User, bool, error) {
	var u User
	err := database.GetDB().QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.Password, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get user: %w", err)
	}
	return &u, true, nil
}
