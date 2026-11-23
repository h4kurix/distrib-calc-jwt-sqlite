// Package id генерирует уникальные идентификаторы сущностей.
package id

import "github.com/google/uuid"

// New возвращает идентификатор вида <prefix>-<uuid>.
func New(prefix string) string { return prefix + "-" + uuid.New().String() }
