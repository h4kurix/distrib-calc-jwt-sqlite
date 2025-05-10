package store

import (
	"calc-service/pkg/database"
	"calc-service/pkg/logger"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User представляет пользователя
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // Не показываем пароль в JSON
	CreatedAt time.Time `json:"created_at"`
}

// CreateUser создает нового пользователя в базе данных
func CreateUser(username, password string) (*User, error) {
	// Проверяем, существует ли уже пользователь с таким именем
	exists, err := UsernameExists(username)
	if err != nil {
		logger.Error("failed to check if username exists: %v", err)
		return nil, fmt.Errorf("failed to check if username exists: %w", err)
	}
	if exists {
		logger.Info("username already exists: %s", username)
		return nil, fmt.Errorf("username already exists")
	}

	// Хэшируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("failed to hash password: %v", err)
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Создаем нового пользователя
	id := fmt.Sprintf("user-%d", time.Now().UnixNano())
	now := time.Now()

	user := &User{
		ID:        id,
		Username:  username,
		Password:  string(hashedPassword),
		CreatedAt: now,
	}

	// Вставляем пользователя в базу данных
	db := database.GetDB()
	_, err = db.Exec(
		"INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)",
		user.ID, user.Username, user.Password, user.CreatedAt,
	)
	if err != nil {
		logger.Error("failed to insert user into database: %v", err)
		return nil, fmt.Errorf("failed to insert user into database: %w", err)
	}

	logger.Info("created user: %s", user.Username)
	return user, nil
}

// UsernameExists проверяет, существует ли пользователь с таким именем
func UsernameExists(username string) (bool, error) {
	db := database.GetDB()
	var exists bool

	err := db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM users WHERE username = ? LIMIT 1)",
		username,
	).Scan(&exists)

	if err != nil {
		logger.Error("Failed to check username existence: %v", err)
		return false, fmt.Errorf("failed to check username existence: %w", err)
	}

	return exists, nil
}

// GetUserByUsername получает пользователя по имени пользователя
func GetUserByUsername(username string) (*User, bool) {
	db := database.GetDB()
	var user User

	err := db.QueryRow(
		"SELECT id, username, password_hash, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		logger.Error("Database error in GetUserByUsername: %v", err)
		return nil, false
	}

	return &user, true
}

// ValidateUser проверяет правильность пароля пользователя
func ValidateUser(username, password string) (*User, bool) {
	user, found := GetUserByUsername(username)
	if !found {
		return nil, false
	}

	// Сравниваем хэшированный пароль с введенным
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return nil, false
	}

	return user, true
}

// Session представляет сессию пользователя
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewSession создает новую сессию
func NewSession(userID string) (*Session, error) {
	id := fmt.Sprintf("session-%d", time.Now().UnixNano())
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour) // Срок действия сессии 24 часа

	session := &Session{
		ID:        id,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	// Вставка в базу данных
	db := database.GetDB()
	_, err := db.Exec(
		"INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		session.ID, session.UserID, session.CreatedAt, session.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert session: %w", err)
	}

	return session, nil
}

// GetSessionByID получает сессию по ID
func GetSessionByID(sessionID string) (*Session, bool) {
	db := database.GetDB()
	var session Session

	err := db.QueryRow(
		"SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		logger.Error("Database error in GetSessionByID: %v", err)
		return nil, false
	}

	return &session, true
}

// DeleteSession удаляет сессию
func DeleteSession(sessionID string) error {
	db := database.GetDB()
	_, err := db.Exec(
		"DELETE FROM sessions WHERE id = ?",
		sessionID,
	)
	if err != nil {
		logger.Error("Failed to delete session: %v", err)
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// ListSessions возвращает все сессии для конкретного пользователя
func ListSessions(userID string) ([]*Session, error) {
	db := database.GetDB()
	rows, err := db.Query(
		"SELECT id, user_id, created_at, expires_at FROM sessions WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		logger.Error("Database error in ListSessions: %v", err)
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt); err != nil {
			logger.Error("Error scanning session row: %v", err)
			continue
		}
		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// UpdateSessionExpiration обновляет срок действия сессии
func UpdateSessionExpiration(sessionID string, expiresAt time.Time) error {
	db := database.GetDB()
	_, err := db.Exec(
		"UPDATE sessions SET expires_at = ? WHERE id = ?",
		expiresAt, sessionID,
	)
	if err != nil {
		logger.Error("Failed to update session expiration: %v", err)
		return fmt.Errorf("failed to update session expiration: %w", err)
	}

	return nil
}
