package store

import (
	"calc-service/pkg/database"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User представляет пользователя.
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateUser создаёт нового пользователя.
func CreateUser(username, password string) (*User, error) {
	if username == "" || len(username) > 64 {
		return nil, fmt.Errorf("username must be 1-64 characters")
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	exists, err := UsernameExists(username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("username already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		ID:        fmt.Sprintf("user-%d", time.Now().UnixNano()),
		Username:  username,
		Password:  string(hash),
		CreatedAt: time.Now(),
	}
	_, err = database.GetDB().Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		user.ID, user.Username, user.Password, user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return user, nil
}

func UsernameExists(username string) (bool, error) {
	var exists bool
	err := database.GetDB().QueryRow(
		`SELECT EXISTS(SELECT 1 FROM users WHERE username = ? LIMIT 1)`, username,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check username: %w", err)
	}
	return exists, nil
}

// GetUserByUsername возвращает пользователя. Ошибка БД отделена от «не найдено».
func GetUserByUsername(username string) (*User, bool, error) {
	var u User
	err := database.GetDB().QueryRow(
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

func ValidateUser(username, password string) (*User, bool, error) {
	u, found, err := GetUserByUsername(username)
	if err != nil || !found {
		return nil, false, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return nil, false, nil
	}
	return u, true, nil
}

// Session представляет сессию пользователя.
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewSession(userID string) (*Session, error) {
	now := time.Now()
	s := &Session{
		ID:        fmt.Sprintf("session-%d", now.UnixNano()),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
	_, err := database.GetDB().Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		s.ID, s.UserID, s.CreatedAt, s.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return s, nil
}

func GetSessionByID(id string) (*Session, bool, error) {
	var s Session
	err := database.GetDB().QueryRow(
		`SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get session: %w", err)
	}
	return &s, true, nil
}

func DeleteSession(id string) error {
	_, err := database.GetDB().Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func ListSessions(userID string) ([]*Session, error) {
	rows, err := database.GetDB().Query(
		`SELECT id, user_id, created_at, expires_at FROM sessions WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessions, nil
}
