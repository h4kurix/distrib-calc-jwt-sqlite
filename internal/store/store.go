package store

import "calc-service/internal/database"

// Store даёт доступ к хранилищу. Экземпляр создаётся при старте сервиса.
type Store struct {
	db *database.DB
}

// New создаёт Store поверх открытого соединения.
func New(db *database.DB) *Store { return &Store{db: db} }
